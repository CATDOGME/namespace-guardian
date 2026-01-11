package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceRequestSpec：用户提交的申请
type NamespaceRequestSpec struct {
	// Tenant 必填：租户ID（对应 Tenant.metadata.name）
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Tenant string `json:"tenant"`

	// Env 可选：dev/test/prod，默认 dev
	// +kubebuilder:default:=dev
	// +kubebuilder:validation:Enum=dev;test;prod
	Env string `json:"env,omitempty"`

	// OwnerGroup 必填：申请主体所在的组（用于“一组一个 ns”的唯一性约束）
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	OwnerGroup string `json:"ownerGroup"`
}

type NamespaceRequestPhase string

const (
	PhasePending     NamespaceRequestPhase = "Pending"
	PhaseProvisioned NamespaceRequestPhase = "Provisioned"
	PhaseFailed      NamespaceRequestPhase = "Failed"
)

// NamespaceRequestStatus：系统回写状态
type NamespaceRequestStatus struct {
	Phase NamespaceRequestPhase `json:"phase,omitempty"`

	// NamespaceName：最终创建出的 namespace 名称
	NamespaceName string `json:"namespaceName,omitempty"`

	// Reason/Message：失败原因（阶段1先留接口）
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=nsreq
// +kubebuilder:subresource:status
type NamespaceRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceRequestSpec   `json:"spec,omitempty"`
	Status NamespaceRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NamespaceRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NamespaceRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NamespaceRequest{}, &NamespaceRequestList{})
}
