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
	envSyncConfigs  = "SYNC_CONFIGS"
	configSeparator = ";"
	fieldSeparator  = ","
)

type Config struct {
	secretName      string
	sourceNamespace string
	targetImage     string
}

type PodController struct {
	clientset *kubernetes.Clientset
	queue     workqueue.RateLimitingInterface
	configs   []Config
	processed sync.Map
}

func main() {
	klog.InitFlags(nil)
	configs := loadConfig()
	klog.Info("Starting secret clone controller...")
	clientset := createClientSet()
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	controller := NewPodController(clientset, queue, configs)
	controller.Run()
}

func loadConfig() []Config {
	configsStr := getEnvOrFatal(envSyncConfigs)
	configStrs := strings.Split(configsStr, configSeparator)
	configs := make([]Config, len(configStrs))
	for i, configStr := range configStrs {
		fields := strings.Split(configStr, fieldSeparator)
		if len(fields) != 3 {
			klog.Fatalf("Invalid config format: %s", configStr)
		}
		configs[i] = Config{
			secretName:      fields[0],
			sourceNamespace: fields[1],
			targetImage:     fields[2],
		}
	}
	return configs
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

func NewPodController(clientset *kubernetes.Clientset, queue workqueue.RateLimitingInterface, configs []Config) *PodController {
	return &PodController{
		clientset: clientset,
		queue:     queue,
		configs:   configs,
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
	for _, config := range c.configs {
		if strings.Contains(pod.Spec.Containers[0].Image, config.targetImage) {
			c.queue.Add(pod)
		}
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
		klog.Infof("Processed pod '%s' in namespace '%s' in %v", pod.Name, pod.Namespace, time.Since(startTime))
	}()
	for _, config := range c.configs {
		if _, exists := c.processed.Load(pod.Namespace + config.secretName); exists {
			continue
		}
		if err := c.cloneSecretToNamespace(pod.Namespace, config); err != nil {
			return err
		}
		c.processed.Store(pod.Namespace+config.secretName, struct{}{})
	}
	return nil
}

func (c *PodController) cloneSecretToNamespace(namespace string, config Config) error {
	if _, err := c.clientset.CoreV1().Secrets(namespace).Get(context.TODO(), config.secretName, metav1.GetOptions{}); err == nil {
		klog.Infof("Secret '%s' already exists in namespace '%s'", config.secretName, namespace)
		return nil
	}
	secret, err := c.clientset.CoreV1().Secrets(config.sourceNamespace).Get(context.TODO(), config.secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret '%s' from source namespace '%s': %v", config.secretName, config.sourceNamespace, err)
		return err
	}
	secret.Namespace = namespace
	secret.ResourceVersion = ""
	secret.UID = ""
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := c.clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Failed to clone secret '%s' to namespace '%s': %v", config.secretName, namespace, err)
			return err
		}
		klog.Infof("Successfully cloned secret '%s' to namespace '%s'", config.secretName, namespace)
		return nil
	})
}
