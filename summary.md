# SwarmRoute tuning: latency-aware penalty and final results

seed=123456789

This run integrates concrete fixes:
- Slow-but-successful calls are treated as bad if latency > slowThreshold (70ms in harness).
- Positive vs. negative reinforcement rebalanced (k_pos=0.25, k_neg=1.2).
- Base weight lowered (0.05) so clearly bad endpoints sink close to zero probability.
- Per-request evaporation retained (half-life ≈ 2000 requests) and periodic exploration every 500 picks.

Raw output (go run ./cmd/harness):

```
seed=123456789
Random: success=9616/10000 (96.2%), mean=44.6ms p95=121.4ms
  http://a:8080: 3344
  http://b:8080: 3311
  http://c:8080: 3345
  phase[0-1999]: success=1978/2000 (98.9%), mean=35.1ms p95=54.6ms
  phase[2000-5999]: success=3687/4000 (92.2%), mean=59.9ms p95=129.4ms
  phase[6000-...]: success=3951/4000 (98.8%), mean=35.1ms p95=54.8ms
  bad-window share to degraded (http://b:8080): 33.8%
RoundRobin: success=9646/10000 (96.5%), mean=44.6ms p95=122.0ms
  http://a:8080: 3334
  http://b:8080: 3333
  http://c:8080: 3333
  phase[0-1999]: success=1980/2000 (99.0%), mean=35.2ms p95=54.7ms
  phase[2000-5999]: success=3713/4000 (92.8%), mean=59.8ms p95=130.2ms
  phase[6000-...]: success=3953/4000 (98.8%), mean=35.0ms p95=54.6ms
  bad-window share to degraded (http://b:8080): 33.3%
PowerOfTwoChoices: success=9873/10000 (98.7%), mean=33.4ms p95=52.9ms
  http://a:8080: 6185
  http://b:8080: 794
  http://c:8080: 3021
  phase[0-1999]: success=1985/2000 (99.2%), mean=32.6ms p95=48.9ms
  phase[2000-5999]: success=3933/4000 (98.3%), mean=33.6ms p95=53.8ms
  phase[6000-...]: success=3955/4000 (98.9%), mean=33.7ms p95=54.1ms
  bad-window share to degraded (http://b:8080): 0.2%
LeastLatency: success=9105/10000 (91.0%), mean=64.4ms p95=130.8ms
  http://b:8080: 10000
  phase[0-1999]: success=1985/2000 (99.2%), mean=35.3ms p95=51.8ms
  phase[2000-5999]: success=3151/4000 (78.8%), mean=119.8ms p95=137.2ms
  phase[6000-...]: success=3969/4000 (99.2%), mean=35.0ms p95=52.3ms
  bad-window share to degraded (http://b:8080): 100.0%
SwarmRoute: success=9890/10000 (98.9%), mean=31.8ms p95=48.8ms
  http://a:8080: 7398
  http://b:8080: 1902
  http://c:8080: 700
  phase[0-1999]: success=1985/2000 (99.2%), mean=35.0ms p95=51.6ms
  phase[2000-5999]: success=3943/4000 (98.6%), mean=30.8ms p95=46.7ms
  phase[6000-...]: success=3962/4000 (99.0%), mean=31.2ms p95=48.2ms
  bad-window share to degraded (http://b:8080): 0.4%
```

Interpretation
- Goal check: Slow-but-successful counts as bad is working.
  - SwarmRoute’s bad-window share to degraded b dropped to 0.4% (≪ 10%), essentially matching PowerOfTwoChoices at 0.2%.
- Overall performance: SwarmRoute leads in this run.
  - Success: 98.9% (best).
  - Mean/p95 latency: 31.8/48.8 ms (best), edging PoTC (33.4/52.9 ms).
  - Recovery window (6000+): 31.2/48.2 ms with 99.0% success.
- Traffic allocation: SwarmRoute concentrates on the fastest healthy endpoint (a: ~74%).
  - With request-based evaporation and low base weight, this heavy exploitation is expected.
  - Periodic exploration (every 500 picks) and slow-threshold allow rapid detection of degrade and recovery while keeping latency low.
- Baselines:
  - Random/RoundRobin: ~33% routed to degraded b during the bad window, significantly worse success% and p95.
  - LeastLatency: locks onto b entirely in this seed; catastrophic during degrade.

Sanity-check experiments (harness tests)
- Always-bad endpoint (100% failures): share stays near zero (assertion <3%); passes.
- Always-slow endpoint (4× slower, 0% errors): share kept low (assertion <15%); passes.

What changed technically
- Library: Added slow-threshold and bad-event positive-decay knobs and applied logic in ReportResult.
- Harness: Rebalanced k_pos/k_neg, lowered baseWeight, set slowThresholdSec=70ms; retained per-request evaporation and periodic exploration.

Big picture
- The harness previously revealed SwarmRoute was too forgiving of “slow-but-successful.”
- By bringing latency into the penalty signal and ensuring the expected update for a degraded endpoint is negative, SwarmRoute now avoids b in the bad window almost entirely while achieving best-in-class latency.

Follow-ups (optional)
- If you prefer less concentration on a single healthy endpoint, consider slightly higher req-evap (faster cooling) or more frequent/temperatured exploration.
  - Example: SetRequestEvapRate ≈ 0.0005 (half-life ~1386 requests) or exploration every 300 picks.
