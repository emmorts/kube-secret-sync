package main

import (
	"context"
	"flag"
	"strings"

	v1 "k8s.io/api/core/v1" // Added this line
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

var (
	secretName      string
	sourceNamespace string
	targetImage     string
)

func init() {
	// Command-line flags for configuration
	flag.StringVar(&secretName, "secret-name", "gitea-creds", "Name of the secret to clone")
	flag.StringVar(&sourceNamespace, "source-namespace", "default", "Namespace of the source secret")
	flag.StringVar(&targetImage, "target-image", "git.stropus.dev", "Image string to look for in pods")
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	klog.Info("Starting secret clone controller")

	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Error getting in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating clientset: %v", err)
	}

	watchPods(clientset)
}

func watchPods(clientset *kubernetes.Clientset) {
	podListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "pods", metav1.NamespaceAll, fields.Everything())

	_, controller := cache.NewInformer(
		podListWatcher,
		&v1.Pod{},
		0, // Duration is set to 0 for no resync
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				handlePod(clientset, pod)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				newPod := newObj.(*v1.Pod)
				handlePod(clientset, newPod)
			},
		},
	)

	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(stop)

	select {} // Block forever
}

func handlePod(clientset *kubernetes.Clientset, pod *v1.Pod) {
	if strings.Contains(pod.Spec.Containers[0].Image, targetImage) {
		klog.Infof("Found pod with target image in namespace: %s", pod.Namespace)
		cloneSecretToNamespace(clientset, pod.Namespace)
	}
}

func cloneSecretToNamespace(clientset *kubernetes.Clientset, namespace string) {
	secret, err := clientset.CoreV1().Secrets(sourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret from source namespace: %v", err)
		return
	}

	secret.Namespace = namespace
	secret.ResourceVersion = "" // Clear the resource version

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		return err
	})

	if err != nil {
		klog.Errorf("Failed to clone secret to namespace %s: %v", namespace, err)
	} else {
		klog.Infof("Successfully cloned secret to namespace %s", namespace)
	}
}

// Note: Ensure appropriate RBAC roles and rolebindings are configured for this controller
// to watch pods across all namespaces and manage secrets.
