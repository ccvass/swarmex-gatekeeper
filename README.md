<p align="center"><img src="https://raw.githubusercontent.com/ccvass/swarmex/main/docs/assets/logo.svg" alt="Swarmex" width="400"></p>

[![Test, Build & Deploy](https://github.com/ccvass/swarmex-gatekeeper/actions/workflows/publish.yml/badge.svg)](https://github.com/ccvass/swarmex-gatekeeper/actions/workflows/publish.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

# Swarmex Gatekeeper

Readiness probes for Docker Swarm — enables Traefik routing only when services are healthy.

Part of [Swarmex](https://github.com/ccvass/swarmex) — enterprise-grade orchestration for Docker Swarm.

## What It Does

Performs periodic health checks against service endpoints and controls Traefik routing based on readiness. Services only receive traffic after passing the configured health threshold, preventing users from hitting unhealthy instances.

## Labels

```yaml
deploy:
  labels:
    swarmex.gatekeeper.enabled: "true"     # Enable readiness probes
    swarmex.gatekeeper.path: "/health"     # Health check endpoint
    swarmex.gatekeeper.interval: "10"      # Seconds between checks
    swarmex.gatekeeper.timeout: "5"        # Seconds before check times out
    swarmex.gatekeeper.threshold: "3"      # Consecutive passes to mark ready
```

## How It Works

1. Discovers services with gatekeeper labels via Docker API.
2. Polls each service's health endpoint at the configured interval.
3. After consecutive successful checks meet the threshold, marks the service as ready.
4. Enables Traefik routing labels so traffic flows to the service.
5. Disables routing if the service starts failing checks.

## Quick Start

```bash
docker service update \
  --label-add swarmex.gatekeeper.enabled=true \
  --label-add swarmex.gatekeeper.path=/health \
  --label-add swarmex.gatekeeper.threshold=3 \
  my-app
```

## Verified

Service marked as ready after passing health checks — log confirmed: "service READY, enabling Traefik".

## License

Apache-2.0
