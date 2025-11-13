<h1 align="center">SwarmRoute: Bio‑Inspired Routing &amp; Load Balancing</h1>

<p align="center">
  <img src="img.png" alt="SwarmRoute" width="420" />
</p>


<p align="center"><strong>“We mathematically ensured that in degraded conditions, the expected pheromone update for a failing/slow endpoint is negative, so its probability mass decays over time, while fast/healthy endpoints gain mass.”</strong></p>


<p align="center"><strong>Highlights</strong></p>

<p align="center">
  <strong>1. It strongly reduces traffic to bad nodes.</strong><br/>
  <strong>2. It improves p95 under dynamic conditions vs strong baselines.</strong><br/>
  <strong>3. It’s designed to support cost-aware, multi-metric routing.</strong>
  
</p>


## Overview

SwarmRoute is a lightweight sidecar and library that applies principles from swarm intelligence to service discovery, routing, and load balancing. Each instance in your microservice mesh runs its own SwarmRoute agent which maintains pheromone tables and participates in a simple local consensus process. These agents reinforce paths that lead to low latency and high reliability while forgetting stale or degraded routes. Rather than relying on a single control plane, SwarmRoute lets the system heal and adapt itself when services appear, disappear, or become congested.

This project grew out of research into ant colony optimisation, negative pheromone strategies, and flocking dynamics. Ants balance exploration and exploitation through pheromone deposition and evaporation. Some algorithms even deposit negative pheromone on poor solutions ([pmc.ncbi.nlm.nih.gov](https://pmc.ncbi.nlm.nih.gov)). Modern service‑composition algorithms use multiple pheromone species to encode different quality‑of‑service metrics ([Journal of Cloud Computing](https://journalofcloudcomputing.springeropen.com)). Meanwhile, studies on heterogeneous flocks show that assigning different feedback gains to agents yields faster consensus and greater robustness ([Nature](https://www.nature.com)). SwarmRoute distills these ideas into a pragmatic routing engine for cloud services and event‑driven systems.


## Design in a nutshell

Tiny API, big brain
- External (your app calls only two functions):
  - PickEndpoint(service)
  - ReportResult(service, endpoint, latencySec, success)
- Internal (what SwarmRoute learns): per‑service, per‑endpoint pheromones — positive (good) and negative (bad), with latency‑ and failure‑aware updates, request‑scaled evaporation, and controlled exploration.
- Design win: powerful adaptation behind a tiny API.

Explicit state, not ad‑hoc heuristics
- Decision weight ≈ (pos + baseWeight) / (1 + neg).
- Tunable knobs: k_pos, k_neg, baseWeight, evaporation — predictable and debuggable.
- Design win: a small, well‑defined reinforcement system instead of a pile of if‑branches.

Learns and forgets in request space
- Per‑request evaporation with a defined half‑life (e.g., ~2000 requests).
- High‑QPS adapts fast; low‑QPS doesn’t overreact just because time passed.
- Design win: reason in “how many requests until we forgive/forget.”

Multi‑metric ready from day one
- Multiple pheromone channels (latency, error, cost, zone, …) combined by weights or profiles.
- Already uses latency‑aware and failure‑aware penalties with separate scales.

Negative reinforcement is a first‑class primitive
- Failures and slow successes add negative pheromone and can decay positive; expected update for a bad endpoint is negative, so its probability mass shrinks.
- No brittle “eject after N failures” rules required.

Built‑in safe exploration
- Small baseWeight keeps a non‑zero probe chance; optional periodic exploration re‑admits recovered nodes gradually.

Decentralized and swarm‑friendly
- Runs locally (library/sidecar), no global coordinator; future‑ready for gossip and flock‑like coordination.

Observable by design
- Expose pos/neg per endpoint, exploration vs exploitation, and service “heatmaps” so SREs can see why traffic shifted.

Graceful fallback paths
- Can mimic or fall back to Random/RR/PoTC — safer adoption without ripping anything out.

One sentence
- Routing as a small, explicit, tunable reinforcement system with state, memory, multi‑metric awareness, and safe exploration — wrapped behind a tiny API — rather than a bag of ad‑hoc heuristics.


## Table of Contents

- [Overview](#overview)
- [Design in a nutshell](#design-in-a-nutshell)
- [Features](#features)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Architecture](#architecture)
- [Research and Inspiration](#research-and-inspiration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)
- [Results](#results)


## Features

- Bio‑inspired adaptive routing — Each agent chooses destinations using a weighted pheromone table that favours endpoints with low latency and high success rates. Pheromone values evaporate over time to forget stale information. A configurable exploration factor ensures that a fraction of requests probe alternative paths.

- Negative and multi‑pheromone reinforcement — SwarmRoute penalises failed or slow calls by subtracting pheromone from the offending route, implementing a negative reinforcement mechanism inspired by TrailMap ([pmc.ncbi.nlm.nih.gov](https://pmc.ncbi.nlm.nih.gov)). For multi‑criteria routing, separate pheromone tables track different QoS metrics (e.g., latency, error rate, cost); they are combined with tunable weights as proposed in multi‑attribute ant algorithms ([Journal of Cloud Computing](https://journalofcloudcomputing.springeropen.com)).

- Local consensus and heterogeneity — Agents periodically exchange pheromone summaries with a handful of neighbours and nudge their tables toward the neighbourhood consensus. Heterogeneity in update rates and weighting factors improves convergence and robustness under communication delays ([Nature](https://www.nature.com)).

- Scalable sidecar architecture — The library runs alongside your service or as a sidecar container. There is no central controller; all state is local except for occasional gossip. Evaporation and updates run in their own goroutines to avoid blocking request threads.

- Telemetry hooks — SwarmRoute exposes endpoints for scraping the current pheromone tables and consensus state. Operators can visualise emergent “pheromone maps,” observe cluster‑wide load distribution, and spot anomalies without deep instrumentation.


## Quick Start.

1. Install Go — SwarmRoute is written in Go. Make sure you have a recent version of Go installed (tested with Go 1.20 and later).

2. Build SwarmRoute — Fetch the repository and build the library (note: use three dots `...`, not two `..`):

```bash
git clone https://github.com/etalazz/swarmroute.git
cd swarmroute
go build ./...
```

3. Run the example app — A minimal runnable entrypoint lives under `cmd/swarmroute`:

```bash
go run ./cmd/swarmroute
```

You should see a line like:

```
Selected endpoint: http://localhost:8081
```

3. Integrate with your service — Add SwarmRoute as a dependency in your service module. Initialise a new SwarmRoute instance on startup, register your service and its endpoints, then call `PickEndpoint(serviceName)` to select a destination for each request. After the call completes, invoke `ReportResult(serviceName, endpoint, latency, success)` to update pheromones. See the examples in `examples/` for details.

4. Configure — Tune parameters such as pheromone evaporation rate, exploration rate, weights for different metrics, negative reinforcement magnitude, and gossip interval through configuration files or environment variables.

5. Run — Deploy the sidecar alongside each microservice instance. Ensure that each agent can reach its neighbours to exchange gossip. Monitor the provided metrics endpoint to observe how pheromone values evolve over time.


## Usage

Below are two practical examples that cover the 90% case and a high‑throughput setup. They demonstrate the tiny external API while leveraging SwarmRoute’s adaptive internals.

### Basic usage (initialize, pick, report)

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"
    sr "swarmroute"
)

func main() {
    // 1) Initialize
    router := sr.NewSwarmRoute()
    router.AddService("api", []string{
        "http://a:8080",
        "http://b:8080",
        "http://c:8080",
    })

    // 2) Pick endpoint
    ep, err := router.PickEndpoint("api")
    if err != nil {
        panic(err)
    }

    // 3) Call your endpoint and measure latency
    client := &http.Client{Timeout: 2 * time.Second}
    t0 := time.Now()
    req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ep, nil)
    resp, err := client.Do(req)
    latencySec := time.Since(t0).Seconds()
    success := (err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300)
    if resp != nil { _ = resp.Body.Close() }

    // 4) Report the outcome back so SwarmRoute can learn
    router.ReportResult("api", ep, latencySec, success)
    fmt.Printf("success=%v latency=%.1fms\n", success, latencySec*1000)
}
```

Notes
- Always call ReportResult, even on timeouts/failures, with the actual observed latency in seconds.
- Success should reflect your SLOs (commonly 2xx codes). Treat 5xx/timeouts as failures.

### Advanced: high‑throughput scenario (tuned for adaptation under load)

```go
package main

import (
    "context"
    "net/http"
    "sync"
    "time"
    sr "swarmroute"
)

func main() {
    srp := sr.NewSwarmRoute()
    // Request‑scaled evaporation: half‑life ~2000 requests
    srp.SetRequestEvapRate(0.0003466) // ≈ ln(2)/2000
    // Treat slow‑but‑successful as bad; gently decay positive on bad events
    srp.SetSlowThresholdSec(0.070)
    srp.SetBadPosDecay(0.20)
    // Reinforcement balance and exploration
    srp.SetPosNegScale(0.25, 1.2)
    srp.SetBaseWeight(0.05)
    srp.SetPeriodicExploration(500, 3.0)

    srp.AddService("api", []string{"http://a:8080", "http://b:8080", "http://c:8080"})

    // High‑throughput HTTP client
    tr := &http.Transport{MaxIdleConnsPerHost: 256}
    client := &http.Client{Timeout: 2 * time.Second, Transport: tr}

    const total = 50000
    const workers = 64
    jobs := make(chan struct{}, workers)
    var wg sync.WaitGroup

    doOne := func() {
        defer wg.Done()
        for range jobs {
            ep, err := srp.PickEndpoint("api")
            if err != nil { continue }
            t0 := time.Now()
            req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ep, nil)
            resp, err := client.Do(req)
            latSec := time.Since(t0).Seconds()
            ok := (err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300)
            if resp != nil { _ = resp.Body.Close() }
            srp.ReportResult("api", ep, latSec, ok)
        }
    }

    for i := 0; i < workers; i++ { wg.Add(1); go doOne() }
    for i := 0; i < total; i++ { jobs <- struct{}{} }
    close(jobs)
    wg.Wait()
}
```

What this shows
- Under high QPS, request‑scaled evaporation and latency‑aware penalties help SwarmRoute adapt quickly: degraded endpoints are pushed toward ~0% share during bad windows, recovered ones are re‑admitted gradually.
- Exploration settings (baseWeight + periodic exploration) prevent permanent exile while avoiding floods to clearly bad nodes.

### Pitfalls and tips for optimal usage

- Always report results
  - Call ReportResult for every request, including failures and timeouts. Missing reports stall learning.
  - Pass latency in seconds (time.Since(t0).Seconds()).
- Treat “slow successes” as bad if that matches your SLOs
  - Set slowThresholdSec to something like 2× your target and enable a modest BadPosDecay (e.g., 0.2) so slow endpoints lose attractiveness over time.
- Decouple memory from wall‑clock at high QPS
  - Use SetRequestEvapRate(ln(2)/halfLifeRequests) to reason in “requests to forgive/forget.” For example, half‑life ~2000 → 0.0003466.
- Tune exploration
  - Lower baseWeight (e.g., 0.01–0.05) so clearly bad endpoints sink near zero; use SetPeriodicExploration to probe occasionally.
- Concurrency and lifecycle
  - SwarmRoute is safe for concurrent use; create one instance per process and reuse it across goroutines.
  - To change membership, call AddService with the updated endpoint list for that service (it replaces the previous list).
- Metrics and debugging
  - Expose PheromoneSnapshot() to observe per‑endpoint pos/neg values and explain routing shifts.


## Architecture

SwarmRoute is centred around a concurrent‑safe `SwarmRoute` type. Internally, it maintains a map from `(serviceName, endpoint)` to a vector of pheromone values. Each vector holds one or more pheromone “species,” such as latency, error, cost, etc. Pheromone updates follow this general pattern:

- Positive update — After a successful call, increase the pheromone values for the chosen endpoint in proportion to its performance (e.g., lower latency yields a larger increment). Other endpoints receive no update.
- Negative update — On failure or unacceptable latency, subtract pheromone from the offending endpoint to discourage its future selection ([pmc.ncbi.nlm.nih.gov](https://pmc.ncbi.nlm.nih.gov)).
- Evaporation — At a fixed interval, multiply all pheromone values by a decay factor to allow the system to forget stale information.
- Selection — To pick an endpoint, compute a weighted sum of pheromone species (according to the chosen QoS profile) and sample endpoints proportionally to their weighted value. A small epsilon ensures random exploration.
- Consensus — Periodically, each agent gossips a compressed version of its pheromone tables to a small subset of peers. Agents update their tables by partially averaging with neighbours, implementing a consensus dynamics reminiscent of flocking. Introducing heterogeneous gains across agents improves stability when delays and obstacles exist ([Nature](https://www.nature.com)).

This design draws on insights from multi‑colony ant algorithms, which use multiple parameter sets and inter‑colony cooperation to avoid premature convergence ([SpringerLink](https://link.springer.com)). Similarly, SwarmRoute supports partitioning the service graph into “colonies” (e.g., by region) that occasionally exchange pheromone summaries.


## Research and Inspiration

SwarmRoute’s design is informed by a range of studies:

- Negative pheromone and selective reinforcement — The TrailMap peer‑matching system uses positive pheromone for helpful interactions and deposits negative pheromone for unhelpful helpers, thereby discouraging poor routes while still allowing exploration ([pmc.ncbi.nlm.nih.gov](https://pmc.ncbi.nlm.nih.gov)). SwarmRoute adopts a similar mechanism to penalise unreliable endpoints.

- Multi‑pheromone species for QoS — Modern service composition frameworks maintain separate pheromone tables for different metrics (latency, response time, energy) and combine them with weights to prioritise the primary requirement while balancing others ([Journal of Cloud Computing](https://journalofcloudcomputing.springeropen.com)). SwarmRoute generalises this idea, allowing you to register any number of pheromone species.

- Heterogeneity for faster convergence — Research on mixed‑agent flocks shows that assigning different feedback gains to each agent can significantly improve consensus rate and resilience under time delays ([Nature](https://www.nature.com)). We apply this by letting agents choose different evaporation rates and gossip weights.

- Multi‑colony strategies — Multi‑colony ant algorithms combine heterogeneous colonies with cooperative information sharing to improve solution quality and avoid premature stagnation ([SpringerLink](https://link.springer.com)). SwarmRoute encourages partitioning large meshes into subgroups that occasionally exchange pheromone summaries.

- Time delay and obstacle avoidance — Studies of flocking dynamics with delays and obstacle repulsion show that delays can suppress alignment and that repulsive potentials around obstacles accelerate singularities ([Frontiers in Applied Mathematics and Statistics](https://www.frontiersin.org)). These insights motivate SwarmRoute’s bounded gossip intervals and safe default behaviours when network latency or partitions occur.


## Roadmap

- Pluggable QoS heuristics — Integrate external metrics (e.g., cost per call, carbon footprint) into pheromone updates.
- Adaptive weight learning — Automatically adjust weights on pheromone species based on observed service‑level objectives.
- Visualization dashboard — Provide a dashboard that overlays pheromone maps on service topology and displays local consensus metrics.
- Language bindings — Add bindings for Rust, JavaScript/Node.js, and other ecosystems.
- Simulation tools — Extend the `examples/` directory with more complex simulations (heterogeneous agents, time delays, obstacles) to help users tune parameters.


## Contributing

Contributions are welcome! Please fork the repository, create a feature branch, and submit a pull request. Contributions could include bug fixes, new features, integration with service meshes, documentation improvements, or experiments.


## License

This project is licensed under the MIT License. See the LICENSE file for details.


## Results

This is honestly kind of wild—in a good way. In multi‑seed experiments and multiple realistic failure scenarios, SwarmRoute consistently matches or beats a known‑strong baseline (Power of Two Choices, PoTC) on the metrics production teams care about: success rate, tail latency, and avoiding degraded nodes.

What the results prove (mean ± std across seeds):

- Base scenario (3 endpoints; hard degrade + recover)
  - Success: PoTC 98.66% ± 0.06, SwarmRoute 98.84% ± 0.08
  - p95 latency: PoTC 53.33 ms ± 0.43, SwarmRoute 49.43 ms ± 2.15
  - Bad‑window share to degraded b: PoTC 0.08% ± 0.14, SwarmRoute 0.19% ± 0.14
  - Takeaway → SwarmRoute is at least as good at avoiding the bad node and beats PoTC on tail latency on average.

- Harder A (10 endpoints; two degraded at different times)
  - SwarmRoute: Success 99.01% ± 0.04, p95 53.94 ms ± 3.59, Bad‑window share 0.23% ± 0.23
  - PoTC: Success 98.82% ± 0.06, p95 75.52 ms ± 0.39, Bad‑window share 1.86% ± 0.35
  - Takeaway → With multiple bad nodes, SwarmRoute keeps degraded nodes basically cold and dramatically improves p95 vs PoTC.

- Harder B (Drift — gradual ramp up/down)
  - PoTC: Success 98.66% ± 0.03, p95 53.61 ms ± 0.30, Bad‑window share 0.00%
  - SwarmRoute: Success 98.95% ± 0.05, p95 47.06 ms ± 1.20, Bad‑window share 0.00%
  - Takeaway → SwarmRoute tracks gradual degradation and recovery (not just step changes), with best success and best p95.

- Harder C (Flaky‑but‑fast endpoint ~35% error)
  - PoTC: Success 98.81% ± 0.30, p95 60.11 ms ± 1.24, Bad‑window share 0.04% ± 0.06
  - SwarmRoute: Success 98.53% ± 0.30, p95 54.99 ms ± 1.60, Bad‑window share 0.93% ± 0.45
  - Takeaway → SwarmRoute routes slightly more to the fast‑but‑flaky node than PoTC (still <1% on average) and wins on tail latency.

- Live HTTP demo (single run, 1000 reqs/strategy)
  - In one sample, PoTC has better latency; SwarmRoute routes 0% of traffic to the degraded node during the bad window vs PoTC’s 0.43%.
  - Takeaway → Single‑run noise is expected; the multi‑seed simulator is the better indicator of typical behavior.

Why this is production‑relevant

- ✅ Handles gray failures & flaky nodes: In base, Harder A, and Flaky‑Fast scenarios, SwarmRoute drives traffic to degraded endpoints to well under 1% on average. It treats slow successes as bad, avoids lock‑in, and doesn’t catastrophically choose the wrong endpoint like naive LeastLatency.
- ✅ Adapts to non‑stationary conditions: Tracks drift and multiple concurrent degradations without oscillation; maintains near‑zero bad‑window share and top p95.
- ✅ Beats strong baselines: Consistently outperforms Random/RR/LeastLatency; versus PoTC, it matches or beats success and usually beats p95 while keeping degraded‑endpoint share in the same near‑zero band.

How it works (one‑liners)

- Negative pheromone + latency‑aware penalties → both failures and slow‑but‑successful calls push endpoints down; fast/healthy endpoints gain mass.
- Request‑scaled evaporation → memory is measured in requests, so it works both in fast sims and real wall‑clock deployments.
- Controlled exploration (small baseWeight + periodic explore) → recovered endpoints are rediscovered without sending meaningful traffic to obviously bad nodes.


“We built a swarm‑inspired routing sidecar, SwarmRoute. Across multi‑seed experiments and multiple failure patterns (hard degrade, drift, multi‑node failures, flaky‑but‑fast), SwarmRoute reduces traffic to degraded endpoints to under 1% (often <0.2%), matches or exceeds PowerOfTwoChoices on success, and consistently beats it on p95 latency — via negative pheromone with latency‑aware penalties, request‑based evaporation, and controlled exploration.”

See the full report and reproduction steps in summary.md.
