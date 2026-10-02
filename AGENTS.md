# Guardrails

- Always work in a dedicated Git worktree on a task branch. Create or reuse one and switch into it before editing; leave the primary checkout untouched.
- Use Crossplane v2 namespaced composite resources: XRDs must use `apiextensions.crossplane.io/v2` with `spec.scope: Namespaced`. Use namespaced managed resources and set `metadata.namespace` on resource instances in manifests and examples. Keep composed resources in the composite's namespace.
- Do not introduce cluster-scoped composite or managed resources, legacy claims, or cluster-scoped workload resources. Crossplane infrastructure objects that are inherently cluster-scoped (such as XRDs, Compositions, and Functions) are the exception; do not add namespaces to them.
