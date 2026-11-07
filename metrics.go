package batch

// Metrics contains the package statistics.
type Metrics struct {
	ServedCount        int64 // successfully flushed
	ServedWithErrCount int64 // failed to flush
	RejectedCount      int64 // failed to add to buffer

	FlushesCount          int64   // total number of flushes
	FlushesPerThreadCount []int64 // total number of flushes, distributed across the flush threads

	FlushIntervalMs int64 // the adaptive flush interval value at the moment, millisec
}

func newMetrics(threads int) Metrics {
	return Metrics{
		FlushesPerThreadCount: make([]int64, threads),
	}
}
