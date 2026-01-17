package batch

import (
	"sync"
	"sync/atomic"
	"time"
)

/******************************************************************************
*  Architecture of package (processing request stages):
*  - create the operation for incoming request
*  - send it via 'to_collector' channel to collector
*  - collector() routine collects operations into the batch
*  - when the batch is ready send it via 'to_writer' channel to writer
*  - writer() routine calls the flash routine for the batch and returns results
******************************************************************************/

type Options[ItemType any] struct {
	MaxLifetime  time.Duration // default 100ms
	MaxSize      int           // default 1000
	FlushThreads int           // default 1
	FlushFunc    func(thread int, items []ItemType) error
}

func (o Options[ItemType]) withDefaults() Options[ItemType] {
	if o.MaxLifetime == 0 {
		o.MaxLifetime = 100 * time.Millisecond
	}
	if o.MaxSize == 0 {
		o.MaxSize = 1000
	}
	if o.FlushThreads == 0 {
		o.FlushThreads = 1
	}
	if o.FlushFunc == nil {
		o.FlushFunc = func(int, []ItemType) error {
			return nil
		}
	}
	return o
}

type Batch[ItemType any] struct {
	toCollectorChan chan *operation[ItemType]
	collectorWg     sync.WaitGroup

	toWriterChan chan *operationsBatch[ItemType]
	writerWg     sync.WaitGroup

	operationsBatchPool sync.Pool

	metrics Metrics
	options Options[ItemType]
}

type operation[ItemType any] struct {
	items  []ItemType
	result chan error
}

type operationsBatch[ItemType any] struct {
	items   []ItemType
	results []chan error
}

func newOperationsBatch[ItemType any](size int) *operationsBatch[ItemType] {
	return &operationsBatch[ItemType]{
		items:   make([]ItemType, 0, size),
		results: make([]chan error, 0, size),
	}
}

func (ob *operationsBatch[ItemType]) reset() *operationsBatch[ItemType] {
	ob.items = ob.items[:0]
	ob.results = ob.results[:0]
	return ob
}

func (ob *operationsBatch[ItemType]) append(op *operation[ItemType]) {
	ob.items = append(ob.items, op.items...)
	ob.results = append(ob.results, op.result)
}

func New[ItemType any](options Options[ItemType]) *Batch[ItemType] {
	b := &Batch[ItemType]{
		toCollectorChan: make(chan *operation[ItemType]),
		toWriterChan:    make(chan *operationsBatch[ItemType]),
		options:         options.withDefaults(),
	}
	b.operationsBatchPool.New = func() any {
		return newOperationsBatch[ItemType](b.options.MaxSize)
	}
	b.metrics = newMetrics(b.options.FlushThreads)

	b.collectorWg.Add(1)
	go b.collector()

	for i := 0; i < b.options.FlushThreads; i++ {
		b.writerWg.Add(1)
		go b.writer(i)
	}

	return b
}

func (b *Batch[ItemType]) collector() {
	defer b.collectorWg.Done()

	ob := b.operationsBatchPool.Get().(*operationsBatch[ItemType])

	ticker := time.NewTicker(b.options.MaxLifetime)
	defer ticker.Stop()

	for {
		var done bool
		var flush bool
		select {
		case op, ok := <-b.toCollectorChan:
			if !ok {
				flush = len(ob.items) > 0
				done = true
			} else if len(ob.items)+len(op.items) <= b.options.MaxSize {
				ob.append(op)
			} else if len(ob.items) > len(op.items) {
				b.toWriterChan <- ob
				ob = b.operationsBatchPool.Get().(*operationsBatch[ItemType])
				ob.append(op)
			} else {
				tmp := b.operationsBatchPool.Get().(*operationsBatch[ItemType])
				tmp.append(op)
				b.toWriterChan <- tmp
			}
		case <-ticker.C:
			flush = len(ob.items) > 0
		}
		if flush || len(ob.items) >= b.options.MaxSize {
			b.toWriterChan <- ob
			ob = b.operationsBatchPool.Get().(*operationsBatch[ItemType])
		}
		if done {
			break
		}
	}
}

func (b *Batch[ItemType]) writer(thread int) {
	defer b.writerWg.Done()

	for ob := range b.toWriterChan {
		err := b.options.FlushFunc(thread, ob.items)
		atomic.AddInt64(&b.metrics.FlushesPerThreadCount[thread], 1)
		for _, ch := range ob.results {
			ch <- err
			close(ch)
		}
		b.operationsBatchPool.Put(ob.reset())
	}
}

func (b *Batch[ItemType]) Put(item ItemType) error {
	return b.Puts([]ItemType{item})
}

func (b *Batch[ItemType]) Puts(items []ItemType) error {
	if len(items) == 0 {
		return nil
	}
	op := &operation[ItemType]{
		items:  items,
		result: make(chan error),
	}

	atomic.AddInt64(&b.metrics.IncomingCount, int64(len(items)))
	defer atomic.AddInt64(&b.metrics.ServedCount, int64(len(items)))

	b.toCollectorChan <- op
	err := <-op.result
	return err
}

func (b *Batch[ItemType]) Metrics() Metrics {
	var flushesCount int64
	flushesPerThreadCount := make([]int64, len(b.metrics.FlushesPerThreadCount))
	for i := range flushesPerThreadCount {
		flushesPerThreadCount[i] = atomic.LoadInt64(&b.metrics.FlushesPerThreadCount[i])
		flushesCount += flushesPerThreadCount[i]
	}
	return Metrics{
		IncomingCount:         atomic.LoadInt64(&b.metrics.IncomingCount),
		ServedCount:           atomic.LoadInt64(&b.metrics.ServedCount),
		FlushesCount:          flushesCount,
		FlushesPerThreadCount: flushesPerThreadCount,
	}
}

func (b *Batch[ItemType]) Close() {
	close(b.toCollectorChan)
	b.collectorWg.Wait()

	close(b.toWriterChan)
	b.writerWg.Wait()
}
