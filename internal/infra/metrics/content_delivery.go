package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultRollingWindowSize = 256

type rollingWindow struct {
	values []float64
	next   int
	count  int
	sum    float64
}

func newRollingWindow(size int) *rollingWindow {
	if size <= 0 {
		size = 1
	}
	return &rollingWindow{values: make([]float64, size)}
}

func (w *rollingWindow) Add(value float64) {
	if len(w.values) == 0 {
		return
	}
	if w.count < len(w.values) {
		w.values[w.next] = value
		w.sum += value
		w.count++
		w.next = (w.next + 1) % len(w.values)
		return
	}
	old := w.values[w.next]
	w.values[w.next] = value
	w.sum += value - old
	w.next = (w.next + 1) % len(w.values)
}

func (w *rollingWindow) Average() float64 {
	if w.count == 0 {
		return 0
	}
	return w.sum / float64(w.count)
}

func (w *rollingWindow) Count() int {
	return w.count
}

type ContentDeliveryMetrics struct {
	ActiveDownloads int64
	ActiveMerges    int64
	ActiveHLS       int64

	TotalDownloads int64
	TotalMerges    int64
	TotalFailures  int64

	HeadHits   int64
	HeadMisses int64

	DownloadBytes int64
	MergeBytes    int64

	MergeQueueDepth    int64
	MergeQueueCapacity int64
	TotalCancellations int64

	mu                   sync.Mutex
	windowSize           int
	durations            map[string]*rollingWindow
	throughput           map[string]*rollingWindow
	mergeQueueWait       *rollingWindow
	cancellationByReason map[string]int64
}

func NewContentDeliveryMetrics() *ContentDeliveryMetrics {
	return newContentDeliveryMetricsWithWindow(defaultRollingWindowSize)
}

func newContentDeliveryMetricsWithWindow(windowSize int) *ContentDeliveryMetrics {
	if windowSize <= 0 {
		windowSize = defaultRollingWindowSize
	}
	return &ContentDeliveryMetrics{
		windowSize:           windowSize,
		durations:            map[string]*rollingWindow{},
		throughput:           map[string]*rollingWindow{},
		mergeQueueWait:       newRollingWindow(windowSize),
		cancellationByReason: map[string]int64{},
	}
}

func (m *ContentDeliveryMetrics) IncActiveDownloads() { atomic.AddInt64(&m.ActiveDownloads, 1) }
func (m *ContentDeliveryMetrics) DecActiveDownloads() { atomic.AddInt64(&m.ActiveDownloads, -1) }
func (m *ContentDeliveryMetrics) IncActiveMerges()    { atomic.AddInt64(&m.ActiveMerges, 1) }
func (m *ContentDeliveryMetrics) DecActiveMerges()    { atomic.AddInt64(&m.ActiveMerges, -1) }
func (m *ContentDeliveryMetrics) IncActiveHLS()       { atomic.AddInt64(&m.ActiveHLS, 1) }
func (m *ContentDeliveryMetrics) DecActiveHLS()       { atomic.AddInt64(&m.ActiveHLS, -1) }
func (m *ContentDeliveryMetrics) IncHeadHit()         { atomic.AddInt64(&m.HeadHits, 1) }
func (m *ContentDeliveryMetrics) IncHeadMiss()        { atomic.AddInt64(&m.HeadMisses, 1) }

func (m *ContentDeliveryMetrics) AddDownload(bytes int64, d time.Duration) {
	atomic.AddInt64(&m.TotalDownloads, 1)
	atomic.AddInt64(&m.DownloadBytes, bytes)
	m.record("download", d, bytes)
}

func (m *ContentDeliveryMetrics) AddMerge(bytes int64, d time.Duration) {
	atomic.AddInt64(&m.TotalMerges, 1)
	atomic.AddInt64(&m.MergeBytes, bytes)
	m.record("merge", d, bytes)
}

func (m *ContentDeliveryMetrics) AddFailure() {
	atomic.AddInt64(&m.TotalFailures, 1)
}

func (m *ContentDeliveryMetrics) SetMergeQueueDepth(depth int) {
	if depth < 0 {
		depth = 0
	}
	atomic.StoreInt64(&m.MergeQueueDepth, int64(depth))
}

func (m *ContentDeliveryMetrics) SetMergeQueueCapacity(capacity int) {
	if capacity < 0 {
		capacity = 0
	}
	atomic.StoreInt64(&m.MergeQueueCapacity, int64(capacity))
}

func (m *ContentDeliveryMetrics) ObserveMergeQueueWait(d time.Duration) {
	if d < 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeQueueWait.Add(d.Seconds())
}

func (m *ContentDeliveryMetrics) AddCancellation(reason string) {
	atomic.AddInt64(&m.TotalCancellations, 1)
	clean := normalizeCancellationReason(reason)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancellationByReason[clean]++
}

func (m *ContentDeliveryMetrics) record(op string, d time.Duration, bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	durations, ok := m.durations[op]
	if !ok {
		durations = newRollingWindow(m.windowSize)
		m.durations[op] = durations
	}
	durations.Add(d.Seconds())
	if d > 0 {
		values, ok := m.throughput[op]
		if !ok {
			values = newRollingWindow(m.windowSize)
			m.throughput[op] = values
		}
		values.Add(float64(bytes) / d.Seconds())
	}
}

func (m *ContentDeliveryMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(m.PrometheusText()))
	})
}

func (m *ContentDeliveryMetrics) PrometheusText() string {
	lines := []string{
		"# HELP downaria_active_downloads Number of active downloads",
		"# TYPE downaria_active_downloads gauge",
		fmt.Sprintf("downaria_active_downloads %d", atomic.LoadInt64(&m.ActiveDownloads)),
		"# HELP downaria_active_merges Number of active merges",
		"# TYPE downaria_active_merges gauge",
		fmt.Sprintf("downaria_active_merges %d", atomic.LoadInt64(&m.ActiveMerges)),
		"# HELP downaria_active_hls_streams Number of active hls requests",
		"# TYPE downaria_active_hls_streams gauge",
		fmt.Sprintf("downaria_active_hls_streams %d", atomic.LoadInt64(&m.ActiveHLS)),
		"# TYPE downaria_total_downloads counter",
		fmt.Sprintf("downaria_total_downloads %d", atomic.LoadInt64(&m.TotalDownloads)),
		"# TYPE downaria_total_merges counter",
		fmt.Sprintf("downaria_total_merges %d", atomic.LoadInt64(&m.TotalMerges)),
		"# TYPE downaria_total_failures counter",
		fmt.Sprintf("downaria_total_failures %d", atomic.LoadInt64(&m.TotalFailures)),
		"# TYPE downaria_head_cache_hits counter",
		fmt.Sprintf("downaria_head_cache_hits %d", atomic.LoadInt64(&m.HeadHits)),
		"# TYPE downaria_head_cache_misses counter",
		fmt.Sprintf("downaria_head_cache_misses %d", atomic.LoadInt64(&m.HeadMisses)),
		"# HELP downaria_merge_queue_depth Current merge worker queue depth",
		"# TYPE downaria_merge_queue_depth gauge",
		fmt.Sprintf("downaria_merge_queue_depth %d", atomic.LoadInt64(&m.MergeQueueDepth)),
		"# HELP downaria_merge_queue_capacity Configured merge worker queue capacity",
		"# TYPE downaria_merge_queue_capacity gauge",
		fmt.Sprintf("downaria_merge_queue_capacity %d", atomic.LoadInt64(&m.MergeQueueCapacity)),
		"# TYPE downaria_total_cancellations counter",
		fmt.Sprintf("downaria_total_cancellations %d", atomic.LoadInt64(&m.TotalCancellations)),
		"# TYPE downaria_active_goroutines gauge",
		fmt.Sprintf("downaria_active_goroutines %d", runtime.NumGoroutine()),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	ops := make([]string, 0, len(m.durations))
	for op := range m.durations {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	for _, op := range ops {
		if m.durations[op].Count() > 0 {
			lines = append(lines, fmt.Sprintf("downaria_%s_duration_seconds_avg %.6f", op, m.durations[op].Average()))
		}
		if m.throughput[op].Count() > 0 {
			lines = append(lines, fmt.Sprintf("downaria_%s_throughput_bytes_per_second_avg %.2f", op, m.throughput[op].Average()))
		}
	}
	if m.mergeQueueWait.Count() > 0 {
		lines = append(lines, fmt.Sprintf("downaria_merge_queue_wait_seconds_avg %.6f", m.mergeQueueWait.Average()))
	}
	reasons := make([]string, 0, len(m.cancellationByReason))
	for reason := range m.cancellationByReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		lines = append(lines, fmt.Sprintf("downaria_cancellations_by_reason_total{reason=%q} %d", reason, m.cancellationByReason[reason]))
	}

	return strings.Join(lines, "\n") + "\n"
}

func normalizeCancellationReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "unspecified"
	}
	return trimmed
}
