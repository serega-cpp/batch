package batch

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

///////////////////////////////////////////////////////////////////////////////
// Processing stages (and terminology):
// - Create an operation for the incoming item(s)
// - Send the operation to the collector
// - The collector() routine groups operations into batches
// - When a batch is ready (based on size or time), send it to the writer
// - The writer() routine flushes batches concurrently across flush threads
// - When flush completes, it returns the result, thus completing the request
///////////////////////////////////////////////////////////////////////////////

// Options contains the configuration of the batch
// instance (each parameter has a default value).
type Options[ItemType any] struct {
	BatchTimeout       time.Duration // default 1s
	BatchFlushInterval time.Duration // default 100ms
	BatchSize          int           // default 1000
	FlushThreadsCount  int           // default 1
	FlushFunc          func(ctx context.Context, thread int, items []ItemType) error
}

func (o Options[ItemType]) withDefaults() Options[ItemType] {
	if o.BatchTimeout == 0 {
		o.BatchTimeout = time.Second
	}
	if o.BatchFlushInterval == 0 {
		o.BatchFlushInterval = 100 * time.Millisecond
	}
	if o.BatchSize == 0 {
		o.BatchSize = 1000
	}
	if o.FlushThreadsCount == 0 {
		o.FlushThreadsCount = 1
	}
	if o.FlushFunc == nil {
		o.FlushFunc = func(context.Context, int, []ItemType) error {
			return nil
		}
	}
	return o
}

///////////////////////////////////////////////////////////////////////////////

type Batch[ItemType any] struct {
	toCollectorChan chan *operation[ItemType]
	collectorWg     sync.WaitGroup

	toWriterChan chan *operationsBatch[ItemType]
	writerWg     sync.WaitGroup

	operationsBatchPool sync.Pool

	metrics Metrics
	options Options[ItemType]
}

// New creates the batch using provided options
func New[ItemType any](options Options[ItemType]) *Batch[ItemType] {
	b := &Batch[ItemType]{
		toCollectorChan: make(chan *operation[ItemType]),
		toWriterChan:    make(chan *operationsBatch[ItemType]),
		options:         options.withDefaults(),
	}
	b.operationsBatchPool.New = func() any {
		return newOperationsBatch[ItemType](b.options.BatchSize)
	}
	b.metrics = newMetrics(b.options.FlushThreadsCount)

	b.collectorWg.Add(1)
	go b.collector()

	for i := 0; i < b.options.FlushThreadsCount; i++ {
		b.writerWg.Add(1)
		go b.writer(i)
	}

	return b
}

// Put processes the single item
func (b *Batch[ItemType]) Put(ctx context.Context, item ItemType) error {
	return b.Puts(ctx, []ItemType{item})
}

// Puts processes items slice
func (b *Batch[ItemType]) Puts(ctx context.Context, items []ItemType) error {
	if len(items) == 0 {
		return nil
	}
	op := &operation[ItemType]{
		items:  items,
		result: make(chan error),
	}
	select {
	case b.toCollectorChan <- op:
		defer atomic.AddInt64(&b.metrics.ServedCount, int64(len(items)))
		return <-op.result
	case <-ctx.Done():
		atomic.AddInt64(&b.metrics.RejectedCount, int64(len(items)))
		return ctx.Err()
	}
}

// PutsMuch allows you to process huge data sets that exceed the bucket size
// by splitting them into smaller segments using the splitBy parameter.
func (b *Batch[ItemType]) PutsMuch(ctx context.Context, items []ItemType, splitBy int) []ItemsSegment {
	if splitBy == 0 {
		splitBy = b.options.BatchSize
	}
	var segments []ItemsSegment
	for i := 0; i < len(items); i += splitBy {
		end := min(i+splitBy, len(items))
		err := b.Puts(ctx, items[i:end])
		segments = append(segments, ItemsSegment{
			Start: i,
			End:   end,
			Err:   err,
		})
		if err != nil {
			break
		}
	}
	return segments
}

// Metrics returns the actual snapshot of the metrics state.
func (b *Batch[ItemType]) Metrics() Metrics {
	var flushesCount int64
	flushesPerThreadCount := make([]int64, len(b.metrics.FlushesPerThreadCount))
	for i := range flushesPerThreadCount {
		flushesPerThreadCount[i] = atomic.LoadInt64(&b.metrics.FlushesPerThreadCount[i])
		flushesCount += flushesPerThreadCount[i]
	}
	return Metrics{
		ServedCount:           atomic.LoadInt64(&b.metrics.ServedCount),
		ServedWithErrCount:    atomic.LoadInt64(&b.metrics.ServedWithErrCount),
		RejectedCount:         atomic.LoadInt64(&b.metrics.RejectedCount),
		FlushesCount:          flushesCount,
		FlushesPerThreadCount: flushesPerThreadCount,
	}
}

// Close terminates processing of the batch and releases all resources.
func (b *Batch[ItemType]) Close() {
	close(b.toCollectorChan)
	b.collectorWg.Wait()

	close(b.toWriterChan)
	b.writerWg.Wait()
}

func (b *Batch[ItemType]) collector() {
	defer b.collectorWg.Done()

	ob := b.operationsBatchPool.Get().(*operationsBatch[ItemType])

	ticker := time.NewTicker(b.options.BatchFlushInterval)
	defer ticker.Stop()

	for {
		var done bool
		var flush bool
		select {
		case op, ok := <-b.toCollectorChan:
			if !ok {
				flush = len(ob.items) > 0
				done = true
				break
			}
			// In case of expected batch overflow, if there are already
			// enough items in the batch, we will send it to writer immediately
			if len(ob.items)+len(op.items) > b.options.BatchSize && len(ob.items) > len(op.items) {
				b.toWriterChan <- ob
				ob = b.operationsBatchPool.Get().(*operationsBatch[ItemType])
			}
			// Set a batch timeout when the first item is added to the batch
			if len(ob.items) == 0 {
				ob.initCtx(b.options.BatchTimeout)
			}
			ob.append(op)
		case <-ticker.C:
			flush = len(ob.items) > 0
		}
		if flush || len(ob.items) >= b.options.BatchSize {
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
		err := b.options.FlushFunc(ob.ctx, thread, ob.items)
		atomic.AddInt64(&b.metrics.FlushesPerThreadCount[thread], 1)
		if err != nil {
			atomic.AddInt64(&b.metrics.ServedWithErrCount, int64(len(ob.items)))
		}
		for _, ch := range ob.results {
			ch <- err
			close(ch)
		}
		b.operationsBatchPool.Put(ob.reset())
	}
}

///////////////////////////////////////////////////////////////////////////////

type operation[ItemType any] struct {
	items  []ItemType
	result chan error
}

type operationsBatch[ItemType any] struct {
	items   []ItemType
	results []chan error
	ctx     context.Context
	cancel  context.CancelFunc
}

func newOperationsBatch[ItemType any](size int) *operationsBatch[ItemType] {
	return &operationsBatch[ItemType]{
		items:   make([]ItemType, 0, size),
		results: make([]chan error, 0, size),
	}
}

func (ob *operationsBatch[ItemType]) initCtx(timeout time.Duration) {
	ob.ctx, ob.cancel = context.WithTimeout(context.Background(), timeout)
}

func (ob *operationsBatch[ItemType]) reset() *operationsBatch[ItemType] {
	ob.items = ob.items[:0]
	ob.results = ob.results[:0]
	ob.cancel()
	return ob
}

func (ob *operationsBatch[ItemType]) append(op *operation[ItemType]) {
	ob.items = append(ob.items, op.items...)
	ob.results = append(ob.results, op.result)
}

///////////////////////////////////////////////////////////////////////////////

// ItemsSegment contains information about an items
// segment, including the result of its processing.
type ItemsSegment struct {
	Start int   // start of the segment in the items slice
	End   int   // end of the segment in the items slice
	Err   error // segment processing error (if any)
}
