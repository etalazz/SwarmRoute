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

package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"swarmroute/harness"
	"time"
)

type endpointConfig struct {
	Addr           string
	BaseLat        time.Duration
	Jitter         time.Duration
	BaseErr        float64
	DegradeLat     time.Duration
	DegradeErr     float64
	DegradeStart   time.Duration
	DegradeEnd     time.Duration
	IsDegradedNode bool // true for the one we degrade
}

func startServer(cfg endpointConfig, start time.Time) *http.Server {
	mux := http.NewServeMux()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// choose parameters depending on time window
		now := time.Since(start)
		lat := cfg.BaseLat
		errRate := cfg.BaseErr
		if cfg.IsDegradedNode && now >= cfg.DegradeStart && now < cfg.DegradeEnd {
			lat = cfg.DegradeLat
			errRate = cfg.DegradeErr
		}
		// jittered latency, truncated
		jitter := cfg.Jitter
		mean := float64(lat)
		sd := float64(jitter)
		sample := mean + rng.NormFloat64()*sd
		minLat := 0.2 * mean
		maxLat := 5.0 * mean
		if sample < minLat {
			sample = minLat
		}
		if sample > maxLat {
			sample = maxLat
		}
		time.Sleep(time.Duration(sample))
		if rng.Float64() < errRate {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: cfg.Addr[len("http://"):], Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	return srv
}

type runResult struct {
	Strategy string
	Total    int
	Success  int
	MeanMS   float64
	P95MS    float64
	Select   map[string]int
	BadShare float64 // % selections to degraded during window
}

func main() {
	strategies := []harness.Strategy{
		harness.NewRandomStrategy(1),
		harness.NewRoundRobinStrategy(),
		harness.NewPowerOfTwoChoicesStrategy(2, 0.2),
		harness.NewLeastLatencyStrategy(3, 0.2),
		harness.NewSwarmRouteAdapter(),
	}
	svc := "api"
	client := &http.Client{Timeout: 2 * time.Second}

	for _, s := range strategies {
		// For fair comparison, start fresh servers per strategy so degrade window aligns with this run
		start := time.Now()
		dStart := 4 * time.Second
		dEnd := 12 * time.Second
		a := endpointConfig{Addr: "http://127.0.0.1:8091", BaseLat: 30 * time.Millisecond, Jitter: 9 * time.Millisecond, BaseErr: 0.01}
		b := endpointConfig{Addr: "http://127.0.0.1:8092", BaseLat: 35 * time.Millisecond, Jitter: 10*time.Millisecond + 500*time.Microsecond, BaseErr: 0.01, DegradeLat: 120 * time.Millisecond, DegradeErr: 0.20, DegradeStart: dStart, DegradeEnd: dEnd, IsDegradedNode: true}
		c := endpointConfig{Addr: "http://127.0.0.1:8093", BaseLat: 40 * time.Millisecond, Jitter: 12 * time.Millisecond, BaseErr: 0.02}
		servers := []*http.Server{startServer(a, start), startServer(b, start), startServer(c, start)}
		// Allow to start
		time.Sleep(200 * time.Millisecond)

		eps := []string{a.Addr, b.Addr, c.Addr}
		fmt.Println("HTTP demo (", s.Name(), "): degrade=", dStart, "..", dEnd, "on", b.Addr)
		s.AddService(svc, eps)
		r := runHTTP(client, s, svc, eps, start, dStart, dEnd, b.Addr, 1000)
		fmt.Printf("%s: success=%d/%d (%.1f%%), mean=%.1fms p95=%.1fms, bad-window share=%.2f%%\n",
			r.Strategy, r.Success, r.Total, 100.0*float64(r.Success)/float64(r.Total), r.MeanMS, r.P95MS, r.BadShare)
		// Print selections
		keys := make([]string, 0, len(r.Select))
		for k := range r.Select {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s: %d\n", k, r.Select[k])
		}
		// Shutdown servers
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		for _, srv := range servers {
			_ = srv.Shutdown(ctx)
		}
		cancel()
		// Give sockets a moment to release
		time.Sleep(200 * time.Millisecond)
	}
}

func runHTTP(client *http.Client, strat harness.Strategy, svc string, eps []string, start time.Time, dStart, dEnd time.Duration, degraded string, total int) runResult {
	sel := make(map[string]int)
	success := 0
	lats := make([]float64, 0, total)
	badSel := 0
	badTotal := 0
	for i := 0; i < total; i++ {
		addr, err := strat.PickEndpoint(svc)
		if err != nil {
			continue
		}
		sel[addr]++
		t0 := time.Now()
		resp, err := client.Get(addr)
		lat := time.Since(t0)
		latSec := float64(lat) / float64(time.Second)
		ok := (err == nil && resp != nil && resp.StatusCode == http.StatusOK)
		if resp != nil {
			_ = resp.Body.Close()
		}
		strat.ReportResult(svc, addr, latSec, ok)

		now := time.Since(start)
		if now >= dStart && now < dEnd {
			badTotal++
			if addr == degraded {
				badSel++
			}
		}

		if ok {
			success++
			lats = append(lats, float64(lat)/float64(time.Millisecond))
		}
	}
	mean, p95 := meanP95(lats)
	share := 0.0
	if badTotal > 0 {
		share = 100.0 * float64(badSel) / float64(badTotal)
	}
	return runResult{Strategy: strat.Name(), Total: total, Success: success, MeanMS: mean, P95MS: p95, Select: sel, BadShare: share}
}

func meanP95(xs []float64) (mean, p95 float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	mean = s / float64(len(xs))
	cp := append([]float64(nil), xs...)
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
