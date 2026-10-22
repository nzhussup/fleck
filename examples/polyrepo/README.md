# Split application and deployment repositories

These sibling directories represent repositories maintained by different
teams. They deliberately do not pretend Fleck has evidence that is unavailable
inside the current repository.

```sh
fleck check .
fleck check . --target customer-api-development
fleck check . --target customer-api-production
```

The workspace-level `fleck.yaml` joins source and image metadata from
`application-repo` with raw manifests from `deployment-repo`. It intentionally
spells `REDIS_ADDRESS` differently in the Dockerfile.

Two targets reuse that application evidence against separate development and
production manifest directories. This models one application repository being
deployed into multiple environments without copying either repository into the
other.

The deployment repository is a GitOps-style Kubernetes-only repository. It
demonstrates CFG005 because its Service routes to `9090` while the selected
container declares `8080`. Fleck reports a cautious warning, not a claim about
the actual application listener. Its externally managed Secret also appears as
an unverified-reference diagnostic because the Secret is not checked into this
repository.

Because the joined source proves that the application listens on `8080`, the
development Service route also produces the stronger CFG003 error. Production
routes correctly. The development target is intentionally faulty, so checking
the whole workspace exits with status `1` by default.

In a real organization these directories can be Git submodules, regular clones,
or CI checkouts. Only their paths relative to the workspace configuration
matter; Fleck does not require them to share Git history.
