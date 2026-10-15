# TLS ingress

Requires Crossplane v2, cert-manager, an nginx ingress controller, and the
Functions in `functions.yaml`. Apply `rbac.yaml` so Crossplane can manage
Certificates and Ingresses through its aggregated ClusterRole. This grants
permissions cluster-wide; resource types and API groups are explicitly limited.

The v2 XRD is explicitly `Namespaced`. Each XR composes resources in its own
namespace, which `composition.yaml` reads from `metadata.namespace`. The example
uses namespace `default`, Service `app:8080`, and ClusterIssuer `letsencrypt-prod`;
provision those dependencies separately.

The example XR is `default/tls-ingress-demo` and explicitly selects
`xingresstls.platform.mujak.com`. Its Certificate and Ingress use the same name,
and its TLS Secret is `default/tls-ingress-demo-tls`.

`kubectl kustomize .` renders only the XRD and Composition. Install Functions and
RBAC separately. See the root [naming convention](../README.md#naming-and-installation).

## Provisioning readiness

The template reports each composed resource's readiness through
`gotemplating.fn.crossplane.io/ready`; Crossplane derives the XR's Ready
condition. No generic auto-ready step or forced XR Ready condition is used.

- A Certificate is ready only when its Ready condition is True for its current
  generation and `status.notAfter` is valid and strictly in the future. Missing
  resources, missing/stale observedGeneration, missing expiry, and invalid or
  expired certificates are not ready.
- `status.certificate.ready` uses this same predicate (previously it only checked
  the Ready condition). Health remains separate: missing expiry is `Pending`,
  invalid/expired expiry is `Expired`, and at most 15 whole days remaining is
  `Warning`. Otherwise a ready certificate is `Healthy`, or `Pending` if unready.
  Near-expiry certificates remain ready while reporting `Warning`.
- `daysRemaining` is the difference in Unix seconds divided by 86400, truncated
  toward zero. Observed expiry is copied to XR status and the Ingress annotation
  `platform.mujak.com/cert-not-after`. Invalid dates retain the existing zero-time
  fallback for the days calculation and never make the Certificate ready.
- An Ingress is ready only when its observed rules (hosts, paths/path types,
  backend Service/port), TLS hosts/Secret, and any explicitly requested class
  match the desired configuration. Extra rules, paths or TLS entries are not a
  match. Equivalent numeric representations are accepted.
- Empty load-balancer status and unrelated controller metadata/status are ignored.
  When no class is requested, the template leaves it unset and tolerates an
  admission-defaulted observed class.

Provisioning readiness does not test DNS, client CA trust, backend Service or
endpoints availability, or successful HTTPS traffic. Check application health
separately; neither a ready Certificate nor a matching Ingress proves it.

## Local regression checks

Requires Python 3.9+, PyYAML, the Crossplane CLI (`composition render`), and Docker.
The tests run the actual `function-go-templating:v0.11.2` package through the
Crossplane v2.4.2 render engine; they never connect to a Kubernetes cluster.
Only the template's clock is replaced in memory for deterministic expiry tests.

Run `bash tests/render.sh -v` from this directory after installing PyYAML
(`python3 -m pip install PyYAML==6.0.3` in your Python environment).
The checks cover missing resources/fields, condition freshness, expiry and warning
boundaries, Ingress mismatches/defaulting, resource identities/specs/status, and
unready -> ready -> unready transitions. They assert readiness derived by the
render engine from Function responses, not only intermediate YAML annotations.

For a normal render using the real clock, from the repository root:

```sh
crossplane composition render 01-tls-ingress/examples/xr.yaml \
  01-tls-ingress/composition.yaml 01-tls-ingress/functions.yaml \
  --crossplane-version=v2.4.2 \
  --observed-resources=01-tls-ingress/examples/observed/healthy.yaml
```

## GitOps rollout

The downstream `k8s-crossplane` example reads these definitions from `main`, so
merging a Composition change may trigger deployment. Coordinate synchronization
and record the previous source commit/CompositionRevision before merging.
No Function package release or XR/XRD migration is needed for this template fix.
Capture the XR, Certificate and Ingress UIDs before rollout, verify current-generation
Synced/Ready and unchanged identities afterward, and check example 02 separately.
Rollback through the GitOps source of truth; do not delete/recreate resources.

## Migrating an existing LegacyCluster XRD

Scope cannot be changed in place. Do not simply apply the new XRD over the old
one or delete its generated CRD while XRs still exist.

1. Publish this fix and include `rbac.yaml` in Argo's definitions source. Prevent
   Argo/ApplicationSet synchronization of this group during the migration, and
   ensure it cannot recreate the old definition or XR.
2. Back up the existing XRD, generated CRD, and all XRs. Inventory resourceRefs,
   actual composed resources, owner references, and finalizers. Check that no
   unrelated workloads will be deleted. Apply RBAC and verify list/watch access.
3. Delete the old XRs and allow their finalizers to complete normally. If deletion
   stalls on malformed resourceRefs, investigate before bypassing any finalizer.
4. Delete the old XRD and wait for both it and its generated CRD to disappear.
5. Apply the new namespaced XRD and wait for the generated CRD and XRD to become
   Established. Verify the generated CRD scope is Namespaced and Functions are
   healthy before reapplying the example XR.
6. Restore synchronization. Check the XR's Synced condition and resourceRefs;
   all composed resources must target its real namespace, never `<no value>`.

An XR reporting Synced does not guarantee certificate issuance or a working
backend; verify the Certificate, issuer, Ingress, and Service separately.
