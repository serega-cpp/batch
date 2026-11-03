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

const FlushInterval = 100 * time.Millisecond

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
		flushesPerThreadCount := []int64{0, 0, 0, 0}
		options := batch.Options[string]{
			BatchFlushInterval: FlushInterval,
			BatchFlushSize:     len(items),
			FlushThreadsCount:  threadsCount,
			FlushFunc: func(_ context.Context, thread int, _ []string) error {
				flushesPerThreadCount[thread]++
				return nil
			},
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

		require.Equal(t, batch.Metrics{
			ServedCount:            int64(threadsCount * len(items)),
			ServedWithErrCount:     0,
			RejectedCount:          0,
			FlushesCount:           int64(threadsCount),
			FlushesPerThreadCount:  flushesPerThreadCount,
			CurrentFlushIntervalMs: FlushInterval.Milliseconds(),
		}, bat.Metrics())
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
			FlushFunc: func(_ context.Context, _ int, items []string) error {
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
			FlushFunc: func(ctx context.Context, _ int, items []string) error {
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
			BatchFlushSize: batchSize,
			FlushFunc: func(_ context.Context, _ int, items []string) error {
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

	t.Run("Puts flush by time", func(t *testing.T) {
		ctx := context.Background()

		items := []string{"test1", "test2", "test3", "test4"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			BatchFlushInterval: 100 * time.Millisecond,
			BatchFlushSize:     100,
			FlushFunc: func(_ context.Context, _ int, items []string) error {
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		timer := time.NewTimer(options.BatchFlushInterval * 2)
		defer timer.Stop()

		errCh := make(chan error, 1)
		go func() {
			errCh <- bat.Puts(ctx, items)
		}()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-timer.C:
			t.Fatal("The flush was not done within the expected interval")
		}

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("Batch overflow special case", func(t *testing.T) {
		ctx := context.Background()

		// expecting that on overflow we flush the batch
		// immediately and place newly received items in it
		batchSize := 6
		items1 := []string{"test1", "test2", "test3", "test4"}
		items2 := []string{"test5", "test6", "test7"}
		results := make(chan string, len(items1)+len(items2)+1)

		options := batch.Options[string]{
			BatchFlushInterval: 1 * time.Second,
			BatchFlushSize:     batchSize,
			FlushFunc: func(_ context.Context, _ int, items []string) error {
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
		time.Sleep(25 * time.Millisecond) // guarantee the expected order of requests
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

	t.Run("Flush error", func(t *testing.T) {
		options := batch.Options[string]{
			BatchFlushInterval: FlushInterval,
			BatchFlushSize:     1,
			FlushFunc: func(_ context.Context, _ int, _ []string) error {
				return context.Canceled
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		err := bat.Put(context.Background(), "test")
		require.ErrorIs(t, err, context.Canceled)

		require.Equal(t, batch.Metrics{
			ServedCount:            1,
			ServedWithErrCount:     1,
			RejectedCount:          0,
			FlushesCount:           1,
			FlushesPerThreadCount:  []int64{1},
			CurrentFlushIntervalMs: FlushInterval.Milliseconds(),
		}, bat.Metrics())
	})

	t.Run("Put context timeout", func(t *testing.T) {
		options := batch.Options[string]{
			BatchFlushInterval: FlushInterval,
			BatchFlushSize:     1,
			FlushFunc: func(_ context.Context, _ int, _ []string) error {
				// sleep function ensures that all requests below are received and
				// their context able timeout before the 1-st data flush is complete
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
		time.Sleep(25 * time.Millisecond) // guarantee the expected order of requests
		go func() {
			defer wg.Done()
			// 2-nd put goes to the new buffer, so no context timeout
			err := bat.Put(ctx, "test")
			require.NoError(t, err)
		}()
		time.Sleep(25 * time.Millisecond) // guarantee the expected order of requests
		go func() {
			defer wg.Done()
			// 3-rd put needs to wait for the buffer, so will be canceled by timeout
			err := bat.Put(ctx, "test")
			require.ErrorIs(t, err, context.DeadlineExceeded)
		}()
		wg.Wait()

		require.Equal(t, batch.Metrics{
			ServedCount:            2,
			ServedWithErrCount:     0,
			RejectedCount:          1,
			FlushesCount:           2,
			FlushesPerThreadCount:  []int64{2},
			CurrentFlushIntervalMs: FlushInterval.Milliseconds(),
		}, bat.Metrics())
	})

	t.Run("Flush context timeout", func(t *testing.T) {
		options := batch.Options[string]{
			BatchTimeout:       100 * time.Millisecond,
			BatchFlushInterval: FlushInterval,
			BatchFlushSize:     1,
			FlushFunc: func(ctx context.Context, _ int, _ []string) error {
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

		require.Equal(t, batch.Metrics{
			ServedCount:            1,
			ServedWithErrCount:     1,
			RejectedCount:          0,
			FlushesCount:           1,
			FlushesPerThreadCount:  []int64{1},
			CurrentFlushIntervalMs: FlushInterval.Milliseconds(),
		}, bat.Metrics())
	})

	t.Run("PutsMuch", func(t *testing.T) {
		ctx := context.Background()

		items := []string{"test1", "test2", "test3", "test4", "test5"}
		results := make(chan string, len(items)+1)

		options := batch.Options[string]{
			BatchFlushSize: 2,
			FlushFunc: func(_ context.Context, _ int, items []string) error {
				for i := range items {
					results <- items[i]
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		pr := bat.PutsMuch(ctx, items, 0)
		require.Equal(t, []batch.SegmentStatus{
			{
				Start: 0,
				End:   2,
				Err:   nil,
			}, {
				Start: 2,
				End:   4,
				Err:   nil,
			}, {
				Start: 4,
				End:   5,
				Err:   nil,
			},
		}, pr)

		for i := range items {
			result := <-results
			require.Equal(t, items[i], result)
		}
	})

	t.Run("PutsMuch error", func(t *testing.T) {
		ctx := context.Background()

		items := []string{"test1", "test2", "test3", "test4", "test5"}
		batchNumber := 0

		options := batch.Options[string]{
			BatchFlushSize: 2,
			FlushFunc: func(_ context.Context, _ int, _ []string) error {
				batchNumber++
				if batchNumber == 2 {
					return context.DeadlineExceeded
				}
				return nil
			},
		}

		bat := batch.New(options)
		defer bat.Close()

		pr := bat.PutsMuch(ctx, items, 0)
		require.Equal(t, []batch.SegmentStatus{
			{
				Start: 0,
				End:   2,
				Err:   nil,
			}, {
				Start: 2,
				End:   4,
				Err:   context.DeadlineExceeded,
			},
		}, pr)
	})
}

func TestAdaptiveFlushInterval(t *testing.T) {
	// adaptive timeout range: 300 .. 50, so the
	// sequence is: 300 <-> 150 <-> 75 <-> 50.
	client := batch.New(batch.Options[empty]{
		BatchTimeout:          1 * time.Second,
		BatchFlushInterval:    300 * time.Millisecond,
		BatchMinFlushInterval: 50 * time.Millisecond,
		FlushThreadsCount:     1,
		FlushFunc: func(_ context.Context, _ int, items []empty) error {
			return nil
		},
	})
	defer client.Close()

	ctx := context.Background()

	workCh1 := make(chan empty)
	workCh2 := make(chan empty)
	workDoneCh := make(chan int)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for range workCh1 {
			err := client.Put(ctx, empty{})
			require.NoError(t, err)
			workDoneCh <- 1
		}
	}()
	go func() {
		defer wg.Done()
		for range workCh2 {
			err := client.Put(ctx, empty{})
			require.NoError(t, err)
			workDoneCh <- 2
		}
	}()

	workCh1 <- empty{}
	time.Sleep(100 * time.Millisecond) // the last item goes to [25;50]%
	workCh2 <- empty{}
	<-workDoneCh
	<-workDoneCh
	// expect the timeout to remain the same
	require.Equal(t, int64(300), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	<-workDoneCh
	// expect the timeout to decrease
	require.Equal(t, int64(150), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	<-workDoneCh
	// expect the timeout to decrease
	require.Equal(t, int64(75), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	<-workDoneCh
	// expect the timeout to decrease and be truncated to a min
	require.Equal(t, int64(50), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	time.Sleep(30 * time.Millisecond) // the last item goes to [50;100]%
	workCh2 <- empty{}
	<-workDoneCh
	<-workDoneCh
	// expect the timeout to increase
	require.Equal(t, int64(100), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	time.Sleep(60 * time.Millisecond) // the last item goes to [50;100]%
	workCh2 <- empty{}
	<-workDoneCh
	<-workDoneCh
	// expect the timeout to increase
	require.Equal(t, int64(200), client.Metrics().CurrentFlushIntervalMs)

	workCh1 <- empty{}
	time.Sleep(120 * time.Millisecond) // the last item goes to [50;100]%
	workCh2 <- empty{}
	<-workDoneCh
	<-workDoneCh
	// expect the timeout to increase and be truncated to a max
	require.Equal(t, int64(300), client.Metrics().CurrentFlushIntervalMs)

	close(workCh1)
	close(workCh2)
	wg.Wait()
}

type empty struct{}
