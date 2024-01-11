package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

var (
	secretName          string
	sourceNamespace     string
	targetImage         string
	processedNamespaces sync.Map
)

func init() {
	secretName = os.Getenv("SECRET_NAME")
	if secretName == "" {
		klog.Fatalf("SECRET_NAME environment variable not set")
	}

	sourceNamespace = os.Getenv("SOURCE_NAMESPACE")
	if sourceNamespace == "" {
		klog.Fatalf("SOURCE_NAMESPACE environment variable not set")
	}

	targetImage = os.Getenv("TARGET_IMAGE")
	if targetImage == "" {
		klog.Fatalf("TARGET_IMAGE environment variable not set")
	}

	klog.InitFlags(nil)
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

	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pods")
	podController := NewPodController(clientset, queue)
	podController.Run()
}

type PodController struct {
	clientset *kubernetes.Clientset
	queue     workqueue.RateLimitingInterface
}

func NewPodController(clientset *kubernetes.Clientset, queue workqueue.RateLimitingInterface) *PodController {
	return &PodController{
		clientset: clientset,
		queue:     queue,
	}
}

func (c *PodController) Run() {
	podListWatcher := cache.NewListWatchFromClient(c.clientset.CoreV1().RESTClient(), "pods", metav1.NamespaceAll, fields.Everything())

	_, informer := cache.NewInformer(
		podListWatcher,
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.enqueuePod,
			UpdateFunc: func(old, new interface{}) {
				c.enqueuePod(new)
			},
		},
	)

	stop := make(chan struct{})
	defer close(stop)
	go informer.Run(stop)

	for {
		pod, shutdown := c.queue.Get()
		if shutdown {
			break
		}

		if err := c.processPod(pod.(*v1.Pod)); err != nil {
			klog.Errorf("Error processing pod: %v", err)
			c.queue.AddRateLimited(pod)
		} else {
			c.queue.Forget(pod)
		}
		c.queue.Done(pod)
	}
}

func (c *PodController) enqueuePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	if strings.Contains(pod.Spec.Containers[0].Image, targetImage) {
		c.queue.Add(pod)
	}
}

func (c *PodController) processPod(pod *v1.Pod) error {
	startTime := time.Now()
	defer func() {
		klog.Infof("Processed pod in namespace: %s in %v", pod.Namespace, time.Since(startTime))
	}()

	klog.Infof("Processing pod in namespace: %s", pod.Namespace)
	if _, exists := processedNamespaces.Load(pod.Namespace); exists {
		return nil
	}

	if _, err := c.clientset.CoreV1().Secrets(pod.Namespace).Get(context.TODO(), secretName, metav1.GetOptions{}); err == nil {
		klog.Infof("Secret %s already exists in namespace %s", secretName, pod.Namespace)
		return nil
	}

	secret, err := c.clientset.CoreV1().Secrets(sourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret from source namespace: %v", err)
		return err
	}

	secret.Namespace = pod.Namespace
	secret.ResourceVersion = ""
	secret.UID = ""

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := c.clientset.CoreV1().Secrets(pod.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		return err
	})

	if err != nil {
		klog.Errorf("Failed to clone secret to namespace %s: %v", pod.Namespace, err)
		return err
	}

	klog.Infof("Successfully cloned secret to namespace %s", pod.Namespace)
	processedNamespaces.Store(pod.Namespace, struct{}{})
	return nil
}
