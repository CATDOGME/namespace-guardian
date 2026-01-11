package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TenantSpec struct {
	// Owner 可选：负责人/团队
	// +kubebuilder:validation:MaxLength=63
	Owner string `json:"owner,omitempty"`

	// DefaultEnv 可选：默认环境（dev/test/prod）
	// +kubebuilder:default:=dev
	// +kubebuilder:validation:Enum=dev;test;prod
	DefaultEnv string `json:"defaultEnv,omitempty"`

	// +kubebuilder:validation:MinItems=1
	AllowedGroups []string `json:"allowedGroups"`
}

type TenantStatus struct {
	// 先留空，阶段1不需要
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=ten
// +kubebuilder:subresource:status
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
