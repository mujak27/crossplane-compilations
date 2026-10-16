// Package v1alpha1 defines the example composite API.
// +kubebuilder:object:generate=true
// +groupName=template.example.org
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// XExampleSpec is the desired message.
type XExampleSpec struct {
	// Message is copied into the composed ConfigMap.
	// +kubebuilder:validation:Required
	Message *string `json:"message"`
}

// XExample composes a namespaced ConfigMap.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=xexamples,singular=xexample
type XExample struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:Required
	Spec XExampleSpec `json:"spec"`
}

// XExampleList contains example composites.
// +kubebuilder:object:root=true
type XExampleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []XExample `json:"items"`
}
