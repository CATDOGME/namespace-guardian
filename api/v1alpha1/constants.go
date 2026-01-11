package v1alpha1

const (
	LabelTenant         = "guardian.io/tenant"
	LabelEnv            = "guardian.io/env"
	LabelOwnerGroup     = "guardian.io/owner-group"
	LabelOwnerGroupHash = "guardian.io/owner-group-hash"
	LabelManaged        = "guardian.io/managed"
	AnnOwnerGroupRaw    = "guardian.io/owner-group-raw" // 推荐：raw 放 annotation
	AnnRequestRaw       = "guardian.io/request-raw"     // 推荐：request 原文放 annotation
	LabelRequestHash    = "guardian.io/request-hash"    // 推荐：request 用 hash label
)
