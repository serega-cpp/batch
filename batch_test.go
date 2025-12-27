package batch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serega-cpp/batch"
)

func TestBatch(t *testing.T) {
	t.Run("Options", func(t *testing.T) {
		var options batch.Options[int]

		b := batch.New(options)
		defer b.Close()

		err := b.AddOne(0)
		require.NoError(t, err)
	})

	t.Run("AddOne", func(t *testing.T) {
		item := "test1"
		results := make(chan string, 1+1)

		options := batch.Options[string]{
			FlushFunc: func(_ int, items []string) error {
				assert.Equal(t, len(items), 1)
				results <- items[0]
				return nil
			},
		}

		b := batch.New(options)
		defer b.Close()

		err := b.AddOne(item)
		require.NoError(t, err)

		result := <-results
		require.Equal(t, item, result)
	})

	t.Run("AddMany", func(t *testing.T) {
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

		b := batch.New(options)
		defer b.Close()

		err := b.AddMany(items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("AddMany flush by size", func(t *testing.T) {
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

		b := batch.New(options)
		defer b.Close()

		err := b.AddMany(items)
		require.NoError(t, err)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Batch overflow case one", func(t *testing.T) {
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

		b := batch.New(options)
		defer b.Close()

		go func() {
			err := b.AddMany(items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := b.AddMany(items2)
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

		b := batch.New(options)
		defer b.Close()

		go func() {
			err := b.AddMany(items1)
			require.NoError(t, err)
		}()
		time.Sleep(100 * time.Millisecond)
		go func() {
			err := b.AddMany(items2)
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
