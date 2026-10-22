# Fleck

**Catch configuration inconsistencies before deployment.**

Fleck is a local static-analysis CLI for cloud-native Go applications. It links
configuration reads in source code to Docker image defaults and rendered Helm /
Kubernetes configuration, then reports cross-layer inconsistencies that each
layer's syntax checker would miss. It does not need a cluster and does not run
the target application.

## MVP support

- Go `os.Getenv` / `os.LookupEnv` reads and common `net/http` listeners
- Helm-rendered Deployments, Services, ConfigMaps, and Secrets
- Kubernetes `env`, `envFrom`, key references, selectors, and named ports
- Final-runtime-stage Dockerfile `ENV`, `EXPOSE`, `ENTRYPOINT`, and `CMD`
- A typed Configuration Dependency Graph with deterministic DOT export
- Text and versioned JSON reports

| Rule | Severity | Description |
|---|---|---|
| CFG001 | error | Missing required environment variable |
| CFG002 | warning | Probable environment-variable naming mismatch |
| CFG003 | error | Port configuration mismatch |
| CFG004 | info | Potentially unused supplied environment variable |

Fleck explicitly distinguishes required, optional, and unknown reads. It also
distinguishes absence from an unresolved external reference. CFG002 and CFG004
are deliberately worded as heuristics rather than proofs.

## Install and build

Requirements are Go 1.24 or later and Helm on `PATH` for real analyses.

```sh
go build -o fleck ./cmd/fleck
./fleck version
```

Helm charts are rendered by directly executing `helm template`; Helm plugins or
chart functions are not a security sandbox, so only analyze repositories you
would render locally.

## Quick start

```sh
./fleck rules list
./fleck check ./examples/payment-service
./fleck check ./examples/payment-service --format json
./fleck graph ./examples/payment-service --format dot --output graph.dot
```

The example is a compilable HTTP service with health/readiness endpoints,
required `DATABASE_URL`, optional `ENABLE_METRICS`, an `APP_PORT` fallback, a
Dockerfile, and a Helm chart. It checks that `DATABASE_URL` is present but does
not claim to connect to a database.

Inject the four primary faults with one overlay:

```sh
./fleck check ./examples/payment-service \
  --values deploy/helm/payment-service/values-faulty.yaml
```

## Target contract

One `fleck.yaml` explicitly maps one Go application to one workload, container,
Dockerfile, and Helm rendering:

```yaml
version: 1
application:
  language: go
  path: ./cmd/api
deployment:
  type: helm
  chart: ./deploy/helm/payment-service
  release: payment-service
  values: [./deploy/helm/payment-service/values-valid.yaml]
  workload: {kind: Deployment, name: payment-service}
  container: payment-service
dockerfile:
  path: ./Dockerfile
analysis:
  rules: [CFG001, CFG002, CFG003, CFG004]
```

Paths in this file are relative to the file itself. Configured values overlays
are passed to Helm in listed order, so later overlays win. CLI `--values` flags
replace the configured list and are resolved from the target repository.

## Commands and exit status

```text
fleck check [repository] [--config file] [--values file] [--format text|json]
            [--output file] [--rules CFG001,CFG003]
            [--severity info|warning|error] [--fail-on info|warning|error]
fleck graph [repository] --format dot [--output graph.dot]
fleck rules list
fleck version
```

`--severity` filters displayed findings. `--fail-on` independently controls the
status using all enabled findings. Exit status `0` means no finding met the
threshold, `1` means analysis completed and at least one did, and `2` means the
analysis could not be completed. The default failure threshold is `error`.

JSON uses schema version `1`, stable rule IDs, deterministic finding ordering,
an explicit empty findings array, summaries, diagnostics, and phase timings.
Secret values are never emitted into text, JSON, or DOT.

## Development

```sh
go test ./...
go vet ./...
go test -race ./...
cd examples/payment-service && go test ./...
```

Normal tests inject a renderer and do not require Helm, Kind, Docker, or a live
cluster. See [architecture](docs/architecture.md), [supported patterns](docs/supported-patterns.md),
[rule semantics](docs/rules.md), and [limitations](docs/limitations.md).

## Current limitations and roadmap

The MVP intentionally excludes multi-service discovery, Java and other source
languages, full dataflow/call-graph analysis, arbitrary frameworks, runtime
inspection, automatic fixes, and scientific result generation. Dynamic reads,
external resources, complex Docker expansion, and multi-level returned-error
propagation remain unknown when they cannot be defended statically.

Likely next steps are labeled fault-injection automation, Kind runtime
validation, a simple name-matching baseline for research comparison, and a
separate Java extractor that emits the same normalized facts.
