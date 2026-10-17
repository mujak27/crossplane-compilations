package main

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/util/validation"
)

const xrJSON = `{"apiVersion":"platform.mujak.com/v1alpha1","kind":"XIstioSecureEgress","metadata":{"name":"istio-secure-egress-demo","namespace":"team-a"},"spec":{"hostname":"api.example.com","gatewayProfile":"east-west"}}`
const profileJSON = `{"apiVersion":"apiextensions.crossplane.io/v1beta1","kind":"EnvironmentConfig","metadata":{"name":"istio-egress-east-west"},"data":{"gateway":{"namespace":"istio-gateway-east-west","serviceFQDN":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local","selector":{"istio":"egressgateway"},"port":80}}}`

func requestWithProfile() *fnv1.RunFunctionRequest {
	req := &fnv1.RunFunctionRequest{
		Meta:              &fnv1.RequestMeta{Tag: "test", Capabilities: []fnv1.Capability{fnv1.Capability_CAPABILITY_CAPABILITIES, fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES}},
		Observed:          &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(xrJSON)}},
		RequiredResources: map[string]*fnv1.Resources{"gateway-profile": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(profileJSON)}}}},
		Context:           resource.MustStructJSON(`{"pipeline":"preserved"}`),
		Desired: &fnv1.State{
			Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"status":{"other":"preserved"}}`), Ready: fnv1.Ready_READY_TRUE},
			Resources: map[string]*fnv1.Resource{"other": {Resource: resource.MustStructJSON(`{"kind":"ConfigMap"}`), Ready: fnv1.Ready_READY_TRUE, ConnectionDetails: map[string][]byte{"key": []byte("value")}}},
		},
	}
	for _, message := range []proto.Message{req.Desired, req.Desired.Composite, req.Desired.Resources["other"]} {
		message.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
	}
	for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
		req.Desired.Resources[key] = proto.Clone(req.Desired.Resources["other"]).(*fnv1.Resource)
	}
	return req
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
	if !reflect.DeepEqual(req.GetDesired().ProtoReflect().GetUnknown(), rsp.GetDesired().ProtoReflect().GetUnknown()) {
		t.Fatal("lost desired-state protocol fields")
	}
	for key, previous := range req.Desired.Resources {
		current := rsp.Desired.Resources[key]
		if current == nil {
			t.Fatalf("lost desired resource %s", key)
		}
		protocol := proto.Clone(current).(*fnv1.Resource)
		protocol.Resource, protocol.Ready = previous.Resource, previous.Ready
		if !proto.Equal(previous, protocol) {
			t.Fatalf("lost %s protocol fields", key)
		}
	}
	return rsp
}

func TestInvalidInputsFailWithoutReplacingState(t *testing.T) {
	for _, tc := range []struct {
		name, old, value string
		profile          bool
	}{
		{"missing hostname", `"api.example.com"`, `""`, false},
		{"absent spec", `"spec":`, `"unknown":`, false},
		{"null spec", `{"hostname":"api.example.com","gatewayProfile":"east-west"}`, `null`, false},
		{"absent hostname", `"hostname":`, `"unknown":`, false},
		{"null hostname", `"api.example.com"`, `null`, false},
		{"typed hostname", `"api.example.com"`, `80`, false},
		{"wildcard", `"api.example.com"`, `"*.example.com"`, false},
		{"IP", `"api.example.com"`, `"192.0.2.1"`, false},
		{"IPv6", `"api.example.com"`, `"2001:db8::1"`, false},
		{"URL", `"api.example.com"`, `"https://api.example.com"`, false},
		{"port", `"api.example.com"`, `"api.example.com:443"`, false},
		{"long host", `"api.example.com"`, `"` + strings.Repeat("a", 64) + `.example.com"`, false},
		{"empty profile", `"east-west"`, `""`, false},
		{"absent profile", `"gatewayProfile":`, `"unknown":`, false},
		{"null profile", `"east-west"`, `null`, false},
		{"typed profile", `"east-west"`, `true`, false},
		{"invalid profile", `"east-west"`, `"east.west"`, false},
		{"long profile resource", `"east-west"`, `"` + strings.Repeat("a", 51) + `"`, false},
		{"no name", `"istio-secure-egress-demo"`, `""`, false},
		{"absent metadata", `"metadata":`, `"unknown":`, false},
		{"typed metadata", `{"name":"istio-secure-egress-demo","namespace":"team-a"}`, `[]`, false},
		{"invalid name", `"istio-secure-egress-demo"`, `"Bad/Name"`, false},
		{"no namespace", `"team-a"`, `""`, false},
		{"invalid namespace", `"team-a"`, `"team.a"`, false},
		{"long namespace", `"team-a"`, `"` + strings.Repeat("n", 64) + `"`, false},
		{"absent data", `"data":`, `"unknown":`, true},
		{"null data", `"data":{"gateway":`, `"data":null,"unknown":{"gateway":`, true},
		{"scalar data", `"data":{"gateway":`, `"data":42,"unknown":{"gateway":`, true},
		{"null gateway", `"gateway":`, `"gateway":null,"unknown":`, true},
		{"typed gateway", `"gateway":`, `"gateway":[],"unknown":`, true},
		{"no gateway namespace", `"namespace":"istio-gateway-east-west"`, `"namespace":""`, true},
		{"absent gateway namespace", `"namespace":"istio-gateway-east-west",`, ``, true},
		{"typed gateway namespace", `"namespace":"istio-gateway-east-west"`, `"namespace":80`, true},
		{"invalid gateway namespace", `"namespace":"istio-gateway-east-west"`, `"namespace":"bad.ns"`, true},
		{"wrong service namespace", `.istio-gateway-east-west.svc.cluster.local`, `.other.svc.cluster.local`, true},
		{"short service name", `istio-egressgateway.istio-gateway-east-west.svc.cluster.local`, `istio-egressgateway`, true},
		{"non-service DNS", `.svc.cluster.local`, `.example.com`, true},
		{"invalid suffix", `.cluster.local`, `.bad..suffix`, true},
		{"absent service", `"serviceFQDN":`, `"unknown":`, true},
		{"null service", `"serviceFQDN":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local"`, `"serviceFQDN":null`, true},
		{"typed service", `"serviceFQDN":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local"`, `"serviceFQDN":80`, true},
		{"invalid service label", `istio-egressgateway.`, `1-egress.`, true},
		{"no selector", `{"istio":"egressgateway"}`, `{}`, true},
		{"null selector", `{"istio":"egressgateway"}`, `null`, true},
		{"list selector", `{"istio":"egressgateway"}`, `[]`, true},
		{"string selector", `{"istio":"egressgateway"}`, `"istio=egressgateway"`, true},
		{"bad selector key", `"istio":"egressgateway"`, `"bad key":"egressgateway"`, true},
		{"bad selector value", `"istio":"egressgateway"`, `"istio":"bad/value"`, true},
		{"typed selector", `"istio":"egressgateway"`, `"istio":80`, true},
		{"null selector value", `"istio":"egressgateway"`, `"istio":null`, true},
		{"boolean selector value", `"istio":"egressgateway"`, `"istio":true`, true},
		{"object selector value", `"istio":"egressgateway"`, `"istio":{}`, true},
		{"absent port", `"port":80`, `"unknown":80`, true},
		{"null port", `"port":80`, `"port":null`, true},
		{"negative port", `"port":80`, `"port":-1`, true},
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

func TestExportsUseExplicitDeduplicatedNamespaces(t *testing.T) {
	for _, namespace := range []string{"team-a", "istio-gateway-east-west"} {
		t.Run(namespace, func(t *testing.T) {
			req := requestWithProfile()
			req.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, "team-a", namespace, 1))
			rsp := run(t, req)
			want := []any{namespace}
			if namespace == "team-a" {
				want = []any{"team-a", "istio-gateway-east-west"}
			}
			for _, key := range []string{"service-entry", "virtual-service"} {
				spec := rsp.Desired.Resources[key].Resource.AsMap()["spec"].(map[string]any)
				if !reflect.DeepEqual(spec["exportTo"], want) {
					t.Fatalf("%s exports = %v, want %v", key, spec["exportTo"], want)
				}
			}
		})
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
	// Unrelated results neither satisfy the exact lookup nor change its requirements.
	req.RequiredResources = map[string]*fnv1.Resources{"unrelated-profile": {Items: []*fnv1.Resource{nil}}}
	if pending := run(t, req); len(pending.Results) != 0 || !proto.Equal(req.Desired, pending.Desired) || !proto.Equal(want, pending.Requirements) {
		t.Fatalf("pending exact lookup: %v", pending)
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
		{"no observed state", func(r *fnv1.RunFunctionRequest) { r.Observed = nil }, "composite"},
		{"no observed composite", func(r *fnv1.RunFunctionRequest) { r.Observed.Composite = nil }, "composite"},
		{"no observed resource", func(r *fnv1.RunFunctionRequest) { r.Observed.Composite.Resource = nil }, "metadata"},
		{"unsupported", func(r *fnv1.RunFunctionRequest) {
			r.Meta.Capabilities = []fnv1.Capability{fnv1.Capability_CAPABILITY_CAPABILITIES}
		}, "required-resources"},
		{"missing", func(r *fnv1.RunFunctionRequest) { r.RequiredResources["gateway-profile"] = &fnv1.Resources{} }, "not found"},
		{"nil lookup result", func(r *fnv1.RunFunctionRequest) { r.RequiredResources["gateway-profile"] = nil }, "not found"},
		{"duplicate profile", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items = append(r.RequiredResources["gateway-profile"].Items, r.RequiredResources["gateway-profile"].Items[0])
		}, "expected one"},
		{"nil profile", func(r *fnv1.RunFunctionRequest) { r.RequiredResources["gateway-profile"].Items[0] = nil }, "identity"},
		{"no profile object", func(r *fnv1.RunFunctionRequest) { r.RequiredResources["gateway-profile"].Items[0].Resource = nil }, "identity"},
		{"deleting", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"name":"istio-egress-east-west"`, `"name":"istio-egress-east-west","deletionTimestamp":"2026-01-01T00:00:00Z"`, 1))
		}, "deleting"},
		{"wrong profile", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"istio-egress-east-west"`, `"istio-egress-other"`, 1))
		}, "identity"},
		{"wrong API", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, "apiextensions.crossplane.io/v1beta1", "other.example.org/v1", 1))
		}, "identity"},
		{"wrong kind", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, "EnvironmentConfig", "ConfigMap", 1))
		}, "identity"},
		{"namespaced profile", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"metadata":{`, `"metadata":{"namespace":"team-a",`, 1))
		}, "identity"},
		{"typed identity", func(r *fnv1.RunFunctionRequest) {
			r.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, `"name":"istio-egress-east-west"`, `"name":42`, 1))
		}, "decode profile identity"},
		{"reselection with previous lookup", func(r *fnv1.RunFunctionRequest) {
			r.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, "east-west", "regional", 1))
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
		"service-entry": `{"hosts":["api.example.com"],"location":"MESH_EXTERNAL","resolution":"DNS","exportTo":["team-a","istio-gateway-east-west"],"ports":[{"number":80,"name":"http","protocol":"HTTP"},{"number":443,"name":"https","protocol":"HTTPS"}]}`,
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
	wantVS := resource.MustStructJSON(`{"hosts":["api.example.com"],"exportTo":["team-a","istio-gateway-east-west"],"gateways":["mesh","team-a/istio-secure-egress-demo"],"http":[{"match":[{"gateways":["mesh"],"port":80}],"route":[{"destination":{"host":"istio-egressgateway.istio-gateway-east-west.svc.cluster.local","subset":"` + subset + `","port":{"number":80}}}]},{"match":[{"gateways":["team-a/istio-secure-egress-demo"],"port":80}],"route":[{"destination":{"host":"api.example.com","port":{"number":443}}}]}]}`).AsMap()
	if !reflect.DeepEqual(rsp.Desired.Resources["virtual-service"].Resource.AsMap()["spec"], wantVS) {
		t.Fatalf("virtual service = %v", rsp.Desired.Resources["virtual-service"])
	}
}

func TestIdentityStabilityAndResourceProtocolPreservation(t *testing.T) {
	seen := map[string]bool{}
	for _, identity := range []struct{ name, namespace string }{
		{"same", "team-a"}, {"same", "team-b"}, {strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61), strings.Repeat("n", 63)},
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
			if rsp.Desired.Resources[key].Ready != fnv1.Ready_READY_FALSE {
				t.Fatal("missing observations must remain unready")
			}
		}
		subset := dr.Resource.AsMap()["spec"].(map[string]any)["subsets"].([]any)[0].(map[string]any)["name"].(string)
		if seen[subset] {
			t.Fatalf("subset collision for %v", identity)
		}
		if len(validation.IsDNS1123Label(subset)) != 0 {
			t.Fatalf("subset is not DNS-label safe: %s", subset)
		}
		for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
			if !reflect.DeepEqual(rsp.Desired.Resources[key].Resource.AsMap()["metadata"], map[string]any{"name": identity.name, "namespace": identity.namespace}) {
				t.Fatalf("%s identity was truncated or moved", key)
			}
		}
		seen[subset] = true
		next := run(t, req)
		if !proto.Equal(rsp.Desired, next.Desired) || !proto.Equal(rsp.Requirements, next.Requirements) {
			t.Fatal("non-deterministic output")
		}
	}
}

func TestObservedConfigurationReadiness(t *testing.T) {
	req := requestWithProfile()
	pending := run(t, req)
	for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
		if pending.Desired.Resources[key].Ready != fnv1.Ready_READY_FALSE {
			t.Fatalf("%s missing observation must be unready", key)
		}
	}
	req.Observed.Resources = proto.Clone(pending.Desired).(*fnv1.State).Resources
	matching := run(t, req)
	if len(matching.Results) != 0 || len(matching.Conditions) != 0 {
		t.Fatalf("configuration readiness must not invent health conditions: %v", matching)
	}
	for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
		if matching.Desired.Resources[key].Ready != fnv1.Ready_READY_TRUE {
			t.Fatalf("%s matching observation must be ready", key)
		}
	}
}

func TestObservedDriftIsUnreadyPerResource(t *testing.T) {
	for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
		for _, tc := range []struct{ name, old, replacement string }{
			{"missing", "", ""},
			{"nil resource", "", ""},
			{"wrong version", `networking.istio.io/v1`, `networking.istio.io/v1beta1`},
			{"wrong kind", `"kind":`, `"unknown":`},
			{"wrong name", `"name":"istio-secure-egress-demo"`, `"name":"other"`},
			{"wrong namespace", `"namespace":"team-a"`, `"namespace":"team-b"`},
			{"missing namespace", `"namespace":"team-a"`, `"unknown":"team-a"`},
			{"deleting", `"metadata":{`, `"metadata":{"deletionTimestamp":"2026-10-17T00:00:00Z",`},
			{"missing spec", `"spec":`, `"unknown":`},
			{"null spec", `"spec":{`, `"spec":null,"unknown":{`},
			{"injected policy", `"spec":{`, `"spec":{"trafficPolicy":{"tls":{"mode":"DISABLE"}},`},
		} {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				req := requestWithProfile()
				req.Observed.Resources = run(t, req).Desired.Resources
				encoded, err := json.Marshal(req.Observed.Resources[key].Resource.AsMap())
				if err != nil {
					t.Fatal(err)
				}
				switch tc.name {
				case "missing":
					delete(req.Observed.Resources, key)
				case "nil resource":
					req.Observed.Resources[key] = nil
				default:
					if !strings.Contains(string(encoded), tc.old) {
						t.Fatal("mutation did not apply")
					}
					req.Observed.Resources[key].Resource = resource.MustStructJSON(strings.Replace(string(encoded), tc.old, tc.replacement, 1))
				}
				rsp := run(t, req)
				for _, other := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
					want := fnv1.Ready_READY_TRUE
					if other == key {
						want = fnv1.Ready_READY_FALSE
					}
					if rsp.Desired.Resources[other].Ready != want {
						t.Fatalf("%s readiness = %v, want %v", other, rsp.Desired.Resources[other].Ready, want)
					}
				}
			})
		}
	}
}

func TestRoutingAndSecurityDriftIsUnready(t *testing.T) {
	for _, tc := range []struct{ key, old, replacement string }{
		{"service-entry", `"DNS"`, `"STATIC"`},
		{"service-entry", `"MESH_EXTERNAL"`, `"MESH_INTERNAL"`},
		{"service-entry", `"HTTP"`, `"TCP"`},
		{"service-entry", `"number":443`, `"number":8443`},
		{"service-entry", `"number":80`, `"targetPort":8080,"number":80`},
		{"service-entry", `"team-a","istio-gateway-east-west"`, `"*"`},
		{"service-entry", `"hosts":[`, `"addresses":["192.0.2.1"],"hosts":[`},
		{"service-entry", `"hosts":[`, `"endpoints":[{"address":"192.0.2.1"}],"hosts":[`},
		{"gateway", `"egressgateway"`, `"other"`},
		{"gateway", `"selector":{`, `"selector":{"extra":"restriction",`},
		{"gateway", `"./api.example.com"`, `"*/api.example.com"`},
		{"gateway", `"HTTPS"`, `"HTTP"`},
		{"gateway", `"number":80`, `"number":8443`},
		{"gateway", `"ISTIO_MUTUAL"`, `"SIMPLE"`},
		{"gateway", `"tls":{`, `"tls":{"subjectAltNames":["other"],`},
		{"gateway", `"tls":{`, `"tls":{"httpsRedirect":true,`},
		{"gateway-destination-rule", `"ISTIO_MUTUAL"`, `"DISABLE"`},
		{"gateway-destination-rule", `"sni":"api.example.com"`, `"sni":"other.example.com"`},
		{"gateway-destination-rule", `"number":80`, `"number":8443`},
		{"gateway-destination-rule", `"subsets":[{`, `"subsets":[{"labels":{"app":"other"},`},
		{"gateway-destination-rule", `"tls":{`, `"tls":{"insecureSkipVerify":true,`},
		{"gateway-destination-rule", `"exportTo":["."]`, `"exportTo":["*"]`},
		{"virtual-service", `"host":"api.example.com"`, `"host":"other.example.com"`},
		{"virtual-service", `"number":443`, `"number":80`},
		{"virtual-service", `"destination":{`, `"weight":50,"destination":{`},
		{"virtual-service", `"gateways":["mesh"`, `"gateways":["other"`},
		{"virtual-service", `"port":80`, `"port":8080`},
		{"virtual-service", `"http":[{`, `"http":[{"rewrite":{"authority":"other.example.com"},`},
		{"virtual-service", `"http":[{`, `"http":[{"headers":{"request":{"set":{"authorization":"injected"}}},`},
		{"virtual-service", `"team-a","istio-gateway-east-west"`, `"*"`},
	} {
		t.Run(tc.key+"/"+tc.replacement, func(t *testing.T) {
			req := requestWithProfile()
			req.Observed.Resources = run(t, req).Desired.Resources
			encoded, err := json.Marshal(req.Observed.Resources[tc.key].Resource.AsMap())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), tc.old) {
				t.Fatal("mutation did not apply")
			}
			req.Observed.Resources[tc.key].Resource = resource.MustStructJSON(strings.Replace(string(encoded), tc.old, tc.replacement, 1))
			rsp := run(t, req)
			if rsp.Desired.Resources[tc.key].Ready != fnv1.Ready_READY_FALSE {
				t.Fatalf("drifted %s is ready", tc.key)
			}
		})
	}
}

func TestProfileAndHostnameChangesResetObservedReadiness(t *testing.T) {
	for _, change := range []string{"profile update", "profile selection", "hostname"} {
		t.Run(change, func(t *testing.T) {
			req := requestWithProfile()
			req.Observed.Resources = run(t, req).Desired.Resources
			req.Desired = run(t, req).Desired
			for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
				if req.Desired.Resources[key].Ready != fnv1.Ready_READY_TRUE {
					t.Fatal("initial configuration not ready")
				}
			}
			if change == "hostname" {
				req.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, "api.example.com", "updated.example.com", 1))
			} else {
				profile := strings.ReplaceAll(strings.ReplaceAll(profileJSON, "istio-gateway-east-west", "gateway-regional"), `"port":80`, `"port":8443`)
				if change == "profile selection" {
					req.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, "east-west", "regional", 1))
					profile = strings.Replace(profile, "istio-egress-east-west", "istio-egress-regional", 1)
				}
				req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(profile)
			}
			pending := run(t, req)
			for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
				if pending.Desired.Resources[key].Ready != fnv1.Ready_READY_FALSE {
					t.Fatalf("%s stale configuration stayed ready", key)
				}
			}
			req.Observed.Resources = proto.Clone(pending.Desired).(*fnv1.State).Resources
			matching := run(t, req)
			for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
				if matching.Desired.Resources[key].Ready != fnv1.Ready_READY_TRUE {
					t.Fatalf("%s updated observation not ready", key)
				}
			}
		})
	}
}

func TestControllerDefaultsDoNotPreventConfigurationReadiness(t *testing.T) {
	req := requestWithProfile()
	req.Observed.Resources = run(t, req).Desired.Resources
	for key, observed := range req.Observed.Resources {
		if key == "other" {
			continue
		}
		encoded, err := json.Marshal(observed.Resource.AsMap())
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(encoded), `"metadata":{`, `"status":{"conditions":[{"type":"Ready","status":"False"}]},"metadata":{"uid":"controller-uid","resourceVersion":"42","generation":3,"deletionTimestamp":null,"labels":{"controller":"owned"},"annotations":{"controller":"owned"},"managedFields":[{"manager":"crossplane"}],`, 1)
		switch key {
		case "service-entry":
			text = strings.ReplaceAll(text, `"number":`, `"targetPort":0,"number":`)
		case "gateway":
			text = strings.Replace(text, `"tls":{`, `"tls":{"httpsRedirect":false,`, 1)
		case "gateway-destination-rule":
			text = strings.Replace(text, `"tls":{`, `"tls":{"insecureSkipVerify":false,`, 1)
		case "virtual-service":
			text = strings.ReplaceAll(text, `"destination":{`, `"weight":100,"destination":{`)
		}
		observed.Resource = resource.MustStructJSON(text)
	}
	rsp := run(t, req)
	for _, key := range []string{"service-entry", "gateway", "gateway-destination-rule", "virtual-service"} {
		if rsp.Desired.Resources[key].Ready != fnv1.Ready_READY_TRUE {
			t.Fatalf("%s harmless defaults/metadata must be ready", key)
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
		if !reflect.DeepEqual(spec["exportTo"], []any{"team-b", "gateway-regional"}) {
			t.Fatalf("wrong visibility: %v", spec)
		}
	}
}

func TestProfileUpdateAndReselectionReplaceAllManagedConfiguration(t *testing.T) {
	req := requestWithProfile()
	initial := run(t, req)
	subset := initial.Desired.Resources["gateway-destination-rule"].Resource.AsMap()["spec"].(map[string]any)["subsets"].([]any)[0].(map[string]any)["name"].(string)
	previous := initial
	for _, profileName := range []string{"east-west", "regional"} {
		// First edit the existing profile, then select a differently named profile.
		req.Desired = previous.Desired
		req.Observed.Composite.Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(xrJSON, "east-west", profileName), "api.example.com", "new.example.org"))
		req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(`{"apiVersion":"apiextensions.crossplane.io/v1beta1","kind":"EnvironmentConfig","metadata":{"name":"istio-egress-` + profileName + `"},"data":{"gateway":{"namespace":"gateway-regional","serviceFQDN":"egress.gateway-regional.svc.corp.example","selector":{"app":"regional-egress"},"port":8443}}}`)
		if profileName == "regional" {
			// The newly selected profile points elsewhere, not at the previous gateway.
			req.Observed.Composite.Resource = resource.MustStructJSON(strings.Replace(xrJSON, "east-west", "regional", 1))
			req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.Replace(profileJSON, "istio-egress-east-west", "istio-egress-regional", 1))
		}
		rsp := run(t, req)
		if len(rsp.Results) != 0 || len(rsp.Desired.Resources) != 5 {
			t.Fatalf("profile %s: %v", profileName, rsp)
		}
		wantRequirements := &fnv1.Requirements{Resources: map[string]*fnv1.ResourceSelector{"gateway-profile": {ApiVersion: "apiextensions.crossplane.io/v1beta1", Kind: "EnvironmentConfig", Match: &fnv1.ResourceSelector_MatchName{MatchName: "istio-egress-" + profileName}}}}
		if !proto.Equal(rsp.Requirements, wantRequirements) {
			t.Fatalf("wrong exact lookup: %v", rsp.Requirements)
		}
		if profileName == "regional" {
			if !proto.Equal(rsp.Desired, initial.Desired) {
				t.Fatalf("reselection left previous gateway references or changed identities: %v", rsp.Desired)
			}
			continue
		}
		wantSpecs := map[string]string{
			"service-entry":            `{"hosts":["new.example.org"],"location":"MESH_EXTERNAL","resolution":"DNS","exportTo":["team-a","gateway-regional"],"ports":[{"number":80,"name":"http","protocol":"HTTP"},{"number":443,"name":"https","protocol":"HTTPS"}]}`,
			"gateway":                  `{"selector":{"app":"regional-egress"},"servers":[{"port":{"number":8443,"name":"https-egress","protocol":"HTTPS"},"hosts":["./new.example.org"],"tls":{"mode":"ISTIO_MUTUAL"}}]}`,
			"gateway-destination-rule": `{"host":"egress.gateway-regional.svc.corp.example","exportTo":["."],"subsets":[{"name":"` + subset + `","trafficPolicy":{"portLevelSettings":[{"port":{"number":8443},"tls":{"mode":"ISTIO_MUTUAL","sni":"new.example.org"}}]}}]}`,
			"virtual-service":          `{"hosts":["new.example.org"],"exportTo":["team-a","gateway-regional"],"gateways":["mesh","team-a/istio-secure-egress-demo"],"http":[{"match":[{"gateways":["mesh"],"port":80}],"route":[{"destination":{"host":"egress.gateway-regional.svc.corp.example","subset":"` + subset + `","port":{"number":8443}}}]},{"match":[{"gateways":["team-a/istio-secure-egress-demo"],"port":8443}],"route":[{"destination":{"host":"new.example.org","port":{"number":443}}}]}]}`,
		}
		for key, spec := range wantSpecs {
			obj := rsp.Desired.Resources[key].Resource.AsMap()
			if !reflect.DeepEqual(obj["spec"], resource.MustStructJSON(spec).AsMap()) {
				t.Fatalf("profile %s left stale %s configuration: %v", profileName, key, obj)
			}
			if !reflect.DeepEqual(obj["metadata"], initial.Desired.Resources[key].Resource.AsMap()["metadata"]) {
				t.Fatalf("profile change churned %s identity", key)
			}
		}
		next := run(t, req)
		if !proto.Equal(rsp.Desired, next.Desired) || !proto.Equal(rsp.Requirements, next.Requirements) {
			t.Fatal("profile reconciliation is not deterministic")
		}
		previous = rsp
	}
}

func TestValidContractBoundaries(t *testing.T) {
	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			req := requestWithProfile()
			host := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
			profile := strings.Repeat("p", 50)
			req.Observed.Composite.Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(xrJSON, "api.example.com", host), "east-west", profile))
			req.RequiredResources["gateway-profile"].Items[0].Resource = resource.MustStructJSON(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(profileJSON, "istio-egress-east-west", "istio-egress-"+profile), `"port":80`, `"port":`+port), `"istio":"egressgateway"`, `"example.org/gateway":""`))
			rsp := run(t, req)
			if len(rsp.Results) != 0 || rsp.Requirements.Resources["gateway-profile"].GetMatchName() != "istio-egress-"+profile {
				t.Fatalf("valid hostname/profile/selector/port boundary rejected: %v", rsp)
			}
		})
	}
}
