package kubernetes

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// IncidentTrigger interface for creating incidents from K8s events
type IncidentTrigger interface {
	TriggerFromK8s(eventType, namespace, name, kind, reason, message string) error
}

// Collector watches Kubernetes resources and maintains state
type Collector struct {
	client    *kubernetes.Clientset
	config    *rest.Config

	// Cached state
	clusterInfo  *ClusterInfo
	nodes        map[string]*Node
	namespaces   map[string]*Namespace
	pods         map[string]*Pod         // key: namespace/name
	deployments  map[string]*Deployment  // key: namespace/name
	services     map[string]*Service     // key: namespace/name
	daemonSets   map[string]*DaemonSet   // key: namespace/name
	statefulSets map[string]*StatefulSet // key: namespace/name
	replicaSets  map[string]*ReplicaSet  // key: namespace/name
	jobs         map[string]*Job         // key: namespace/name
	cronJobs     map[string]*CronJob     // key: namespace/name
	ingresses    map[string]*Ingress     // key: namespace/name
	events       []*Event                // recent events (last 100)
	pvcs         map[string]*PersistentVolumeClaim

	// Incident trigger
	pager IncidentTrigger

	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	started  bool
}

// NewCollector creates a new Kubernetes collector
func NewCollector() (*Collector, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Collector{
		client:       client,
		config:       config,
		nodes:        make(map[string]*Node),
		namespaces:   make(map[string]*Namespace),
		pods:         make(map[string]*Pod),
		deployments:  make(map[string]*Deployment),
		services:     make(map[string]*Service),
		daemonSets:   make(map[string]*DaemonSet),
		statefulSets: make(map[string]*StatefulSet),
		replicaSets:  make(map[string]*ReplicaSet),
		jobs:         make(map[string]*Job),
		cronJobs:     make(map[string]*CronJob),
		ingresses:    make(map[string]*Ingress),
		events:       make([]*Event, 0),
		pvcs:         make(map[string]*PersistentVolumeClaim),
	}, nil
}

// getKubeConfig returns the Kubernetes config (in-cluster or from kubeconfig)
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// SetPager sets the incident trigger for K8s events
func (c *Collector) SetPager(pager IncidentTrigger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pager = pager
}

// Start begins watching Kubernetes resources
func (c *Collector) Start() error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return fmt.Errorf("collector already started")
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.started = true
	c.mu.Unlock()

	// Get cluster info
	if err := c.fetchClusterInfo(); err != nil {
		log.Printf("[k8s] Warning: could not get cluster info: %v", err)
	}

	// Initial fetch of all resources
	c.fetchAll()

	// Start watchers
	go c.watchNodes()
	go c.watchNamespaces()
	go c.watchPods()
	go c.watchDeployments()
	go c.watchServices()
	go c.watchDaemonSets()
	go c.watchStatefulSets()
	go c.watchJobs()
	go c.watchCronJobs()
	go c.watchIngresses()
	go c.watchEvents()

	// Periodic refresh (in case watchers miss something)
	go c.refreshLoop()

	log.Printf("[k8s] Collector started")
	return nil
}

// Stop stops the collector
func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}

	if c.cancel != nil {
		c.cancel()
	}
	c.started = false
	log.Printf("[k8s] Collector stopped")
}

// fetchClusterInfo gets cluster version and info
func (c *Collector) fetchClusterInfo() error {
	version, err := c.client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.clusterInfo = &ClusterInfo{
		Version:     version.GitVersion,
		Platform:    detectPlatform(version.GitVersion),
		ConnectedAt: time.Now(),
	}
	c.mu.Unlock()

	return nil
}

func detectPlatform(version string) string {
	v := strings.ToLower(version)
	switch {
	case strings.Contains(v, "gke"):
		return "gke"
	case strings.Contains(v, "eks"):
		return "eks"
	case strings.Contains(v, "aks"):
		return "aks"
	case strings.Contains(v, "k3s"):
		return "k3s"
	case strings.Contains(v, "kind"):
		return "kind"
	case strings.Contains(v, "minikube"):
		return "minikube"
	case strings.Contains(v, "rancher"):
		return "rancher"
	default:
		return "kubernetes"
	}
}

// fetchAll fetches all resources
func (c *Collector) fetchAll() {
	c.fetchNodes()
	c.fetchNamespaces()
	c.fetchPods()
	c.fetchDeployments()
	c.fetchServices()
	c.fetchDaemonSets()
	c.fetchStatefulSets()
	c.fetchJobs()
	c.fetchCronJobs()
	c.fetchIngresses()
	c.fetchEvents()
}

// refreshLoop periodically refreshes all data
func (c *Collector) refreshLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.fetchAll()
		}
	}
}

// fetchNodes fetches all nodes
func (c *Collector) fetchNodes() {
	nodes, err := c.client.CoreV1().Nodes().List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching nodes: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, n := range nodes.Items {
		c.nodes[n.Name] = convertNode(&n)
	}
}

// fetchNamespaces fetches all namespaces
func (c *Collector) fetchNamespaces() {
	nsList, err := c.client.CoreV1().Namespaces().List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching namespaces: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ns := range nsList.Items {
		c.namespaces[ns.Name] = convertNamespace(&ns)
	}
}

// fetchPods fetches all pods
func (c *Collector) fetchPods() {
	pods, err := c.client.CoreV1().Pods("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching pods: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear existing pods and rebuild
	c.pods = make(map[string]*Pod)
	for _, p := range pods.Items {
		key := p.Namespace + "/" + p.Name
		c.pods[key] = convertPod(&p)
	}

	// Update namespace pod counts
	for _, ns := range c.namespaces {
		ns.PodCount = 0
	}
	for _, pod := range c.pods {
		if ns, ok := c.namespaces[pod.Namespace]; ok {
			ns.PodCount++
		}
	}

	// Update node pod counts
	for _, node := range c.nodes {
		node.RunningPods = 0
	}
	for _, pod := range c.pods {
		if pod.Status == "Running" {
			if node, ok := c.nodes[pod.NodeName]; ok {
				node.RunningPods++
			}
		}
	}
}

// fetchDeployments fetches all deployments
func (c *Collector) fetchDeployments() {
	deps, err := c.client.AppsV1().Deployments("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching deployments: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.deployments = make(map[string]*Deployment)
	for _, d := range deps.Items {
		key := d.Namespace + "/" + d.Name
		c.deployments[key] = convertDeployment(&d)
	}
}

// fetchServices fetches all services
func (c *Collector) fetchServices() {
	svcs, err := c.client.CoreV1().Services("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching services: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.services = make(map[string]*Service)
	for _, s := range svcs.Items {
		key := s.Namespace + "/" + s.Name
		c.services[key] = convertService(&s)
	}
}

// fetchDaemonSets fetches all daemonsets
func (c *Collector) fetchDaemonSets() {
	dss, err := c.client.AppsV1().DaemonSets("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching daemonsets: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.daemonSets = make(map[string]*DaemonSet)
	for _, ds := range dss.Items {
		key := ds.Namespace + "/" + ds.Name
		c.daemonSets[key] = convertDaemonSet(&ds)
	}
}

// fetchStatefulSets fetches all statefulsets
func (c *Collector) fetchStatefulSets() {
	stss, err := c.client.AppsV1().StatefulSets("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching statefulsets: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.statefulSets = make(map[string]*StatefulSet)
	for _, sts := range stss.Items {
		key := sts.Namespace + "/" + sts.Name
		c.statefulSets[key] = convertStatefulSet(&sts)
	}
}

// fetchJobs fetches all jobs
func (c *Collector) fetchJobs() {
	jobs, err := c.client.BatchV1().Jobs("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching jobs: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.jobs = make(map[string]*Job)
	for _, j := range jobs.Items {
		key := j.Namespace + "/" + j.Name
		c.jobs[key] = convertJob(&j)
	}
}

// fetchCronJobs fetches all cronjobs
func (c *Collector) fetchCronJobs() {
	cjs, err := c.client.BatchV1().CronJobs("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching cronjobs: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cronJobs = make(map[string]*CronJob)
	for _, cj := range cjs.Items {
		key := cj.Namespace + "/" + cj.Name
		c.cronJobs[key] = convertCronJob(&cj)
	}
}

// fetchIngresses fetches all ingresses
func (c *Collector) fetchIngresses() {
	ings, err := c.client.NetworkingV1().Ingresses("").List(c.ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("[k8s] Error fetching ingresses: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.ingresses = make(map[string]*Ingress)
	for _, ing := range ings.Items {
		key := ing.Namespace + "/" + ing.Name
		c.ingresses[key] = convertIngress(&ing)
	}
}

// fetchEvents fetches recent events
func (c *Collector) fetchEvents() {
	events, err := c.client.CoreV1().Events("").List(c.ctx, metav1.ListOptions{
		Limit: 100,
	})
	if err != nil {
		log.Printf("[k8s] Error fetching events: %v", err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = make([]*Event, 0, len(events.Items))
	for _, e := range events.Items {
		c.events = append(c.events, convertEvent(&e))
	}

	// Sort by timestamp (newest first)
	sort.Slice(c.events, func(i, j int) bool {
		return c.events[i].LastTimestamp.After(c.events[j].LastTimestamp)
	})

	// Keep only last 100
	if len(c.events) > 100 {
		c.events = c.events[:100]
	}
}

// Watcher functions
func (c *Collector) watchNodes() {
	c.watchResource("nodes", func() (watch.Interface, error) {
		return c.client.CoreV1().Nodes().Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		node, ok := event.Object.(*corev1.Node)
		if !ok {
			return
		}
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.nodes[node.Name] = convertNode(node)
		case watch.Deleted:
			delete(c.nodes, node.Name)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchNamespaces() {
	c.watchResource("namespaces", func() (watch.Interface, error) {
		return c.client.CoreV1().Namespaces().Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		ns, ok := event.Object.(*corev1.Namespace)
		if !ok {
			return
		}
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.namespaces[ns.Name] = convertNamespace(ns)
		case watch.Deleted:
			delete(c.namespaces, ns.Name)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchPods() {
	c.watchResource("pods", func() (watch.Interface, error) {
		return c.client.CoreV1().Pods("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			return
		}
		key := pod.Namespace + "/" + pod.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			oldPod := c.pods[key]
			newPod := convertPod(pod)
			c.pods[key] = newPod

			// Check for crash loop or other issues
			c.checkPodIssues(oldPod, newPod)
		case watch.Deleted:
			delete(c.pods, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchDeployments() {
	c.watchResource("deployments", func() (watch.Interface, error) {
		return c.client.AppsV1().Deployments("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		dep, ok := event.Object.(*appsv1.Deployment)
		if !ok {
			return
		}
		key := dep.Namespace + "/" + dep.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.deployments[key] = convertDeployment(dep)
		case watch.Deleted:
			delete(c.deployments, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchServices() {
	c.watchResource("services", func() (watch.Interface, error) {
		return c.client.CoreV1().Services("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		svc, ok := event.Object.(*corev1.Service)
		if !ok {
			return
		}
		key := svc.Namespace + "/" + svc.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.services[key] = convertService(svc)
		case watch.Deleted:
			delete(c.services, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchDaemonSets() {
	c.watchResource("daemonsets", func() (watch.Interface, error) {
		return c.client.AppsV1().DaemonSets("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		ds, ok := event.Object.(*appsv1.DaemonSet)
		if !ok {
			return
		}
		key := ds.Namespace + "/" + ds.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.daemonSets[key] = convertDaemonSet(ds)
		case watch.Deleted:
			delete(c.daemonSets, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchStatefulSets() {
	c.watchResource("statefulsets", func() (watch.Interface, error) {
		return c.client.AppsV1().StatefulSets("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		sts, ok := event.Object.(*appsv1.StatefulSet)
		if !ok {
			return
		}
		key := sts.Namespace + "/" + sts.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.statefulSets[key] = convertStatefulSet(sts)
		case watch.Deleted:
			delete(c.statefulSets, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchJobs() {
	c.watchResource("jobs", func() (watch.Interface, error) {
		return c.client.BatchV1().Jobs("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		job, ok := event.Object.(*batchv1.Job)
		if !ok {
			return
		}
		key := job.Namespace + "/" + job.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.jobs[key] = convertJob(job)
		case watch.Deleted:
			delete(c.jobs, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchCronJobs() {
	c.watchResource("cronjobs", func() (watch.Interface, error) {
		return c.client.BatchV1().CronJobs("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		cj, ok := event.Object.(*batchv1.CronJob)
		if !ok {
			return
		}
		key := cj.Namespace + "/" + cj.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.cronJobs[key] = convertCronJob(cj)
		case watch.Deleted:
			delete(c.cronJobs, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchIngresses() {
	c.watchResource("ingresses", func() (watch.Interface, error) {
		return c.client.NetworkingV1().Ingresses("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		ing, ok := event.Object.(*networkingv1.Ingress)
		if !ok {
			return
		}
		key := ing.Namespace + "/" + ing.Name
		c.mu.Lock()
		switch event.Type {
		case watch.Added, watch.Modified:
			c.ingresses[key] = convertIngress(ing)
		case watch.Deleted:
			delete(c.ingresses, key)
		}
		c.mu.Unlock()
	})
}

func (c *Collector) watchEvents() {
	c.watchResource("events", func() (watch.Interface, error) {
		return c.client.CoreV1().Events("").Watch(c.ctx, metav1.ListOptions{})
	}, func(event watch.Event) {
		e, ok := event.Object.(*corev1.Event)
		if !ok {
			return
		}
		if event.Type == watch.Added || event.Type == watch.Modified {
			converted := convertEvent(e)

			c.mu.Lock()
			// Add to front of list
			c.events = append([]*Event{converted}, c.events...)
			// Keep only last 100
			if len(c.events) > 100 {
				c.events = c.events[:100]
			}
			pager := c.pager
			c.mu.Unlock()

			// Trigger incident for warning events
			if e.Type == "Warning" && pager != nil {
				go pager.TriggerFromK8s(
					e.Type,
					e.InvolvedObject.Namespace,
					e.InvolvedObject.Name,
					e.InvolvedObject.Kind,
					e.Reason,
					e.Message,
				)
			}
		}
	})
}

// watchResource is a helper for watching K8s resources with reconnection
func (c *Collector) watchResource(name string, createWatch func() (watch.Interface, error), handler func(watch.Event)) {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		w, err := createWatch()
		if err != nil {
			log.Printf("[k8s] Error watching %s: %v, retrying in 5s", name, err)
			time.Sleep(5 * time.Second)
			continue
		}

		for event := range w.ResultChan() {
			select {
			case <-c.ctx.Done():
				w.Stop()
				return
			default:
				handler(event)
			}
		}

		log.Printf("[k8s] Watch %s closed, reconnecting...", name)
		time.Sleep(time.Second)
	}
}

// checkPodIssues checks for pod issues that should trigger incidents
func (c *Collector) checkPodIssues(oldPod, newPod *Pod) {
	if c.pager == nil {
		return
	}

	// Check for CrashLoopBackOff
	for _, container := range newPod.Containers {
		if container.State.Reason == "CrashLoopBackOff" {
			// Check if this is a new crash loop
			wasInCrashLoop := false
			if oldPod != nil {
				for _, oldC := range oldPod.Containers {
					if oldC.Name == container.Name && oldC.State.Reason == "CrashLoopBackOff" {
						wasInCrashLoop = true
						break
					}
				}
			}

			if !wasInCrashLoop {
				go c.pager.TriggerFromK8s(
					"Warning",
					newPod.Namespace,
					newPod.Name,
					"Pod",
					"CrashLoopBackOff",
					fmt.Sprintf("Container %s is in CrashLoopBackOff", container.Name),
				)
			}
		}
	}

	// Check for OOMKilled
	for _, container := range newPod.Containers {
		if container.LastState != nil && container.LastState.Reason == "OOMKilled" {
			go c.pager.TriggerFromK8s(
				"Warning",
				newPod.Namespace,
				newPod.Name,
				"Pod",
				"OOMKilled",
				fmt.Sprintf("Container %s was OOMKilled", container.Name),
			)
		}
	}
}

// Getter methods
func (c *Collector) GetClusterInfo() *ClusterInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.clusterInfo == nil {
		return nil
	}

	info := *c.clusterInfo
	info.NodeCount = len(c.nodes)
	info.PodCount = len(c.pods)
	info.NamespaceCount = len(c.namespaces)
	return &info
}

func (c *Collector) GetSummary() *ClusterSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	summary := &ClusterSummary{
		Nodes:      len(c.nodes),
		Namespaces: len(c.namespaces),
		Pods:       len(c.pods),
		Services:   len(c.services),
		Ingresses:  len(c.ingresses),
		UpdatedAt:  time.Now(),
	}

	for _, node := range c.nodes {
		if node.Status == "Ready" {
			summary.NodesReady++
		}
	}

	for _, pod := range c.pods {
		switch pod.Status {
		case "Running":
			summary.PodsRunning++
		case "Pending":
			summary.PodsPending++
		case "Failed":
			summary.PodsFailed++
		}
	}

	summary.Deployments = len(c.deployments)
	for _, dep := range c.deployments {
		if dep.Status == "Healthy" {
			summary.DeploymentsHealthy++
		}
	}

	summary.DaemonSets = len(c.daemonSets)
	for _, ds := range c.daemonSets {
		if ds.Status == "Healthy" {
			summary.DaemonSetsHealthy++
		}
	}

	summary.StatefulSets = len(c.statefulSets)
	for _, sts := range c.statefulSets {
		if sts.Status == "Healthy" {
			summary.StatefulSetsHealthy++
		}
	}

	summary.Jobs = len(c.jobs)
	for _, job := range c.jobs {
		switch job.Status {
		case "Running":
			summary.JobsRunning++
		case "Complete":
			summary.JobsSucceeded++
		case "Failed":
			summary.JobsFailed++
		}
	}

	summary.CronJobs = len(c.cronJobs)
	for _, cj := range c.cronJobs {
		if cj.ActiveJobs > 0 {
			summary.CronJobsActive++
		}
	}

	for _, event := range c.events {
		if event.Type == "Warning" {
			summary.WarningEvents++
		}
	}

	return summary
}

func (c *Collector) GetNodes() []*Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

func (c *Collector) GetNamespaces() []*Namespace {
	c.mu.RLock()
	defer c.mu.RUnlock()

	namespaces := make([]*Namespace, 0, len(c.namespaces))
	for _, ns := range c.namespaces {
		namespaces = append(namespaces, ns)
	}
	return namespaces
}

func (c *Collector) GetPods(namespace string) []*Pod {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pods := make([]*Pod, 0)
	for _, p := range c.pods {
		if namespace == "" || p.Namespace == namespace {
			pods = append(pods, p)
		}
	}
	return pods
}

func (c *Collector) GetPod(namespace, name string) *Pod {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pods[namespace+"/"+name]
}

func (c *Collector) GetDeployments(namespace string) []*Deployment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	deps := make([]*Deployment, 0)
	for _, d := range c.deployments {
		if namespace == "" || d.Namespace == namespace {
			deps = append(deps, d)
		}
	}
	return deps
}

func (c *Collector) GetServices(namespace string) []*Service {
	c.mu.RLock()
	defer c.mu.RUnlock()

	svcs := make([]*Service, 0)
	for _, s := range c.services {
		if namespace == "" || s.Namespace == namespace {
			svcs = append(svcs, s)
		}
	}
	return svcs
}

func (c *Collector) GetDaemonSets(namespace string) []*DaemonSet {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dss := make([]*DaemonSet, 0)
	for _, ds := range c.daemonSets {
		if namespace == "" || ds.Namespace == namespace {
			dss = append(dss, ds)
		}
	}
	return dss
}

func (c *Collector) GetStatefulSets(namespace string) []*StatefulSet {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stss := make([]*StatefulSet, 0)
	for _, sts := range c.statefulSets {
		if namespace == "" || sts.Namespace == namespace {
			stss = append(stss, sts)
		}
	}
	return stss
}

func (c *Collector) GetJobs(namespace string) []*Job {
	c.mu.RLock()
	defer c.mu.RUnlock()

	jobs := make([]*Job, 0)
	for _, j := range c.jobs {
		if namespace == "" || j.Namespace == namespace {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

func (c *Collector) GetCronJobs(namespace string) []*CronJob {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cjs := make([]*CronJob, 0)
	for _, cj := range c.cronJobs {
		if namespace == "" || cj.Namespace == namespace {
			cjs = append(cjs, cj)
		}
	}
	return cjs
}

func (c *Collector) GetIngresses(namespace string) []*Ingress {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ings := make([]*Ingress, 0)
	for _, ing := range c.ingresses {
		if namespace == "" || ing.Namespace == namespace {
			ings = append(ings, ing)
		}
	}
	return ings
}

func (c *Collector) GetEvents(namespace string, limit int) []*Event {
	c.mu.RLock()
	defer c.mu.RUnlock()

	events := make([]*Event, 0)
	for _, e := range c.events {
		if namespace == "" || e.Namespace == namespace {
			events = append(events, e)
			if limit > 0 && len(events) >= limit {
				break
			}
		}
	}
	return events
}

func (c *Collector) GetWorkloadHealth() []*WorkloadHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	health := make([]*WorkloadHealth, 0)

	for _, dep := range c.deployments {
		h := &WorkloadHealth{
			Namespace: dep.Namespace,
			Name:      dep.Name,
			Kind:      "Deployment",
			Replicas:  dep.Replicas,
			Ready:     dep.ReadyReplicas,
			Available: dep.AvailableReplicas,
			Status:    dep.Status,
		}

		if dep.Replicas > 0 {
			h.HealthScore = float64(dep.ReadyReplicas) / float64(dep.Replicas) * 100
		} else {
			h.HealthScore = 100
		}

		health = append(health, h)
	}

	for _, ds := range c.daemonSets {
		h := &WorkloadHealth{
			Namespace: ds.Namespace,
			Name:      ds.Name,
			Kind:      "DaemonSet",
			Replicas:  ds.DesiredNumberScheduled,
			Ready:     ds.NumberReady,
			Available: ds.NumberAvailable,
			Status:    ds.Status,
		}

		if ds.DesiredNumberScheduled > 0 {
			h.HealthScore = float64(ds.NumberReady) / float64(ds.DesiredNumberScheduled) * 100
		} else {
			h.HealthScore = 100
		}

		health = append(health, h)
	}

	for _, sts := range c.statefulSets {
		h := &WorkloadHealth{
			Namespace: sts.Namespace,
			Name:      sts.Name,
			Kind:      "StatefulSet",
			Replicas:  sts.Replicas,
			Ready:     sts.ReadyReplicas,
			Available: sts.ReadyReplicas,
			Status:    sts.Status,
		}

		if sts.Replicas > 0 {
			h.HealthScore = float64(sts.ReadyReplicas) / float64(sts.Replicas) * 100
		} else {
			h.HealthScore = 100
		}

		health = append(health, h)
	}

	// Sort by health score (worst first)
	sort.Slice(health, func(i, j int) bool {
		return health[i].HealthScore < health[j].HealthScore
	})

	return health
}
