package goddgs

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusCollectorObserveCircuitEvent(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewPrometheusCollector(reg)

	c.ObserveCircuitEvent("ddg", CircuitEvent{State: CircuitStateOpen, Trigger: "threshold_reached"})
	c.ObserveCircuitEvent("ddg", CircuitEvent{State: CircuitStateOpen, Trigger: "fail_fast"})
	c.ObserveCircuitEvent("ddg", CircuitEvent{State: CircuitStateClosed, Trigger: "success_reset"})

	openCount := testutil.ToFloat64(c.circuitEvents.WithLabelValues("ddg", string(CircuitStateOpen), "threshold_reached"))
	if openCount != 1 {
		t.Fatalf("open transition count = %v, want 1", openCount)
	}
	ffCount := testutil.ToFloat64(c.circuitEvents.WithLabelValues("ddg", string(CircuitStateOpen), "fail_fast"))
	if ffCount != 1 {
		t.Fatalf("fail-fast count = %v, want 1", ffCount)
	}
	closedCount := testutil.ToFloat64(c.circuitEvents.WithLabelValues("ddg", string(CircuitStateClosed), "success_reset"))
	if closedCount != 1 {
		t.Fatalf("closed transition count = %v, want 1", closedCount)
	}

	openGauge := testutil.ToFloat64(c.circuitOpen.WithLabelValues("ddg"))
	if openGauge != 0 {
		t.Fatalf("final circuit gauge = %v, want 0 (closed)", openGauge)
	}
}
