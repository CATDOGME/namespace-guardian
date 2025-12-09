package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"namespace-guardian/pkg/reconciler"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	v1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	v1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	controllerName = "namespace-guardian"
)

// NamespaceController 负责：监听 NS 事件 -> 入队 -> 调 Reconciler
type NamespaceController struct {
	clientset kubernetes.Interface

	nsLister v1listers.NamespaceLister
	nsSynced cache.InformerSynced

	queue      workqueue.RateLimitingInterface
	reconciler reconciler.NamespaceReconciler
}

func NewNamespaceController(
	clientset kubernetes.Interface,
	nsInformer v1informers.NamespaceInformer,
	reconciler reconciler.NamespaceReconciler,
) (*NamespaceController, error) {

	c := &NamespaceController{
		clientset: clientset,
		nsLister:  nsInformer.Lister(),
		nsSynced:  nsInformer.Informer().HasSynced,
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			controllerName,
		),
		reconciler: reconciler,
	}

	nsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				log.Printf("[add] enqueue namespace: %s", ns.Name)
				c.enqueue(ns.Name)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				log.Printf("[update] enqueue namespace: %s", ns.Name)
				c.enqueue(ns.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// 删除一般不做处理，可记录日志
		},
	})

	return c, nil
}

func (c *NamespaceController) enqueue(nsName string) {
	c.queue.Add(nsName)
}

func (c *NamespaceController) Run(workers int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	log.Printf("%s starting", controllerName)

	if ok := cache.WaitForCacheSync(stopCh, c.nsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	log.Printf("%s stopping", controllerName)
	return nil
}

func (c *NamespaceController) runWorker() {
	for c.processNextItem() {
	}
}

func (c *NamespaceController) processNextItem() bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	nsName, ok := key.(string)
	if !ok {
		c.queue.Forget(key)
		return true
	}

	if err := c.syncNamespace(nsName); err != nil {
		log.Printf("sync namespace %s failed: %v", nsName, err)
		c.queue.AddRateLimited(nsName)
	} else {
		c.queue.Forget(nsName)
	}
	return true
}

func (c *NamespaceController) syncNamespace(nsName string) error {
	ctx := context.Background()

	ns, err := c.nsLister.Get(nsName)
	if err != nil {
		// NotFound 时通常说明对象已被删除，可忽略
		return err
	}

	if err := c.reconciler.Reconcile(ctx, ns); err != nil {
		return err
	}
	return nil
}
