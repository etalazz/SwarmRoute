// Copyright 2025 Esteban Alvarez. All Rights Reserved.
//
// Created: November 2025
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package swarmroute

import (
	"math"
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

	// Assert positive pheromone increased more for A than B
	posA, _ := getPosNeg(t, sr, svc, a)
	posB, _ := getPosNeg(t, sr, svc, b)
	if !(posA > posB && posA > 0) {
		t.Fatalf("expected A to have higher positive pheromone than B: posA=%f posB=%f", posA, posB)
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

func TestEvaporationExactTick(t *testing.T) {
	rand.Seed(101)
	sr := NewSwarmRoute()
	sr.evaporationRate = 0.2 // 20%
	svc := "svc"
	a := "A"
	sr.AddService(svc, []string{a})

	// Set exact starting values
	sr.mu.Lock()
	ep := sr.services[svc][0]
	ep.Pheromones["latency"].Pos = 1.0
	ep.Pheromones["error"].Neg = 0.5
	sr.mu.Unlock()

	sr.evaporateOnce()

	posAfter, negAfter := getPosNeg(t, sr, svc, a)
	if math.Abs(posAfter-0.8) > 1e-12 {
		t.Fatalf("expected pos to be 0.8 after one tick, got %f", posAfter)
	}
	if math.Abs(negAfter-0.4) > 1e-12 {
		t.Fatalf("expected neg to be 0.4 after one tick, got %f", negAfter)
	}
}

func TestExplorationNonZeroOtherEndpoints(t *testing.T) {
	rand.Seed(2024)
	sr := NewSwarmRoute()
	sr.evaporationRate = 0
	svc := "svc"
	a, b, c := "A", "B", "C"
	sr.AddService(svc, []string{a, b, c})

	// Make A dominate by positive pheromone; others remain at 0
	sr.mu.Lock()
	sr.services[svc][0].Pheromones["latency"].Pos = 100.0
	sr.services[svc][0].Pheromones["error"].Neg = 0.0
	sr.services[svc][1].Pheromones["latency"].Pos = 0.0
	sr.services[svc][1].Pheromones["error"].Neg = 0.0
	sr.services[svc][2].Pheromones["latency"].Pos = 0.0
	sr.services[svc][2].Pheromones["error"].Neg = 0.0
	sr.mu.Unlock()

	total := 20000
	countA, countB, countC := 0, 0, 0
	for i := 0; i < total; i++ {
		addr, err := sr.PickEndpoint(svc)
		if err != nil {
			t.Fatalf("unexpected error picking endpoint: %v", err)
		}
		switch addr {
		case a:
			countA++
		case b:
			countB++
		case c:
			countC++
		}
	}

	if countA < int(0.8*float64(total)) {
		t.Fatalf("expected high-pheromone endpoint A to dominate, got %d/%d", countA, total)
	}
	if countB == 0 || countC == 0 {
		t.Fatalf("expected exploration to give non-zero selections to others; got B=%d C=%d", countB, countC)
	}
}
