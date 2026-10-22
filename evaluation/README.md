# Evaluation readiness

The `fixtures/cases.yaml` catalog labels reproducible cases without claiming
scientific results. A future Kind-based harness can run Fleck, deploy each case,
and record true/false positives and negatives from observed outcomes.

The current analyzer itself needs neither Kind nor a Kubernetes cluster. Its
versioned JSON report contains stable rule IDs and phase timings. Timings are
observations, not benchmark claims.
