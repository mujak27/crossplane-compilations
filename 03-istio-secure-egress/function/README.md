# Secure-egress function module

Module: `github.com/mujak27/function-istio-secure-egress`.
See [the local development workflow](../README.md) for generation and fixture-backed
rendering. `api/v1alpha1/types.go` owns the generated XR schema and deepcopy code.

```sh
go test ./...
go run . --insecure --debug
```

The development server listens on localhost port 9443. Insecure mode is local only.
No Kubernetes or Istio API client is used: Crossplane supplies the selected profile
through its required-resources protocol. No readiness detector is implemented yet.
