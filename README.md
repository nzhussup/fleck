# Fleck

**Catch configuration inconsistencies before deployment.**

Fleck is a local static consistency analyzer. It links facts from the supported
inputs that a project provides—Go source, Dockerfiles, and Helm or raw
Kubernetes manifests—and reports inconsistencies that each layer's syntax
checker would miss. It does not need a cluster and does not run the target
application.

## MVP support

- Go `os.Getenv` / `os.LookupEnv` reads and common `net/http` listeners
- Helm-rendered or raw Kubernetes Deployments, Services, ConfigMaps, and Secrets
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
| CFG005 | warning | Docker/Kubernetes port metadata has no common declaration |
| CFG006 | error | Proven invalid Kubernetes configuration reference |

Fleck explicitly distinguishes required, optional, and unknown reads. It also
distinguishes absence from an unresolved external reference. CFG002 and CFG004
are deliberately worded as heuristics rather than proofs. Findings expose
confidence separately from severity, and related findings share a correlation
key so reports can count unique issues without hiding individual rule results.

## Install and build

Requirements are Go 1.24 or later. Helm is needed on `PATH` only when analyzing
a deployment with `type: helm`.

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

## Example project layouts

The example suite includes several repository shapes and partial-analysis
cases:

| Path | Repository shape | Inputs |
|---|---|---|
| `examples/payment-service` | Standalone service with Helm | Go + Docker + Helm |
| `examples/monolith` | Multi-package monolith | Go + raw Kubernetes |
| `examples/microservices-monorepo` | Two-target monorepo workspace | Mixed per target |
| `examples/polyrepo` | Application and GitOps repositories joined by a workspace | Go + Docker + raw Kubernetes |

See the [examples catalog](examples/README.md) for commands and expected
findings. A workspace uses named targets to explicitly associate each
application with its image and deployment inputs.

## Workspace and target contract

A root `fleck.yaml` can declare several named targets. Each target explicitly
associates application source, image metadata, and deployment configuration;
paths may point into different Git clones or submodules beneath the workspace:

```yaml
version: 1
repositories:
  customer-app: ./repos/customer-api
  platform: ./repos/platform
targets:
  - name: customer-api-development
    application:
      repository: customer-app
      language: go
      path: ./cmd/api
    dockerfile:
      repository: customer-app
      path: ./Dockerfile
    deployment:
      repository: platform
      type: kubernetes
      manifests: [./clusters/development/customer-api]
      workload: {kind: Deployment, name: customer-api, namespace: production}
      container: customer-api
    analysis:
      rules: [CFG001, CFG002, CFG003, CFG004, CFG005, CFG006]

  - name: billing-worker
    application:
      language: go
      path: ./repos/billing/cmd/worker
    dockerfile:
      path: ./repos/billing/Dockerfile
    analysis:
      rules: [CFG001, CFG002, CFG004]
```

`fleck check .` analyzes every target. `fleck check . --target billing-worker`
selects one. A target's `application`, `deployment`, and `dockerfile` sections
are independently optional, but at least one must be present.

Rules are configured per target, allowing a service or environment to enable
only the checks appropriate for its available inputs. Omitting a target's rule
list runs the complete registered rule set; CLI `--rules` temporarily overrides
the configured list for every selected target.

Repository aliases are filesystem roots. They work the same whether the
directory is a Git submodule, a regular clone, or a CI checkout. A layer without
`repository` resolves paths directly from the workspace root.

The original top-level single-target form remains supported:

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
  workload: {kind: Deployment, name: payment-service, namespace: default}
  container: payment-service
dockerfile:
  path: ./Dockerfile
analysis:
  rules: [CFG001, CFG002, CFG003, CFG004, CFG005, CFG006]
```

Raw Kubernetes manifests can replace Helm:

```yaml
version: 1
deployment:
  type: kubernetes
  manifests: [./k8s]
  workload: {kind: Deployment, name: payment-service}
  container: payment-service
dockerfile:
  path: ./Dockerfile
analysis:
  rules: [CFG005, CFG006]
```

Fleck runs only rules whose required capabilities are available. A successfully
analyzed layer that contains no matching facts is distinct from a layer that was
not provided. Text and JSON reports include layer availability and skipped
checks. Cross-layer coverage therefore grows as more inputs are configured.

Paths in this file are relative to the file itself. Configured values overlays
are passed to Helm in listed order, so later overlays win. CLI `--values` flags
replace the configured list and are resolved from the target repository. In a
multi-target workspace, use `--target` together with `--values` so the override
cannot be applied to the wrong Helm release.

## Commands and exit status

```text
fleck check [repository] [--config file] [--values file] [--format text|json]
            [--target name] [--output file] [--rules CFG001,CFG003]
            [--severity info|warning|error] [--fail-on info|warning|error]
fleck graph [repository] [--target name] --format dot [--output graph.dot]
fleck rules list
fleck version
```

`--severity` filters displayed findings. `--fail-on` independently controls the
status using all enabled findings. Exit status `0` means no finding met the
threshold, `1` means analysis completed and at least one did, and `2` means the
analysis could not be completed. The default failure threshold is `error`.

JSON uses schema version `1`, stable rule IDs, deterministic finding ordering,
explicit findings and skipped-check arrays, layer availability, summaries,
diagnostics, confidence, correlation keys, unique-issue counts, and phase
timings. Workspace reports contain a named `targets` array plus an aggregate
summary. Proven invalid references are CFG006 findings; incomplete analysis and
external uncertainty remain diagnostics.
Secret values are never emitted into text, JSON, or DOT.

## Development

```sh
go test ./...
go vet ./...
go test -race ./...
cd examples/payment-service && go test ./...
cd ../monolith && go test ./...
cd ../microservices-monorepo && go test ./...
cd ../polyrepo/application-repo && go test ./...
```

Normal tests inject a renderer and do not require Helm, Kind, Docker, or a live
cluster. See [architecture](docs/architecture.md), [supported patterns](docs/supported-patterns.md),
[rule semantics](docs/rules.md), and [limitations](docs/limitations.md).

## Current limitations and roadmap

The MVP intentionally excludes multi-service discovery, additional source
languages, full dataflow/call-graph analysis, arbitrary frameworks, runtime
inspection, automatic fixes, and scientific result generation. Dynamic reads,
external resources, complex Docker expansion, and multi-level returned-error
propagation remain unknown when they cannot be defended statically.

The separate follow-up phase is labeled fault-injection automation, Kind
runtime validation, a simple name-matching baseline, quantitative metrics, and
layer-ablation experiments.
