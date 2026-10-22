# Inventory monolith

This example represents a multi-package Go monolith. Its `application.path`
selects the web command; Fleck also follows its local package dependencies, such
as `internal/catalog`. Deployment configuration is checked directly from raw
Kubernetes YAML; there is no Dockerfile.

```sh
fleck check .
fleck graph . --format dot
```

Expected result: all enabled rules are applicable, with zero findings and zero
skipped checks. Docker is absent but none of these checks needs it.
