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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
var namespacerequestlog = logf.Log.WithName("namespacerequest-webhook")

const (
	LabelTenant     = "guardian.io/tenant"
	LabelEnv        = "guardian.io/env"
	LabelOwnerGroup = "guardian.io/owner-group"
	LabelManaged    = "guardian.io/managed"
)

// SetupNamespaceRequestWebhookWithManager registers the webhook for NamespaceRequest in the manager.
func SetupNamespaceRequestWebhookWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(&guardianv1alpha1.NamespaceRequest{}).
		WithValidator(&NamespaceRequestCustomValidator{Client: c}).
		WithDefaulter(&NamespaceRequestCustomDefaulter{Client: c}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-guardian-guardian-io-v1alpha1-namespacerequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=guardian.guardian.io,resources=namespacerequests,verbs=create;update,versions=v1alpha1,name=mnamespacerequest-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceRequestCustomDefaulter sets default values.
type NamespaceRequestCustomDefaulter struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &NamespaceRequestCustomDefaulter{}

// Default implements webhook.CustomDefaulter.
func (d *NamespaceRequestCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	nr, ok := obj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return fmt.Errorf("expected a NamespaceRequest object but got %T", obj)
	}

	// 1) 默认 env=dev
	nr.Spec.Tenant = strings.TrimSpace(nr.Spec.Tenant)
	nr.Spec.Env = strings.TrimSpace(nr.Spec.Env)
	nr.Spec.OwnerGroup = strings.TrimSpace(nr.Spec.OwnerGroup)

	if nr.Spec.Env == "" {
		nr.Spec.Env = "dev"
	}

	// 2) 补 labels（用于唯一性 selector，避免全量扫描）
	if nr.Labels == nil {
		nr.Labels = map[string]string{}
	}
	nr.Labels[LabelTenant] = nr.Spec.Tenant
	nr.Labels[LabelEnv] = nr.Spec.Env
	nr.Labels[LabelOwnerGroup] = nr.Spec.OwnerGroup
	nr.Labels[LabelManaged] = "true"

	return nil
}

// +kubebuilder:webhook:path=/validate-guardian-guardian-io-v1alpha1-namespacerequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=guardian.guardian.io,resources=namespacerequests,verbs=create;update,versions=v1alpha1,name=vnamespacerequest-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceRequestCustomValidator validates NamespaceRequest.
type NamespaceRequestCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &NamespaceRequestCustomValidator{}

// ValidateCreate validates create.
func (v *NamespaceRequestCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	nr, ok := obj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceRequest object but got %T", obj)
	}
	if v.Client == nil {
		return nil, fmt.Errorf("validator client not initialized")
	}

	tenant := strings.TrimSpace(nr.Spec.Tenant)
	env := strings.TrimSpace(nr.Spec.Env)
	if env == "" {
		env = "dev"
	}
	ownerGroup := strings.TrimSpace(nr.Spec.OwnerGroup)

	// 基础字段校验
	if tenant == "" {
		return nil, fmt.Errorf("spec.tenant is required")
	}
	if ownerGroup == "" {
		return nil, fmt.Errorf("spec.ownerGroup is required")
	}

	// 1) tenant 必须存在
	var t guardianv1alpha1.Tenant
	if err := v.Client.Get(ctx, types.NamespacedName{Name: tenant}, &t); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("tenant %q not found", tenant)
		}
		return nil, err
	}

	// 2) 唯一性：同 (tenant, ownerGroup, env) 只能一个
	sel := labels.Set{
		LabelTenant:     tenant,
		LabelEnv:        env,
		LabelOwnerGroup: ownerGroup,
	}.AsSelector()

	// 2.1 先查 NamespaceRequest（避免 namespace 尚未创建的并发窗口）
	var reqList guardianv1alpha1.NamespaceRequestList
	if err := v.Client.List(ctx, &reqList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return nil, err
	}
	for i := range reqList.Items {
		exist := &reqList.Items[i]
		if exist.Name == nr.Name {
			continue
		}
		// 只要不是 Failed，就认为这个组合已占用
		if exist.Status.Phase != guardianv1alpha1.PhaseFailed {
			return nil, fmt.Errorf(
				"namespace request already exists for tenant=%s ownerGroup=%s env=%s (existing nsreq=%s)",
				tenant, ownerGroup, env, exist.Name,
			)
		}
	}

	// 2.2 再查 Namespace（防止历史/手工创建冲突）
	var nsList corev1.NamespaceList
	if err := v.Client.List(ctx, &nsList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return nil, err
	}
	if len(nsList.Items) > 0 {
		return nil, fmt.Errorf(
			"namespace already exists for tenant=%s ownerGroup=%s env=%s (e.g. %s)",
			tenant, ownerGroup, env, nsList.Items[0].Name,
		)
	}

	return nil, nil
}

// ValidateUpdate validates update (immutability).
func (v *NamespaceRequestCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldNr, ok := oldObj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return nil, fmt.Errorf("expected oldObj NamespaceRequest but got %T", oldObj)
	}
	newNr, ok := newObj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return nil, fmt.Errorf("expected newObj NamespaceRequest but got %T", newObj)
	}

	// 3) 关键字段不可变
	if newNr.Spec.Tenant != oldNr.Spec.Tenant {
		return nil, fmt.Errorf("spec.tenant is immutable")
	}
	if newNr.Spec.Env != oldNr.Spec.Env {
		return nil, fmt.Errorf("spec.env is immutable")
	}
	if newNr.Spec.OwnerGroup != oldNr.Spec.OwnerGroup {
		return nil, fmt.Errorf("spec.ownerGroup is immutable")
	}

	return nil, nil
}

// ValidateDelete validates delete (阶段2先放行).
func (v *NamespaceRequestCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceRequest object but got %T", obj)
	}
	_ = ctx
	return nil, nil
}

// 兼容性占位：避免 admissionv1 未使用（如果你后续要做 delete 校验可用）
var _ = admissionv1.Create
