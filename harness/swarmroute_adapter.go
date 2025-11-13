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

package harness

import (
	lib "swarmroute"
)

// SwarmRouteAdapter satisfies the Strategy interface by delegating to the library.
type SwarmRouteAdapter struct {
	sr *lib.SwarmRoute
}

func NewSwarmRouteAdapter() *SwarmRouteAdapter {
	a := &SwarmRouteAdapter{sr: lib.NewSwarmRoute()}
	// Configure tuning for simulation runs:
	// - Decouple evaporation from wall-clock: half-life ~2000 requests.
	a.sr.SetRequestEvapRate(0.0003466)
	// - Lower base weight to allow truly bad endpoints to sink closer to zero.
	a.sr.SetBaseWeight(0.05)
	// - Tune positive vs negative update scales for net-negative expected update on bad endpoints.
	a.sr.SetPosNegScale(0.25, 1.2)
	// - Treat slow-but-successful as bad; set decay of positive pheromone on bad events.
	a.sr.SetSlowThresholdSec(0.070) // ~70ms ≈ 2x healthy target
	a.sr.SetBadPosDecay(0.20)
	// - Optional periodic exploration to avoid over-concentration (every 500 picks).
	a.sr.SetPeriodicExploration(500, 3.0)
	return a
}

func (a *SwarmRouteAdapter) Name() string { return "SwarmRoute" }

func (a *SwarmRouteAdapter) AddService(name string, endpoints []string) {
	a.sr.AddService(name, endpoints)
}

func (a *SwarmRouteAdapter) PickEndpoint(service string) (string, error) {
	return a.sr.PickEndpoint(service)
}

func (a *SwarmRouteAdapter) ReportResult(service, endpoint string, latencySec float64, success bool) {
	a.sr.ReportResult(service, endpoint, latencySec, success)
}
