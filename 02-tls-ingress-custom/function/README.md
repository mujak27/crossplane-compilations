# function-tls-ingress

Go composition function generated from Crossplane's `function-template-go`.
See [the example README](../README.md) for usage and packaging instructions.

The function has no pipeline input: its contract is the `XIngressTLSCustom` schema
in `../xrd.yaml`. The Go types in `api/v1alpha1/types.go` mirror this schema; keep
them aligned when changing the XRD. The observed composite is decoded with
`runtime.DefaultUnstructuredConverter.FromUnstructured`, and required spec
fields use pointers so missing values remain fatal without rejecting explicit
zero values. Only the typed certificate status is converted back to unstructured
and written to the desired composite, preserving unrelated status fields.

Certificate and Ingress resources remain unstructured. The function uses
observed certificate state, never desired state, to project expiry and readiness
onto the composite and Ingress.
