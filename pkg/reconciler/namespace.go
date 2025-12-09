package reconciler

import (
	"context"
	"fmt"
	"log"

	"namespace-guardian/pkg/config"
	"namespace-guardian/pkg/templates"
	"namespace-guardian/pkg/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	labelInitialized = "guardian.io/initialized"
	labelEnv         = "env"
	labelManagedBy   = "managed-by"
	managedByValue   = "namespace-guardian"
)

var prometheusRuleGVR = schema.GroupVersionResource{
	Group:    "monitoring.coreos.com",
	Version:  "v1",
	Resource: "prometheusrules",
}

// Reconciler 接口，便于测试与扩展
type NamespaceReconciler interface {
	Reconcile(ctx context.Context, ns *corev1.Namespace) error
}

type namespaceReconcilerImpl struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	cfg           *config.Config
}

func NewNamespaceReconciler(
	restCfg *rest.Config,
	clientset kubernetes.Interface,
	cfg *config.Config,
) (NamespaceReconciler, error) {
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("new dynamic client: %w", err)
	}

	return &namespaceReconcilerImpl{
		clientset:     clientset,
		dynamicClient: dyn,
		cfg:           cfg,
	}, nil
}

// 核心业务流程：对单个 Namespace 做自动接入
func (r *namespaceReconcilerImpl) Reconcile(ctx context.Context, ns *corev1.Namespace) error {
	// 1. 过滤系统命名空间
	if isSystemNamespace(ns.Name) {
		return nil
	}

	// 2. 删除中的命名空间跳过
	if ns.DeletionTimestamp != nil {
		return nil
	}

	// 3. 检查是否显式关闭自动管理（例如通过 Label/Annotation）
	if ns.Labels != nil {
		if v, ok := ns.Labels["guardian.io/managed"]; ok && v == "false" {
			return nil
		}
	}

	// 4. 确定 env（来自 label / annotation / 默认值）
	env := r.cfg.DefaultEnv
	if ns.Labels != nil {
		if v, ok := ns.Labels[labelEnv]; ok && v != "" {
			env = v
		}
	}

	// 5. Patch Namespace 标签：initialized + env + managed-by
	labelsToPatch := map[string]string{
		labelInitialized: "true",
		labelEnv:         env,
		labelManagedBy:   managedByValue,
	}
	if err := util.PatchNamespaceLabels(ctx, r.clientset, ns.Name, labelsToPatch); err != nil {
		return fmt.Errorf("patch ns labels: %w", err)
	}

	// 6. 确保 ResourceQuota
	quotaName := "default-quota"
	rq := templates.BuildResourceQuota(quotaName, env, r.cfg)
	if err := util.ApplyResourceQuota(ctx, r.clientset, ns.Name, rq); err != nil {
		return fmt.Errorf("apply resourcequota: %w", err)
	}

	// 7. 确保 LimitRange
	lrName := "default-lr"
	lr := templates.BuildLimitRange(lrName)
	if err := util.ApplyLimitRange(ctx, r.clientset, ns.Name, lr); err != nil {
		return fmt.Errorf("apply limitrange: %w", err)
	}

	// 8. 可选 NetworkPolicy（此处留空 TODO）
	// if r.cfg.NetworkPolicyEnabled { ... }

	// 9. 确保 PrometheusRule
	if err := r.ensurePrometheusRule(ctx, ns.Name, env); err != nil {
		return fmt.Errorf("ensure prometheusrule: %w", err)
	}

	// 10. 确保 Grafana Dashboard ConfigMap（可选）
	if r.cfg.DashboardEnabled {
		if err := r.ensureDashboardConfigMap(ctx, ns.Name); err != nil {
			return fmt.Errorf("ensure dashboard configmap: %w", err)
		}
	}

	log.Printf("namespace %s initialized by namespace-guardian", ns.Name)
	return nil
}

func isSystemNamespace(name string) bool {
	switch name {
	case "kube-system", "kube-public", "kube-node-lease", "default":
		return true
	default:
		return false
	}
}

// 生成并下发 PrometheusRule
func (r *namespaceReconcilerImpl) ensurePrometheusRule(ctx context.Context, ns, env string) error {
	ruleName := fmt.Sprintf("guardian-%s-rules", ns)

	yamlStr := templates.BuildPrometheusRuleYAML(ruleName, ns, env)
	obj, err := util.YAMLToUnstructured(yamlStr)
	if err != nil {
		return fmt.Errorf("build unstructured from yaml: %w", err)
	}

	// 放到监控命名空间
	monitoringNs := r.cfg.MonitoringNamespace
	if monitoringNs == "" {
		monitoringNs = "monitoring"
	}

	if err := util.ApplyUnstructured(ctx, r.dynamicClient, prometheusRuleGVR, monitoringNs, obj); err != nil {
		return fmt.Errorf("apply prometheusrule: %w", err)
	}
	return nil
}

func (r *namespaceReconcilerImpl) ensureDashboardConfigMap(ctx context.Context, ns string) error {
	cmName := fmt.Sprintf("guardian-dashboard-%s", ns)
	monitoringNs := r.cfg.MonitoringNamespace
	if monitoringNs == "" {
		monitoringNs = "monitoring"
	}

	cm := templates.BuildDashboardConfigMap(cmName, ns, "", monitoringNs)
	if err := util.ApplyConfigMap(ctx, r.clientset, monitoringNs, cm); err != nil {
		return fmt.Errorf("apply dashboard configmap: %w", err)
	}
	return nil
}
