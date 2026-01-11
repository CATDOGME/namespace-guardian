package controller

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"regexp"
	"strings"

	guardiov1alpha1 "github.com/CATDOGME/namespace-guardian/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RBAC（阶段1最小集合）
// +kubebuilder:rbac:groups=guardian.guardian.io,resources=namespacerequests,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=guardian.guardian.io,resources=namespacerequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=guardian.guardian.io,resources=tenants,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch

// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch

type NamespaceRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NamespaceRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var nr guardiov1alpha1.NamespaceRequest
	if err := r.Get(ctx, req.NamespacedName, &nr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 删除场景：阶段1不处理回收，直接忽略
	if !nr.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// 若已经 Provisioned，保持幂等（也可以做“确保 namespace 存在”）
	if nr.Status.Phase == guardiov1alpha1.PhaseProvisioned && nr.Status.NamespaceName != "" {
		return ctrl.Result{}, nil
	}

	tenant := strings.TrimSpace(nr.Spec.Tenant)
	env := strings.TrimSpace(nr.Spec.Env)
	if env == "" {
		env = "dev"
	}

	// 校验 Tenant 是否存在（阶段1用 controller 做基本校验；阶段2会移到 webhook）
	if err := r.ensureTenantExists(ctx, tenant); err != nil {
		l.Error(err, "tenant not found", "tenant", tenant)
		return ctrl.Result{}, r.setStatusFailed(ctx, &nr, "TenantNotFound", fmt.Sprintf("tenant %q not found", tenant))
	}

	nsName := buildNamespaceName(tenant, env)

	// 创建 Namespace（若已存在则继续）
	if err := r.ensureNamespace(ctx, nsName, &nr); err != nil {
		l.Error(err, "ensure namespace failed", "namespace", nsName)
		return ctrl.Result{}, r.setStatusFailed(ctx, &nr, "NamespaceCreateFailed", err.Error())
	}

	// 创建 namespace 成功后，下发 baseline
	var t guardiov1alpha1.Tenant
	if err := r.Get(ctx, types.NamespacedName{Name: tenant}, &t); err != nil {
		l.Error(err, "get tenant failed", "tenant", tenant)
	}
	if err := EnsureBaseline(ctx, r.Client, nsName, BaselineSpec{
		Tenant:      tenant,
		Env:         env,
		OwnerGroup:  strings.TrimSpace(nr.Spec.OwnerGroup),
		RequestName: nr.Name,
		TenantObj:   &t,
	}); err != nil {
		l.Error(err, "ensure baseline failed", "namespace", nsName)
		return ctrl.Result{}, r.setStatusFailed(ctx, &nr, "BaselineFailed", err.Error())
	}

	// 回写 status
	nr.Status.Phase = guardiov1alpha1.PhaseProvisioned
	nr.Status.NamespaceName = nsName
	nr.Status.Reason = ""
	nr.Status.Message = ""

	if err := r.Status().Update(ctx, &nr); err != nil {
		// 常见冲突：重试即可
		return ctrl.Result{}, err
	}

	l.Info("namespace provisioned", "nsreq", req.Name, "namespace", nsName)
	return ctrl.Result{}, nil
}

func (r *NamespaceRequestReconciler) ensureTenantExists(ctx context.Context, tenant string) error {
	if tenant == "" {
		return fmt.Errorf("spec.tenant is empty")
	}
	var t guardiov1alpha1.Tenant
	return r.Get(ctx, types.NamespacedName{Name: tenant}, &t)
}

func (r *NamespaceRequestReconciler) ensureNamespace(ctx context.Context, nsName string, nr *guardiov1alpha1.NamespaceRequest) error {
	var ns corev1.Namespace
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, &ns)
	if err == nil {
		// 已存在：确保关键标签存在（阶段1最小幂等）
		desired := desiredNSLabels(ns.Labels, nsName, nr)
		if !labelsEqual(ns.Labels, desired) {
			ns.Labels = desired
			return r.Update(ctx, &ns)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	// 不存在：创建
	ns = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nsName,
			Labels: desiredNSLabels(nil, nsName, nr),
		},
	}
	return r.Create(ctx, &ns)
}

func (r *NamespaceRequestReconciler) setStatusFailed(ctx context.Context, nr *guardiov1alpha1.NamespaceRequest, reason, msg string) error {
	nr.Status.Phase = guardiov1alpha1.PhaseFailed
	nr.Status.Reason = reason
	nr.Status.Message = msg
	// NamespaceName 保留为空
	return r.Status().Update(ctx, nr)
}

// buildNamespaceName: <tenant>-<env>
// 生产建议加 ownerGroup/team 等，阶段1先最小化
func buildNamespaceName(tenant, env string) string {
	raw := fmt.Sprintf("%s-%s", tenant, env)
	return sanitizeDNS1123(raw)
}

// DNS1123 label: lowercase alphanum or '-', start/end alphanum, max 63
var dns1123Re = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeDNS1123(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = dns1123Re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "ns"
	}
	if len(s) > 63 {
		s = s[:63]
		s = strings.Trim(s, "-")
		if s == "" {
			return "ns"
		}
	}
	return s
}

func desiredNSLabels(existing map[string]string, nsName string, nr *guardiov1alpha1.NamespaceRequest) map[string]string {
	out := map[string]string{}
	for k, v := range existing {
		out[k] = v
	}
	out["guardian.io/tenant"] = nr.Spec.Tenant
	out["guardian.io/env"] = nr.Spec.Env
	if out["guardian.io/env"] == "" {
		out["guardian.io/env"] = "dev"
	}
	ownerGroup := strings.TrimSpace(nr.Spec.OwnerGroup)
	out["guardian.io/owner-group"] = guardiov1alpha1.ShortHash16(ownerGroup)
	out["guardian.io/managed"] = "true"
	out["guardian.io/request"] = nr.Name
	return out
}

func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (r *NamespaceRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&guardiov1alpha1.NamespaceRequest{}).
		Complete(r)
}
