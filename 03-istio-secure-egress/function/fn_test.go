package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/proto"
)

const xrJSON = `{"apiVersion":"platform.mujak.com/v1alpha1","kind":"XIstioSecureEgress","metadata":{"name":"istio-secure-egress-demo","namespace":"team-a"},"spec":{"hostname":"api.example.com","gatewayProfile":"east-west"}}`
const profileJSON = `{"apiVersion":"apiextensions.crossplane.io/v1beta1","kind":"EnvironmentConfig","metadata":{"name":"istio-egress-east-west"},"data":{"gateway":{"namespace":"istio-gateway-east-west","serviceFQDN":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local","selector":{"istio":"egressgateway"},"port":80}}}`

func requestWithProfile() *fnv1.RunFunctionRequest {
	return &fnv1.RunFunctionRequest{
		Meta:              &fnv1.RequestMeta{Tag: "test", Capabilities: []fnv1.Capability{fnv1.Capability_CAPABILITY_CAPABILITIES, fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES}},
		Observed:          &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(xrJSON)}},
		RequiredResources: map[string]*fnv1.Resources{"gateway-profile": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(profileJSON)}}}},
		Context:           resource.MustStructJSON(`{"pipeline":"preserved"}`),
		Desired: &fnv1.State{
			Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"status":{"other":"preserved"}}`), Ready: fnv1.Ready_READY_TRUE},
			Resources: map[string]*fnv1.Resource{"other": {Resource: resource.MustStructJSON(`{"kind":"ConfigMap"}`), Ready: fnv1.Ready_READY_TRUE, ConnectionDetails: map[string][]byte{"key": []byte("value")}}},
		},
	}
}

func run(t *testing.T, req *fnv1.RunFunctionRequest) *fnv1.RunFunctionResponse {
	t.Helper()
	before := proto.Clone(req).(*fnv1.RunFunctionRequest)
	rsp, err := (&Function{}).RunFunction(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(before, req) {
		t.Fatal("request mutated")
	}
	if !proto.Equal(req.Context, rsp.Context) || req.GetMeta().GetTag() != rsp.GetMeta().GetTag() {
		t.Fatal("lost context or tag")
	}
	if !proto.Equal(req.GetDesired().GetComposite(), rsp.GetDesired().GetComposite()) || !proto.Equal(req.GetDesired().GetResources()["other"], rsp.GetDesired().GetResources()["other"]) {
		t.Fatal("lost unrelated pipeline state")
	}
	return rsp
}

func TestInvalidInputsFailWithoutReplacingState(t *testing.T) {
	for _, tc := range []struct {
		name, old, value string
		profile          bool
	}{
		{"missing hostname", `"api.example.com"`, `""`, false},
		{"null hostname", `"api.example.com"`, `null`, false},
		{"typed hostname", `"api.example.com"`, `80`, false},
		{"wildcard", `"api.example.com"`, `"*.example.com"`, false},
		{"IP", `"api.example.com"`, `"192.0.2.1"`, false},
		{"IPv6", `"api.example.com"`, `"2001:db8::1"`, false},
		{"URL", `"api.example.com"`, `"https://api.example.com"`, false},
		{"port", `"api.example.com"`, `"api.example.com:443"`, false},
		{"long host", `"api.example.com"`, `"` + strings.Repeat("a", 64) + `.example.com"`, false},
		{"empty profile", `"east-west"`, `""`, false},
		{"invalid profile", `"east-west"`, `"east.west"`, false},
		{"long profile resource", `"east-west"`, `"` + strings.Repeat("a", 51) + `"`, false},
		{"no name", `"istio-secure-egress-demo"`, `""`, false},
		{"invalid name", `"istio-secure-egress-demo"`, `"Bad/Name"`, false},
		{"no namespace", `"team-a"`, `""`, false},
		{"invalid namespace", `"team-a"`, `"team.a"`, false},
		{"no gateway namespace", `"namespace":"istio-gateway-east-west"`, `"namespace":""`, true},
		{"invalid gateway namespace", `"namespace":"istio-gateway-east-west"`, `"namespace":"bad.ns"`, true},
		{"wrong service namespace", `.istio-gateway-east-west.svc.cluster.local`, `.other.svc.cluster.local`, true},
		{"short service name", `istio-egressgateway.istio-gateway-east-west.svc.cluster.local`, `istio-egressgateway`, true},
		{"non-service DNS", `.svc.cluster.local`, `.example.com`, true},
		{"invalid suffix", `.cluster.local`, `.bad..suffix`, true},
		{"no selector", `{"istio":"egressgateway"}`, `{}`, true},
		{"bad selector key", `"istio":"egressgateway"`, `"bad key":"egressgateway"`, true},
		{"bad selector value", `"istio":"egressgateway"`, `"istio":"bad/value"`, true},
		{"typed selector", `"istio":"egressgateway"`, `"istio":80`, true},
		{"null selector value", `"istio":"egressgateway"`, `"istio":null`, true},
		{"zero port", `"port":80`, `"port":0`, true},
		{"large port", `"port":80`, `"port":65536`, true},
		{"overflow port", `"port":80`, `"port":4294967376`, true},
		{"fractional port", `"port":80`, `"port":80.5`, true},
		{"string port", `"port":80`, `"port":"80"`, true},
		{"no gateway data", `"gateway":`, `"unknown":`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := requestWithProfile()
			if tc.profile {
				req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, tc.old, tc.value, 1))
			} else {
				req.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, tc.old, tc.value, 1))
			}
			rsp := run(t, req)
			if len(rsp.Results) != 1 || rsp.Results[0].Severity != fnv1.Severity_SEVERITY_FATAL || !proto.Equal(req.Desired, rsp.Desired) {
				t.Fatalf("expected fatal with desired preserved: %v", rsp)
			}
		})
	}
}

func TestEmptySelectorLabelValueAccepted(t *testing.T) {
	req := requestWithProfile()
	req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"egressgateway"`, `""`, 1))
	rsp := run(t, req)
	if len(rsp.Results) != 0 {
		t.Fatalf("empty string is a valid label value: %v", rsp.Results)
	}
	selector := rsp.Desired.Resources["gateway"].Resource.AsMap()["spec"].(map[string]any)["selector"]
	if !reflect.DeepEqual(selector, map[string]any{"istio": ""}) {
		t.Fatalf("empty string label value changed: %v", selector)
	}
}

func TestExactProfileHandshake(t *testing.T) {
	req := requestWithProfile()
	req.RequiredResources = nil
	rsp := run(t, req)
	want := &fnv1.Requirements{Resources: map[string]*fnv1.ResourceSelector{"gateway-profile": {ApiVersion: "apiextensions.crossplane.io/v1beta1", Kind: "EnvironmentConfig", Match: &fnv1.ResourceSelector_MatchName{MatchName: "istio-egress-east-west"}}}}
	if len(rsp.Results) != 0 || !proto.Equal(req.Desired, rsp.Desired) || !proto.Equal(want, rsp.Requirements) {
		t.Fatalf("initial fetch: %v", rsp)
	}
	req.RequiredResources = requestWithProfile().RequiredResources
	next := run(t, req)
	if len(next.Results) != 0 || !proto.Equal(rsp.Requirements, next.Requirements) || len(next.Desired.Resources) != 5 {
		t.Fatalf("returned profile: %v", next)
	}
	// Pre-advertisement Crossplane is not evidence that required resources are unsupported.
	req.Meta.Capabilities = nil
	if rsp := run(t, req); len(rsp.Results) != 0 {
		t.Fatalf("legacy capabilities: %v", rsp.Results)
	}
	for _, tc := range []struct {
		name   string
		change func(*fnv1.RunFunctionRequest)
		want   string
	}{
		{"unsupported", func(r *fnv1.RunFunctionRequest) {
			r.Meta.Capabilities = []fnv1.Capability{fnv1.Capability_CAPABILITY_CAPABILITIES}
		}, "required-resources"},
		{"missing", func(r *fnv1.RunFunctionRequest) { r.RequiredResources["gateway-profile"] = &fnv1.Resources{} }, "not found"},
		{"deleting", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"name":"istio-egress-east-west"`, `"name":"istio-egress-east-west","deletionTimestamp":"2026-01-01T00:00:00Z"`, 1))
		}, "deleting"},
		{"wrong profile", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"istio-egress-east-west"`, `"istio-egress-other"`, 1))
		}, "identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := requestWithProfile()
			tc.change(r)
			rsp := run(t, r)
			if len(rsp.Results) != 1 || rsp.Results[0].Severity != fnv1.Severity_SEVERITY_FATAL || !strings.Contains(rsp.Results[0].Message, tc.want) || !proto.Equal(r.Desired, rsp.Desired) {
				t.Fatalf("want fatal %q without state changes: %v", tc.want, rsp)
			}
		})
	}
}

func TestExternalProfileProducesApplicationRoute(t *testing.T) {
	rsp := run(t, requestWithProfile())
	if len(rsp.Results) != 0 || len(rsp.GetDesired().GetResources()) != 5 {
		t.Fatalf("expected four composed objects plus existing state: %v", rsp)
	}
	wantSpecs := map[string]string{
		"service-entry": `{"hosts":["api.example.com"],"location":"MESH_EXTERNAL","resolution":"DNS","exportTo":[".","istio-gateway-east-west"],"ports":[{"number":80,"name":"http","protocol":"HTTP"},{"number":443,"name":"https","protocol":"HTTPS"}]}`,
		"gateway":       `{"selector":{"istio":"egressgateway"},"servers":[{"port":{"number":80,"name":"https-egress","protocol":"HTTPS"},"hosts":["./api.example.com"],"tls":{"mode":"ISTIO_MUTUAL"}}]}`,
	}
	kinds := map[string]string{"service-entry": "ServiceEntry", "gateway": "Gateway", "gateway-destination-rule": "DestinationRule", "virtual-service": "VirtualService"}
	for key, kind := range kinds {
		obj := rsp.Desired.Resources[key].Resource.AsMap()
		if obj["apiVersion"] != "networking.istio.io/v1" || obj["kind"] != kind || !reflect.DeepEqual(obj["metadata"], map[string]any{"name": "istio-secure-egress-demo", "namespace": "team-a"}) {
			t.Fatalf("wrong identity for %s: %v", key, obj)
		}
		if spec, ok := wantSpecs[key]; ok && !reflect.DeepEqual(obj["spec"], resource.MustStructJSON(spec).AsMap()) {
			t.Errorf("%s spec = %v", key, obj["spec"])
		}
	}
	dr := rsp.Desired.Resources["gateway-destination-rule"].Resource.AsMap()["spec"].(map[string]any)
	subsets := dr["subsets"].([]any)
	subset := subsets[0].(map[string]any)["name"].(string)
	if !strings.HasPrefix(subset, "egress-") || len(subset) > 63 {
		t.Fatalf("invalid subset: %s", subset)
	}
	wantDR := resource.MustStructJSON(`{"host":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local","exportTo":["."],"subsets":[{"name":"` + subset + `","trafficPolicy":{"portLevelSettings":[{"port":{"number":80},"tls":{"mode":"ISTIO_MUTUAL","sni":"api.example.com"}}]}}]}`).AsMap()
	if !reflect.DeepEqual(dr, wantDR) {
		t.Fatalf("destination rule = %v", dr)
	}
	wantVS := resource.MustStructJSON(`{"hosts":["api.example.com"],"exportTo":[".","istio-gateway-east-west"],"gateways":["mesh","team-a/istio-secure-egress-demo"],"http":[{"match":[{"gateways":["mesh"],"port":80}],"route":[{"destination":{"host":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local","subset":"` + subset + `","port":{"number":80}}}]},{"match":[{"gateways":["team-a/istio-secure-egress-demo"],"port":80}],"route":[{"destination":{"host":"api.example.com","port":{"number":443}}}]}]}`).AsMap()
	if !reflect.DeepEqual(rsp.Desired.Resources["virtual-service"].Resource.AsMap()["spec"], wantVS) {
		t.Fatalf("virtual service = %v", rsp.Desired.Resources["virtual-service"])
	}
}

func TestIdentityStabilityAndResourceProtocolPreservation(t *testing.T) {
	seen := map[string]bool{}
	for _, identity := range []struct{ name, namespace string }{
		{"same", "team-a"}, {"same", "team-b"}, {strings.Repeat("a", 63) + "." + strings.Repeat("b", 63), "team-a"},
	} {
		req := requestWithProfile()
		req.Observed.Composite.Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(xrJSON, "istio-secure-egress-demo", identity.name), "team-a", identity.namespace))
		previous := &fnv1.Resource{Resource: resource.MustStructJSON(`{"kind":"Old"}`), ConnectionDetails: map[string][]byte{"preserved": []byte("yes")}, Ready: fnv1.Ready_READY_TRUE}
		previous.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
		req.Desired.Resources["gateway-destination-rule"] = previous
		rsp := run(t, req)
		if len(rsp.Results) != 0 {
			t.Fatal(rsp.Results)
		}
		dr := rsp.Desired.Resources["gateway-destination-rule"]
		if !reflect.DeepEqual(previous.ConnectionDetails, dr.ConnectionDetails) || !reflect.DeepEqual(previous.ProtoReflect().GetUnknown(), dr.ProtoReflect().GetUnknown()) {
			t.Fatal("lost owned-resource protocol fields")
		}
		for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
			if rsp.Desired.Resources[key].Ready != fnv1.Ready_READY_UNSPECIFIED {
				t.Fatal("readiness must remain unspecified until #24")
			}
		}
		subset := dr.Resource.AsMap()["spec"].(map[string]any)["subsets"].([]any)[0].(map[string]any)["name"].(string)
		if seen[subset] {
			t.Fatalf("subset collision for %v", identity)
		}
		seen[subset] = true
		next := run(t, req)
		if !proto.Equal(rsp.Desired, next.Desired) || !proto.Equal(rsp.Requirements, next.Requirements) {
			t.Fatal("non-deterministic output")
		}
	}
}

func TestAlternateProfileSupportsNondefaultServiceDNSSuffix(t *testing.T) {
	req := requestWithProfile()
	req.Observed.Composite.Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(xrJSON, "east-west", "regional"), "team-a", "team-b"))
	req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(`{"apiVersion":"apiextensions.crossplane.io/v1beta1","kind":"EnvironmentConfig","metadata":{"name":"istio-egress-regional"},"data":{"gateway":{"namespace":"gateway-regional","serviceFQDN":"egress.gateway-regional.svc.corp.example","selector":{"app":"regional-egress"},"port":8443}}}`)
	rsp := run(t, req)
	if len(rsp.Results) != 0 {
		t.Fatal(rsp.Results)
	}
	if rsp.Requirements.Resources["gateway-profile"].GetMatchName() != "istio-egress-regional" {
		t.Fatal("wrong profile selected")
	}
	for _, key := range []string{"gateway", "service-entry", "gateway-destination-rule", "virtual-service"} {
		if rsp.Desired.Resources[key].Resource.AsMap()["metadata"].(map[string]any)["namespace"] != "team-b" {
			t.Fatal("cross-namespace composition")
		}
	}
	g := rsp.Desired.Resources["gateway"].Resource.AsMap()["spec"].(map[string]any)
	if !reflect.DeepEqual(g["selector"], map[string]any{"app": "regional-egress"}) || g["servers"].([]any)[0].(map[string]any)["port"].(map[string]any)["number"] != float64(8443) {
		t.Fatalf("wrong gateway settings: %v", g)
	}
	dr := rsp.Desired.Resources["gateway-destination-rule"].Resource.AsMap()["spec"].(map[string]any)
	if dr["host"] != "egress.gateway-regional.svc.corp.example" {
		t.Fatalf("wrong service: %v", dr)
	}
	for _, key := range []string{"service-entry", "virtual-service"} {
		spec := rsp.Desired.Resources[key].Resource.AsMap()["spec"].(map[string]any)
		if !reflect.DeepEqual(spec["exportTo"], []any{".", "gateway-regional"}) {
			t.Fatalf("wrong visibility: %v", spec)
		}
	}
}
