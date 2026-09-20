package mqtt

import "github.com/prometheus/client_golang/prometheus"

// Metrics uses bounded, enumerated labels only. Callers can provide the
// server's registry; nil keeps the package usable in isolated tests.
type Metrics struct {
	Rejections         *prometheus.CounterVec
	ContractViolations *prometheus.CounterVec
	Accepted           *prometheus.CounterVec
	Rejected           *prometheus.CounterVec
	Retries            *prometheus.CounterVec
	Timeouts           *prometheus.CounterVec
	QueueDrops         *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Rejections:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_rejections_total", Help: "Rejected MQTT ingress messages."}, []string{"reason"}),
		ContractViolations: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_contract_violations_total", Help: "MQTT ingress contract violations tolerated during delivery."}, []string{"kind"}),
		Accepted:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_accepted_total", Help: "Accepted MQTT ingress messages."}, []string{"kind"}),
		Rejected:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_rejected_total", Help: "Permanently rejected MQTT ingress messages."}, []string{"kind"}),
		Retries:            prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_retries_total", Help: "MQTT ingress messages requested for retry."}, []string{"kind"}),
		Timeouts:           prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_consumer_timeouts_total", Help: "MQTT ingress consumer timeouts."}, []string{"kind"}),
		QueueDrops:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mqtt_ingress_queue_drops_total", Help: "Dropped MQTT ingress messages."}, []string{"queue"}),
	}
	if reg != nil {
		for _, collector := range []prometheus.Collector{m.Rejections, m.ContractViolations, m.Accepted, m.Rejected, m.Retries, m.Timeouts, m.QueueDrops} {
			_ = reg.Register(collector)
		}
	}
	return m
}
