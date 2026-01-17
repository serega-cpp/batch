package batch

type Metrics struct {
	IncomingCount int64
	ServedCount   int64

	FlushesCount          int64
	FlushesPerThreadCount []int64
}

func newMetrics(threads int) Metrics {
	return Metrics{
		FlushesPerThreadCount: make([]int64, threads),
	}
}
