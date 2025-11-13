package swarmroute

import (
	"math/rand"
	"testing"
	"time"
)

// Helper to get current pos/neg for a service endpoint from snapshot
func getPosNeg(t *testing.T, sr *SwarmRoute, svc, addr string) (float64, float64) {
	t.Helper()
	snap := sr.PheromoneSnapshot()
	svcMap, ok := snap[svc]
	if !ok {
		t.Fatalf("service %s not found in snapshot", svc)
	}
	p, ok := svcMap[addr]
	if !ok {
		t.Fatalf("endpoint %s not found in snapshot for service %s", addr, svc)
	}
	return p.Pos, p.Neg
}

func TestPickEndpointNoEndpoints(t *testing.T) {
	rand.Seed(42)
	sr := NewSwarmRoute()
	// Unknown service should error
	if _, err := sr.PickEndpoint("missing"); err == nil {
		t.Fatalf("expected error when picking from missing service")
	}

	// Service with zero endpoints should also error
	sr.AddService("empty", []string{})
	if _, err := sr.PickEndpoint("empty"); err == nil {
		t.Fatalf("expected error when picking from empty service")
	}
}

func TestSelectionBiasToLowerLatency(t *testing.T) {
	rand.Seed(7)
	sr := NewSwarmRoute()
	// Disable background drift for stable probabilities
	sr.evaporationRate = 0
	svc := "api"
	a := "A"
	b := "B"
	sr.AddService(svc, []string{a, b})

	// Reinforce A with lower latency; reinforce B with higher latency
	for i := 0; i < 200; i++ {
		sr.ReportResult(svc, a, 0.01, true) // bigger positive pheromone
		sr.ReportResult(svc, b, 0.5, true)  // smaller positive pheromone
	}

	// Sample selections
	total := 1000
	countA := 0
	for i := 0; i < total; i++ {
		addr, err := sr.PickEndpoint(svc)
		if err != nil {
			t.Fatalf("unexpected error picking endpoint: %v", err)
		}
		if addr == a {
			countA++
		}
	}

	if countA < int(float64(total)*0.9) {
		t.Fatalf("expected strong bias toward low-latency endpoint A; got %d/%d picks", countA, total)
	}
}

func TestNegativeReinforcementReducesSelection(t *testing.T) {
	rand.Seed(11)
	sr := NewSwarmRoute()
	sr.evaporationRate = 0 // keep negative pheromone from evaporating during the test
	svc := "svc"
	a := "A"
	b := "B"
	sr.AddService(svc, []string{a, b})

	// Penalize endpoint B with repeated failures
	for i := 0; i < 100; i++ {
		sr.ReportResult(svc, b, 0, false)
	}

	// Now A should be chosen overwhelmingly more often than B
	total := 1000
	countB := 0
	for i := 0; i < total; i++ {
		addr, err := sr.PickEndpoint(svc)
		if err != nil {
			t.Fatalf("unexpected error picking endpoint: %v", err)
		}
		if addr == b {
			countB++
		}
	}

	// With 100 negative pheromone on B and exploration epsilon 0.1 in weight, B's probability is ~ <1%
	if countB > int(float64(total)*0.05) {
		t.Fatalf("expected endpoint B to be rarely chosen after negative reinforcement; got %d/%d picks", countB, total)
	}
}

func TestSuccessReducesErrorNeg(t *testing.T) {
	rand.Seed(23)
	sr := NewSwarmRoute()
	svc := "svc"
	a := "A"
	sr.AddService(svc, []string{a})

	// Create some error pheromone with a failure
	sr.ReportResult(svc, a, 0, false)
	_, negBefore := getPosNeg(t, sr, svc, a)
	if negBefore <= 0 {
		t.Fatalf("expected negative pheromone after failure, got %f", negBefore)
	}

	// Set a known evaporationRate and report success; success should reduce error by (1 - evaporationRate)
	sr.evaporationRate = 0.4
	sr.ReportResult(svc, a, 0.05, true)
	_, negAfter := getPosNeg(t, sr, svc, a)

	expectedMax := negBefore // should not increase
	expectedMin := negBefore*(1-sr.evaporationRate) - 1e-9
	if !(negAfter <= expectedMax && negAfter >= expectedMin) {
		t.Fatalf("expected neg after success to be roughly negBefore*(1-evapRate). before=%f after=%f evap=%f", negBefore, negAfter, sr.evaporationRate)
	}
}

func TestEvaporationLoopDecaysValues(t *testing.T) {
	rand.Seed(99)
	sr := NewSwarmRoute()
	// Use a high evaporation rate to observe noticeable decay quickly
	sr.evaporationRate = 0.5
	svc := "svc"
	a := "A"
	sr.AddService(svc, []string{a})

	// Deposit both positive and negative pheromones
	sr.ReportResult(svc, a, 0.01, true) // pos increase
	sr.ReportResult(svc, a, 0, false)   // neg increase

	posBefore, negBefore := getPosNeg(t, sr, svc, a)
	if posBefore <= 0 || negBefore <= 0 {
		t.Fatalf("expected positive pos and neg before evaporation; got pos=%f neg=%f", posBefore, negBefore)
	}

	// Wait a bit over one tick to allow evaporationLoop to run at least once
	time.Sleep(1200 * time.Millisecond)

	posAfter, negAfter := getPosNeg(t, sr, svc, a)
	if !(posAfter < posBefore) {
		t.Fatalf("expected pos pheromone to decay; before=%f after=%f", posBefore, posAfter)
	}
	if !(negAfter < negBefore) {
		t.Fatalf("expected neg pheromone to decay; before=%f after=%f", negBefore, negAfter)
	}
}
