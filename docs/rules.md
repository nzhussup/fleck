# Fleck rules

Rule IDs are stable because they appear in CLI filters, JSON reports, and
evaluation data.

| Rule | Severity | Meaning |
|---|---|---|
| **CFG001** | Error | A required environment variable is definitely absent from the selected container. Optional, unknown, or potentially externally supplied variables are not reported. |
| **CFG002** | Warning | An unmatched application environment-variable name closely resembles an unmatched supplied name. This suggests a typo; it does not prove both names refer to the same setting. |
| **CFG003** | Error | A resolved Kubernetes Service `targetPort` differs from a statically resolved application listening port. `Service.port`, `containerPort`, and Dockerfile `EXPOSE` are not treated as application listeners. |
| **CFG004** | Info | A supplied environment variable has no recognized read in the analyzed application. It may still be consumed by libraries, scripts, or other runtime components. |

CFG001 is suppressed when unresolved `envFrom` or an exact unresolved reference
could supply the variable. CFG002 reports one best candidate. CFG003 compares
the resolved `targetPort`, never `Service.port`, `containerPort`, or `EXPOSE` as
if those made an application listen. CFG004 suppresses all suggestions when a
dynamic environment read exists and ignores common system variables.

## Adding a rule

Create `internal/detector/rules/cfgNNN/rule.go` and `rule_test.go`, implement
`detector.Detector`, and register its constructor in
`internal/detector/registry/registry.go`.
