package batch

type Metrics struct {
	ItemsReceived  int64
	ItemsCompleted int64

	Flushes []int64
}

func NewMetrics(threads int) Metrics {
	return Metrics{
		Flushes: make([]int64, threads),
	}
}

func (m *Metrics) FlushesTotal() int64 {
	var total int64
	for _, v := range m.Flushes {
		total += v
	}
	return total
}
