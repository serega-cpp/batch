package batch_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serega-cpp/batch"
	"github.com/stretchr/testify/require"
)

var batchSizes = []int{1, 10, 100, 1000}

func BenchmarkBatch(b *testing.B) {
	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("Put(%d) benchmark", size), func(b *testing.B) {
			ctx := context.Background()

			var remained int64

			client := batch.New(batch.Options[empty]{
				FlushThreadsCount: 1,
				FlushFunc: func(_ context.Context, _ int, items []empty) error {
					atomic.AddInt64(&remained, int64(-len(items)))
					return nil
				},
			})
			defer client.Close()

			N := b.N / size
			items := make([]empty, size)

			var completeWg sync.WaitGroup
			completeWg.Add(N)

			b.ReportAllocs()
			b.ResetTimer()

			for range N {
				go func() {
					atomic.AddInt64(&remained, int64(len(items)))
					err := client.Puts(ctx, items)
					require.NoError(b, err)
					completeWg.Done()
				}()
			}

			b.StopTimer()
			completeWg.Wait()

			require.Equal(b, remained, int64(0))
		})
	}
}

func BenchmarkSingleThreadClient(b *testing.B) {
	var flushes int32

	client := batch.New(batch.Options[empty]{
		BatchTimeout:          1 * time.Second,
		BatchFlushInterval:    200 * time.Millisecond,
		BatchMinFlushInterval: 10 * time.Millisecond,
		FlushThreadsCount:     1,
		FlushFunc: func(_ context.Context, _ int, items []empty) error {
			atomic.AddInt32(&flushes, int32(1))
			return nil
		},
	})
	defer client.Close()

	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		err := client.Put(ctx, empty{})
		require.NoError(b, err)
	}

	require.Equal(b, flushes, int32(b.N))
}
