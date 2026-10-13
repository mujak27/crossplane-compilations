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
	"github.com/crossplane/function-tls-ingress/api/v1alpha1"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

	var composite v1alpha1.XIngressTLSCustom
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(xr.Resource.Object, &composite); err != nil {
		return fmt.Errorf("convert observed composite: %w", err)
	}
	spec := composite.Spec
	if spec.Hostname == nil || spec.Ingress.ServiceName == nil || spec.Ingress.ServicePort == nil ||
		spec.Certificate.IssuerRef.Name == nil || spec.Certificate.IssuerRef.Kind == nil {
		return fmt.Errorf("composite spec requires hostname, ingress.serviceName, ingress.servicePort, certificate.issuerRef.name and certificate.issuerRef.kind")
	}
	host := *spec.Hostname

	now := time.Now
	if f.now != nil {
		now = f.now
	}
	var cert map[string]interface{}
	if c, ok := observed["certificate"]; ok && c.Resource != nil {
		cert = c.Resource.Object
	}
	status := certificateStatus(cert, now())
	name, namespace := composite.Name, composite.Namespace
	secret := name + "-tls"
	metadata := func() map[string]interface{} {
		return map[string]interface{}{"name": name, "namespace": namespace}
	}
	certificate := map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": metadata(),
		"spec": map[string]interface{}{
			"secretName": secret, "dnsNames": []interface{}{host},
			"issuerRef": map[string]interface{}{"name": *spec.Certificate.IssuerRef.Name, "kind": *spec.Certificate.IssuerRef.Kind, "group": "cert-manager.io"},
		},
	}
	ingressMeta := metadata()
	if status.NotAfter != "" {
		ingressMeta["annotations"] = map[string]interface{}{"platform.mujak.com/cert-not-after": status.NotAfter}
	}
	ingressSpec := map[string]interface{}{
		"tls": []interface{}{map[string]interface{}{"hosts": []interface{}{host}, "secretName": secret}},
		"rules": []interface{}{map[string]interface{}{
			"host": host,
			"http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{
				"path": "/", "pathType": "Prefix",
				"backend": map[string]interface{}{"service": map[string]interface{}{
					"name": *spec.Ingress.ServiceName, "port": map[string]interface{}{"number": *spec.Ingress.ServicePort},
				}},
			}}},
		}},
	}
	if spec.Ingress.ClassName != "" {
		ingressSpec["ingressClassName"] = spec.Ingress.ClassName
	}
	ingress := map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": ingressMeta, "spec": ingressSpec,
	}
	// SDK resource keys replace the go-templating-specific resource-name annotation.
	desired["certificate"] = &resource.DesiredComposed{Resource: &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: certificate}}}
	desired["ingress"] = &resource.DesiredComposed{Resource: &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: ingress}}}
	certificateStatusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return fmt.Errorf("convert certificate status: %w", err)
	}
	if err := unstructured.SetNestedMap(dxr.Resource.Object, certificateStatusMap, "status", "certificate"); err != nil {
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
func certificateStatus(cert map[string]interface{}, now time.Time) v1alpha1.CertificateStatus {
	ready := false
	conditions, _, _ := unstructured.NestedSlice(cert, "status", "conditions")
	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if ok && condition["type"] == "Ready" {
			ready = condition["status"] == "True"
			break
		}
	}
	status := v1alpha1.CertificateStatus{Ready: ready, Health: "Pending"}
	notAfter, _, _ := unstructured.NestedString(cert, "status", "notAfter")
	if notAfter == "" {
		return status
	}
	status.NotAfter = notAfter
	// Sprig's toDate returns the zero time on invalid input; preserve that behavior.
	expiry, _ := time.Parse("2006-01-02T15:04:05Z", notAfter)
	seconds := expiry.Unix() - now.Unix()
	days := seconds / 86400
	status.DaysRemaining = days
	switch {
	case seconds <= 0:
		status.Health = "Expired"
	case days <= 15:
		status.Health = "Warning"
	case ready:
		status.Health = "Healthy"
	}
	return status
}
