# Guardrails

- Always work in a dedicated Git worktree on a task branch. Create or reuse one and switch into it before editing; leave the primary checkout untouched.
- Use Crossplane v2 namespaced composite resources: XRDs must use `apiextensions.crossplane.io/v2` with `spec.scope: Namespaced`. Use namespaced managed resources and set `metadata.namespace` on resource instances in manifests and examples. Keep composed resources in the composite's namespace.
- Do not introduce cluster-scoped composite or managed resources, legacy claims, or cluster-scoped workload resources. Crossplane infrastructure objects that are inherently cluster-scoped (such as XRDs, Compositions, and Functions) are the exception; do not add namespaces to them.

## Agent skills

### Issue tracker

Issues and specs live in GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five default triage labels. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` and `docs/adr/`. Before exploring, read `docs/agents/domain.md`.

### Crossplane functions

When adding or changing a composition function, read `docs/adding-a-function.md`.
