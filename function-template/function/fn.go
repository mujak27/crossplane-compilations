package main

import (
	"context"
	"fmt"

	"example.org/function-template/api/v1alpha1"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Function composes one ConfigMap without replacing other pipeline resources.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}

func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)
	if req.Desired != nil {
		rsp.Desired = proto.Clone(req.Desired).(*fnv1.State)
	}
	if err := f.compose(req, rsp); err != nil {
		if f.log != nil {
			f.log.Info("Cannot compose example", "error", err.Error())
		}
		response.Fatal(rsp, err)
	}
	return rsp, nil
}

func (f *Function) compose(req *fnv1.RunFunctionRequest, rsp *fnv1.RunFunctionResponse) error {
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		return fmt.Errorf("get observed composite: %w", err)
	}
	var composite v1alpha1.XExample
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(xr.Resource.Object, &composite); err != nil {
		return fmt.Errorf("convert observed composite: %w", err)
	}
	if composite.Name == "" || composite.Namespace == "" {
		return fmt.Errorf("composite metadata.name and metadata.namespace are required")
	}
	if composite.Spec.Message == nil {
		return fmt.Errorf("composite spec.message is required and must be a string")
	}
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		return fmt.Errorf("get desired resources: %w", err)
	}
	desired["configmap"] = &resource.DesiredComposed{
		Resource: &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": composite.Name, "namespace": composite.Namespace},
			"data":     map[string]interface{}{"message": *composite.Spec.Message},
		}}},
	}
	// Only rewrite our key: the SDK's typed view omits other resource protocol fields.
	if err := response.SetDesiredComposedResources(rsp, map[resource.Name]*resource.DesiredComposed{"configmap": desired["configmap"]}); err != nil {
		return fmt.Errorf("set desired resources: %w", err)
	}
	return nil
}
