package templates

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// 根据 namespace 构建一个简单 Grafana Dashboard ConfigMap
// 内容只示意，实际可以用完整 JSON
func BuildDashboardConfigMap(cmName, ns, dashboardJSON string, monitoringNs string) *corev1.ConfigMap {
	if dashboardJSON == "" {
		// 极简 JSON 示例，实际项目建议使用 embed 文件或模板引擎
		dashboardJSON = fmt.Sprintf(`{
  "title": "Namespace %s Overview",
  "panels": [
    {
      "type": "graph",
      "title": "CPU Usage",
      "targets": [
        {
          "expr": "sum(rate(container_cpu_usage_seconds_total{namespace=\"%s\"}[5m]))"
        }
      ]
    }
  ]
}`, ns, ns)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: monitoringNs,
			Labels: map[string]string{
				"guardian.io/managed": "true",
				"grafana_dashboard":   "1",
			},
		},
		Data: map[string]string{
			"dashboard.json": dashboardJSON,
		},
	}
}
