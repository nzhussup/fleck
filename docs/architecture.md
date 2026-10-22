# Architecture

Fleck is a modular monolith. The Cobra CLI validates presentation options and
hands an analysis request to `internal/app.Service`. The service loads
`fleck.yaml`, runs each configured extractor once, records the capabilities that
were actually analyzed, builds normalized facts and the typed dependency graph,
invokes applicable rules from a compile-time detector registry, and returns
findings plus skipped checks to format-neutral reporters.

## Pipeline and boundaries

1. `internal/config` loads and validates the workspace contract. A workspace
   contains one or more named targets; legacy single-target files are normalized
   to the same model. Go, deployment, and Docker inputs are optional within each
   target. Optional repository aliases provide filesystem roots for separate Git
   clones or submodules; unqualified paths remain relative to the root
   configuration. Rule selection belongs to each named target; the legacy
   top-level form retains its top-level rule list.
2. `internal/extractor/golang` uses `go/packages`, Go ASTs, constants, and type
   information. It never creates findings.
3. `internal/extractor/helm` executes `helm template` for Helm deployments with
   an explicit argument list and context cancellation. Raw Kubernetes inputs
   are loaded directly and deterministically.
4. `internal/extractor/kubernetes` parses Deployments, Services, ConfigMaps and
   Secrets into typed facts and resolves only same-namespace references.
5. `internal/extractor/dockerfile` uses BuildKit's Dockerfile parser and reads
   the final runtime stage plus statically named parent stages.
6. `internal/graph` creates stable occurrence nodes, typed directed edges, and
   focused query helpers. Graph state is not mutated after construction.
7. `internal/detector` defines rule contracts and runs enabled rules only when
   their declared capabilities are available. Independent implementations live
   under `internal/detector/rules/<rule-id>`, while `internal/detector/registry`
   is the composition root that registers them.
8. `internal/report` renders findings as text or schema-versioned JSON; graph
   export is deterministic DOT.

Unknown values and unresolved references are explicit states. A missing
external resource is not converted into proof that an environment variable is
absent. Secret key presence is retained, but Secret values are neither decoded
nor copied into facts, findings, JSON, or DOT.

Capability availability records successful analysis, not fact count. For
example, a Go package with zero environment reads still provides the
`env_reads` capability; an omitted application section does not. This lets the
engine distinguish a rule that found nothing from a rule it could not execute.

## Extending Fleck

A language extractor should emit `model.EnvRead` and `model.ListenPort` facts;
it must not expose its syntax tree outside the extractor. Register the language
in configuration and orchestration without changing graph or detectors.

A detector lives in its own `internal/detector/rules/<rule-id>` package,
implements `detector.Detector`, and exposes a constructor returning that
interface. Add the constructor to `detector/registry.All` and keep focused tests
beside the rule. The core detector package never imports implementations, which
keeps dependency direction acyclic as the catalog grows. Reporters read generic
findings, so they do not require rule-specific changes.

## Internal package dependency direction

Dependencies point inward toward stable domain contracts:

```text
cli -> app -> extractors / graph / detector engine
                         detector registry -> detector rules -> detector contracts
report -> model          extractors / graph / detector contracts -> model
```

Technology-specific implementations belong below their extension point, as the
existing `extractor/<technology>` and `detector/rules/<rule-id>` layouts show.
Create another package when it owns a distinct responsibility or dependency—not
only to shorten a file. Registries are explicit composition roots; feature
packages do not register themselves through global mutable state.
