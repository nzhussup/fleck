# Fleck examples

The examples intentionally use different repository layouts and input-layer
combinations. Run commands from the repository root after building `./fleck`.

| Example | Structure | Available layers | Expected result |
|---|---|---|---|
| `payment-service` | One service with a Helm chart | Go + Docker + Helm | Clean by default; values overlays inject faults |
| `monolith` | Multi-package Go monolith with raw manifests | Go + Kubernetes | Clean; Docker is not required |
| `microservices-monorepo` | Two targets with different rule sets | Mixed per target | Checks both services in one run |
| `polyrepo` | Two environment targets join application and GitOps repositories | Go + Docker + Kubernetes | Development has an intentional CFG003 error |

```sh
./fleck check ./examples/monolith
./fleck check ./examples/microservices-monorepo
./fleck check ./examples/microservices-monorepo --target orders
./fleck check ./examples/polyrepo
```

A root `fleck.yaml` may contain many named targets. Each target defines one
application/deployment consistency boundary and may reference paths belonging
to different Git repositories checked out under the workspace. Rule lists are
also target-specific.
