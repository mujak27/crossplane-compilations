package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/mujak27/function-istio-secure-egress/api/v1alpha1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Function renders application routing, not platform TLS policy or application health.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}

// GatewayProfileData is the platform-owned EnvironmentConfig data contract.
type GatewayProfileData struct {
	Gateway GatewayProfile `json:"gateway"`
}

// GatewayProfile describes the exposed Service and its gateway pods.
type GatewayProfile struct {
	Namespace   string            `json:"namespace"`
	ServiceFQDN string            `json:"serviceFQDN"`
	Selector    map[string]string `json:"selector"`
	Port        int32             `json:"port"`
}

func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)
	if req.Desired != nil {
		rsp.Desired = proto.Clone(req.Desired).(*fnv1.State)
	}
	if err := f.compose(req, rsp); err != nil {
		response.Fatal(rsp, err)
	}
	return rsp, nil
}

func (f *Function) compose(req *fnv1.RunFunctionRequest, rsp *fnv1.RunFunctionResponse) error {
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		return fmt.Errorf("get observed composite: %w", err)
	}
	var composite v1alpha1.XIstioSecureEgress
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(xr.Resource.Object, &composite); err != nil {
		return fmt.Errorf("decode composite: %w", err)
	}
	if composite.Name == "" || len(validation.IsDNS1123Subdomain(composite.Name)) != 0 || composite.Namespace == "" || len(validation.IsDNS1123Label(composite.Namespace)) != 0 {
		return fmt.Errorf("composite metadata.name and metadata.namespace must be valid nonempty Kubernetes names")
	}
	if !validHostname(composite.Spec.Hostname) {
		return fmt.Errorf("spec.hostname must be one exact DNS hostname, not an IP, URL, wildcard or host:port")
	}
	if len(validation.IsDNS1123Label(composite.Spec.GatewayProfile)) != 0 || len("istio-egress-"+composite.Spec.GatewayProfile) > 63 {
		return fmt.Errorf("spec.gatewayProfile must be a DNS label of 1-50 characters (profile resource name at most 63)")
	}
	if request.AdvertisesCapabilities(req) && !request.HasCapability(req, fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES) {
		return fmt.Errorf("Crossplane does not support required-resources")
	}
	profileName := "istio-egress-" + composite.Spec.GatewayProfile
	rsp.Requirements = &fnv1.Requirements{Resources: map[string]*fnv1.ResourceSelector{
		"gateway-profile": {
			ApiVersion: "apiextensions.crossplane.io/v1beta1", Kind: "EnvironmentConfig",
			Match: &fnv1.ResourceSelector_MatchName{MatchName: profileName},
		},
	}}
	profiles, fetched := req.GetRequiredResources()["gateway-profile"]
	if !fetched {
		return nil
	}
	if len(profiles.GetItems()) == 0 {
		return fmt.Errorf("gateway profile %s not found", profileName)
	}
	if len(profiles.GetItems()) != 1 {
		return fmt.Errorf("expected one gateway profile %s", profileName)
	}
	profileObject := profiles.Items[0].GetResource().AsMap()
	var metadata struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Name              string `json:"name"`
			Namespace         string `json:"namespace"`
			DeletionTimestamp string `json:"deletionTimestamp"`
		} `json:"metadata"`
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(profileObject, &metadata); err != nil {
		return fmt.Errorf("decode profile identity: %w", err)
	}
	if metadata.APIVersion != "apiextensions.crossplane.io/v1beta1" || metadata.Kind != "EnvironmentConfig" || metadata.Metadata.Name != profileName || metadata.Metadata.Namespace != "" {
		return fmt.Errorf("gateway profile identity does not match %s", profileName)
	}
	if metadata.Metadata.DeletionTimestamp != "" {
		return fmt.Errorf("gateway profile %s is deleting", profileName)
	}
	data := profileObject["data"]
	var profile GatewayProfileData
	object, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("profile data must be an object")
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("encode profile data: %w", err)
	}
	// JSON decoding rejects integer overflow instead of narrowing an unstructured number.
	if err := json.Unmarshal(encoded, &profile); err != nil {
		return fmt.Errorf("decode profile: %w", err)
	}
	g := profile.Gateway
	if len(validation.IsDNS1123Label(g.Namespace)) != 0 {
		return fmt.Errorf("profile gateway.namespace must be a DNS label")
	}
	parts := strings.Split(g.ServiceFQDN, ".")
	if !validHostname(g.ServiceFQDN) || len(parts) < 4 || len(validation.IsDNS1035Label(parts[0])) != 0 || parts[1] != g.Namespace || parts[2] != "svc" {
		return fmt.Errorf("profile gateway.serviceFQDN must be <service>.%s.svc.<cluster DNS suffix>", g.Namespace)
	}
	if len(g.Selector) == 0 {
		return fmt.Errorf("profile gateway.selector must be nonempty")
	}
	for key, value := range g.Selector {
		if len(validation.IsQualifiedName(key)) != 0 || len(validation.IsValidLabelValue(value)) != 0 {
			return fmt.Errorf("profile gateway.selector contains invalid label %q", key)
		}
	}
	if g.Port < 1 || g.Port > 65535 {
		return fmt.Errorf("profile gateway.port must be a Service port in 1-65535")
	}
	host := composite.Spec.Hostname
	visibility := []any{"."}
	if g.Namespace != composite.Namespace {
		visibility = append(visibility, g.Namespace)
	}
	sum := sha256.Sum256([]byte(composite.Namespace + "/" + composite.Name))
	subset := fmt.Sprintf("egress-%x", sum[:16])
	gateway := composite.Namespace + "/" + composite.Name
	port := func(n int32) map[string]any { return map[string]any{"number": n} }
	destination := func(host string, n int32, subset string) map[string]any {
		d := map[string]any{"host": host, "port": port(n)}
		if subset != "" {
			d["subset"] = subset
		}
		return map[string]any{"destination": d}
	}
	specs := map[string]struct {
		kind string
		spec map[string]any
	}{
		"service-entry": {"ServiceEntry", map[string]any{
			"hosts": []any{host}, "location": "MESH_EXTERNAL", "resolution": "DNS", "exportTo": visibility,
			"ports": []any{
				map[string]any{"number": 80, "name": "http", "protocol": "HTTP"},
				map[string]any{"number": 443, "name": "https", "protocol": "HTTPS"},
			},
		}},
		"gateway": {"Gateway", map[string]any{
			"selector": g.Selector,
			"servers": []any{map[string]any{
				"port":  map[string]any{"number": g.Port, "name": "https-egress", "protocol": "HTTPS"},
				"hosts": []any{"./" + host}, "tls": map[string]any{"mode": "ISTIO_MUTUAL"},
			}},
		}},
		"gateway-destination-rule": {"DestinationRule", map[string]any{
			"host": g.ServiceFQDN, "exportTo": []any{"."},
			"subsets": []any{map[string]any{
				"name": subset,
				"trafficPolicy": map[string]any{"portLevelSettings": []any{map[string]any{
					"port": port(g.Port), "tls": map[string]any{"mode": "ISTIO_MUTUAL", "sni": host},
				}}},
			}},
		}},
		"virtual-service": {"VirtualService", map[string]any{
			"hosts": []any{host}, "exportTo": visibility, "gateways": []any{"mesh", gateway},
			"http": []any{
				map[string]any{
					"match": []any{map[string]any{"gateways": []any{"mesh"}, "port": 80}},
					"route": []any{destination(g.ServiceFQDN, g.Port, subset)},
				},
				map[string]any{
					"match": []any{map[string]any{"gateways": []any{gateway}, "port": g.Port}},
					"route": []any{destination(host, 443, "")},
				},
			},
		}},
	}
	// Construct every resource before changing desired state; errors cannot emit partial replacements.
	objects := map[string]*structpb.Struct{}
	for key, value := range specs {
		// The Kubernetes converter normalizes typed selector maps for protobuf JSON.
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&map[string]any{"apiVersion": "networking.istio.io/v1", "kind": value.kind, "metadata": map[string]any{"name": composite.Name, "namespace": composite.Namespace}, "spec": value.spec})
		if err != nil {
			return fmt.Errorf("encode %s: %w", key, err)
		}
		objects[key], err = structpb.NewStruct(obj)
		if err != nil {
			return fmt.Errorf("encode %s: %w", key, err)
		}
	}
	if rsp.Desired == nil {
		rsp.Desired = &fnv1.State{}
	}
	if rsp.Desired.Resources == nil {
		rsp.Desired.Resources = map[string]*fnv1.Resource{}
	}
	for key, obj := range objects {
		previous := rsp.Desired.Resources[key]
		if previous == nil {
			previous = &fnv1.Resource{}
			rsp.Desired.Resources[key] = previous
		}
		previous.Resource = obj
		previous.Ready = fnv1.Ready_READY_UNSPECIFIED
	}
	return nil
}

func validHostname(host string) bool {
	if net.ParseIP(host) != nil || len(validation.IsDNS1123Subdomain(host)) != 0 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(validation.IsDNS1123Label(label)) != 0 {
			return false
		}
	}
	return true
}
