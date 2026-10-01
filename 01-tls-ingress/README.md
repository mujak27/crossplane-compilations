# TLS ingress

Requires Crossplane v2, cert-manager, an nginx ingress controller, and the
Functions in `functions.yaml`. Apply `rbac.yaml` so Crossplane can manage
Certificates and Ingresses through its aggregated ClusterRole. This grants
permissions cluster-wide; resource types and API groups are explicitly limited.

The v2 XRD is explicitly `Namespaced`. Each XR composes resources in its own
namespace, which `composition.yaml` reads from `metadata.namespace`. The example
uses namespace `default`, Service `app:8080`, and ClusterIssuer `letsencrypt-prod`;
provision those dependencies separately.

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
