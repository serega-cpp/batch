package batch_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serega-cpp/batch"
)

func TestBatch(t *testing.T) {
	t.Run("Default options", func(t *testing.T) {
		var options batch.Options[empty]

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(empty{})
		require.NoError(t, err)
	})

	t.Run("Metrics", func(t *testing.T) {
		threadsCount := 4
		items := []string{"test1", "test2", "test3", "test4"}
		options := batch.Options[string]{
			MaxSize:      len(items),
			FlushThreads: threadsCount,
		}

		bat := batch.New(options)
		defer bat.Close()

		var wg sync.WaitGroup
		for range threadsCount {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := bat.Puts(items)
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
		var options batch.Options[empty]

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts([]empty{})
		require.NoError(t, err)
	})

	t.Run("Put", func(t *testing.T) {
		item := "test1"
		results := make(chan string, 1+1)

		options := batch.Options[string]{
			FlushFunc: func(_ int, items []string) error {
				assert.Equal(t, len(items), 1)
				results <- items[0]
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(item)
		require.NoError(t, err)
	})

	t.Run("Puts", func(t *testing.T) {
		items := []string{"test1", "test2", "test3", "test4"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			FlushFunc: func(_ int, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts(items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Puts flush by size", func(t *testing.T) {
		batchSize := 4
		items := []string{"test1", "test2", "test3", "test4"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			MaxSize: batchSize,
			FlushFunc: func(_ int, items []string) error {
				assert.Greater(t, len(items), 0)
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Puts(items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Batch overflow case one", func(t *testing.T) {
		// expecting that on overflow we flush the latest
		// incoming items insted of the items from the buffer
		batchSize := 6
		items1 := []string{"test1", "test2", "test3"}
		items2 := []string{"test4", "test5", "test6", "test7"}
		results := make(chan string, len(items1)+len(items2)+1)

		options := batch.Options[string]{
			MaxLifetime: 1 * time.Second,
			MaxSize:     batchSize,
			FlushFunc: func(_ int, items []string) error {
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
			err := bat.Puts(items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := bat.Puts(items2)
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
		// expecting that on overflow we flush the buffer
		// and put the latest incoming items into it
		batchSize := 6
		items1 := []string{"test1", "test2", "test3", "test4"}
		items2 := []string{"test5", "test6", "test7"}
		results := make(chan string, len(items1)+len(items2)+1)

		options := batch.Options[string]{
			MaxLifetime: 1 * time.Second,
			MaxSize:     batchSize,
			FlushFunc: func(_ int, items []string) error {
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
			err := bat.Puts(items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := bat.Puts(items2)
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
}

type empty struct{}
