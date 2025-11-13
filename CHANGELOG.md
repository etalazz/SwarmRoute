# Changelog

All notable changes to this project will be documented in this file.

This project loosely follows the Keep a Changelog format and Semantic Versioning where possible.

## [Unreleased]

### Added
- Simulation harness to compare SwarmRoute against baseline balancers:
  - Baselines: Random, RoundRobin, PowerOfTwoChoices, and LeastLatency.
  - Deterministic in-memory world simulator with events (latency/error changes).
  - Runnable entrypoint at `cmd/harness` that prints a concise side-by-side report.
- Per-endpoint jitter support in the simulator (EndpointSpec.JitterSec) and optional jitter updates via events, aligning with the “simulated world” model (L, J, p_fail).
- Phase-aware reporting in harness (per-window success%, mean, p95 split at steps 2000 and 6000).
- Reproducibility: the harness prints the RNG seed and uses a pinned default seed in `cmd/harness/main.go`.
- Adaptation metric: share of selections routed to the degraded endpoint during the bad window [2000, 6000).
- Library API: Latency-aware penalty controls for slow-but-successful calls:
  - SetSlowThresholdSec(sec): treat successes with latency above threshold as bad events.
  - SetBadPosDecay(alpha): decay positive pheromone by a fraction on bad events.
- Harness tests: sanity checks for SwarmRoute behavior
  - TestAlwaysBadEndpoint (100% failing endpoint is rapidly and persistently avoided).
  - TestAlwaysSlowEndpoint (3–4× slower endpoint gets a small share under latency-aware penalty).
- Multi-seed experiments:
  - Aggregation API `AggregateMultiSeed` and pretty printer `FormatAggregatedResults` under `harness/`.
  - New CLI `cmd/experiments` to run the canonical scenario and harder variants across multiple seeds and report mean ± stddev of success, p95, and bad-window share.
- Harder scenarios (all in `cmd/experiments`):
  - Many endpoints (10), degrade two at different times with later recovery.
  - Drift (gradual latency ramp/de-ramp) instead of step changes.
  - Flaky-but-fast endpoint (very low latency with 30–40% errors).
- Real HTTP microservice demo: `cmd/httpdemo` spins up 3 local servers and replays the degrade window; prints success, mean/p95, and bad-window share per strategy.
- Documentation: New README “Usage” section with
  - Basic example (initialization, PickEndpoint, ReportResult),
  - Advanced high-throughput example with tuning knobs (request-scaled evaporation, slow-threshold, exploration), and
  - Pitfalls & tips for optimal usage.

### Changed
- Library: Introduced tuning knobs and APIs in SwarmRoute to support request-scaled adaptation:
  - SetRequestEvapRate(r): optional per-request evaporation (decoupled from wall-clock) to achieve a target half-life in requests.
  - SetBaseWeight(w): adjust additive base weight to slightly increase exploration probability.
  - SetPosNegScale(kpos, kneg): tune positive vs. negative reinforcement magnitudes.
  - SetPeriodicExploration(everyN, negThreshold): optional periodic uniform exploration among non-terrible endpoints.
- Harness: SwarmRoute adapter now enables these tunings for simulations (half-life ~2000 requests, baseWeight ~0.05, k_pos=0.25, k_neg=1.2, slow-threshold ~70ms, bad-event pos decay=0.20, periodic exploration every 500 requests).
  This makes slow-but-successful calls count as bad during the degraded window and drives bad-window share well below 10%.

## [0.1.1] - 2025-11-12

### Added
- Comprehensive unit tests (`swarmroute_test.go`) verifying routing behavior described in the README:
  - Bias toward lower-latency endpoints after successful calls.
  - Negative reinforcement on failures reducing selection probability.
  - Successful calls reducing accumulated error pheromone.
  - Periodic evaporation decaying both positive and negative pheromones.
  - Error handling when a service has no endpoints.

### Changed
- README image made smaller and centered for better presentation.

## [0.1.0] - 2025-11-12

### Added
- Initial implementation of SwarmRoute library and example entrypoint under `cmd/swarmroute`.
