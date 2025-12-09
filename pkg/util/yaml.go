package util

import (
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// 将 YAML 字符串转换为 Unstructured 对象，用于 dynamic client
func YAMLToUnstructured(yamlStr string) (*unstructured.Unstructured, error) {
	jsonData, err := yaml.YAMLToJSON([]byte(yamlStr))
	if err != nil {
		return nil, err
	}
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(jsonData); err != nil {
		return nil, err
	}
	return obj, nil
}
