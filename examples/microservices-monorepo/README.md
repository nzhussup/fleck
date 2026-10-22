# Microservices monorepo

This repository contains multiple deployable services under one Go module.
The root `fleck.yaml` declares one named target per application/workload
boundary. Paths are resolved from the monorepo root.

```sh
fleck check .
fleck check . --target orders
fleck check . --target notifications
```

- `orders` has Go source, a Dockerfile, and raw Kubernetes manifests. All five
  rules run and the configuration is consistent.
- `notifications` is a worker with Go source and a Dockerfile but no deployment
  manifests. Its target enables only CFG002 and CFG004, demonstrating that rule
  selection is independent for each service.

Without `--target`, both services are checked in one workspace run. Both can
import code from the repository's `shared` package without changing their Fleck
target boundary.
