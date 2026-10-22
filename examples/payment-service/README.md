# payment-service example

This executable Go service exposes `/health`, `/ready`, and optional `/metrics`
endpoints. It requires `DATABASE_URL`, but only validates that the value is
present; the example does not connect to a database.

The default Fleck configuration uses `values-valid.yaml`. To inject all four
MVP fault categories, run:

```sh
fleck check . --values deploy/helm/payment-service/values-faulty.yaml --fail-on error
```

Build the container and load it into Kind before installing the chart when
testing deployment behavior. The placeholder Secret is for local demonstration
only and is never included in Fleck output.
