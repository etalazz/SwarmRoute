# Changelog

All notable changes to this project will be documented in this file.

This project loosely follows the Keep a Changelog format and Semantic Versioning where possible.

## [Unreleased]

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
