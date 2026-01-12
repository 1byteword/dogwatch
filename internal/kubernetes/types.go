package kubernetes

import (
	"time"
)

// ClusterInfo represents overall cluster information
type ClusterInfo struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Platform       string    `json:"platform"` // e.g., "gke", "eks", "aks", "k3s", "kind"
	NodeCount      int       `json:"node_count"`
	PodCount       int       `json:"pod_count"`
	NamespaceCount int       `json:"namespace_count"`
	ConnectedAt    time.Time `json:"connected_at"`
}

// Node represents a Kubernetes node
type Node struct {
	Name              string            `json:"name"`
	Status            string            `json:"status"` // Ready, NotReady, Unknown
	Roles             []string          `json:"roles"`  // master, worker, control-plane
	Version           string            `json:"version"`
	InternalIP        string            `json:"internal_ip"`
	ExternalIP        string            `json:"external_ip,omitempty"`
	OS                string            `json:"os"`
	Architecture      string            `json:"architecture"`
	ContainerRuntime  string            `json:"container_runtime"`
	KubeletVersion    string            `json:"kubelet_version"`

	// Capacity
	CPUCapacity       string            `json:"cpu_capacity"`
	MemoryCapacity    string            `json:"memory_capacity"`
	PodCapacity       int               `json:"pod_capacity"`

	// Allocatable
	CPUAllocatable    string            `json:"cpu_allocatable"`
	MemoryAllocatable string            `json:"memory_allocatable"`

	// Usage (from metrics-server if available)
	CPUUsage          string            `json:"cpu_usage,omitempty"`
	MemoryUsage       string            `json:"memory_usage,omitempty"`
	CPUPercent        float64           `json:"cpu_percent,omitempty"`
	MemoryPercent     float64           `json:"memory_percent,omitempty"`

	// Conditions
	Conditions        []NodeCondition   `json:"conditions,omitempty"`

	// Pod counts
	RunningPods       int               `json:"running_pods"`

	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	LastHeartbeat     time.Time         `json:"last_heartbeat"`
}

// NodeCondition represents a node condition
type NodeCondition struct {
	Type    string `json:"type"`    // Ready, MemoryPressure, DiskPressure, PIDPressure
	Status  string `json:"status"`  // True, False, Unknown
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Namespace represents a Kubernetes namespace
type Namespace struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"` // Active, Terminating
	Labels    map[string]string `json:"labels,omitempty"`
	PodCount  int               `json:"pod_count"`
	CreatedAt time.Time         `json:"created_at"`
}

// Pod represents a Kubernetes pod
type Pod struct {
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Status        string            `json:"status"` // Pending, Running, Succeeded, Failed, Unknown
	Phase         string            `json:"phase"`
	Reason        string            `json:"reason,omitempty"`
	Message       string            `json:"message,omitempty"`

	// Ownership
	OwnerKind     string            `json:"owner_kind,omitempty"`  // Deployment, ReplicaSet, DaemonSet, StatefulSet, Job
	OwnerName     string            `json:"owner_name,omitempty"`

	// Node info
	NodeName      string            `json:"node_name"`
	HostIP        string            `json:"host_ip"`
	PodIP         string            `json:"pod_ip"`
	PodIPs        []string          `json:"pod_ips,omitempty"`

	// Containers
	Containers    []Container       `json:"containers"`
	InitContainers []Container      `json:"init_containers,omitempty"`

	// Resource totals
	CPURequest    string            `json:"cpu_request,omitempty"`
	CPULimit      string            `json:"cpu_limit,omitempty"`
	MemoryRequest string            `json:"memory_request,omitempty"`
	MemoryLimit   string            `json:"memory_limit,omitempty"`

	// Usage (from metrics-server)
	CPUUsage      string            `json:"cpu_usage,omitempty"`
	MemoryUsage   string            `json:"memory_usage,omitempty"`

	// Counts
	RestartCount  int               `json:"restart_count"`
	ReadyCount    int               `json:"ready_count"`
	TotalCount    int               `json:"total_count"`

	// QoS
	QOSClass      string            `json:"qos_class"` // Guaranteed, Burstable, BestEffort

	Labels        map[string]string `json:"labels,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
}

// Container represents a container within a pod
type Container struct {
	Name         string         `json:"name"`
	Image        string         `json:"image"`
	ImageID      string         `json:"image_id,omitempty"`
	Ready        bool           `json:"ready"`
	Started      *bool          `json:"started,omitempty"`
	RestartCount int            `json:"restart_count"`
	State        ContainerState `json:"state"`
	LastState    *ContainerState `json:"last_state,omitempty"`

	// Resources
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`

	// Usage
	CPUUsage    string  `json:"cpu_usage,omitempty"`
	MemoryUsage string  `json:"memory_usage,omitempty"`
}

// ContainerState represents the state of a container
type ContainerState struct {
	Type       string     `json:"type"` // waiting, running, terminated
	Reason     string     `json:"reason,omitempty"`
	Message    string     `json:"message,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int32     `json:"exit_code,omitempty"`
	Signal     *int32     `json:"signal,omitempty"`
}

// Deployment represents a Kubernetes deployment
type Deployment struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`

	// Replicas
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	UpdatedReplicas   int32             `json:"updated_replicas"`
	UnavailableReplicas int32           `json:"unavailable_replicas"`

	// Strategy
	Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
	MaxSurge          string            `json:"max_surge,omitempty"`
	MaxUnavailable    string            `json:"max_unavailable,omitempty"`

	// Status
	Conditions        []DeploymentCondition `json:"conditions,omitempty"`

	// Selector
	Selector          map[string]string `json:"selector,omitempty"`

	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`

	// Computed
	Status            string            `json:"status"` // Healthy, Progressing, Degraded
}

// DeploymentCondition represents a deployment condition
type DeploymentCondition struct {
	Type    string    `json:"type"`   // Available, Progressing, ReplicaFailure
	Status  string    `json:"status"` // True, False, Unknown
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`
	LastUpdate time.Time `json:"last_update"`
}

// Service represents a Kubernetes service
type Service struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Type         string            `json:"type"` // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP    string            `json:"cluster_ip"`
	ExternalIPs  []string          `json:"external_ips,omitempty"`
	LoadBalancerIP string          `json:"load_balancer_ip,omitempty"`
	Ports        []ServicePort     `json:"ports"`
	Selector     map[string]string `json:"selector,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`

	// Computed
	EndpointCount int              `json:"endpoint_count"`
}

// ServicePort represents a port on a service
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// DaemonSet represents a Kubernetes DaemonSet
type DaemonSet struct {
	Name                   string            `json:"name"`
	Namespace              string            `json:"namespace"`
	DesiredNumberScheduled int32             `json:"desired"`
	CurrentNumberScheduled int32             `json:"current"`
	NumberReady            int32             `json:"ready"`
	NumberAvailable        int32             `json:"available"`
	NumberUnavailable      int32             `json:"unavailable"`
	Selector               map[string]string `json:"selector,omitempty"`
	Labels                 map[string]string `json:"labels,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	Status                 string            `json:"status"` // Healthy, Degraded
}

// StatefulSet represents a Kubernetes StatefulSet
type StatefulSet struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Replicas        int32             `json:"replicas"`
	ReadyReplicas   int32             `json:"ready_replicas"`
	CurrentReplicas int32             `json:"current_replicas"`
	UpdatedReplicas int32             `json:"updated_replicas"`
	ServiceName     string            `json:"service_name"`
	Selector        map[string]string `json:"selector,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	Status          string            `json:"status"` // Healthy, Degraded
}

// ReplicaSet represents a Kubernetes ReplicaSet
type ReplicaSet struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	OwnerName         string            `json:"owner_name,omitempty"`
	Selector          map[string]string `json:"selector,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
}

// Job represents a Kubernetes Job
type Job struct {
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	Completions    *int32            `json:"completions,omitempty"`
	Parallelism    *int32            `json:"parallelism,omitempty"`
	Succeeded      int32             `json:"succeeded"`
	Failed         int32             `json:"failed"`
	Active         int32             `json:"active"`
	StartTime      *time.Time        `json:"start_time,omitempty"`
	CompletionTime *time.Time        `json:"completion_time,omitempty"`
	Duration       string            `json:"duration,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	Status         string            `json:"status"` // Running, Complete, Failed
}

// CronJob represents a Kubernetes CronJob
type CronJob struct {
	Name               string            `json:"name"`
	Namespace          string            `json:"namespace"`
	Schedule           string            `json:"schedule"`
	Suspend            bool              `json:"suspend"`
	LastScheduleTime   *time.Time        `json:"last_schedule_time,omitempty"`
	LastSuccessfulTime *time.Time        `json:"last_successful_time,omitempty"`
	ActiveJobs         int               `json:"active_jobs"`
	Labels             map[string]string `json:"labels,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
}

// Event represents a Kubernetes event
type Event struct {
	Name           string    `json:"name"`
	Namespace      string    `json:"namespace"`
	Type           string    `json:"type"` // Normal, Warning
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`

	// Involved object
	ObjectKind     string    `json:"object_kind"`
	ObjectName     string    `json:"object_name"`
	ObjectNamespace string   `json:"object_namespace,omitempty"`

	// Source
	SourceComponent string   `json:"source_component,omitempty"`
	SourceHost      string   `json:"source_host,omitempty"`

	// Timing
	FirstTimestamp  time.Time `json:"first_timestamp"`
	LastTimestamp   time.Time `json:"last_timestamp"`
	Count           int32     `json:"count"`
}

// Ingress represents a Kubernetes Ingress
type Ingress struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Class       string            `json:"class,omitempty"`
	Rules       []IngressRule     `json:"rules"`
	TLS         []IngressTLS      `json:"tls,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`

	// Status
	LoadBalancerIPs []string       `json:"load_balancer_ips,omitempty"`
}

// IngressRule represents an ingress rule
type IngressRule struct {
	Host  string        `json:"host,omitempty"`
	Paths []IngressPath `json:"paths"`
}

// IngressPath represents a path in an ingress rule
type IngressPath struct {
	Path        string `json:"path"`
	PathType    string `json:"path_type"`
	ServiceName string `json:"service_name"`
	ServicePort string `json:"service_port"`
}

// IngressTLS represents TLS configuration for an ingress
type IngressTLS struct {
	Hosts      []string `json:"hosts"`
	SecretName string   `json:"secret_name"`
}

// PersistentVolumeClaim represents a PVC
type PersistentVolumeClaim struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Status       string            `json:"status"` // Pending, Bound, Lost
	StorageClass string            `json:"storage_class,omitempty"`
	AccessModes  []string          `json:"access_modes"`
	Capacity     string            `json:"capacity,omitempty"`
	VolumeName   string            `json:"volume_name,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ConfigMap represents a ConfigMap (metadata only)
type ConfigMap struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	DataKeys  []string          `json:"data_keys"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// Secret represents a Secret (metadata only, no data)
type Secret struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	DataKeys  []string          `json:"data_keys"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// ClusterSummary provides high-level cluster metrics
type ClusterSummary struct {
	// Counts
	Nodes       int `json:"nodes"`
	NodesReady  int `json:"nodes_ready"`
	Namespaces  int `json:"namespaces"`

	Pods        int `json:"pods"`
	PodsRunning int `json:"pods_running"`
	PodsPending int `json:"pods_pending"`
	PodsFailed  int `json:"pods_failed"`

	Deployments        int `json:"deployments"`
	DeploymentsHealthy int `json:"deployments_healthy"`

	Services    int `json:"services"`
	Ingresses   int `json:"ingresses"`

	DaemonSets        int `json:"daemonsets"`
	DaemonSetsHealthy int `json:"daemonsets_healthy"`

	StatefulSets        int `json:"statefulsets"`
	StatefulSetsHealthy int `json:"statefulsets_healthy"`

	Jobs          int `json:"jobs"`
	JobsRunning   int `json:"jobs_running"`
	JobsSucceeded int `json:"jobs_succeeded"`
	JobsFailed    int `json:"jobs_failed"`

	CronJobs       int `json:"cronjobs"`
	CronJobsActive int `json:"cronjobs_active"`

	// Resource totals
	CPUCapacity    string `json:"cpu_capacity"`
	CPUAllocatable string `json:"cpu_allocatable"`
	CPURequested   string `json:"cpu_requested"`

	MemoryCapacity    string `json:"memory_capacity"`
	MemoryAllocatable string `json:"memory_allocatable"`
	MemoryRequested   string `json:"memory_requested"`

	// Events
	WarningEvents int `json:"warning_events"`

	// Timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkloadHealth represents aggregated workload health
type WorkloadHealth struct {
	Namespace   string  `json:"namespace"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"` // Deployment, DaemonSet, StatefulSet
	Replicas    int32   `json:"replicas"`
	Ready       int32   `json:"ready"`
	Available   int32   `json:"available"`
	HealthScore float64 `json:"health_score"` // 0-100
	Status      string  `json:"status"`       // Healthy, Degraded, Critical
	Issues      []string `json:"issues,omitempty"`
}
