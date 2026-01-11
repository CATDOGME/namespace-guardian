package controller

import (
	"context"
	"fmt"
	guardiov1alpha1 "github.com/CATDOGME/namespace-guardian/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// BaselineSpec：后续你可以把这些默认值挪到 Tenant CRD / ConfigMap / flags

type BaselineSpec struct {
	Tenant     string
	Env        string
	OwnerGroup string
	TenantObj  *guardiov1alpha1.Tenant

	// 用于追踪/审计
	RequestName string
}

func selectQuotaHard(t *guardiov1alpha1.Tenant, env string) (guardiov1alpha1.QuotaHard, bool) {
	if t == nil || t.Spec.Quota == nil {
		return guardiov1alpha1.QuotaHard{}, false
	}
	// env 覆盖优先
	if t.Spec.Quota.ByEnv != nil {
		if q, ok := t.Spec.Quota.ByEnv[env]; ok {
			return q, true
		}
	}
	// fallback default
	return t.Spec.Quota.Default, true
}

func quotaHardToResourceList(q guardiov1alpha1.QuotaHard) (corev1.ResourceList, error) {
	out := corev1.ResourceList{}

	put := func(name corev1.ResourceName, s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		qty, err := resource.ParseQuantity(s)
		if err != nil {
			return fmt.Errorf("invalid quantity for %s=%q: %w", name, s, err)
		}
		out[name] = qty
		return nil
	}

	if err := put(corev1.ResourceRequestsCPU, q.RequestsCPU); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourceRequestsMemory, q.RequestsMemory); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourceLimitsCPU, q.LimitsCPU); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourceLimitsMemory, q.LimitsMemory); err != nil {
		return nil, err
	}

	if err := put(corev1.ResourcePods, q.Pods); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourceServices, q.Services); err != nil {
		return nil, err
	}
	// 下面这些资源名不是常量，需要用字符串
	if err := put(corev1.ResourceName("configmaps"), q.ConfigMaps); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourceName("secrets"), q.Secrets); err != nil {
		return nil, err
	}
	if err := put(corev1.ResourcePersistentVolumeClaims, q.PersistentVolumeClaims); err != nil {
		return nil, err
	}

	if strings.TrimSpace(q.NvidiaGPU) != "" {
		if err := put(corev1.ResourceName("nvidia.com/gpu"), q.NvidiaGPU); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// EnsureBaseline 在 namespace 内创建/更新：RBAC + Quota + LimitRange + NetworkPolicy
func EnsureBaseline(ctx context.Context, c client.Client, namespace string, spec BaselineSpec) error {
	// 1) RBAC：ownerGroup -> edit
	if err := ensureOwnerEditRoleBinding(ctx, c, namespace, spec); err != nil {
		return fmt.Errorf("ensure owner edit rolebinding: %w", err)
	}

	// 2) RBAC：adminGroup -> admin（可选但生产常用）
	if err := ensureTenantAdminRoleBinding(ctx, c, namespace, spec); err != nil {
		return fmt.Errorf("ensure tenant admin rolebinding: %w", err)
	}

	// 3) ResourceQuota
	if err := ensureResourceQuota(ctx, c, namespace, spec); err != nil {
		return fmt.Errorf("ensure resourcequota: %w", err)
	}

	// 4) LimitRange
	if err := ensureLimitRange(ctx, c, namespace, spec); err != nil {
		return fmt.Errorf("ensure limitrange: %w", err)
	}

	// 5) NetworkPolicy：deny + allow-dns + allow-same-namespace
	if err := ensureNetworkPolicies(ctx, c, namespace, spec); err != nil {
		return fmt.Errorf("ensure networkpolicies: %w", err)
	}

	return nil
}

func baselineLabels(spec BaselineSpec) map[string]string {
	return map[string]string{
		guardiov1alpha1.LabelManaged: "true",
		guardiov1alpha1.LabelTenant:  spec.Tenant,
		guardiov1alpha1.LabelEnv:     spec.Env,

		guardiov1alpha1.LabelOwnerGroupHash: guardiov1alpha1.ShortHash16(spec.OwnerGroup),
		guardiov1alpha1.LabelRequestHash:    guardiov1alpha1.ShortHash16(spec.RequestName),
	}
}

func ensureOwnerEditRoleBinding(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-owner-edit"
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, rb, func() error {
		ensureBaselineMeta(&rb.ObjectMeta, spec)
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:     rbacv1.GroupKind,
				APIGroup: rbacv1.GroupName,
				Name:     spec.OwnerGroup,
			},
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "guardian-tenant-edit",
		}
		return nil
	})
	return err
}

func ensureTenantAdminRoleBinding(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-tenant-admin.yaml"
	adminGroup := spec.Tenant + ":ns-admin"

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, rb, func() error {
		ensureBaselineMeta(&rb.ObjectMeta, spec)
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:     rbacv1.GroupKind,
				APIGroup: rbacv1.GroupName,
				Name:     adminGroup,
			},
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "guardian-tenant-edit",
		}
		return nil
	})
	return err
}

func ensureResourceQuota(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-rq-default"
	rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}

	_, err := controllerutil.CreateOrUpdate(ctx, c, rq, func() error {
		ensureBaselineMeta(&rq.ObjectMeta, spec)

		hard := corev1.ResourceList{}

		// 1) 先用 Tenant 下发
		if spec.TenantObj != nil {
			q, ok := selectQuotaHard(spec.TenantObj, spec.Env)
			if ok {
				rl, err := quotaHardToResourceList(q)
				if err != nil {
					return err
				}
				hard = rl
			}
		}

		// 2) 没配置就用全局默认（兜底）
		if len(hard) == 0 {
			hard = defaultGlobalResourceQuota()
		}

		rq.Spec.Hard = hard
		return nil
	})
	return err
}

func defaultGlobalResourceQuota() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceRequestsCPU:            resource.MustParse("8"),
		corev1.ResourceRequestsMemory:         resource.MustParse("16Gi"),
		corev1.ResourceLimitsCPU:              resource.MustParse("16"),
		corev1.ResourceLimitsMemory:           resource.MustParse("32Gi"),
		corev1.ResourcePods:                   resource.MustParse("100"),
		corev1.ResourceServices:               resource.MustParse("20"),
		corev1.ResourceName("configmaps"):     resource.MustParse("200"),
		corev1.ResourceName("secrets"):        resource.MustParse("200"),
		corev1.ResourcePersistentVolumeClaims: resource.MustParse("20"),
	}
}

func ensureLimitRange(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-lr-default"
	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, lr, func() error {
		ensureBaselineMeta(&lr.ObjectMeta, spec)

		lr.Spec.Limits = []corev1.LimitRangeItem{
			{
				Type: corev1.LimitTypeContainer,
				DefaultRequest: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Default: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		}
		return nil
	})
	return err
}

func ensureNetworkPolicies(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	// A) 默认 deny ingress+egress
	if err := ensureNPDefaultDeny(ctx, c, ns, spec); err != nil {
		return err
	}

	// B) 允许 DNS 到 kube-system
	if err := ensureNPAllowDNS(ctx, c, ns, spec); err != nil {
		return err
	}

	// C) 允许同 namespace 内互通（常见 baseline，不然默认 deny 会导致同 ns 都不通）
	if err := ensureNPAllowSameNamespace(ctx, c, ns, spec); err != nil {
		return err
	}

	return nil
}

func ensureNPDefaultDeny(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-np-default-deny"
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, np, func() error {
		ensureBaselineMeta(&np.ObjectMeta, spec)
		np.Spec.PodSelector = metav1.LabelSelector{} // all pods
		np.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}
		np.Spec.Ingress = nil
		np.Spec.Egress = nil
		return nil
	})
	return err
}

func ensureNPAllowDNS(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-np-allow-dns"
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, np, func() error {
		ensureBaselineMeta(&np.ObjectMeta, spec)
		np.Spec.PodSelector = metav1.LabelSelector{} // all pods
		np.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeEgress,
		}

		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
			{
				To: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								// kube-system 在 1.21+ 会自动带这个 label
								"kubernetes.io/metadata.name": "kube-system",
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: protoPtr(corev1.ProtocolUDP), Port: intstrPtr(53)},
					{Protocol: protoPtr(corev1.ProtocolTCP), Port: intstrPtr(53)},
				},
			},
		}
		return nil
	})
	return err
}

func ensureNPAllowSameNamespace(ctx context.Context, c client.Client, ns string, spec BaselineSpec) error {
	name := "guardian-np-allow-same-namespace"
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, np, func() error {
		ensureBaselineMeta(&np.ObjectMeta, spec)
		np.Spec.PodSelector = metav1.LabelSelector{} // all pods
		np.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}

		// 同 namespace 的 podSelector
		sameNSPeer := networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{},
		}

		np.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{
			{From: []networkingv1.NetworkPolicyPeer{sameNSPeer}},
		}
		np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{
			{To: []networkingv1.NetworkPolicyPeer{sameNSPeer}},
		}
		return nil
	})
	return err
}

func mergeLabels(dst, src map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func protoPtr(p corev1.Protocol) *corev1.Protocol { return &p }

// 为了不额外 import intstr 包到上面逻辑里，这里直接引用
func intstrPtr(n int32) *intstr.IntOrString {
	v := intstr.IntOrString{Type: intstr.Int, IntVal: n}
	return &v
}
func ensureBaselineMeta(obj metav1.Object, spec BaselineSpec) {
	obj.SetLabels(mergeLabels(obj.GetLabels(), baselineLabels(spec)))

	ann := obj.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[guardiov1alpha1.AnnOwnerGroupRaw] = spec.OwnerGroup
	ann[guardiov1alpha1.AnnRequestRaw] = spec.RequestName
	obj.SetAnnotations(ann)
}
