package participant

import (
	"errors"
	"net/http"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type realMetrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	packets  *prometheus.CounterVec
}

func newRealMetrics(server *RealServer) *realMetrics {
	m := &realMetrics{registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "real_dkg_requests_total", Help: "Real participant RPC requests by bounded operation and result.",
		}, []string{"operation", "result"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "real_dkg_request_duration_seconds", Help: "Real participant RPC latency in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		}, []string{"operation"}),
		packets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "real_dkg_packets_total", Help: "Real DKG packet acceptance by bounded outcome.",
		}, []string{"outcome"}),
	}
	phase := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "real_dkg_participant_phase_code",
		Help: "Current real DKG phase: INIT=0, DEAL=1, COLLECT_DEALS=2, COLLECT_RESPONSES=3, COLLECT_JUSTIFICATIONS=4, FINALIZE=5, TIMED_OUT=6, ABORTED=7.",
	}, func() float64 {
		server.mu.Lock()
		defer server.mu.Unlock()
		switch server.node.Stage() {
		case "DEAL":
			return 1
		case "COLLECT_DEALS":
			return 2
		case "COLLECT_RESPONSES":
			return 3
		case "COLLECT_JUSTIFICATIONS":
			return 4
		case "FINALIZE":
			return 5
		case "TIMED_OUT":
			return 6
		case "ABORTED":
			return 7
		default:
			return 0
		}
	})
	m.registry.MustRegister(m.requests, m.latency, m.packets, phase)
	return m
}

func (m *realMetrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *realMetrics) observeRequest(operation string, ok bool, duration time.Duration) {
	switch operation {
	case "real-identity", "real-configure", "real-deals", "real-accept", "real-process-deals", "real-process-responses", "real-process-justifications", "real-result", "real-timeout", "real-abort", "real-status", "real-p2p-start", "decode":
	default:
		operation = "unknown"
	}
	result := "error"
	if ok {
		result = "success"
	}
	m.requests.WithLabelValues(operation, result).Inc()
	m.latency.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *realMetrics) observePacket(duplicate bool, err error) {
	outcome := "accepted"
	if duplicate {
		outcome = "duplicate"
	} else if errors.Is(err, cryptoadapter.ErrStaleSession) {
		outcome = "stale"
	} else if err != nil {
		outcome = "error"
	}
	m.packets.WithLabelValues(outcome).Inc()
}
