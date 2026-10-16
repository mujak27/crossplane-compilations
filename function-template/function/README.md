# Function module

Run generation and the local render workflow from the parent directory; see
[the template quick start](../README.md). `api/v1alpha1/types.go` is the source
of truth for the generated schema and deepcopy code. `example.org/function-template`
is a placeholder module path, not a published module.

For manual development:

```sh
go test ./...
go run . --insecure --debug
```

The development server listens on port 9443. Use insecure mode only locally.
