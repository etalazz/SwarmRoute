package harness

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// EndpointSpec defines the initial environment for an endpoint.
type EndpointSpec struct {
	Addr           string
	MeanLatencySec float64
	// JitterSec is the standard deviation of latency noise in seconds.
	// If zero, a default jitter of 30% of MeanLatencySec is used.
	JitterSec float64
	ErrorRate float64 // 0.0..1.0
}

// EnvironmentEvent changes an endpoint's environment at a specific request index (step).
// Any field set to nil is left unchanged.
type EnvironmentEvent struct {
	Step           int
	Endpoint       string
	NewMeanLatency *float64
	// Optional: update jitter (stddev) for the endpoint at this step.
	NewJitterSec *float64
	NewErrorRate *float64
}

// Scenario is the full simulation definition.
type Scenario struct {
	Service       string
	Endpoints     []EndpointSpec
	Events        []EnvironmentEvent
	TotalRequests int
	Seed          int64
}

// Results are aggregated per strategy after a run.
type Results struct {
	Strategy  string
	Total     int
	Success   int
	Failure   int
	MeanLatMS float64
	P95LatMS  float64
	Selection map[string]int
	// Phase-aware metrics: [0]=0..1999, [1]=2000..5999, [2]=6000..
	Phases [3]PhaseMetrics
	// Heuristically detected degraded endpoint at step 2000 (if any)
	DegradedEndpoint string
	// Share of selections to the degraded endpoint during bad window [2000,6000)
	BadWindowDegradedShare float64
}

// PhaseMetrics summarizes a time window inside the run.
type PhaseMetrics struct {
	Total     int
	Success   int
	MeanLatMS float64
	P95LatMS  float64
}

// RunScenario executes the scenario for a single strategy and returns aggregated results.
func RunScenario(sc Scenario, s Strategy) Results {
	// Copy environment into a map for quick updates
	env := make(map[string]*EndpointSpec)
	eps := make([]string, 0, len(sc.Endpoints))
	for _, e := range sc.Endpoints {
		v := e // copy
		env[e.Addr] = &v
		eps = append(eps, e.Addr)
	}
	s.AddService(sc.Service, eps)

	// Index events by step for O(1) lookup
	byStep := make(map[int][]EnvironmentEvent)
	for _, ev := range sc.Events {
		byStep[ev.Step] = append(byStep[ev.Step], ev)
	}

	rng := rand.New(rand.NewSource(sc.Seed))

	selections := make(map[string]int)
	latencies := make([]float64, 0, sc.TotalRequests)
	success := 0

	// Per-phase tracking
	perPhaseLat := [3][]float64{}
	perPhaseSel := [3]map[string]int{make(map[string]int), make(map[string]int), make(map[string]int)}
	perPhaseTotal := [3]int{}
	perPhaseSuccess := [3]int{}

	// Detect degraded endpoint at step 2000 by looking at events applied at that step
	degradedEndpoint := ""

	for step := 0; step < sc.TotalRequests; step++ {
		// Apply events
		if arr := byStep[step]; len(arr) > 0 {
			// For degrade detection, inspect values before applying
			if step == 2000 {
				bestScore := 0.0
				for _, ev := range arr {
					st, ok := env[ev.Endpoint]
					if !ok {
						continue
					}
					oldMean := st.MeanLatencySec
					oldErr := st.ErrorRate
					// Compute score for worsening
					score := 0.0
					if ev.NewMeanLatency != nil {
						if oldMean > 0 {
							score += (*ev.NewMeanLatency/oldMean - 1.0)
						} else {
							if *ev.NewMeanLatency > 0 {
								score += 1.0
							}
						}
					}
					if ev.NewErrorRate != nil {
						score += (*ev.NewErrorRate - oldErr)
					}
					if score > 0 && score > bestScore {
						bestScore = score
						degradedEndpoint = ev.Endpoint
					}
				}
			}
			for _, ev := range arr {
				if st, ok := env[ev.Endpoint]; ok {
					if ev.NewMeanLatency != nil {
						st.MeanLatencySec = *ev.NewMeanLatency
					}
					if ev.NewJitterSec != nil {
						st.JitterSec = *ev.NewJitterSec
					}
					if ev.NewErrorRate != nil {
						st.ErrorRate = clamp01(*ev.NewErrorRate)
					}
				}
			}
		}

		// Choose endpoint
		addr, err := s.PickEndpoint(sc.Service)
		if err != nil {
			// If strategy cannot pick, skip this request
			continue
		}
		selections[addr]++
		st := env[addr]
		if st == nil {
			// unknown endpoint (shouldn't happen), skip
			continue
		}

		// Phase index by step
		phase := 0
		switch {
		case step >= 6000:
			phase = 2
		case step >= 2000:
			phase = 1
		default:
			phase = 0
		}
		perPhaseSel[phase][addr]++
		perPhaseTotal[phase]++

		// Sample outcome from environment
		fail := rng.Float64() < st.ErrorRate
		// Sample latency around mean with per-endpoint jitter (stddev), truncated to 0.2x..5x
		jitter := st.JitterSec
		if jitter <= 0 {
			// Default to 30% coefficient of variation if not provided
			jitter = 0.3 * st.MeanLatencySec
		}
		lat := st.MeanLatencySec + rng.NormFloat64()*jitter
		minLat := 0.2 * st.MeanLatencySec
		maxLat := 5.0 * st.MeanLatencySec
		if st.MeanLatencySec == 0 {
			minLat = 0.001
			maxLat = 0.050
		}
		if lat < minLat {
			lat = minLat
		}
		if lat > maxLat {
			lat = maxLat
		}

		// Penalize failures by adding a fixed overhead so strategies can learn from them
		reportLat := lat
		if fail {
			reportLat += 0.250
		}

		s.ReportResult(sc.Service, addr, reportLat, !fail)

		if !fail {
			success++
			latencies = append(latencies, lat)
			perPhaseSuccess[phase]++
			perPhaseLat[phase] = append(perPhaseLat[phase], lat)
		}
	}

	mean, p95 := summarizeLatency(latencies)
	// Build phase metrics
	phases := [3]PhaseMetrics{}
	for i := 0; i < 3; i++ {
		pm := PhaseMetrics{Total: perPhaseTotal[i], Success: perPhaseSuccess[i]}
		m, p := summarizeLatency(perPhaseLat[i])
		pm.MeanLatMS = m * 1000
		pm.P95LatMS = p * 1000
		phases[i] = pm
	}
	// Compute share to degraded endpoint in bad window
	badShare := 0.0
	if degradedEndpoint != "" {
		totalBad := 0
		for _, c := range perPhaseSel[1] {
			totalBad += c
		}
		if totalBad > 0 {
			badShare = float64(perPhaseSel[1][degradedEndpoint]) / float64(totalBad)
		}
	}
	return Results{
		Strategy:               s.Name(),
		Total:                  sc.TotalRequests,
		Success:                success,
		Failure:                sc.TotalRequests - success,
		MeanLatMS:              mean * 1000,
		P95LatMS:               p95 * 1000,
		Selection:              selections,
		Phases:                 phases,
		DegradedEndpoint:       degradedEndpoint,
		BadWindowDegradedShare: badShare,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func summarizeLatency(samples []float64) (mean, p95 float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range samples {
		sum += v
	}
	mean = sum / float64(len(samples))
	cp := append([]float64(nil), samples...)
	sort.Float64s(cp)
	idx := int(math.Ceil(0.95*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	p95 = cp[idx]
	return
}

// RunAll runs the scenario for all provided strategies and returns their results in order.
func RunAll(sc Scenario, strategies []Strategy) []Results {
	out := make([]Results, 0, len(strategies))
	for _, s := range strategies {
		out = append(out, RunScenario(sc, s))
	}
	return out
}

// FormatResults renders a concise, human-readable summary for stdout.
func FormatResults(results []Results) string {
	s := ""
	for _, r := range results {
		s += fmt.Sprintf("%s: success=%d/%d (%.1f%%), mean=%.1fms p95=%.1fms\n", r.Strategy, r.Success, r.Total, 100.0*float64(r.Success)/float64(r.Total), r.MeanLatMS, r.P95LatMS)
		// print selections in deterministic order
		keys := make([]string, 0, len(r.Selection))
		for k := range r.Selection {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s += fmt.Sprintf("  %s: %d\n", k, r.Selection[k])
		}
		// Per-phase stats
		s += fmt.Sprintf("  phase[0-1999]: success=%d/%d (%.1f%%), mean=%.1fms p95=%.1fms\n",
			r.Phases[0].Success, r.Phases[0].Total,
			pct(r.Phases[0].Success, r.Phases[0].Total), r.Phases[0].MeanLatMS, r.Phases[0].P95LatMS)
		s += fmt.Sprintf("  phase[2000-5999]: success=%d/%d (%.1f%%), mean=%.1fms p95=%.1fms\n",
			r.Phases[1].Success, r.Phases[1].Total,
			pct(r.Phases[1].Success, r.Phases[1].Total), r.Phases[1].MeanLatMS, r.Phases[1].P95LatMS)
		s += fmt.Sprintf("  phase[6000-...]: success=%d/%d (%.1f%%), mean=%.1fms p95=%.1fms\n",
			r.Phases[2].Success, r.Phases[2].Total,
			pct(r.Phases[2].Success, r.Phases[2].Total), r.Phases[2].MeanLatMS, r.Phases[2].P95LatMS)
		if r.DegradedEndpoint != "" && r.Phases[1].Total > 0 {
			s += fmt.Sprintf("  bad-window share to degraded (%s): %.1f%%\n", r.DegradedEndpoint, 100.0*r.BadWindowDegradedShare)
		}
	}
	return s
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100.0 * float64(n) / float64(d)
}
