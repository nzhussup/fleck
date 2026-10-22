# Supported patterns

## Go

- Type-resolved `os.Getenv` and `os.LookupEnv` calls.
- Literal and compile-time constant keys.
- Required reads guarded by a direct function-body empty-value or failed-lookup
  branch that unconditionally calls `panic`, `log.Fatal*`, or `os.Exit`.
- One-level returned-error propagation when the same-file `main` function calls
  the helper, checks a returned variable against `nil`, and terminates.
- Optional reads with a direct local non-empty fallback assignment.
- Direct `Getenv(...) == "true"` or `"false"` feature flags.
- Unknown classification for other uses and preservation of dynamic keys.
- Type-resolved `net/http.ListenAndServe` with `":8080"`, a simple literal
  variable, or `":" + port` where `port` is environment-derived and has an
  optional literal fallback.

Other returned errors remain unknown because Fleck does not build a general
call graph or assume that every caller propagates failure to initialization.
Local variables are tracked by `go/types` object identity, so shadowing and
same-name variables in separate functions remain independent. Nested or
otherwise unsupported control flow remains unknown.

## Kubernetes and Helm

Helm charts are rendered once with `helm template`; raw YAML files or directories
can be configured instead. Supported Kubernetes inputs are Deployments,
Services, ConfigMaps, and Secrets. Fleck selects the exact workload and container
named by `fleck.yaml`.

`deployment.workload.namespace` may select a namespace explicitly. If it is
omitted and more than one workload has the configured kind and name, analysis
stops with an ambiguity error.

Supported environment forms are literal `env`, ConfigMap/Secret key references,
and prefixed `envFrom`. Resolution is namespace-aware and preserves optional,
missing, and unverified references. Service selectors match Pod-template labels;
numeric and named `targetPort` values are supported. Named-port resolution
preserves the owning container and protocol; duplicate matches across
containers remain ambiguous. Rendered-YAML resource locations are reported,
but Fleck does not claim they are original Helm-template locations.

Effective environment resolution is centralized. Docker image `ENV` is the
base, Kubernetes `envFrom` follows declaration order, and explicit Kubernetes
`env` has highest precedence. Unknown higher-priority sources prevent fallback
to a lower-priority Docker value. Secret-backed presence can be established
without retaining or displaying its value.

## Dockerfile

The final stage and statically named stage inheritance support `ENV`, `EXPOSE`,
`ENTRYPOINT`, and `CMD`. Simple expansion from earlier `ENV` declarations is
resolved. `ARG` is not treated as runtime supply. Unknown build arguments and
unsupported expansion forms remain unknown rather than becoming fabricated
runtime values.
