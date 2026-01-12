package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	EnvDev  = "dev"
	EnvTest = "test"
	EnvProd = "prod"

	BaselineVersionV1 = "v1"

	NPProfileStandard = "standard" // deny-all + allow-dns + allow-same-namespace
	NPProfileStrict   = "strict"   // deny-all + allow-dns
	NPProfileOpen     = "open"     // no default deny

	DefaultOwnerClusterRole = "guardian-tenant-edit"
	DefaultAdminClusterRole = "guardian-tenant-admin"

	CondValid           = "Valid"
	CondBaselineApplied = "BaselineApplied"
)

type TenantSpec struct {
	// Owner is optional metadata for audit/ops.
	// +kubebuilder:validation:MaxLength=63
	Owner string `json:"owner,omitempty"`

	// DefaultEnv is used when NamespaceRequest.spec.env is empty.
	// +kubebuilder:default:=dev
	// +kubebuilder:validation:Enum=dev;test;prod
	DefaultEnv string `json:"defaultEnv,omitempty"`

	// AllowedGroups are the groups allowed to operate within this tenant (tenant-wide gate).
	// +kubebuilder:validation:MinItems=1
	AllowedGroups []string `json:"allowedGroups"`

	// Suspend stops applying/updating baseline for this tenant (emergency brake).
	// Webhook may still allow/deny NamespaceRequest, but controller should skip baseline reconcile when suspended.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// NamespaceNamePattern optionally constrains generated namespace names for this tenant.
	// Example: ^tenant-a-(dev|test|prod)-[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	NamespaceNamePattern string `json:"namespaceNamePattern,omitempty"`

	// Baseline defines RBAC/Quota/LimitRange/NetworkPolicy defaults and per-env overrides.
	// +optional
	Baseline *TenantBaselineSpec `json:"baseline,omitempty"`
}

type TenantBaselineSpec struct {
	// Version is used for baseline resource versioning and future upgrades.
	// +kubebuilder:default:=v1
	// +kubebuilder:validation:Enum=v1
	Version string `json:"version,omitempty"`

	// RBAC configures which ClusterRoles are bound into namespaces.
	// +optional
	RBAC *TenantRBACSpec `json:"rbac,omitempty"`

	// Quota defines ResourceQuota defaults and per-env overrides.
	// +optional
	Quota *TenantQuotaSpec `json:"quota,omitempty"`

	// LimitRange defines default requests/limits and per-env overrides.
	// +optional
	LimitRange *TenantLimitRangeSpec `json:"limitRange,omitempty"`

	// NetworkPolicy defines namespace isolation baseline and per-env overrides.
	// +optional
	NetworkPolicy *TenantNetworkPolicySpec `json:"networkPolicy,omitempty"`
}

type TenantRBACSpec struct {
	// OwnerClusterRole is bound to NamespaceRequest.spec.ownerGroup (Group subject).
	// +kubebuilder:default:=guardian-tenant-edit
	// +kubebuilder:validation:MinLength=1
	OwnerClusterRole string `json:"ownerClusterRole,omitempty"`

	// AdminClusterRole is bound to <tenant>:ns-admin (Group subject).
	// +kubebuilder:default:=guardian-tenant-admin
	// +kubebuilder:validation:MinLength=1
	AdminClusterRole string `json:"adminClusterRole,omitempty"`
}

type TenantQuotaSpec struct {
	// Default quota (fallback for all env).
	// +optional
	Default QuotaHard `json:"default,omitempty"`

	// ByEnv overrides quota per env (dev/test/prod).
	// +optional
	ByEnv map[string]QuotaHard `json:"byEnv,omitempty"`
}

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

	NvidiaGPU string `json:"nvidiaGPU,omitempty"`
}

type TenantLimitRangeSpec struct {
	// +optional
	Default LimitRangeHard `json:"default,omitempty"`

	// +optional
	ByEnv map[string]LimitRangeHard `json:"byEnv,omitempty"`
}

type LimitRangeHard struct {
	DefaultRequestCPU    string `json:"defaultRequestCPU,omitempty"`
	DefaultRequestMemory string `json:"defaultRequestMemory,omitempty"`
	DefaultLimitCPU      string `json:"defaultLimitCPU,omitempty"`
	DefaultLimitMemory   string `json:"defaultLimitMemory,omitempty"`

	MaxCPU    string `json:"maxCPU,omitempty"`
	MaxMemory string `json:"maxMemory,omitempty"`
	MinCPU    string `json:"minCPU,omitempty"`
	MinMemory string `json:"minMemory,omitempty"`
}

type TenantNetworkPolicySpec struct {
	// +kubebuilder:default:=standard
	// +kubebuilder:validation:Enum=standard;strict;open
	Profile string `json:"profile,omitempty"`

	// +optional
	AllowEgressCIDRs []string `json:"allowEgressCIDRs,omitempty"`

	// +optional
	ByEnv map[string]TenantNetworkPolicyEnvOverride `json:"byEnv,omitempty"`
}

type TenantNetworkPolicyEnvOverride struct {
	// +optional
	// +kubebuilder:validation:Enum=standard;strict;open
	Profile string `json:"profile,omitempty"`

	// +optional
	AllowEgressCIDRs []string `json:"allowEgressCIDRs,omitempty"`
}

type TenantStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ManagedNamespaces is a lightweight summary for ops.
	// +optional
	ManagedNamespaces int32 `json:"managedNamespaces,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
