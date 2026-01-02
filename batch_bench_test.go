package batch_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/serega-cpp/batch"
	"github.com/stretchr/testify/require"
)

var batchSizes = []int{1, 10, 100, 1000}

func BenchmarkBatch(b *testing.B) {
	b.Run("AddOne benchmark", func(b *testing.B) {
		var processed int64
		var iterations int64

		bat := batch.New(batch.Options[empty]{
			FlushThreads: 1,
			FlushFunc: func(thread int, items []empty) error {
				atomic.AddInt64(&processed, int64(-len(items)))
				return nil
			},
		})
		defer bat.Close()

		item := empty{}

		var completeWg sync.WaitGroup
		completeWg.Add(b.N)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			go func() {
				atomic.AddInt64(&iterations, 1)
				atomic.AddInt64(&processed, 1)
				err := bat.AddOne(item)
				require.NoError(b, err)
				completeWg.Done()
			}()
		}

		b.StopTimer()
		completeWg.Wait()

		require.Equal(b, iterations, int64(b.N))
		require.Equal(b, processed, int64(0))
	})
	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("AddMany(%d) benchmark", size), func(b *testing.B) {
			var processed int64
			var iterations int64

			bat := batch.New(batch.Options[empty]{
				FlushThreads: 1,
				FlushFunc: func(thread int, items []empty) error {
					atomic.AddInt64(&processed, int64(-len(items)))
					return nil
				},
			})
			defer bat.Close()

			N := b.N / size
			items := make([]empty, size)

			var completeWg sync.WaitGroup
			completeWg.Add(N)

			b.ReportAllocs()
			b.ResetTimer()

			for range N {
				go func() {
					atomic.AddInt64(&iterations, 1)
					atomic.AddInt64(&processed, int64(len(items)))
					err := bat.AddMany(items)
					require.NoError(b, err)
					completeWg.Done()
				}()
			}

			b.StopTimer()
			completeWg.Wait()

			require.Equal(b, iterations, int64(N))
			require.Equal(b, processed, int64(0))
		})
	}
}
