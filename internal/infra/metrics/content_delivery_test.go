package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestMetricsPrometheusText(t *testing.T) {
	m := NewContentDeliveryMetrics()
	m.IncActiveDownloads()
	m.AddDownload(1024, 100*time.Millisecond)
	out := m.PrometheusText()
	if !strings.Contains(out, "downaria_active_downloads") {
		t.Fatalf("missing metric output")
	}
}

func TestMetricsBoundedRollingWindow(t *testing.T) {
	m := newContentDeliveryMetricsWithWindow(3)
	m.AddDownload(100, 1*time.Second)
	m.AddDownload(100, 2*time.Second)
	m.AddDownload(100, 3*time.Second)
	m.AddDownload(100, 4*time.Second)

	m.mu.Lock()
	dCount := m.durations["download"].Count()
	tCount := m.throughput["download"].Count()
	m.mu.Unlock()

	if dCount != 3 {
		t.Fatalf("expected bounded duration count 3, got %d", dCount)
	}
	if tCount != 3 {
		t.Fatalf("expected bounded throughput count 3, got %d", tCount)
	}

	out := m.PrometheusText()
	if !strings.Contains(out, "downaria_download_duration_seconds_avg 3.000000") {
		t.Fatalf("expected rolling duration average for last 3 values, got: %s", out)
	}
	if !strings.Contains(out, "downaria_download_throughput_bytes_per_second_avg 36.11") {
		t.Fatalf("expected rolling throughput average for last 3 values, got: %s", out)
	}
}

func TestMetricsPrometheusTextIncludesQueueAndCancellationMetrics(t *testing.T) {
	m := NewContentDeliveryMetrics()
	m.SetMergeQueueDepth(4)
	m.SetMergeQueueCapacity(10)
	m.ObserveMergeQueueWait(100 * time.Millisecond)
	m.ObserveMergeQueueWait(200 * time.Millisecond)
	m.AddCancellation("context_canceled")
	m.AddCancellation("")

	out := m.PrometheusText()
	checks := []string{
		"downaria_merge_queue_depth 4",
		"downaria_merge_queue_capacity 10",
		"downaria_merge_queue_wait_seconds_avg 0.150000",
		"downaria_total_cancellations 2",
		"downaria_cancellations_by_reason_total{reason=\"context_canceled\"} 1",
		"downaria_cancellations_by_reason_total{reason=\"unspecified\"} 1",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Fatalf("missing expected metric line %q in output: %s", check, out)
		}
	}
}
