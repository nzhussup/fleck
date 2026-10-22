# Fleck rules

Rule IDs are stable because they appear in CLI filters, JSON reports, and
evaluation data.

| Rule | Severity | Confidence | Meaning |
|---|---|---|---|
| **CFG001** | Error | High | A required environment variable is definitely absent from the selected container. Optional, unknown, or potentially externally supplied variables are not reported. |
| **CFG002** | Warning | Medium | An unmatched application environment-variable name closely resembles an unmatched supplied name. This suggests a typo; it does not prove both names refer to the same setting. |
| **CFG003** | Error | High | A resolved Kubernetes Service `targetPort` differs from one definitely identified application listening port. `Service.port`, `containerPort`, and Dockerfile `EXPOSE` are not treated as application listeners. |
| **CFG004** | Info | Low | A supplied environment variable has no recognized read in the analyzed application. It may still be consumed by libraries, scripts, or other runtime components. |
| **CFG005** | Warning | Medium | Dockerfile `EXPOSE` and Kubernetes `containerPort` metadata have no port in common. These declarations are not proof of the real application listener. |
| **CFG006** | Error | High | A mandatory ConfigMap/Secret key or named Service target is proven invalid by resources present in the analyzed manifests. External resources absent from the input remain unverified instead. |

CFG001 is suppressed when unresolved `envFrom` or an exact unresolved reference
could supply the variable. CFG002 compares unique names deterministically and
suppresses equally plausible candidates. Its normalized Levenshtein threshold
is `0.80`, and the score is emitted as structured evidence. CFG003 compares the resolved
`targetPort` only when exactly one application listener is known; it never uses
`Service.port`, `containerPort`, or `EXPOSE` as proof that an application
listens. CFG004 suppresses all suggestions when a dynamic environment read
exists and ignores common system variables.

CFG005 is deliberately cautious: it reports only when concrete Docker and
Kubernetes metadata declarations exist on both sides and their sets are
disjoint. It never claims that `EXPOSE` or `containerPort` makes a process
listen. Rules whose required source capabilities are unavailable are reported
as skipped rather than silently treated as passed.

Applicability is capability-based. CFG001 and CFG003 require application plus
deployment evidence. CFG002 and CFG004 can compare application reads with
supplies from either Docker or Kubernetes. CFG005 requires Kubernetes container
port analysis and incorporates Docker declarations when Docker was also
analyzed. CFG006 requires Kubernetes deployment evidence. The presence of a
capability means extraction completed, even if it produced zero matching facts.

Severity controls operational impact; confidence describes evidentiary strength.
Findings for the same target and configuration occurrence share a correlation
key. Reports retain each rule result while also exposing a deduplicated `issues`
count.

In a workspace configuration, put `analysis.rules` inside each target. This
lets services and environments choose checks that match their available
inputs. Omitting the list enables every registered rule for that target. The
CLI `--rules` option is a temporary override for every target selected by that
command.

## Adding a rule

Create `internal/detector/rules/cfgNNN/rule.go` and `rule_test.go`, implement
`detector.Detector`, and register its constructor in
`internal/detector/registry/registry.go`.
