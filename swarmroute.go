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
}

// NewSwarmRoute returns a new SwarmRoute with sensible defaults and starts
// a background evaporation goroutine.
func NewSwarmRoute() *SwarmRoute {
	sr := &SwarmRoute{
		services:        make(map[string][]*Endpoint),
		evaporationRate: 0.05, // 5% evaporation per second
		posReinforce:    1.0,
		negReinforce:    1.0,
	}
	go sr.evaporateLoop()
	return sr
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
	sr.mu.RLock()
	eps, ok := sr.services[service]
	sr.mu.RUnlock()
	if !ok || len(eps) == 0 {
		return "", fmt.Errorf("no endpoints for service %s", service)
	}
	weights := make([]float64, len(eps))
	total := 0.0
	for i, ep := range eps {
		// combine latency positive pheromone and error negative pheromone.
		pos := ep.Pheromones["latency"].Pos
		neg := ep.Pheromones["error"].Neg
		// avoid zero weight by adding a small constant.
		weight := (pos + 0.1) / (1.0 + neg)
		weights[i] = weight
		total += weight
	}
	// sample using cumulative distribution.
	r := rand.Float64() * total
	cum := 0.0
	for i, w := range weights {
		cum += w
		if r <= cum {
			return eps[i].Address, nil
		}
	}
	// fallback (should not happen).
	return eps[len(eps)-1].Address, nil
}

// ReportResult updates the pheromone values after a call has completed.  A
// successful call deposits positive pheromone inversely proportional to
// observed latency and slightly reduces accumulated error pheromone.  A
// failed call deposits negative pheromone.
func (sr *SwarmRoute) ReportResult(service, endpoint string, latency float64, success bool) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	eps, ok := sr.services[service]
	if !ok {
		return
	}
	for _, ep := range eps {
		if ep.Address == endpoint {
			if success {
				// deposit positive pheromone: better latency yields more pheromone.
				delta := sr.posReinforce / (latency + 1e-6)
				ep.Pheromones["latency"].Pos += delta
				// decay some of the error pheromone if present.
				ep.Pheromones["error"].Neg *= (1 - sr.evaporationRate)
			} else {
				// deposit negative pheromone on error channel.
				ep.Pheromones["error"].Neg += sr.negReinforce
			}
			break
		}
	}
}

// evaporateLoop runs in a separate goroutine and periodically decays all
// pheromone values to allow the system to forget outdated information.
func (sr *SwarmRoute) evaporateLoop() {
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		sr.mu.Lock()
		for _, eps := range sr.services {
			for _, ep := range eps {
				for _, p := range ep.Pheromones {
					p.Pos *= (1.0 - sr.evaporationRate)
					p.Neg *= (1.0 - sr.evaporationRate)
				}
			}
		}
		sr.mu.Unlock()
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
