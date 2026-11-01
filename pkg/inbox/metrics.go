package inbox

import "github.com/prometheus/client_golang/prometheus"

// Metrics are shared by every consumer of a service; register them once.
type Metrics struct {
	messages *prometheus.CounterVec
	parked   *prometheus.CounterVec
	dlqDepth *prometheus.GaugeVec
}

// NewMetrics returns unregistered inbox metrics.
func NewMetrics() *Metrics {
	return &Metrics{
		messages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "inbox_messages_total",
			Help: "Consumed messages by outcome; retryable counts every failed attempt.",
		}, []string{"consumer", "topic", "outcome"}),
		parked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "inbox_dlq_parked_total",
			Help: "Messages parked on <topic>.dlq after the last retryable attempt.",
		}, []string{"topic"}),
		dlqDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "inbox_dlq_depth",
			Help: "Messages on <topic>.dlq not yet redriven (make dlq-redrive).",
		}, []string{"topic"}),
	}
}

// Collectors are the metrics to register.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.messages, m.parked, m.dlqDepth}
}
