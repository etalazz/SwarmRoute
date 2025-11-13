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
