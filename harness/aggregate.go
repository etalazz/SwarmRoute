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
	"fmt"
	"math"
)

// MultiSeedAggregation holds per-strategy aggregated metrics across seeds.
type MultiSeedAggregation struct {
	Strategy       string
	SuccessPct     []float64
	P95ms          []float64
	BadShare       []float64
	MeanSuccessPct float64
	StdSuccessPct  float64
	MeanP95ms      float64
	StdP95ms       float64
	MeanBadShare   float64
	StdBadShare    float64
}

// AggregateMultiSeed runs the given scenario across multiple seeds for all strategies
// and aggregates the required metrics (overall success%, overall p95 latency, and
// bad-window share to the degraded endpoint).
func AggregateMultiSeed(sc Scenario, strategies []Strategy, seeds []int64) []MultiSeedAggregation {
	// results by strategy name
	agg := make(map[string]*MultiSeedAggregation)
	for _, seed := range seeds {
		sc.Seed = seed
		rs := RunAll(sc, strategies)
		for _, r := range rs {
			a, ok := agg[r.Strategy]
			if !ok {
				a = &MultiSeedAggregation{Strategy: r.Strategy}
				agg[r.Strategy] = a
			}
			succPct := 0.0
			if r.Total > 0 {
				succPct = 100.0 * float64(r.Success) / float64(r.Total)
			}
			a.SuccessPct = append(a.SuccessPct, succPct)
			a.P95ms = append(a.P95ms, r.P95LatMS)
			a.BadShare = append(a.BadShare, 100.0*r.BadWindowDegradedShare) // percent
		}
	}

	out := make([]MultiSeedAggregation, 0, len(agg))
	for _, a := range agg {
		a.MeanSuccessPct, a.StdSuccessPct = meanStd(a.SuccessPct)
		a.MeanP95ms, a.StdP95ms = meanStd(a.P95ms)
		a.MeanBadShare, a.StdBadShare = meanStd(a.BadShare)
		out = append(out, *a)
	}
	return out
}

func meanStd(xs []float64) (mean, std float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	mean = s / float64(len(xs))
	if len(xs) == 1 {
		return mean, 0
	}
	varSum := 0.0
	for _, v := range xs {
		d := v - mean
		varSum += d * d
	}
	std = math.Sqrt(varSum / float64(len(xs)))
	return
}

// FormatAggregatedResults renders mean±stddev for the chosen metrics per strategy.
func FormatAggregatedResults(aggs []MultiSeedAggregation) string {
	s := ""
	for _, a := range aggs {
		s += fmt.Sprintf("%s: success=%.2f%% ± %.2f, p95=%.2fms ± %.2f, bad-window share=%.2f%% ± %.2f\n",
			a.Strategy, a.MeanSuccessPct, a.StdSuccessPct, a.MeanP95ms, a.StdP95ms, a.MeanBadShare, a.StdBadShare)
	}
	return s
}
