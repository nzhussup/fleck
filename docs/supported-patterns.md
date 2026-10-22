# Supported patterns

## Go

- Type-resolved `os.Getenv` and `os.LookupEnv` calls.
- Literal and compile-time constant keys.
- Required reads guarded by an empty-value or failed-lookup branch that calls
  `panic`, `log.Fatal*`, or `os.Exit`.
- One-level returned-error propagation when the same-file `main` function calls
  the helper, checks a returned variable against `nil`, and terminates.
- Optional reads with a local non-empty fallback assignment.
- Direct `Getenv(...) == "true"` or `"false"` feature flags.
- Unknown classification for other uses and preservation of dynamic keys.
- Type-resolved `net/http.ListenAndServe` with `":8080"`, a simple literal
  variable, or `":" + port` where `port` is environment-derived and has an
  optional literal fallback.

Other returned errors remain unknown because Fleck does not build a general
call graph or assume that every caller propagates failure to initialization.

## Kubernetes and Helm

Helm is rendered once with `helm template`. Supported Kubernetes inputs are
Deployments, Services, ConfigMaps, and Secrets. Fleck selects the exact workload
and container named by `fleck.yaml`.

Supported environment forms are literal `env`, ConfigMap/Secret key references,
and prefixed `envFrom`. Resolution is namespace-aware and preserves optional,
missing, and unverified references. Service selectors match Pod-template labels;
numeric and named `targetPort` values are supported. Named ports resolve only
against the selected container with a matching protocol.

## Dockerfile

The final stage and statically named stage inheritance support `ENV`, `EXPOSE`,
`ENTRYPOINT`, and `CMD`. Simple expansion from earlier `ENV` declarations is
resolved. `ARG` is not treated as runtime supply. Unresolved expansion remains
unknown.
