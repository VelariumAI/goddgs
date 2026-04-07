package goddgs

import (
	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusCollector struct {
	requestsTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	blocksTotal   *prometheus.CounterVec
	fallbacks     *prometheus.CounterVec
	providerUp    *prometheus.GaugeVec
	circuitEvents *prometheus.CounterVec
	circuitOpen   *prometheus.GaugeVec
}

func NewPrometheusCollector(reg prometheus.Registerer) *PrometheusCollector {
	c := &PrometheusCollector{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goddgs_requests_total", Help: "Total provider requests"}, []string{"provider", "status"}),
		duration:      prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "goddgs_request_duration_seconds", Help: "Provider request duration", Buckets: prometheus.DefBuckets}, []string{"provider"}),
		blocksTotal:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goddgs_block_events_total", Help: "Detected block events"}, []string{"provider", "signal"}),
		fallbacks:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goddgs_fallback_transitions_total", Help: "Fallback transitions"}, []string{"provider", "kind"}),
		providerUp:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goddgs_provider_enabled", Help: "Provider enabled state"}, []string{"provider"}),
		circuitEvents: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "goddgs_circuit_events_total", Help: "Circuit breaker state events"}, []string{"provider", "state", "trigger"}),
		circuitOpen:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "goddgs_circuit_open", Help: "Circuit breaker open state (1=open, 0=closed)"}, []string{"provider"}),
	}
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	reg.MustRegister(c.requestsTotal, c.duration, c.blocksTotal, c.fallbacks, c.providerUp, c.circuitEvents, c.circuitOpen)
	return c
}

func (c *PrometheusCollector) Hook(ev Event) {
	switch ev.Type {
	case EventProviderEnd:
		status := "error"
		if ev.Success {
			status = "success"
		}
		c.requestsTotal.WithLabelValues(ev.Provider, status).Inc()
		c.duration.WithLabelValues(ev.Provider).Observe(ev.Duration.Seconds())
	case EventBlocked:
		sig := "generic"
		if ev.Block != nil && ev.Block.Signal.String() != "" {
			sig = ev.Block.Signal.String()
		}
		c.blocksTotal.WithLabelValues(ev.Provider, sig).Inc()
	case EventFallback:
		kind := string(ev.ErrKind)
		if kind == "" {
			kind = "unknown"
		}
		c.fallbacks.WithLabelValues(ev.Provider, kind).Inc()
	}
}

func (c *PrometheusCollector) SetProviderEnabled(provider string, enabled bool) {
	if enabled {
		c.providerUp.WithLabelValues(provider).Set(1)
	} else {
		c.providerUp.WithLabelValues(provider).Set(0)
	}
}

// ObserveCircuitEvent records low-level client breaker transitions.
func (c *PrometheusCollector) ObserveCircuitEvent(provider string, ev CircuitEvent) {
	if provider == "" {
		provider = "ddg"
	}
	state := string(ev.State)
	if state == "" {
		state = "unknown"
	}
	trigger := ev.Trigger
	if trigger == "" {
		trigger = "unknown"
	}
	c.circuitEvents.WithLabelValues(provider, state, trigger).Inc()
	if ev.State == CircuitStateOpen {
		c.circuitOpen.WithLabelValues(provider).Set(1)
	} else if ev.State == CircuitStateClosed {
		c.circuitOpen.WithLabelValues(provider).Set(0)
	}
}
