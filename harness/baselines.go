package harness

import (
	"math"
	"math/rand"
)

// RandomStrategy selects uniformly at random among endpoints for a service.
type RandomStrategy struct {
	rng      *rand.Rand
	services map[string][]string
}

func NewRandomStrategy(seed int64) *RandomStrategy {
	return &RandomStrategy{rng: rand.New(rand.NewSource(seed)), services: make(map[string][]string)}
}

func (s *RandomStrategy) Name() string { return "Random" }

func (s *RandomStrategy) AddService(name string, endpoints []string) {
	s.services[name] = append([]string{}, endpoints...)
}

func (s *RandomStrategy) PickEndpoint(service string) (string, error) {
	eps := s.services[service]
	if len(eps) == 0 {
		return "", ErrNoEndpoints
	}
	return eps[s.rng.Intn(len(eps))], nil
}

func (s *RandomStrategy) ReportResult(service, endpoint string, latencySec float64, success bool) {}

// RoundRobinStrategy cycles endpoints in order per service.
type RoundRobinStrategy struct {
	services map[string][]string
	idx      map[string]int
}

func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{services: make(map[string][]string), idx: make(map[string]int)}
}

func (s *RoundRobinStrategy) Name() string { return "RoundRobin" }

func (s *RoundRobinStrategy) AddService(name string, endpoints []string) {
	s.services[name] = append([]string{}, endpoints...)
}

func (s *RoundRobinStrategy) PickEndpoint(service string) (string, error) {
	eps := s.services[service]
	if len(eps) == 0 {
		return "", ErrNoEndpoints
	}
	i := s.idx[service]
	addr := eps[i%len(eps)]
	s.idx[service] = (i + 1) % len(eps)
	return addr, nil
}

func (s *RoundRobinStrategy) ReportResult(service, endpoint string, latencySec float64, success bool) {
}

// PowerOfTwoChoicesStrategy samples two random endpoints and chooses the one
// with lower observed average latency (EWMA). If no data, falls back to random.
type PowerOfTwoChoicesStrategy struct {
	rng      *rand.Rand
	services map[string][]string
	ewma     map[string]map[string]float64 // service -> endpoint -> ewma seconds
	alpha    float64                       // smoothing factor
}

func NewPowerOfTwoChoicesStrategy(seed int64, alpha float64) *PowerOfTwoChoicesStrategy {
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.2
	}
	return &PowerOfTwoChoicesStrategy{rng: rand.New(rand.NewSource(seed)), services: make(map[string][]string), ewma: make(map[string]map[string]float64), alpha: alpha}
}

func (s *PowerOfTwoChoicesStrategy) Name() string { return "PowerOfTwoChoices" }

func (s *PowerOfTwoChoicesStrategy) AddService(name string, endpoints []string) {
	s.services[name] = append([]string{}, endpoints...)
	if _, ok := s.ewma[name]; !ok {
		s.ewma[name] = make(map[string]float64)
	}
}

func (s *PowerOfTwoChoicesStrategy) PickEndpoint(service string) (string, error) {
	eps := s.services[service]
	if len(eps) == 0 {
		return "", ErrNoEndpoints
	}
	if len(eps) == 1 {
		return eps[0], nil
	}
	// sample two distinct indices
	i := s.rng.Intn(len(eps))
	j := s.rng.Intn(len(eps) - 1)
	if j >= i {
		j++
	}
	a, b := eps[i], eps[j]
	ma := s.ewma[service][a]
	mb := s.ewma[service][b]
	// zero means unseen; prefer the one with a smaller non-zero, otherwise break ties randomly
	switch {
	case ma == 0 && mb == 0:
		if s.rng.Intn(2) == 0 {
			return a, nil
		} else {
			return b, nil
		}
	case ma == 0:
		return a, nil
	case mb == 0:
		return b, nil
	default:
		if ma <= mb {
			return a, nil
		} else {
			return b, nil
		}
	}
}

func (s *PowerOfTwoChoicesStrategy) ReportResult(service, endpoint string, latencySec float64, success bool) {
	if _, ok := s.ewma[service]; !ok {
		s.ewma[service] = make(map[string]float64)
	}
	cur := s.ewma[service][endpoint]
	if cur == 0 {
		s.ewma[service][endpoint] = latencySec
	} else {
		s.ewma[service][endpoint] = s.alpha*latencySec + (1-s.alpha)*cur
	}
}

// LeastLatencyStrategy always chooses the endpoint with smallest observed average latency (EWMA).
// If none observed, falls back to random choice.
type LeastLatencyStrategy struct {
	rng      *rand.Rand
	services map[string][]string
	ewma     map[string]map[string]float64
	alpha    float64
}

func NewLeastLatencyStrategy(seed int64, alpha float64) *LeastLatencyStrategy {
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.2
	}
	return &LeastLatencyStrategy{rng: rand.New(rand.NewSource(seed)), services: make(map[string][]string), ewma: make(map[string]map[string]float64), alpha: alpha}
}

func (s *LeastLatencyStrategy) Name() string { return "LeastLatency" }

func (s *LeastLatencyStrategy) AddService(name string, endpoints []string) {
	s.services[name] = append([]string{}, endpoints...)
	if _, ok := s.ewma[name]; !ok {
		s.ewma[name] = make(map[string]float64)
	}
}

func (s *LeastLatencyStrategy) PickEndpoint(service string) (string, error) {
	eps := s.services[service]
	if len(eps) == 0 {
		return "", ErrNoEndpoints
	}
	// find with min positive ewma; if none positive, return random
	best := ""
	bestVal := math.MaxFloat64
	any := false
	for _, e := range eps {
		if v := s.ewma[service][e]; v > 0 {
			any = true
			if v < bestVal {
				bestVal, best = v, e
			}
		}
	}
	if any {
		return best, nil
	}
	return eps[s.rng.Intn(len(eps))], nil
}

func (s *LeastLatencyStrategy) ReportResult(service, endpoint string, latencySec float64, success bool) {
	if _, ok := s.ewma[service]; !ok {
		s.ewma[service] = make(map[string]float64)
	}
	cur := s.ewma[service][endpoint]
	if cur == 0 {
		s.ewma[service][endpoint] = latencySec
	} else {
		s.ewma[service][endpoint] = s.alpha*latencySec + (1-s.alpha)*cur
	}
}
