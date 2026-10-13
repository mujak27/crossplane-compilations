package main

import (
	"context"
	"testing"
	"time"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCertificateStatus(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, condition, expiry, health string
		days                            int64
		ready                           bool
	}{
		{"missing", "", "", "Pending", 0, false},
		{"ready without expiry", "True", "", "Pending", 0, true},
		{"healthy", "True", "2026-11-03T12:00:00Z", "Healthy", 30, true},
		{"not ready", "False", "2026-11-03T12:00:00Z", "Pending", 30, false},
		{"unknown", "Unknown", "2026-11-03T12:00:00Z", "Pending", 30, false},
		{"warning", "True", "2026-10-19T12:00:00Z", "Warning", 15, true},
		{"truncated boundary", "False", "2026-10-20T11:59:59Z", "Warning", 15, false},
		{"sixteen days", "True", "2026-10-20T12:00:00Z", "Healthy", 16, true},
		{"expires now", "True", "2026-10-04T12:00:00Z", "Expired", 0, true},
		{"expired fraction", "False", "2026-10-04T11:59:59Z", "Expired", 0, false},
		{"expired", "True", "2026-10-02T12:00:00Z", "Expired", -2, true},
		{"invalid", "True", "invalid", "Expired", (time.Time{}.Unix() - now.Unix()) / 86400, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cert := map[string]interface{}{"status": map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": tc.condition}},
				"notAfter":   tc.expiry,
			}}
			want := map[string]interface{}{"ready": tc.ready, "health": tc.health, "daysRemaining": tc.days}
			if tc.expiry != "" {
				want["notAfter"] = tc.expiry
			}
			if diff := cmp.Diff(want, certificateStatus(cert, now)); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestRunFunction(t *testing.T) {
	for _, withCertificate := range []bool{false, true} {
		for _, withClass := range []bool{false, true} {
			req := &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{
					"apiVersion":"platform.mujak.com/v1alpha1","kind":"XIngressTLSCustom",
					"metadata":{"name":"demo-custom","namespace":"default"},
					"spec":{"hostname":"app-custom.mujak.com","ingress":{"serviceName":"app","servicePort":8080},
					"certificate":{"issuerRef":{"name":"letsencrypt-prod","kind":"ClusterIssuer"}}}}`)}},
				Desired: &fnv1.State{
					Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"status":{"other":"preserved","certificate":{"notAfter":"stale"}}}`)},
					Resources: map[string]*fnv1.Resource{"other": {Resource: resource.MustStructJSON(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"other"}}`)}},
				},
			}
			req.Desired.Composite.Ready = fnv1.Ready_READY_TRUE
			req.Desired.Resources["other"].Ready = fnv1.Ready_READY_TRUE
			req.Desired.Resources["other"].ConnectionDetails = map[string][]byte{"key": []byte("preserved")}
			if withClass {
				req.Observed.Composite.Resource.Fields["spec"].GetStructValue().Fields["ingress"].GetStructValue().Fields["className"] = resource.MustStructJSON(`{"value":"nginx"}`).Fields["value"]
			}
			if withCertificate {
				req.Observed.Resources = map[string]*fnv1.Resource{"certificate": {Resource: resource.MustStructJSON(`{
					"status":{"notAfter":"2026-11-03T12:00:00Z","conditions":[{"type":"Ready","status":"True"}]}}`)}}
			}
			before := proto.Clone(req)
			f := &Function{now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }}
			rsp, err := f.RunFunction(context.Background(), req)
			if err != nil || len(rsp.Results) != 0 {
				t.Fatalf("err=%v results=%v", err, rsp.Results)
			}
			if !proto.Equal(before, req) {
				t.Fatal("request mutated")
			}
			if rsp.Meta.Tag != "test" || !proto.Equal(req.Desired.Resources["other"], rsp.Desired.Resources["other"]) {
				t.Fatal("incoming state lost")
			}
			if rsp.Desired.Composite.Ready != fnv1.Ready_READY_TRUE {
				t.Fatal("incoming composite readiness lost")
			}
			cert := rsp.Desired.Resources["certificate"].Resource.AsMap()
			wantCert := resource.MustStructJSON(`{"apiVersion":"cert-manager.io/v1","kind":"Certificate","metadata":{"name":"demo-custom","namespace":"default"},"spec":{"secretName":"demo-custom-tls","dnsNames":["app-custom.mujak.com"],"issuerRef":{"name":"letsencrypt-prod","kind":"ClusterIssuer","group":"cert-manager.io"}}}`).AsMap()
			if diff := cmp.Diff(wantCert, cert); diff != "" {
				t.Fatal(diff)
			}
			ingress := rsp.Desired.Resources["ingress"].Resource.AsMap()
			wantIngress := resource.MustStructJSON(`{"apiVersion":"networking.k8s.io/v1","kind":"Ingress","metadata":{"name":"demo-custom","namespace":"default"},"spec":{"tls":[{"hosts":["app-custom.mujak.com"],"secretName":"demo-custom-tls"}],"rules":[{"host":"app-custom.mujak.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"app","port":{"number":8080}}}}]}}]}}`).AsMap()
			if withClass {
				wantIngress["spec"].(map[string]interface{})["ingressClassName"] = "nginx"
			}
			wantStatus := map[string]interface{}{"ready": false, "health": "Pending", "daysRemaining": float64(0)}
			if withCertificate {
				wantIngress["metadata"].(map[string]interface{})["annotations"] = map[string]interface{}{"platform.mujak.com/cert-not-after": "2026-11-03T12:00:00Z"}
				wantStatus = map[string]interface{}{"ready": true, "health": "Healthy", "daysRemaining": float64(30), "notAfter": "2026-11-03T12:00:00Z"}
			}
			if diff := cmp.Diff(wantIngress, ingress); diff != "" {
				t.Fatal(diff)
			}
			gotStatus, _, _ := unstructured.NestedMap(rsp.Desired.Composite.Resource.AsMap(), "status")
			if diff := cmp.Diff(map[string]interface{}{"other": "preserved", "certificate": wantStatus}, gotStatus); diff != "" {
				t.Fatal(diff)
			}
			if rsp.Desired.Resources["certificate"].Ready != fnv1.Ready_READY_UNSPECIFIED {
				t.Fatal("must not add auto-ready behavior")
			}
		}
	}
}

func TestMissingSpecIsFatal(t *testing.T) {
	rsp, err := (&Function{}).RunFunction(context.Background(), &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"spec":{}}`)}},
	})
	if err != nil || len(rsp.Results) != 1 || rsp.Results[0].Severity != fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("err=%v results=%v", err, rsp.Results)
	}
}
