package batch

// Metrics contains the package statistics.
type Metrics struct {
	ServedCount        int64 // successfully flushed
	ServedWithErrCount int64 // failed to flush
	RejectedCount      int64 // failed to add to buffer

	FlushesCount          int64   // total number of flushes
	FlushesPerThreadCount []int64 // total number of flushes, distributed across the flush threads
}

func newMetrics(threads int) Metrics {
	return Metrics{
		FlushesPerThreadCount: make([]int64, threads),
	}
}
