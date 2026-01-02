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
	toCollectorChan chan *Operation[ItemType]
	collectorWg     sync.WaitGroup

	toWriterChan chan *OperationsBatch[ItemType]
	writerWg     sync.WaitGroup

	operationsBatchPool sync.Pool

	metrics Metrics
	options Options[ItemType]
}

type Operation[ItemType any] struct {
	items  []ItemType
	result chan error
}

type OperationsBatch[ItemType any] struct {
	items   []ItemType
	results []chan error
}

func NewOperationsBatch[ItemType any](size int) *OperationsBatch[ItemType] {
	return &OperationsBatch[ItemType]{
		items:   make([]ItemType, 0, size),
		results: make([]chan error, 0, size),
	}
}

func (ob *OperationsBatch[ItemType]) Reset() *OperationsBatch[ItemType] {
	ob.items = ob.items[:0]
	ob.results = ob.results[:0]
	return ob
}

func (ob *OperationsBatch[ItemType]) Append(op *Operation[ItemType]) {
	ob.items = append(ob.items, op.items...)
	ob.results = append(ob.results, op.result)
}

func New[ItemType any](options Options[ItemType]) *Batch[ItemType] {
	b := &Batch[ItemType]{
		toCollectorChan: make(chan *Operation[ItemType]),
		toWriterChan:    make(chan *OperationsBatch[ItemType]),
		options:         options.withDefaults(),
	}
	b.operationsBatchPool.New = func() any {
		return NewOperationsBatch[ItemType](b.options.MaxSize)
	}
	b.metrics = NewMetrics(b.options.FlushThreads)

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

	ob := b.operationsBatchPool.Get().(*OperationsBatch[ItemType])

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
				ob.Append(op)
			} else if len(ob.items) > len(op.items) {
				b.toWriterChan <- ob
				ob = b.operationsBatchPool.Get().(*OperationsBatch[ItemType])
				ob.Append(op)
			} else {
				tmp := b.operationsBatchPool.Get().(*OperationsBatch[ItemType])
				tmp.Append(op)
				b.toWriterChan <- tmp
			}
		case <-ticker.C:
			flush = len(ob.items) > 0
		}
		if flush || len(ob.items) >= b.options.MaxSize {
			b.toWriterChan <- ob
			ob = b.operationsBatchPool.Get().(*OperationsBatch[ItemType])
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
		atomic.AddInt64(&b.metrics.Flushes[thread], 1)
		for _, ch := range ob.results {
			ch <- err
			close(ch)
		}
		b.operationsBatchPool.Put(ob.Reset())
	}
}

func (b *Batch[ItemType]) AddOne(item ItemType) error {
	return b.AddMany([]ItemType{item})
}

func (b *Batch[ItemType]) AddMany(items []ItemType) error {
	op := &Operation[ItemType]{
		items:  items,
		result: make(chan error),
	}

	atomic.AddInt64(&b.metrics.ItemsReceived, int64(len(items)))
	defer atomic.AddInt64(&b.metrics.ItemsCompleted, int64(len(items)))

	b.toCollectorChan <- op
	err := <-op.result
	return err
}

func (b *Batch[ItemType]) Metrics() Metrics {
	flushes := make([]int64, len(b.metrics.Flushes))
	for i := range flushes {
		flushes[i] = atomic.LoadInt64(&b.metrics.Flushes[i])
	}
	return Metrics{
		ItemsReceived:  atomic.LoadInt64(&b.metrics.ItemsReceived),
		ItemsCompleted: atomic.LoadInt64(&b.metrics.ItemsCompleted),
		Flushes:        flushes,
	}
}

func (b *Batch[ItemType]) Close() {
	close(b.toCollectorChan)
	b.collectorWg.Wait()

	close(b.toWriterChan)
	b.writerWg.Wait()
}
