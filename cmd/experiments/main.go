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

package main

import (
	"fmt"
	"swarmroute/harness"
)

func main() {
	seeds := []int64{1, 2, 3, 42, 123456, 987654321}
	fmt.Printf("seeds=%v\n", seeds)

	// Base 3-endpoint scenario with degrade window 2000-6000 on b
	base := baseScenario()
	strategies := createStrategies()
	fmt.Println("\n=== Base scenario (3 endpoints, degrade b at 2000, recover at 6000) ===")
	aggs := harness.AggregateMultiSeed(base, strategies, seeds)
	fmt.Print(harness.FormatAggregatedResults(aggs))

	// Harder scenario A: 10 endpoints, degrade two at different times (one at 2000 so bad-window share applies)
	fmt.Println("\n=== Harder A: 10 endpoints; degrade e3 at 2000 and e7 at 3500; recover later ===")
	many := manyEndpointsScenario()
	aggs = harness.AggregateMultiSeed(many, strategies, seeds)
	fmt.Print(harness.FormatAggregatedResults(aggs))

	// Harder scenario B: Drift on b from 35ms->120ms between 2000..4000; drift back 6000..8000
	fmt.Println("\n=== Harder B: Drift (b ramps latency 35->120ms from 2000..4000, then recovers 6000..8000) ===")
	drift := driftScenario()
	aggs = harness.AggregateMultiSeed(drift, strategies, seeds)
	fmt.Print(harness.FormatAggregatedResults(aggs))

	// Harder scenario C: Flaky-but-fast endpoint; we mark it with an event at 2000 to keep bad-window metric meaningful
	fmt.Println("\n=== Harder C: Flaky-but-fast (one very fast endpoint with ~35% error) ===")
	flaky := flakyFastScenario()
	aggs = harness.AggregateMultiSeed(flaky, strategies, seeds)
	fmt.Print(harness.FormatAggregatedResults(aggs))
}

func createStrategies() []harness.Strategy {
	return []harness.Strategy{
		harness.NewRandomStrategy(1),
		harness.NewRoundRobinStrategy(),
		harness.NewPowerOfTwoChoicesStrategy(2, 0.2),
		harness.NewLeastLatencyStrategy(3, 0.2),
		harness.NewSwarmRouteAdapter(),
	}
}

func baseScenario() harness.Scenario {
	svc := "api"
	e1 := harness.EndpointSpec{Addr: "http://a:8080", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 0.01}
	e2 := harness.EndpointSpec{Addr: "http://b:8080", MeanLatencySec: 0.035, JitterSec: 0.0105, ErrorRate: 0.01}
	e3 := harness.EndpointSpec{Addr: "http://c:8080", MeanLatencySec: 0.040, JitterSec: 0.012, ErrorRate: 0.02}
	slowLat := 0.120
	highErr := 0.20
	normLat := e2.MeanLatencySec
	normErr := e2.ErrorRate
	return harness.Scenario{
		Service:       svc,
		Endpoints:     []harness.EndpointSpec{e1, e2, e3},
		Events:        []harness.EnvironmentEvent{{Step: 2000, Endpoint: e2.Addr, NewMeanLatency: &slowLat, NewErrorRate: &highErr}, {Step: 6000, Endpoint: e2.Addr, NewMeanLatency: &normLat, NewErrorRate: &normErr}},
		TotalRequests: 10000,
	}
}

func manyEndpointsScenario() harness.Scenario {
	svc := "api"
	eps := make([]harness.EndpointSpec, 0, 10)
	// Create 10 endpoints with slight spread in latency and jitter
	base := 0.028
	for i := 0; i < 10; i++ {
		mean := base + float64(i)*0.004 // 28ms..64ms
		jitter := 0.3 * mean
		err := 0.01
		addr := fmt.Sprintf("http://e%d:8080", i+1)
		eps = append(eps, harness.EndpointSpec{Addr: addr, MeanLatencySec: mean, JitterSec: jitter, ErrorRate: err})
	}
	// Degrade e3 at 2000, e7 at 3500; recover at 7000 and 8000
	slow := 0.120
	highErr := 0.25
	norm3 := eps[2].MeanLatencySec
	norm7 := eps[6].MeanLatencySec
	normErr := eps[0].ErrorRate
	events := []harness.EnvironmentEvent{
		{Step: 2000, Endpoint: eps[2].Addr, NewMeanLatency: &slow, NewErrorRate: &highErr},
		{Step: 3500, Endpoint: eps[6].Addr, NewMeanLatency: &slow, NewErrorRate: &highErr},
		{Step: 7000, Endpoint: eps[2].Addr, NewMeanLatency: &norm3, NewErrorRate: &normErr},
		{Step: 8000, Endpoint: eps[6].Addr, NewMeanLatency: &norm7, NewErrorRate: &normErr},
	}
	return harness.Scenario{Service: svc, Endpoints: eps, Events: events, TotalRequests: 12000}
}

func driftScenario() harness.Scenario {
	svc := "api"
	a := harness.EndpointSpec{Addr: "http://a:8080", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 0.01}
	b := harness.EndpointSpec{Addr: "http://b:8080", MeanLatencySec: 0.035, JitterSec: 0.0105, ErrorRate: 0.01}
	c := harness.EndpointSpec{Addr: "http://c:8080", MeanLatencySec: 0.040, JitterSec: 0.012, ErrorRate: 0.02}
	// Create incremental latency increases for b from step 2000 to 4000, then decreases 6000 to 8000
	events := []harness.EnvironmentEvent{}
	// Ramp up 35ms -> 120ms over 10 steps
	for i := 0; i <= 10; i++ {
		step := 2000 + i*200
		val := 0.035 + (0.120-0.035)*(float64(i)/10.0)
		v := val
		events = append(events, harness.EnvironmentEvent{Step: step, Endpoint: b.Addr, NewMeanLatency: &v})
	}
	// Increase error slightly during ramp
	hiErr := 0.20
	events = append(events, harness.EnvironmentEvent{Step: 3000, Endpoint: b.Addr, NewErrorRate: &hiErr})
	// Ramp down 0.120 -> 0.035 from 6000 to 8000
	for i := 0; i <= 10; i++ {
		step := 6000 + i*200
		val := 0.120 - (0.120-0.035)*(float64(i)/10.0)
		v := val
		events = append(events, harness.EnvironmentEvent{Step: step, Endpoint: b.Addr, NewMeanLatency: &v})
	}
	normErr := b.ErrorRate
	events = append(events, harness.EnvironmentEvent{Step: 8000, Endpoint: b.Addr, NewErrorRate: &normErr})
	return harness.Scenario{Service: svc, Endpoints: []harness.EndpointSpec{a, b, c}, Events: events, TotalRequests: 10000}
}

func flakyFastScenario() harness.Scenario {
	svc := "api"
	fast := harness.EndpointSpec{Addr: "http://fast:8080", MeanLatencySec: 0.020, JitterSec: 0.006, ErrorRate: 0.05}
	med := harness.EndpointSpec{Addr: "http://med:8080", MeanLatencySec: 0.035, JitterSec: 0.0105, ErrorRate: 0.01}
	slow := harness.EndpointSpec{Addr: "http://slow:8080", MeanLatencySec: 0.045, JitterSec: 0.0135, ErrorRate: 0.01}
	// At 2000, fast becomes very flaky (so bad-window share refers to fast)
	highErr := 0.35
	events := []harness.EnvironmentEvent{{Step: 2000, Endpoint: fast.Addr, NewErrorRate: &highErr}}
	// Recover at 6000
	normErr := fast.ErrorRate
	events = append(events, harness.EnvironmentEvent{Step: 6000, Endpoint: fast.Addr, NewErrorRate: &normErr})
	return harness.Scenario{Service: svc, Endpoints: []harness.EndpointSpec{fast, med, slow}, Events: events, TotalRequests: 10000}
}
