package participant

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	shares   *prometheus.CounterVec
}

func newMetrics(machine *protocol.Machine) *metrics {
	registry := prometheus.NewRegistry()
	m := &metrics{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "dkg_requests_total", Help: "Participant RPC requests by operation and result.",
		}, []string{"operation", "result"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "dkg_request_duration_seconds", Help: "Participant RPC request duration in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		}, []string{"operation"}),
		shares: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "dkg_share_deliveries_total", Help: "SHARE deliveries by bounded outcome.",
		}, []string{"outcome"}),
	}
	phase := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "dkg_participant_phase_code",
		Help: "Current participant phase: INIT=0, DEAL=1, SHARE_EXCHANGE=2, VERIFY=3, FINALIZE=4, TIMED_OUT=5.",
	}, func() float64 {
		switch machine.Status().Phase {
		case protocol.PhaseDeal:
			return 1
		case protocol.PhaseShareExchange:
			return 2
		case protocol.PhaseVerify:
			return 3
		case protocol.PhaseFinalize:
			return 4
		case protocol.PhaseTimedOut:
			return 5
		default:
			return 0
		}
	})
	registry.MustRegister(m.requests, m.latency, m.shares, phase)
	return m
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *metrics) observeRequest(operation string, ok bool, duration time.Duration) {
	switch operation {
	case "begin", "deliver", "finalize", "timeout", "status", "decode":
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

func (m *metrics) observeShare(outcome protocol.ShareOutcome, err error) {
	label := string(outcome)
	if err != nil {
		switch {
		case errors.Is(err, protocol.ErrStaleMessage):
			label = "stale"
		case errors.Is(err, protocol.ErrInvalidMessage):
			label = "invalid"
		default:
			label = "error"
		}
	}
	m.shares.WithLabelValues(label).Inc()
}
