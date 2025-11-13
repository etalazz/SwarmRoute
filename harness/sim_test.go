package harness

import (
	"testing"
)

// TestAlwaysBadEndpoint ensures SwarmRoute rapidly avoids a 100% failing endpoint
// and keeps its selection share near zero (aside from periodic exploration).
func TestAlwaysBadEndpoint(t *testing.T) {
	svc := "svc"
	good := EndpointSpec{Addr: "good", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 0.0}
	bad := EndpointSpec{Addr: "bad", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 1.0}
	sc := Scenario{
		Service:       svc,
		Endpoints:     []EndpointSpec{good, bad},
		TotalRequests: 3000,
		Seed:          42,
	}
	sr := NewSwarmRouteAdapter()
	r := RunScenario(sc, sr)
	total := r.Total
	badSel := r.Selection[bad.Addr]
	share := float64(badSel) / float64(total)
	if share > 0.03 { // allow a small fraction due to exploration
		t.Fatalf("bad endpoint share too high: got %.2f%% (sel=%d/%d)", 100*share, badSel, total)
	}
}

// TestAlwaysSlowEndpoint ensures SwarmRoute treats slow-but-successful endpoints as bad
// and routes only a small fraction of traffic to them under the configured threshold.
func TestAlwaysSlowEndpoint(t *testing.T) {
	svc := "svc"
	fast := EndpointSpec{Addr: "fast", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 0.0}
	slow := EndpointSpec{Addr: "slow", MeanLatencySec: 0.120, JitterSec: 0.036, ErrorRate: 0.0}
	sc := Scenario{
		Service:       svc,
		Endpoints:     []EndpointSpec{fast, slow},
		TotalRequests: 3000,
		Seed:          424242,
	}
	sr := NewSwarmRouteAdapter()
	r := RunScenario(sc, sr)
	total := r.Total
	slowSel := r.Selection[slow.Addr]
	share := float64(slowSel) / float64(total)
	if share > 0.15 { // should be kept relatively low under latency-aware penalty
		t.Fatalf("slow endpoint share too high: got %.2f%% (sel=%d/%d)", 100*share, slowSel, total)
	}
}
