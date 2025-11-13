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
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Pheromone holds positive and negative pheromone values for a QoS dimension.
// Positive pheromone encourages selection; negative pheromone discourages it.
type Pheromone struct {
	Pos float64
	Neg float64
}

// Endpoint represents a service instance with its pheromone metrics.
type Endpoint struct {
	Address    string
	Pheromones map[string]*Pheromone
}

// SwarmRoute maintains pheromone tables for multiple services and handles
// selection and updates.  It is safe for concurrent use.
type SwarmRoute struct {
	mu sync.RWMutex
	// services maps a service name to a slice of endpoints.
	services map[string][]*Endpoint
	// configuration parameters for pheromone updates.
	evaporationRate float64
	posReinforce    float64
	negReinforce    float64
	// Per-request evaporation decoupled from wall-clock. Each report multiplies
	// pheromones by (1 - reqEvapRate). Choose reqEvapRate ~ ln(2)/halfLifeRequests
	// to achieve a half-life in number of requests.
	reqEvapRate float64
	// Selection exploration base weight to ensure non-zero probability.
	baseWeight float64
	// Optional periodic forced exploration: every Nth pick (per service),
	// do a uniform random among endpoints that aren't clearly terrible.
	// 0 disables forced exploration.
	exploreEveryN int
	// Threshold of negative pheromone to consider an endpoint terrible during exploration.
	exploreNegThreshold float64
	// Per-service pick counters for periodic exploration.
	pickCount map[string]int
	// Slow threshold: if >0 and an observed latency exceeds this value, the
	// event is treated as "bad" even if it succeeded (slow-but-successful).
	slowThresholdSec float64
	// On bad events, reduce accumulated positive pheromone by this fraction
	// (0..1). Default 0 to preserve prior behavior.
	alphaBad float64
}

// NewSwarmRoute returns a new SwarmRoute with sensible defaults and starts
// a background evaporation goroutine.
func NewSwarmRoute() *SwarmRoute {
	sr := &SwarmRoute{
		services:            make(map[string][]*Endpoint),
		evaporationRate:     0.05, // 5% evaporation per second
		posReinforce:        1.0,
		negReinforce:        1.0,
		reqEvapRate:         0.0,  // disabled by default for backward compatibility
		baseWeight:          0.10, // original base exploration weight
		exploreEveryN:       0,    // disabled by default to keep behavior stable
		exploreNegThreshold: 3.0,
		pickCount:           make(map[string]int),
		slowThresholdSec:    0.0, // disabled by default
		alphaBad:            0.0, // no decay on bad events by default
	}
	go sr.evaporateLoop()
	return sr
}

// SetRequestEvapRate sets the per-request evaporation rate (0..1).
// A value around ln(2)/halfLifeRequests yields the desired half-life by requests.
func (sr *SwarmRoute) SetRequestEvapRate(r float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	sr.reqEvapRate = r
}

// SetBaseWeight adjusts the additive base weight used in selection to ensure
// non-zero probability even for cold endpoints.
func (sr *SwarmRoute) SetBaseWeight(w float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if w < 0 {
		w = 0
	}
	sr.baseWeight = w
}

// SetPosNegScale sets the positive and negative reinforcement scales.
func (sr *SwarmRoute) SetPosNegScale(kpos, kneg float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if kpos < 0 {
		kpos = 0
	}
	if kneg < 0 {
		kneg = 0
	}
	sr.posReinforce = kpos
	sr.negReinforce = kneg
}

// SetPeriodicExploration enables or disables periodic forced exploration.
// If everyN <= 0, exploration is disabled. negThreshold defines what is
// considered a "terrible" endpoint by its negative pheromone.
func (sr *SwarmRoute) SetPeriodicExploration(everyN int, negThreshold float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if everyN < 0 {
		everyN = 0
	}
	sr.exploreEveryN = everyN
	if negThreshold < 0 {
		negThreshold = 0
	}
	sr.exploreNegThreshold = negThreshold
}

// SetSlowThresholdSec sets the latency threshold in seconds beyond which a
// successful call is treated as a bad event (slow-but-successful). Set to <=0
// to disable this behavior.
func (sr *SwarmRoute) SetSlowThresholdSec(threshold float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if threshold < 0 {
		threshold = 0
	}
	sr.slowThresholdSec = threshold
}

// SetBadPosDecay configures the fraction (0..1) of positive pheromone to
// reduce on bad events (failures or slow successes). 0 disables the decay.
func (sr *SwarmRoute) SetBadPosDecay(alpha float64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	sr.alphaBad = alpha
}

// AddService registers a service with a list of endpoint addresses.  Each
// endpoint is initialized with empty pheromone values for two QoS channels:
// "latency" and "error".
func (sr *SwarmRoute) AddService(name string, endpoints []string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	eps := make([]*Endpoint, len(endpoints))
	for i, addr := range endpoints {
		eps[i] = &Endpoint{
			Address: addr,
			Pheromones: map[string]*Pheromone{
				"latency": {Pos: 0, Neg: 0},
				"error":   {Pos: 0, Neg: 0},
			},
		}
	}
	sr.services[name] = eps
}

// PickEndpoint selects an endpoint for the given service name using a
// weighted-random strategy based on pheromones.  Endpoints with higher
// positive pheromone and lower negative pheromone are more likely to be
// chosen.  It returns an error if the service has no endpoints.
func (sr *SwarmRoute) PickEndpoint(service string) (string, error) {
	sr.mu.Lock()
	eps, ok := sr.services[service]
	if !ok || len(eps) == 0 {
		sr.mu.Unlock()
		return "", fmt.Errorf("no endpoints for service %s", service)
	}
	// Periodic forced exploration if configured.
	sr.pickCount[service]++
	doExplore := sr.exploreEveryN > 0 && (sr.pickCount[service]%sr.exploreEveryN == 0)
	if doExplore {
		// Build a list of non-terrible endpoints based on negative pheromone.
		candidates := make([]*Endpoint, 0, len(eps))
		for _, ep := range eps {
			if ep.Pheromones["error"].Neg <= sr.exploreNegThreshold {
				candidates = append(candidates, ep)
			}
		}
		if len(candidates) == 0 { // fallback to all if none qualify
			candidates = eps
		}
		// Sample uniformly among candidates.
		idx := rand.Intn(len(candidates))
		addr := candidates[idx].Address
		sr.mu.Unlock()
		return addr, nil
	}
	// Snapshot needed values under lock to avoid races with background evaporation.
	type snap struct {
		addr     string
		pos, neg float64
	}
	snaps := make([]snap, len(eps))
	for i, ep := range eps {
		snaps[i] = snap{addr: ep.Address, pos: ep.Pheromones["latency"].Pos, neg: ep.Pheromones["error"].Neg}
	}
	baseWeight := sr.baseWeight
	sr.mu.Unlock()
	weights := make([]float64, len(snaps))
	total := 0.0
	for i, sp := range snaps {
		// combine latency positive pheromone and error negative pheromone.
		pos := sp.pos
		neg := sp.neg
		// avoid zero weight by adding a small constant.
		weight := (pos + baseWeight) / (1.0 + neg)
		weights[i] = weight
		total += weight
	}
	// sample using cumulative distribution.
	r := rand.Float64() * total
	cum := 0.0
	for i, w := range weights {
		cum += w
		if r <= cum {
			return snaps[i].addr, nil
		}
	}
	// fallback (should not happen).
	return snaps[len(snaps)-1].addr, nil
}

// ReportResult updates the pheromone values after a call has completed.  A
// successful call deposits positive pheromone inversely proportional to
// observed latency and slightly reduces accumulated error pheromone.  A
// failed call deposits negative pheromone.
func (sr *SwarmRoute) ReportResult(service, endpoint string, latency float64, success bool) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	// Apply per-request evaporation across all pheromones to decouple from wall-clock.
	if sr.reqEvapRate > 0 {
		factor := 1.0 - sr.reqEvapRate
		for _, eps := range sr.services {
			for _, ep := range eps {
				for _, p := range ep.Pheromones {
					p.Pos *= factor
					p.Neg *= factor
				}
			}
		}
	}
	eps, ok := sr.services[service]
	if !ok {
		return
	}
	isSlow := sr.slowThresholdSec > 0 && latency > sr.slowThresholdSec
	for _, ep := range eps {
		if ep.Address == endpoint {
			if !success || isSlow {
				// Treat failure or too-slow success as a bad event.
				ep.Pheromones["error"].Neg += sr.negReinforce
				if sr.alphaBad > 0 {
					ep.Pheromones["latency"].Pos *= (1 - sr.alphaBad)
				}
				// Note: do not add positive pheromone on slow successes.
				if success {
					// still allow a small forgiveness of error on success to avoid permanent stickiness
					ep.Pheromones["error"].Neg *= (1 - sr.evaporationRate)
				}
			} else {
				// Fast, successful call → deposit positive pheromone inversely to latency.
				delta := sr.posReinforce / (latency + 1e-6)
				ep.Pheromones["latency"].Pos += delta
				// decay some of the error pheromone if present.
				ep.Pheromones["error"].Neg *= (1 - sr.evaporationRate)
			}
			break
		}
	}
}

// evaporateOnce applies a single evaporation step to all pheromone values.
// It is unexported but testable by package tests for deterministic checks.
func (sr *SwarmRoute) evaporateOnce() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	for _, eps := range sr.services {
		for _, ep := range eps {
			for _, p := range ep.Pheromones {
				p.Pos *= (1.0 - sr.evaporationRate)
				p.Neg *= (1.0 - sr.evaporationRate)
			}
		}
	}
}

// evaporateLoop runs in a separate goroutine and periodically decays all
// pheromone values to allow the system to forget outdated information.
func (sr *SwarmRoute) evaporateLoop() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		sr.evaporateOnce()
	}
}

// PheromoneSnapshot returns a snapshot of current pheromone values for
// monitoring or debugging.  It can be exposed via telemetry.
func (sr *SwarmRoute) PheromoneSnapshot() map[string]map[string]Pheromone {
	snapshot := make(map[string]map[string]Pheromone)
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	for svc, eps := range sr.services {
		svcMap := make(map[string]Pheromone)
		for _, ep := range eps {
			// aggregate both positive and negative for display.
			// We'll store the combined metric: pos and neg separately.
			pos := ep.Pheromones["latency"].Pos
			neg := ep.Pheromones["error"].Neg
			svcMap[ep.Address] = Pheromone{Pos: pos, Neg: neg}
		}
		snapshot[svc] = svcMap
	}
	return snapshot
}
