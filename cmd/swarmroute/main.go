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
	"fmt"
	"swarmroute"
)

// Minimal runnable example using the swarmroute library.
// This entrypoint exists so you can easily Run/Debug the project
// from GoLand (or run via `go run ./cmd/swarmroute`).
func main() {
	sr := swarmroute.NewSwarmRoute()
	sr.AddService("api", []string{"http://localhost:8080", "http://localhost:8081"})

	addr, err := sr.PickEndpoint("api")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("Selected endpoint:", addr)
}
