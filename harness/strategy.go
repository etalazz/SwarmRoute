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
