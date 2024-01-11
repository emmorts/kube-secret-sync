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

const (
	envSecretName      = "SECRET_NAME"
	envSourceNamespace = "SOURCE_NAMESPACE"
	envTargetImage     = "TARGET_IMAGE"
)

type Config struct {
	secretName      string
	sourceNamespace string
	targetImage     string
}

type PodController struct {
	clientset *kubernetes.Clientset
	queue     workqueue.RateLimitingInterface
	config    Config
	processed sync.Map
}

func main() {
	klog.InitFlags(nil)
	config := loadConfig()
	klog.Info("Starting secret clone controller...")

	clientset := createClientSet()
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pods")
	controller := NewPodController(clientset, queue, config)

	controller.Run()
}

func loadConfig() Config {
	return Config{
		secretName:      getEnvOrFatal(envSecretName),
		sourceNamespace: getEnvOrFatal(envSourceNamespace),
		targetImage:     getEnvOrFatal(envTargetImage),
	}
}

func getEnvOrFatal(env string) string {
	value := os.Getenv(env)
	if value == "" {
		klog.Fatalf("%s environment variable not set", env)
	}
	return value
}

func createClientSet() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Error getting in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating clientset: %v", err)
	}
	return clientset
}

func NewPodController(clientset *kubernetes.Clientset, queue workqueue.RateLimitingInterface, config Config) *PodController {
	return &PodController{
		clientset: clientset,
		queue:     queue,
		config:    config,
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

	c.processQueue()
}

func (c *PodController) enqueuePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	if strings.Contains(pod.Spec.Containers[0].Image, c.config.targetImage) {
		c.queue.Add(pod)
	}
}

func (c *PodController) processQueue() {
	for {
		obj, shutdown := c.queue.Get()
		if shutdown {
			break
		}

		if err := c.processPod(obj.(*v1.Pod)); err != nil {
			klog.Errorf("Error processing pod: %v", err)
			c.queue.AddRateLimited(obj)
		} else {
			c.queue.Forget(obj)
		}
		c.queue.Done(obj)
	}
}

func (c *PodController) processPod(pod *v1.Pod) error {
	startTime := time.Now()
	defer func() {
		klog.Infof("Processed pod in namespace: %s in %v", pod.Namespace, time.Since(startTime))
	}()

	if _, exists := c.processed.Load(pod.Namespace); exists {
		return nil
	}

	if err := c.cloneSecretToNamespace(pod.Namespace); err != nil {
		return err
	}

	c.processed.Store(pod.Namespace, struct{}{})
	return nil
}

func (c *PodController) cloneSecretToNamespace(namespace string) error {
	if _, err := c.clientset.CoreV1().Secrets(namespace).Get(context.TODO(), c.config.secretName, metav1.GetOptions{}); err == nil {
		klog.Infof("Secret %s already exists in namespace %s", c.config.secretName, namespace)
		return nil
	}

	secret, err := c.clientset.CoreV1().Secrets(c.config.sourceNamespace).Get(context.TODO(), c.config.secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret from source namespace: %v", err)
		return err
	}

	secret.Namespace = namespace
	secret.ResourceVersion = ""
	secret.UID = ""

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := c.clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Failed to clone secret to namespace %s: %v", namespace, err)
			return err
		}
		klog.Infof("Successfully cloned secret to namespace %s", namespace)
		return nil
	})
}
