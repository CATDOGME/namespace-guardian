package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	guardianv1alpha1 "github.com/CATDOGME/namespace-guardian/api/v1alpha1"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NamespaceRequestAuthzValidator struct {
	Client  client.Client
	Decoder admission.Decoder
}

var _ admission.Handler = &NamespaceRequestAuthzValidator{}

func (v *NamespaceRequestAuthzValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if v.Client == nil {
		return admission.Errored(500, fmt.Errorf("client not initialized"))
	}
	if v.Decoder == nil {
		return admission.Errored(500, fmt.Errorf("decoder not initialized"))
	}

	switch req.Operation {
	case admissionv1.Create:
		return v.validateCreate(ctx, req)
	case admissionv1.Update:
		return v.validateUpdate(ctx, req)
	default:
		return admission.Allowed("ok")
	}
}

func (v *NamespaceRequestAuthzValidator) validateCreate(ctx context.Context, req admission.Request) admission.Response {
	obj := &guardianv1alpha1.NamespaceRequest{}
	if err := v.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(400, err)
	}

	tenant := strings.TrimSpace(obj.Spec.Tenant)
	env := strings.TrimSpace(obj.Spec.Env)
	if env == "" {
		env = "dev"
	}
	ownerGroup := strings.TrimSpace(obj.Spec.OwnerGroup)

	// 用 logger 打印（kubectl logs -c manager 一定能看到）
	namespacerequestlog.Info("AUTHZ_WEBHOOK_HIT",
		"user", req.UserInfo.Username,
		"groups", req.UserInfo.Groups,
		"tenant", tenant,
		"env", env,
		"ownerGroup", ownerGroup,
	)

	// 0) 字段校验
	if tenant == "" {
		return admission.Denied("spec.tenant is required")
	}
	if ownerGroup == "" {
		return admission.Denied("spec.ownerGroup is required")
	}
	if len(ownerGroup) > 63 {
		return admission.Denied("spec.ownerGroup too long (max 63)")
	}

	// env 白名单（可按需缩小，比如只允许 dev/test）
	switch env {
	case "dev", "test", "prod":
	default:
		return admission.Denied(fmt.Sprintf("spec.env must be one of dev/test/prod, got %q", env))
	}

	// 1) tenant 必须存在
	var t guardianv1alpha1.Tenant
	if err := v.Client.Get(ctx, types.NamespacedName{Name: tenant}, &t); err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Denied(fmt.Sprintf("tenant %q not found", tenant))
		}
		return admission.Errored(500, err)
	}

	// 2) 租户级准入：用户 groups 必须命中 tenant.spec.allowedGroups
	if !anyGroupAllowed(req.UserInfo.Groups, t.Spec.AllowedGroups) {
		return admission.Denied(fmt.Sprintf(
			"forbidden: user=%q groups=%v not allowed for tenant=%q allowed=%v",
			req.UserInfo.Username, req.UserInfo.Groups, tenant, t.Spec.AllowedGroups,
		))
	}

	// 3) 环境级准入：admin 放行，否则必须拥有 tenant:<env>
	adminGroup := tenant + ":ns-admin"
	envGroup := tenant + ":" + env
	if !contains(req.UserInfo.Groups, adminGroup) && !contains(req.UserInfo.Groups, envGroup) {
		return admission.Denied(fmt.Sprintf(
			"forbidden: need group %q (or admin %q) to request tenant=%q env=%q, got groups=%v",
			envGroup, adminGroup, tenant, env, req.UserInfo.Groups,
		))
	}

	// 4) 防冒充：ownerGroup 必须属于本人 groups
	if !contains(req.UserInfo.Groups, ownerGroup) {
		return admission.Denied(fmt.Sprintf(
			"forbidden: spec.ownerGroup=%q must be one of your groups=%v",
			ownerGroup, req.UserInfo.Groups,
		))
	}

	// 5) 唯一性：同 (tenant, ownerGroup, env) 只能一个
	sel := labels.Set{
		LabelTenant: tenant,
		LabelEnv:    env,
		//LabelOwnerGroup: ownerGroup,
		LabelOwnerGroupHash: OwnerGroupHash(obj.Spec.OwnerGroup),
	}.AsSelector()

	// 5.1 先查 NamespaceRequest（避免并发窗口）
	var reqList guardianv1alpha1.NamespaceRequestList
	if err := v.Client.List(ctx, &reqList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return admission.Errored(500, err)
	}
	for i := range reqList.Items {
		exist := &reqList.Items[i]
		if exist.Name == obj.Name {
			continue
		}
		if exist.Status.Phase != guardianv1alpha1.PhaseFailed {
			return admission.Denied(fmt.Sprintf(
				"namespace request already exists for tenant=%s ownerGroup=%s env=%s (existing nsreq=%s)",
				tenant, ownerGroup, env, exist.Name,
			))
		}
	}

	// 5.2 再查 Namespace（防止历史/手工创建冲突）
	var nsList corev1.NamespaceList
	if err := v.Client.List(ctx, &nsList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return admission.Errored(500, err)
	}
	if len(nsList.Items) > 0 {
		return admission.Denied(fmt.Sprintf(
			"namespace already exists for tenant=%s ownerGroup=%s env=%s (e.g. %s)",
			tenant, ownerGroup, env, nsList.Items[0].Name,
		))
	}

	return admission.Allowed("ok")
}

func (v *NamespaceRequestAuthzValidator) validateUpdate(ctx context.Context, req admission.Request) admission.Response {
	newObj := &guardianv1alpha1.NamespaceRequest{}
	if err := v.Decoder.Decode(req, newObj); err != nil {
		return admission.Errored(400, err)
	}
	oldObj := &guardianv1alpha1.NamespaceRequest{}
	if err := v.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
		return admission.Errored(400, err)
	}

	// 不可变字段（防绕过）
	if newObj.Spec.Tenant != oldObj.Spec.Tenant {
		return admission.Denied("spec.tenant is immutable")
	}
	if newObj.Spec.Env != oldObj.Spec.Env {
		return admission.Denied("spec.env is immutable")
	}
	if newObj.Spec.OwnerGroup != oldObj.Spec.OwnerGroup {
		return admission.Denied("spec.ownerGroup is immutable")
	}

	tenant := strings.TrimSpace(newObj.Spec.Tenant)
	env := strings.TrimSpace(newObj.Spec.Env)
	if env == "" {
		env = "dev"
	}
	ownerGroup := strings.TrimSpace(newObj.Spec.OwnerGroup)
	if tenant == "" || ownerGroup == "" {
		return admission.Denied("spec.tenant/spec.ownerGroup is required")
	}

	// tenant 必须存在
	var t guardianv1alpha1.Tenant
	if err := v.Client.Get(ctx, types.NamespacedName{Name: tenant}, &t); err != nil {
		if apierrors.IsNotFound(err) {
			return admission.Denied(fmt.Sprintf("tenant %q not found", tenant))
		}
		return admission.Errored(500, err)
	}

	// 租户级准入
	if !anyGroupAllowed(req.UserInfo.Groups, t.Spec.AllowedGroups) {
		return admission.Denied("forbidden: not allowed for this tenant")
	}

	// 环境级准入
	adminGroup := tenant + ":ns-admin"
	envGroup := tenant + ":" + env
	if !contains(req.UserInfo.Groups, adminGroup) && !contains(req.UserInfo.Groups, envGroup) {
		return admission.Denied(fmt.Sprintf(
			"forbidden: need group %q (or admin %q) to update tenant=%q env=%q, got groups=%v",
			envGroup, adminGroup, tenant, env, req.UserInfo.Groups,
		))
	}

	// 防冒充
	if !contains(req.UserInfo.Groups, ownerGroup) {
		return admission.Denied("forbidden: ownerGroup must be one of your groups")
	}

	return admission.Allowed("ok")
}

func anyGroupAllowed(userGroups, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	set := map[string]struct{}{}
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a != "" {
			set[a] = struct{}{}
		}
	}
	for _, g := range userGroups {
		if _, ok := set[g]; ok {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
