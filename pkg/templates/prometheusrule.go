package templates

import "fmt"

// 输出 PrometheusRule 的 YAML 字符串，由 util.YAMLToUnstructured 转换
// 实际规则内容可按需调整
func BuildPrometheusRuleYAML(ruleName, ns, env string) string {
	return fmt.Sprintf(`
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: %s
  labels:
    guardian.io/managed: "true"
    guardian.io/namespace: "%s"
    guardian.io/env: "%s"
spec:
  groups:
    - name: guardian-%s.rules
      rules:
        - alert: NamespaceHighCpuUsage
          expr: sum(rate(container_cpu_usage_seconds_total{namespace="%s"}[5m]))
                / sum(kube_pod_container_resource_requests_cpu_cores{namespace="%s"}) > 0.8
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "Namespace %s CPU usage high"
            description: "Namespace %s CPU usage > 80%% for 10m"
`, ruleName, ns, env, ns, ns, ns, ns, ns)
}
