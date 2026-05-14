package batch

type Metrics struct {
	ServedCount        int64
	ServedWithErrCount int64
	RejectedCount      int64

	FlushesCount          int64
	FlushesPerThreadCount []int64
}

func newMetrics(threads int) Metrics {
	return Metrics{
		FlushesPerThreadCount: make([]int64, threads),
	}
}
