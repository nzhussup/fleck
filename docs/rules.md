# Fleck rules

Rule IDs are stable because they appear in CLI filters, JSON reports, and
evaluation data.

| Rule | Severity | Meaning |
|---|---|---|
| **CFG001** | Error | A required environment variable is definitely absent from the selected container. Optional, unknown, or potentially externally supplied variables are not reported. |
| **CFG002** | Warning | An unmatched application environment-variable name closely resembles an unmatched supplied name. This suggests a typo; it does not prove both names refer to the same setting. |
| **CFG003** | Error | A resolved Kubernetes Service `targetPort` differs from a statically resolved application listening port. `Service.port`, `containerPort`, and Dockerfile `EXPOSE` are not treated as application listeners. |
| **CFG004** | Info | A supplied environment variable has no recognized read in the analyzed application. It may still be consumed by libraries, scripts, or other runtime components. |
| **CFG005** | Warning | Dockerfile `EXPOSE`, Kubernetes `containerPort`, or Service `targetPort` declarations differ. These fields are metadata and routing declarations, not proof of the real application listener. |

CFG001 is suppressed when unresolved `envFrom` or an exact unresolved reference
could supply the variable. CFG002 reports one best candidate. CFG003 compares
the resolved `targetPort`, never `Service.port`, `containerPort`, or `EXPOSE` as
if those made an application listen. CFG004 suppresses all suggestions when a
dynamic environment read exists and ignores common system variables.

CFG005 is deliberately cautious: it reports only when concrete declarations
exist on both sides and never claims that `EXPOSE` or `containerPort` makes a
process listen. Rules whose required source capabilities are unavailable are
reported as skipped rather than silently treated as passed.

Applicability is capability-based. CFG001 and CFG003 require application plus
deployment evidence. CFG002 and CFG004 can compare application reads with
supplies from either Docker or Kubernetes. CFG005 requires Kubernetes container
port analysis and incorporates Docker declarations when Docker was also
analyzed. The presence of a capability means extraction completed, even if it
produced zero matching facts.

In a workspace configuration, put `analysis.rules` inside each target. This
lets services and environments choose checks that match their available
inputs. Omitting the list enables every registered rule for that target. The
CLI `--rules` option is a temporary override for every target selected by that
command.

## Adding a rule

Create `internal/detector/rules/cfgNNN/rule.go` and `rule_test.go`, implement
`detector.Detector`, and register its constructor in
`internal/detector/registry/registry.go`.
