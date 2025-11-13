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
)

// Strategy is a common interface implemented by baseline balancers and the SwarmRoute adapter.
// It is intentionally aligned with the SwarmRoute library API to keep the simulator simple.
type Strategy interface {
	Name() string
	AddService(name string, endpoints []string)
	PickEndpoint(service string) (string, error)
	ReportResult(service, endpoint string, latencySec float64, success bool)
}

// ErrNoEndpoints is returned when a strategy cannot select an endpoint for a service.
var ErrNoEndpoints = fmt.Errorf("no endpoints for service")
