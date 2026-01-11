package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	guardianv1alpha1 "github.com/CATDOGME/namespace-guardian/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 这个 logger 会出现在 controller-manager 的 manager 容器日志中
var namespacerequestlog = logf.Log.WithName("namespacerequest-webhook")

const (
	ValidatePath = "/validate-guardian-guardian-io-v1alpha1-namespacerequest"
)

// SetupNamespaceRequestWebhookWithManager registers the webhook for NamespaceRequest in the manager.
func SetupNamespaceRequestWebhookWithManager(mgr ctrl.Manager) error {
	c := mgr.GetClient()

	// NewDecoder 只返回 1 个值
	dec := admission.NewDecoder(mgr.GetScheme())

	// 1) Mutating/Defaulting：用 builder 注册（由 marker 生成 MWC）
	// 注意：这里只注册 defaulter，不注册 validator（validator 用 server.Register 自己接管）
	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&guardianv1alpha1.NamespaceRequest{}).
		WithDefaulter(&NamespaceRequestCustomDefaulter{Client: c}).
		Complete(); err != nil {
		return err
	}

	// 2) Validating/Authz：手动 Register 固定 validate path
	// 放在 Complete() 之后，避免被 builder 生成的 handler 覆盖路由
	mgr.GetWebhookServer().Register(ValidatePath, &admission.Webhook{
		Handler: &NamespaceRequestAuthzValidator{
			Client:  c,
			Decoder: dec,
		},
	})

	namespacerequestlog.Info("authz validating webhook registered", "path", ValidatePath)
	return nil
}

// +kubebuilder:webhook:path=/mutate-guardian-guardian-io-v1alpha1-namespacerequest,mutating=true,failurePolicy=fail,sideEffects=None,groups=guardian.guardian.io,resources=namespacerequests,verbs=create;update,versions=v1alpha1,name=mnamespacerequest-v1alpha1.kb.io,admissionReviewVersions=v1

// +kubebuilder:webhook:path=/validate-guardian-guardian-io-v1alpha1-namespacerequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=guardian.guardian.io,resources=namespacerequests,verbs=create;update,versions=v1alpha1,name=vnamespacerequest-v1alpha1.kb.io,admissionReviewVersions=v1

// NamespaceRequestCustomDefaulter sets default values and labels on NamespaceRequest.
type NamespaceRequestCustomDefaulter struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &NamespaceRequestCustomDefaulter{}

// Default implements webhook.CustomDefaulter.
func (d *NamespaceRequestCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	nr, ok := obj.(*guardianv1alpha1.NamespaceRequest)
	if !ok {
		return fmt.Errorf("expected NamespaceRequest but got %T", obj)
	}

	// trim
	nr.Spec.Tenant = strings.TrimSpace(nr.Spec.Tenant)
	nr.Spec.Env = strings.TrimSpace(nr.Spec.Env)
	nr.Spec.OwnerGroup = strings.TrimSpace(nr.Spec.OwnerGroup)

	// env default
	if nr.Spec.Env == "" {
		nr.Spec.Env = "dev"
	}

	// labels（用于 selector，避免全量扫描）
	if nr.Labels == nil {
		nr.Labels = map[string]string{}
	}
	nr.Labels[guardianv1alpha1.LabelTenant] = nr.Spec.Tenant
	nr.Labels[guardianv1alpha1.LabelEnv] = nr.Spec.Env
	//nr.Labels[LabelOwnerGroup] = nr.Spec.OwnerGroup
	nr.Labels[guardianv1alpha1.LabelOwnerGroupHash] = guardianv1alpha1.ShortHash16(nr.Spec.OwnerGroup)
	nr.Labels[guardianv1alpha1.LabelManaged] = "true"

	return nil
}
