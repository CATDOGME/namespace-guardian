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

	// +optional
	Quota *TenantQuotaSpec `json:"quota,omitempty"`
}

type TenantQuotaSpec struct {
	// 默认配额（所有 env 兜底）
	Default QuotaHard `json:"default,omitempty"`

	// 按环境覆盖（dev/test/prod）
	ByEnv map[string]QuotaHard `json:"byEnv,omitempty"`
}

// QuotaHard 用字符串表示，方便 YAML 写 resource quantity
// 注意：不要用 map[string]resource.Quantity 作为 CRD 字段（序列化/校验麻烦）
type QuotaHard struct {
	RequestsCPU    string `json:"requestsCPU,omitempty"`
	RequestsMemory string `json:"requestsMemory,omitempty"`
	LimitsCPU      string `json:"limitsCPU,omitempty"`
	LimitsMemory   string `json:"limitsMemory,omitempty"`

	Pods                   string `json:"pods,omitempty"`
	Services               string `json:"services,omitempty"`
	ConfigMaps             string `json:"configMaps,omitempty"`
	Secrets                string `json:"secrets,omitempty"`
	PersistentVolumeClaims string `json:"persistentVolumeClaims,omitempty"`

	// 可选：GPU
	NvidiaGPU string `json:"nvidiaGPU,omitempty"`
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
