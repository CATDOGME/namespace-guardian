package util

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// 构建 *rest.Config：优先使用 kubeconfig，其次 in-cluster
func BuildConfig(masterURL, kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	}

	// 尝试用户默认 kubeconfig
	if home := homeDir(); home != "" {
		kc := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kc); err == nil {
			return clientcmd.BuildConfigFromFlags(masterURL, kc)
		}
	}

	// 回退到 in-cluster
	return rest.InClusterConfig()
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}
