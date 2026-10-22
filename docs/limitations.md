# Current limitations

- Requiredness inference is deliberately local. Returned-error propagation is
  limited to a same-file helper called and fatally checked by `main`.
- Dynamic environment names are preserved but not resolved.
- Only direct standard-library `net/http.ListenAndServe` patterns are modeled;
  wrappers, arbitrary frameworks, multiple conditional listeners, and complex
  address construction remain unknown.
- Third-party libraries, native code, entrypoint scripts, and child processes
  may read variables that CFG004 cannot see.
- Helm values-to-template provenance and original template line numbers are not
  claimed. Rendered documents are the deployment authority.
- External ConfigMaps and Secrets absent from the render are unverified, not
  definitely absent. Secret values are intentionally unavailable for analysis.
- Dockerfile interpretation is not a build or image inspection. Complex stage
  and variable expansion is unresolved, and `EXPOSE` is metadata only.
- A workspace may contain many explicit targets, but each target still selects
  at most one Go application, workload, and primary container. Fleck does not
  infer application-to-deployment ownership automatically.
- The naming and unused-variable rules are heuristics and can produce false
  positives. No finding is an automatic-fix instruction.
