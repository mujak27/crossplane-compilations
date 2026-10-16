package main

import (
	"context"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const exampleJSON = `{"apiVersion":"template.example.org/v1alpha1","kind":"XExample","metadata":{"name":"example","namespace":"apps"},"spec":{"message":"hello"}}`

func TestRunFunction(t *testing.T) {
	req := &fnv1.RunFunctionRequest{
		Meta:     &fnv1.RequestMeta{Tag: "test"},
		Context:  resource.MustStructJSON(`{"pipeline":"preserved"}`),
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(exampleJSON)}},
		Desired: &fnv1.State{
			Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"status":{"other":"preserved"}}`), Ready: fnv1.Ready_READY_TRUE},
			Resources: map[string]*fnv1.Resource{
				"other":     {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"other","namespace":"apps"}}`), Ready: fnv1.Ready_READY_TRUE, ConnectionDetails: map[string][]byte{"key": []byte("value")}},
				"configmap": {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","data":{"message":"old"}}`)},
			},
		},
	}
	before := proto.Clone(req).(*fnv1.RunFunctionRequest)
	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil || len(rsp.Results) != 0 {
		t.Fatalf("RunFunction: %v, results: %v", err, rsp.GetResults())
	}
	if len(rsp.Desired.Resources) != 2 {
		t.Fatalf("unexpected desired resource keys: %v", rsp.Desired.Resources)
	}
	cm := rsp.Desired.Resources["configmap"].Resource.AsMap()
	for path, want := range map[string]string{
		"apiVersion": "v1", "kind": "ConfigMap", "metadata.name": "example",
		"metadata.namespace": "apps", "data.message": "hello",
	} {
		got, _, err := unstructured.NestedString(cm, strings.Split(path, ".")...)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
	if !proto.Equal(before.Desired.Composite, rsp.Desired.Composite) || !proto.Equal(before.Desired.Resources["other"], rsp.Desired.Resources["other"]) {
		t.Fatal("unrelated desired state was modified")
	}
	if !proto.Equal(before, req) {
		t.Fatal("request was modified")
	}
	if rsp.Meta.Tag != "test" {
		t.Fatal("response lost request tag")
	}
	if !proto.Equal(before.Context, rsp.Context) {
		t.Fatal("pipeline context was modified")
	}
	// A subsequent invocation updates the same key, even when message/name change.
	req.Desired = rsp.Desired
	req.Observed.Composite.Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(exampleJSON, `"example"`, `"renamed"`), `"hello"`, `""`))
	next, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil || len(next.Results) != 0 || len(next.Desired.Resources) != 2 {
		t.Fatalf("repeated invocation: %v, %v", err, next)
	}
	cm = next.Desired.Resources["configmap"].Resource.AsMap()
	if name, _, _ := unstructured.NestedString(cm, "metadata", "name"); name != "renamed" {
		t.Fatalf("updated name = %q", name)
	}
	if message, found, _ := unstructured.NestedString(cm, "data", "message"); !found || message != "" {
		t.Fatal("empty string message should be preserved")
	}
}

func TestInvalidComposite(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"missing namespace", strings.Replace(exampleJSON, `,"namespace":"apps"`, "", 1), "metadata.namespace"},
		{"missing name", strings.Replace(exampleJSON, `"name":"example",`, "", 1), "metadata.name"},
		{"malformed namespace", strings.Replace(exampleJSON, `"namespace":"apps"`, `"namespace":123`, 1), "convert observed composite"},
		{"missing message", strings.Replace(exampleJSON, `"message":"hello"`, "", 1), "spec.message"},
		{"null message", strings.Replace(exampleJSON, `"hello"`, "null", 1), "spec.message"},
		{"malformed message", strings.Replace(exampleJSON, `"hello"`, "123", 1), "convert observed composite"},
		{"malformed spec", strings.Replace(exampleJSON, `{"message":"hello"}`, `[]`, 1), "convert observed composite"},
		{"empty composite", `{}`, "metadata.name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &fnv1.RunFunctionRequest{
				Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(tc.input)}},
				Desired:  &fnv1.State{Resources: map[string]*fnv1.Resource{"other": {Resource: resource.MustStructJSON(`{"kind":"ConfigMap"}`)}}},
				Context:  resource.MustStructJSON(`{"pipeline":"preserved"}`),
			}
			before := proto.Clone(req).(*fnv1.RunFunctionRequest)
			rsp, err := (&Function{}).RunFunction(context.Background(), req)
			if err != nil || len(rsp.Results) != 1 || rsp.Results[0].Severity != fnv1.Severity_SEVERITY_FATAL || !strings.Contains(rsp.Results[0].Message, tc.want) {
				t.Fatalf("expected fatal %q, got %v, %v", tc.want, rsp.GetResults(), err)
			}
			if !proto.Equal(before, req) || !proto.Equal(before.Desired, rsp.Desired) || !proto.Equal(before.Context, rsp.Context) {
				t.Fatal("invalid input modified request or pipeline state")
			}
		})
	}
}
