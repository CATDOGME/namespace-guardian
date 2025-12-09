package main

import (
	"flag"
	"log"
	"time"

	"namespace-guardian/pkg/config"
	"namespace-guardian/pkg/controller"
	"namespace-guardian/pkg/reconciler"
	"namespace-guardian/pkg/util"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

func main() {
	var kubeconfig string
	var masterURL string
	var configFile string
	var workers int

	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (empty for in-cluster)")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server")
	flag.StringVar(&configFile, "config", "", "Config file path (optional)")
	flag.IntVar(&workers, "workers", 2, "Number of concurrent workers")
	flag.Parse()

	// 1. 加载配置
	cfgData, err := config.Load(configFile)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	// 2. 构建 Kubernetes rest.Config
	restCfg, err := util.BuildConfig(masterURL, kubeconfig)
	if err != nil {
		log.Fatalf("build rest config failed: %v", err)
	}

	// 3. 构建 clientset
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		log.Fatalf("build clientset failed: %v", err)
	}

	// 4. SharedInformerFactory
	factory := informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	nsInformer := factory.Core().V1().Namespaces()

	// 5. Reconciler（业务逻辑层）
	nsReconciler, err := reconciler.NewNamespaceReconciler(restCfg, clientset, cfgData)
	if err != nil {
		log.Fatalf("create namespace reconciler failed: %v", err)
	}

	// 6. Controller（Informer + 队列 + Reconcile 调度）
	nsController, err := controller.NewNamespaceController(
		clientset,
		nsInformer,
		nsReconciler,
	)
	if err != nil {
		log.Fatalf("create namespace controller failed: %v", err)
	}

	stopCh := make(chan struct{})

	// 7. 启动 informer
	factory.Start(stopCh)

	// 8. 启动 controller
	go func() {
		if err := nsController.Run(workers, stopCh); err != nil {
			log.Fatalf("controller exited with error: %v", err)
		}
	}()

	// 9. 阻塞主协程
	wait.NeverStop <- struct{}{} // 形式上阻塞；也可用 select{}
}
