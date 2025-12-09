package util

import (
	"context"
	"fmt"

	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Patch Namespace 的 labels（MergePatch 模式）
func PatchNamespaceLabels(
	ctx context.Context,
	client kubernetes.Interface,
	nsName string,
	labels map[string]string,
) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	}
	patchBytes, err := jsonMarshal(patch)
	if err != nil {
		return err
	}

	_, err = client.CoreV1().Namespaces().Patch(
		ctx,
		nsName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	return err
}

// Ensure ResourceQuota 存在，若不存在则创建
func ApplyResourceQuota(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	quota *corev1.ResourceQuota,
) error {
	_, err := client.CoreV1().ResourceQuotas(ns).Get(ctx, quota.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().ResourceQuotas(ns).Create(ctx, quota, metav1.CreateOptions{})
	return err
}

// Ensure LimitRange
func ApplyLimitRange(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	lr *corev1.LimitRange,
) error {
	_, err := client.CoreV1().LimitRanges(ns).Get(ctx, lr.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().LimitRanges(ns).Create(ctx, lr, metav1.CreateOptions{})
	return err
}

// Ensure ConfigMap（简单版本：不存在则创建，存在则跳过；可按需改成 Patch）
func ApplyConfigMap(
	ctx context.Context,
	client kubernetes.Interface,
	ns string,
	cm *corev1.ConfigMap,
) error {
	_, err := client.CoreV1().ConfigMaps(ns).Get(ctx, cm.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	_, err = client.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	return err
}

// 通用 Unstructured Apply：Get -> Create（示例版本，可按需扩展为 Patch）
func ApplyUnstructured(
	ctx context.Context,
	dyn dynamic.Interface,
	gvr schema.GroupVersionResource,
	ns string,
	obj *unstructured.Unstructured,
) error {
	_, err := dyn.Resource(gvr).Namespace(ns).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err == nil {
		// 简化处理：已存在则暂不更新
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get %s/%s failed: %w", ns, obj.GetName(), err)
	}
	_, err = dyn.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create %s/%s failed: %w", ns, obj.GetName(), err)
	}
	return nil
}

// 用标准库 json 封装一下，避免频繁 import
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
