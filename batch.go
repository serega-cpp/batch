package batch

import (
	"sync"
	"time"
)

type Options[ItemType any] struct {
	MaxSize     int
	MaxLifetime time.Duration

	FlushFunc func(items []ItemType) error
}

func (o Options[ItemType]) withDefaults() Options[ItemType] {
	if o.MaxSize == 0 {
		o.MaxSize = 1000
	}
	if o.MaxLifetime == 0 {
		o.MaxLifetime = 100 * time.Millisecond
	}
	if o.FlushFunc == nil {
		o.FlushFunc = func([]ItemType) error {
			return nil
		}
	}
	return o
}

type Batch[ItemType any] struct {
	inputChan chan BatchOperation[ItemType]
	writerWg  sync.WaitGroup

	outputChan  chan BatchResult
	responderWg sync.WaitGroup
	resultsPool sync.Pool

	options Options[ItemType]
}

type BatchOperation[ItemType any] struct {
	item       ItemType
	resultChan chan error
}

type BatchResult struct {
	err      error
	channels []chan error
}

func New[ItemType any](options Options[ItemType]) *Batch[ItemType] {
	batch := &Batch[ItemType]{
		inputChan:  make(chan BatchOperation[ItemType]),
		outputChan: make(chan BatchResult),
		resultsPool: sync.Pool{
			New: func() any {
				return make([]chan error, 0, options.MaxSize)
			},
		},
		options: options.withDefaults(),
	}

	batch.writerWg.Add(1)
	go batch.writer()

	batch.responderWg.Add(1)
	go batch.responder()

	return batch
}

func (b *Batch[ItemType]) writer() {
	defer b.writerWg.Done()

	items := make([]ItemType, 0, b.options.MaxSize)
	results := b.resultsPool.Get().([]chan error)

	ticker := time.NewTicker(b.options.MaxLifetime)
	defer ticker.Stop()

	for {
		var done bool
		var flush bool
		select {
		case op, live := <-b.inputChan:
			if !live {
				done = true
				flush = len(items) > 0
			} else {
				items = append(items, op.item)
				results = append(results, op.resultChan)
			}
		case <-ticker.C:
			flush = len(items) > 0
		}
		if flush || len(items) >= b.options.MaxSize {
			err := b.options.FlushFunc(items)
			b.outputChan <- BatchResult{
				err:      err,
				channels: results,
			}
			items = items[:0]
			results = b.resultsPool.Get().([]chan error)
		}
		if done {
			break
		}
	}
}

func (b *Batch[ItemType]) responder() {
	defer b.responderWg.Done()

	for result := range b.outputChan {
		for _, ch := range result.channels {
			ch <- result.err
			close(ch)
		}
		b.resultsPool.Put(result.channels[:0])
	}
}

func (b *Batch[ItemType]) Add(item ItemType) error {
	op := BatchOperation[ItemType]{
		item:       item,
		resultChan: make(chan error),
	}
	b.inputChan <- op
	err := <-op.resultChan
	return err
}

func (b *Batch[ItemType]) Close() {
	close(b.inputChan)
	close(b.outputChan)
	b.writerWg.Wait()
	b.responderWg.Wait()
}
