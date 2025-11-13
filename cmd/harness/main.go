package main

import (
	"fmt"
	"swarmroute/harness"
)

// A tiny world simulator entrypoint to compare SwarmRoute against baseline balancers.
func main() {
	// Define a simple scenario: two healthy endpoints, then one degrades mid-run, later recovers.
	svc := "api"
	// Provide per-endpoint jitter (stddev) ~30% of mean latency
	e1 := harness.EndpointSpec{Addr: "http://a:8080", MeanLatencySec: 0.030, JitterSec: 0.009, ErrorRate: 0.01}
	e2 := harness.EndpointSpec{Addr: "http://b:8080", MeanLatencySec: 0.035, JitterSec: 0.0105, ErrorRate: 0.01}
	e3 := harness.EndpointSpec{Addr: "http://c:8080", MeanLatencySec: 0.040, JitterSec: 0.012, ErrorRate: 0.02}

	// Events: at 2,000 requests, endpoint b gets slower and error-prone; at 6,000 it recovers.
	slowLat := 0.120
	highErr := 0.20
	normLat := e2.MeanLatencySec
	normErr := e2.ErrorRate

	sc := harness.Scenario{
		Service:   svc,
		Endpoints: []harness.EndpointSpec{e1, e2, e3},
		Events: []harness.EnvironmentEvent{
			{Step: 2000, Endpoint: e2.Addr, NewMeanLatency: &slowLat, NewErrorRate: &highErr},
			{Step: 6000, Endpoint: e2.Addr, NewMeanLatency: &normLat, NewErrorRate: &normErr},
		},
		TotalRequests: 10000,
		// Pin the seed for reproducible runs; change if you want different runs.
		Seed: 123456789,
	}

	strategies := []harness.Strategy{
		harness.NewRandomStrategy(1),
		harness.NewRoundRobinStrategy(),
		harness.NewPowerOfTwoChoicesStrategy(2, 0.2),
		harness.NewLeastLatencyStrategy(3, 0.2),
		harness.NewSwarmRouteAdapter(),
	}

	// Print seed so results can be reproduced exactly.
	fmt.Printf("seed=%d\n", sc.Seed)
	results := harness.RunAll(sc, strategies)
	fmt.Print(harness.FormatResults(results))
}
