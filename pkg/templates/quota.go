package templates

import (
	"namespace-guardian/pkg/config"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// 根据配置与 env 选择 ResourceQuota 模板
func BuildResourceQuota(name string, env string, cfg *config.Config) *corev1.ResourceQuota {
	profile, ok := cfg.QuotaProfiles[env]
	if !ok {
		profile = cfg.QuotaProfiles[cfg.DefaultEnv]
	}

	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"guardian.io/managed": "true",
				"guardian.io/env":     env,
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse(profile.CPURequests),
				corev1.ResourceRequestsMemory: resource.MustParse(profile.MemRequests),
				corev1.ResourceLimitsCPU:      resource.MustParse(profile.CPULimits),
				corev1.ResourceLimitsMemory:   resource.MustParse(profile.MemLimits),
				corev1.ResourcePods:           resource.MustParse(profile.Pods),
			},
		},
	}
}
