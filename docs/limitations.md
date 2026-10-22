# Current limitations

- Requiredness inference is deliberately local. Returned-error propagation is
  limited to a same-file helper called and fatally checked by `main`.
- Requiredness checks inside unrelated outer conditions, loops, switches, or
  other unsupported control flow remain unknown.
- Dynamic environment names are preserved but not resolved.
- Only direct standard-library `net/http.ListenAndServe` patterns are modeled;
  wrappers, arbitrary frameworks, multiple conditional listeners, and complex
  address construction remain unknown.
- When several application listeners exist, Fleck does not guess which one a
  Service intends to reach and suppresses a definitive CFG003 mismatch.
- Third-party libraries, native code, entrypoint scripts, and child processes
  may read variables that CFG004 cannot see.
- Helm values-to-template provenance and original template line numbers are not
  claimed. Fleck reports the originating chart template, but Helm does not
  expose a reliable rendered-to-template line mapping, so Helm locations omit
  line numbers. Plain Kubernetes manifests retain their source file and the
  resource's starting line.
- External ConfigMaps and Secrets absent from the render are unverified, not
  definitely absent. Secret values are intentionally unavailable for analysis.
- Dockerfile interpretation is not a build or image inspection. Complex stage
  and variable expansion is unresolved, and `EXPOSE` is metadata only.
- A workspace may contain many explicit targets, but each target still selects
  at most one Go application, workload, and primary container. Fleck does not
  infer application-to-deployment ownership automatically.
- The naming and unused-variable rules are heuristics and can produce false
  positives. Ambiguous equal-scoring naming candidates are suppressed. No
  finding is an automatic-fix instruction.
