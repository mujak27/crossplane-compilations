// Package v1alpha1 defines the application secure-egress API.
// +kubebuilder:object:generate=true
// +groupName=platform.mujak.com
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// XIstioSecureEgressSpec selects one external hostname and platform gateway profile.
type XIstioSecureEgressSpec struct {
	// Hostname is an exact DNS hostname, never a URL, wildcard or IP address.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`
	// +kubebuilder:validation:XValidation:rule="!isIP(self)",message="hostname must not be an IP address"
	Hostname string `json:"hostname"`
	// GatewayProfile selects the EnvironmentConfig istio-egress-<profile>.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	GatewayProfile string `json:"gatewayProfile"`
}

// XIstioSecureEgress composes four application-local Istio resources.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=xistiosecureegresses,singular=xistiosecureegress
type XIstioSecureEgress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:Required
	Spec XIstioSecureEgressSpec `json:"spec"`
}

// XIstioSecureEgressList contains secure-egress composites.
// +kubebuilder:object:root=true
type XIstioSecureEgressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []XIstioSecureEgress `json:"items"`
}
