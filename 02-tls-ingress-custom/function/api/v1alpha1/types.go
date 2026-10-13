package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type XIngressTLSCustom struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   XIngressTLSCustomSpec   `json:"spec"`
	Status XIngressTLSCustomStatus `json:"status,omitempty"`
}

// Required fields use pointers to distinguish missing values from explicit zero values.
type XIngressTLSCustomSpec struct {
	Hostname    *string         `json:"hostname"`
	Ingress     IngressSpec     `json:"ingress"`
	Certificate CertificateSpec `json:"certificate"`
}

type IngressSpec struct {
	ClassName   string  `json:"className,omitempty"`
	ServiceName *string `json:"serviceName"`
	ServicePort *int64  `json:"servicePort"`
}

type CertificateSpec struct {
	IssuerRef IssuerReference `json:"issuerRef"`
}

type IssuerReference struct {
	Name *string `json:"name"`
	Kind *string `json:"kind"`
}

type XIngressTLSCustomStatus struct {
	Certificate CertificateStatus `json:"certificate"`
}

type CertificateStatus struct {
	Ready         bool   `json:"ready"`
	NotAfter      string `json:"notAfter,omitempty"`
	DaysRemaining int64  `json:"daysRemaining"`
	Health        string `json:"health"`
}
