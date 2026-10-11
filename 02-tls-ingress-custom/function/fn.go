package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Function composes an Ingress and Certificate and projects certificate status.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
	now func() time.Time
}

func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)
	if req.Desired != nil {
		rsp.Desired = proto.Clone(req.Desired).(*fnv1.State)
	}
	if err := f.compose(req, rsp); err != nil {
		if f.log != nil {
			f.log.Info("Cannot compose TLS ingress", "error", err.Error())
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
	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		return fmt.Errorf("get desired composite: %w", err)
	}
	desired := make(map[resource.Name]*resource.DesiredComposed)
	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		return fmt.Errorf("get observed resources: %w", err)
	}

	host, err := xr.Resource.GetString("spec.hostname")
	if err != nil {
		return fmt.Errorf("get hostname: %w", err)
	}
	service, err := xr.Resource.GetString("spec.ingress.serviceName")
	if err != nil {
		return fmt.Errorf("get serviceName: %w", err)
	}
	port, err := xr.Resource.GetInteger("spec.ingress.servicePort")
	if err != nil {
		return fmt.Errorf("get servicePort: %w", err)
	}
	issuer, err := xr.Resource.GetString("spec.certificate.issuerRef.name")
	if err != nil {
		return fmt.Errorf("get issuer name: %w", err)
	}
	issuerKind, err := xr.Resource.GetString("spec.certificate.issuerRef.kind")
	if err != nil {
		return fmt.Errorf("get issuer kind: %w", err)
	}
	class, _, err := unstructured.NestedString(xr.Resource.Object, "spec", "ingress", "className")
	if err != nil {
		return fmt.Errorf("get ingress class: %w", err)
	}

	now := time.Now
	if f.now != nil {
		now = f.now
	}
	var cert map[string]interface{}
	if c, ok := observed["certificate"]; ok && c.Resource != nil {
		cert = c.Resource.Object
	}
	status := certificateStatus(cert, now())
	name, namespace := xr.Resource.GetName(), xr.Resource.GetNamespace()
	secret := name + "-tls"
	metadata := func() map[string]interface{} {
		return map[string]interface{}{"name": name, "namespace": namespace}
	}
	certificate := map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": metadata(),
		"spec": map[string]interface{}{
			"secretName": secret, "dnsNames": []interface{}{host},
			"issuerRef": map[string]interface{}{"name": issuer, "kind": issuerKind, "group": "cert-manager.io"},
		},
	}
	ingressMeta := metadata()
	if expiry, ok := status["notAfter"]; ok {
		ingressMeta["annotations"] = map[string]interface{}{"platform.mujak.com/cert-not-after": expiry}
	}
	ingressSpec := map[string]interface{}{
		"tls": []interface{}{map[string]interface{}{"hosts": []interface{}{host}, "secretName": secret}},
		"rules": []interface{}{map[string]interface{}{
			"host": host,
			"http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{
				"path": "/", "pathType": "Prefix",
				"backend": map[string]interface{}{"service": map[string]interface{}{
					"name": service, "port": map[string]interface{}{"number": port},
				}},
			}}},
		}},
	}
	if class != "" {
		ingressSpec["ingressClassName"] = class
	}
	ingress := map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": ingressMeta, "spec": ingressSpec,
	}
	// SDK resource keys replace the go-templating-specific resource-name annotation.
	desired["certificate"] = &resource.DesiredComposed{Resource: &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: certificate}}}
	desired["ingress"] = &resource.DesiredComposed{Resource: &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: ingress}}}
	if err := unstructured.SetNestedMap(dxr.Resource.Object, status, "status", "certificate"); err != nil {
		return fmt.Errorf("set certificate status: %w", err)
	}
	if err := response.SetDesiredCompositeResource(rsp, dxr); err != nil {
		return err
	}
	// The SDK's composite getter does not carry Ready through its wrapper.
	rsp.Desired.Composite.Ready = req.GetDesired().GetComposite().GetReady()
	return response.SetDesiredComposedResources(rsp, desired)
}

// certificateStatus mirrors the original template, including whole-day warning
// calculation and Ready being independent from expiry health.
func certificateStatus(cert map[string]interface{}, now time.Time) map[string]interface{} {
	ready := false
	conditions, _, _ := unstructured.NestedSlice(cert, "status", "conditions")
	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if ok && condition["type"] == "Ready" {
			ready = condition["status"] == "True"
			break
		}
	}
	status := map[string]interface{}{"ready": ready, "health": "Pending", "daysRemaining": int64(0)}
	notAfter, _, _ := unstructured.NestedString(cert, "status", "notAfter")
	if notAfter == "" {
		return status
	}
	status["notAfter"] = notAfter
	// Sprig's toDate returns the zero time on invalid input; preserve that behavior.
	expiry, _ := time.Parse("2006-01-02T15:04:05Z", notAfter)
	seconds := expiry.Unix() - now.Unix()
	days := seconds / 86400
	status["daysRemaining"] = days
	switch {
	case seconds <= 0:
		status["health"] = "Expired"
	case days <= 15:
		status["health"] = "Warning"
	case ready:
		status["health"] = "Healthy"
	}
	return status
}
