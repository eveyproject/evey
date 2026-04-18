package eventbus

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	eventsDroppedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "eventbus_events_dropped_total",
		Help: "Total number of events dropped by the eventbus (queue full, bus stopped, etc.).",
	}, []string{"event_name"})

	queueDepthGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "eventbus_queue_depth",
		Help: "Current depth of the eventbus dispatch queue.",
	})
)

func init() {
	prometheus.MustRegister(eventsDroppedTotal, queueDepthGauge)
}
