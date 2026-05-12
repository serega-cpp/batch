package batch_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serega-cpp/batch"
)

func TestBatch(t *testing.T) {
	t.Run("Default options", func(t *testing.T) {
		ctx := context.Background()

		var options batch.Options[empty]

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(ctx, empty{})
		require.NoError(t, err)
	})

	t.Run("Metrics", func(t *testing.T) {
		ctx := context.Background()

		threadsCount := 4
		items := []string{"test1", "test2", "test3", "test4"}
		options := batch.Options[string]{
			BatchSize:         len(items),
			FlushThreadsCount: threadsCount,
		}

		bat := batch.New(options)
		defer bat.Close()

		var wg sync.WaitGroup
		for range threadsCount {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := bat.Puts(ctx, items)
				require.NoError(t, err)
			}()
		}
		wg.Wait()

		metrics := bat.Metrics()
		var expectedItems int64 = int64(threadsCount * len(items))
		require.Equal(t, expectedItems, metrics.IncomingCount)
		require.Equal(t, expectedItems, metrics.ServedCount)
		require.Equal(t, threadsCount, len(metrics.FlushesPerThreadCount))
		require.Equal(t, int64(threadsCount), metrics.FlushesCount)
	})

	t.Run("Puts empty", func(t *testing.T) {
		ctx := context.Background()

		var options batch.Options[empty]

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts(ctx, []empty{})
		require.NoError(t, err)
	})

	t.Run("Put", func(t *testing.T) {
		ctx := context.Background()

		item := "test1"
		results := make(chan string, 1+1)

		options := batch.Options[string]{
			FlushFunc: func(_ int, _ context.Context, items []string) error {
				assert.Equal(t, len(items), 1)
				results <- items[0]
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(ctx, item)
		require.NoError(t, err)
	})

	t.Run("Puts", func(t *testing.T) {
		ctx := context.Background()

		items := []string{"test1", "test2", "test3", "test4"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			FlushFunc: func(_ int, ctx context.Context, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts(ctx, items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Puts flush by size", func(t *testing.T) {
		ctx := context.Background()

		batchSize := 4
		items := []string{"test1", "test2", "test3", "test4"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			BatchSize: batchSize,
			FlushFunc: func(_ int, _ context.Context, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts(ctx, items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Batch overflow case one", func(t *testing.T) {
		ctx := context.Background()

		// expecting that on overflow we flush the latest
		// incoming items insted of the items from the buffer
		batchSize := 6
		items1 := []string{"test1", "test2", "test3"}
		items2 := []string{"test4", "test5", "test6", "test7"}
		results := make(chan string, len(items1)+len(items2)+1)

		options := batch.Options[string]{
			BatchFlushInterval: 1 * time.Second,
			BatchSize:          batchSize,
			FlushFunc: func(_ int, _ context.Context, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		go func() {
			err := bat.Puts(ctx, items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := bat.Puts(ctx, items2)
			require.NoError(t, err)
		}()

		for i := range items2 {
			result := <-results
			require.Equal(t, items2[i], result)
		}
		for i := range items1 {
			result := <-results
			require.Equal(t, items1[i], result)
		}
	})

	t.Run("Batch overflow case two", func(t *testing.T) {
		ctx := context.Background()

		// expecting that on overflow we flush the buffer
		// and put the latest incoming items into it
		batchSize := 6
		items1 := []string{"test1", "test2", "test3", "test4"}
		items2 := []string{"test5", "test6", "test7"}
		results := make(chan string, len(items1)+len(items2)+1)

		options := batch.Options[string]{
			BatchFlushInterval: 1 * time.Second,
			BatchSize:          batchSize,
			FlushFunc: func(_ int, _ context.Context, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		go func() {
			err := bat.Puts(ctx, items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := bat.Puts(ctx, items2)
			require.NoError(t, err)
		}()

		for i := range items1 {
			result := <-results
			require.Equal(t, items1[i], result)
		}
		for i := range items2 {
			result := <-results
			require.Equal(t, items2[i], result)
		}
	})

	t.Run("Put context timeout", func(t *testing.T) {
		options := batch.Options[string]{
			BatchSize: 1,
			FlushFunc: func(_ int, _ context.Context, _ []string) error {
				time.Sleep(500 * time.Millisecond)
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		wg.Add(3)

		go func() {
			defer wg.Done()
			// 1-st put goes to flush immediatelly, so no context timeout
			err := bat.Put(ctx, "test")
			require.NoError(t, err)
		}()
		time.Sleep(25 * time.Millisecond)
		go func() {
			defer wg.Done()
			// 2-nd put goes to the new buffer, so no context timeout
			err := bat.Put(ctx, "test")
			require.NoError(t, err)
		}()
		time.Sleep(25 * time.Millisecond)
		go func() {
			defer wg.Done()
			// 3-rd put waits for the buffer, so can be canceled by context timeout
			err := bat.Put(ctx, "test")
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}()
		wg.Wait()
	})

	t.Run("Flush context timeout", func(t *testing.T) {
		options := batch.Options[string]{
			TotalTimeout: 100 * time.Millisecond,
			BatchSize:    1,
			FlushFunc: func(_ int, ctx context.Context, _ []string) error {
				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()
				select {
				case <-ticker.C:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(context.Background(), "test")
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

type empty struct{}
