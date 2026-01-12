package kubernetes

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func convertNode(n *corev1.Node) *Node {
	node := &Node{
		Name:             n.Name,
		Version:          n.Status.NodeInfo.KubeletVersion,
		OS:               n.Status.NodeInfo.OperatingSystem,
		Architecture:     n.Status.NodeInfo.Architecture,
		ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
		KubeletVersion:   n.Status.NodeInfo.KubeletVersion,
		Labels:           n.Labels,
		Annotations:      n.Annotations,
		CreatedAt:        n.CreationTimestamp.Time,
	}

	// Get roles from labels
	for label := range n.Labels {
		if label == "node-role.kubernetes.io/master" || label == "node-role.kubernetes.io/control-plane" {
			node.Roles = append(node.Roles, "control-plane")
		} else if label == "node-role.kubernetes.io/worker" {
			node.Roles = append(node.Roles, "worker")
		}
	}
	if len(node.Roles) == 0 {
		node.Roles = []string{"worker"}
	}

	// Get addresses
	for _, addr := range n.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			node.InternalIP = addr.Address
		case corev1.NodeExternalIP:
			node.ExternalIP = addr.Address
		}
	}

	// Capacity
	if cpu := n.Status.Capacity.Cpu(); cpu != nil {
		node.CPUCapacity = cpu.String()
	}
	if mem := n.Status.Capacity.Memory(); mem != nil {
		node.MemoryCapacity = mem.String()
	}
	if pods := n.Status.Capacity.Pods(); pods != nil {
		node.PodCapacity = int(pods.Value())
	}

	// Allocatable
	if cpu := n.Status.Allocatable.Cpu(); cpu != nil {
		node.CPUAllocatable = cpu.String()
	}
	if mem := n.Status.Allocatable.Memory(); mem != nil {
		node.MemoryAllocatable = mem.String()
	}

	// Conditions
	node.Status = "Unknown"
	for _, cond := range n.Status.Conditions {
		node.Conditions = append(node.Conditions, NodeCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})

		if cond.Type == corev1.NodeReady {
			node.LastHeartbeat = cond.LastHeartbeatTime.Time
			if cond.Status == corev1.ConditionTrue {
				node.Status = "Ready"
			} else {
				node.Status = "NotReady"
			}
		}
	}

	return node
}

func convertNamespace(ns *corev1.Namespace) *Namespace {
	return &Namespace{
		Name:      ns.Name,
		Status:    string(ns.Status.Phase),
		Labels:    ns.Labels,
		CreatedAt: ns.CreationTimestamp.Time,
	}
}

func convertPod(p *corev1.Pod) *Pod {
	pod := &Pod{
		Name:        p.Name,
		Namespace:   p.Namespace,
		Phase:       string(p.Status.Phase),
		NodeName:    p.Spec.NodeName,
		HostIP:      p.Status.HostIP,
		PodIP:       p.Status.PodIP,
		Labels:      p.Labels,
		Annotations: p.Annotations,
		CreatedAt:   p.CreationTimestamp.Time,
	}

	// Status reason/message
	if p.Status.Reason != "" {
		pod.Reason = p.Status.Reason
	}
	if p.Status.Message != "" {
		pod.Message = p.Status.Message
	}

	// Pod IPs
	for _, ip := range p.Status.PodIPs {
		pod.PodIPs = append(pod.PodIPs, ip.IP)
	}

	// Start time
	if p.Status.StartTime != nil {
		t := p.Status.StartTime.Time
		pod.StartedAt = &t
	}

	// Owner reference
	for _, owner := range p.OwnerReferences {
		if owner.Controller != nil && *owner.Controller {
			pod.OwnerKind = owner.Kind
			pod.OwnerName = owner.Name
			break
		}
	}

	// QoS class
	pod.QOSClass = string(p.Status.QOSClass)

	// Containers
	for i, c := range p.Spec.Containers {
		container := convertContainer(&c)

		// Container status
		if i < len(p.Status.ContainerStatuses) {
			status := p.Status.ContainerStatuses[i]
			container.Ready = status.Ready
			container.Started = status.Started
			container.RestartCount = int(status.RestartCount)
			container.ImageID = status.ImageID
			container.State = convertContainerState(&status.State)
			if status.LastTerminationState.Waiting != nil ||
				status.LastTerminationState.Running != nil ||
				status.LastTerminationState.Terminated != nil {
				lastState := convertContainerState(&status.LastTerminationState)
				container.LastState = &lastState
			}
		}

		pod.Containers = append(pod.Containers, container)
		pod.RestartCount += container.RestartCount
		pod.TotalCount++
		if container.Ready {
			pod.ReadyCount++
		}
	}

	// Init containers
	for i, c := range p.Spec.InitContainers {
		container := convertContainer(&c)

		if i < len(p.Status.InitContainerStatuses) {
			status := p.Status.InitContainerStatuses[i]
			container.Ready = status.Ready
			container.Started = status.Started
			container.RestartCount = int(status.RestartCount)
			container.ImageID = status.ImageID
			container.State = convertContainerState(&status.State)
		}

		pod.InitContainers = append(pod.InitContainers, container)
	}

	// Compute overall status
	pod.Status = computePodStatus(p)

	// Resource totals
	for _, c := range p.Spec.Containers {
		if req := c.Resources.Requests.Cpu(); req != nil && !req.IsZero() {
			pod.CPURequest = addQuantities(pod.CPURequest, req.String())
		}
		if lim := c.Resources.Limits.Cpu(); lim != nil && !lim.IsZero() {
			pod.CPULimit = addQuantities(pod.CPULimit, lim.String())
		}
		if req := c.Resources.Requests.Memory(); req != nil && !req.IsZero() {
			pod.MemoryRequest = addQuantities(pod.MemoryRequest, req.String())
		}
		if lim := c.Resources.Limits.Memory(); lim != nil && !lim.IsZero() {
			pod.MemoryLimit = addQuantities(pod.MemoryLimit, lim.String())
		}
	}

	return pod
}

func convertContainer(c *corev1.Container) Container {
	container := Container{
		Name:  c.Name,
		Image: c.Image,
	}

	if req := c.Resources.Requests.Cpu(); req != nil {
		container.CPURequest = req.String()
	}
	if lim := c.Resources.Limits.Cpu(); lim != nil {
		container.CPULimit = lim.String()
	}
	if req := c.Resources.Requests.Memory(); req != nil {
		container.MemoryRequest = req.String()
	}
	if lim := c.Resources.Limits.Memory(); lim != nil {
		container.MemoryLimit = lim.String()
	}

	return container
}

func convertContainerState(state *corev1.ContainerState) ContainerState {
	cs := ContainerState{}

	if state.Waiting != nil {
		cs.Type = "waiting"
		cs.Reason = state.Waiting.Reason
		cs.Message = state.Waiting.Message
	} else if state.Running != nil {
		cs.Type = "running"
		if !state.Running.StartedAt.IsZero() {
			t := state.Running.StartedAt.Time
			cs.StartedAt = &t
		}
	} else if state.Terminated != nil {
		cs.Type = "terminated"
		cs.Reason = state.Terminated.Reason
		cs.Message = state.Terminated.Message
		cs.ExitCode = &state.Terminated.ExitCode
		if state.Terminated.Signal != 0 {
			cs.Signal = &state.Terminated.Signal
		}
		if !state.Terminated.StartedAt.IsZero() {
			t := state.Terminated.StartedAt.Time
			cs.StartedAt = &t
		}
		if !state.Terminated.FinishedAt.IsZero() {
			t := state.Terminated.FinishedAt.Time
			cs.FinishedAt = &t
		}
	}

	return cs
}

func computePodStatus(p *corev1.Pod) string {
	// Check for terminating
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}

	// Check container statuses for more specific status
	for _, status := range p.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			if status.State.Waiting.Reason != "" {
				return status.State.Waiting.Reason
			}
		}
		if status.State.Terminated != nil {
			if status.State.Terminated.Reason != "" {
				return status.State.Terminated.Reason
			}
		}
	}

	// Fall back to phase
	return string(p.Status.Phase)
}

func convertDeployment(d *appsv1.Deployment) *Deployment {
	dep := &Deployment{
		Name:                d.Name,
		Namespace:           d.Namespace,
		Replicas:            *d.Spec.Replicas,
		ReadyReplicas:       d.Status.ReadyReplicas,
		AvailableReplicas:   d.Status.AvailableReplicas,
		UpdatedReplicas:     d.Status.UpdatedReplicas,
		UnavailableReplicas: d.Status.UnavailableReplicas,
		Strategy:            string(d.Spec.Strategy.Type),
		Labels:              d.Labels,
		Annotations:         d.Annotations,
		CreatedAt:           d.CreationTimestamp.Time,
	}

	// Selector
	if d.Spec.Selector != nil {
		dep.Selector = d.Spec.Selector.MatchLabels
	}

	// Rolling update params
	if d.Spec.Strategy.RollingUpdate != nil {
		if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			dep.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.String()
		}
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			dep.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.String()
		}
	}

	// Conditions
	for _, cond := range d.Status.Conditions {
		dep.Conditions = append(dep.Conditions, DeploymentCondition{
			Type:       string(cond.Type),
			Status:     string(cond.Status),
			Reason:     cond.Reason,
			Message:    cond.Message,
			LastUpdate: cond.LastUpdateTime.Time,
		})
	}

	// Compute status
	if dep.ReadyReplicas == dep.Replicas && dep.AvailableReplicas == dep.Replicas {
		dep.Status = "Healthy"
	} else if dep.ReadyReplicas > 0 {
		dep.Status = "Progressing"
	} else {
		dep.Status = "Degraded"
	}

	return dep
}

func convertService(s *corev1.Service) *Service {
	svc := &Service{
		Name:       s.Name,
		Namespace:  s.Namespace,
		Type:       string(s.Spec.Type),
		ClusterIP:  s.Spec.ClusterIP,
		Selector:   s.Spec.Selector,
		Labels:     s.Labels,
		CreatedAt:  s.CreationTimestamp.Time,
	}

	// External IPs
	svc.ExternalIPs = s.Spec.ExternalIPs

	// LoadBalancer IP
	if len(s.Status.LoadBalancer.Ingress) > 0 {
		svc.LoadBalancerIP = s.Status.LoadBalancer.Ingress[0].IP
	}

	// Ports
	for _, p := range s.Spec.Ports {
		svc.Ports = append(svc.Ports, ServicePort{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			NodePort:   p.NodePort,
		})
	}

	return svc
}

func convertDaemonSet(ds *appsv1.DaemonSet) *DaemonSet {
	d := &DaemonSet{
		Name:                   ds.Name,
		Namespace:              ds.Namespace,
		DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
		CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
		NumberReady:            ds.Status.NumberReady,
		NumberAvailable:        ds.Status.NumberAvailable,
		NumberUnavailable:      ds.Status.NumberUnavailable,
		Labels:                 ds.Labels,
		CreatedAt:              ds.CreationTimestamp.Time,
	}

	if ds.Spec.Selector != nil {
		d.Selector = ds.Spec.Selector.MatchLabels
	}

	if d.NumberReady == d.DesiredNumberScheduled {
		d.Status = "Healthy"
	} else {
		d.Status = "Degraded"
	}

	return d
}

func convertStatefulSet(sts *appsv1.StatefulSet) *StatefulSet {
	s := &StatefulSet{
		Name:            sts.Name,
		Namespace:       sts.Namespace,
		Replicas:        *sts.Spec.Replicas,
		ReadyReplicas:   sts.Status.ReadyReplicas,
		CurrentReplicas: sts.Status.CurrentReplicas,
		UpdatedReplicas: sts.Status.UpdatedReplicas,
		ServiceName:     sts.Spec.ServiceName,
		Labels:          sts.Labels,
		CreatedAt:       sts.CreationTimestamp.Time,
	}

	if sts.Spec.Selector != nil {
		s.Selector = sts.Spec.Selector.MatchLabels
	}

	if s.ReadyReplicas == s.Replicas {
		s.Status = "Healthy"
	} else {
		s.Status = "Degraded"
	}

	return s
}

func convertJob(j *batchv1.Job) *Job {
	job := &Job{
		Name:        j.Name,
		Namespace:   j.Namespace,
		Completions: j.Spec.Completions,
		Parallelism: j.Spec.Parallelism,
		Succeeded:   j.Status.Succeeded,
		Failed:      j.Status.Failed,
		Active:      j.Status.Active,
		Labels:      j.Labels,
		CreatedAt:   j.CreationTimestamp.Time,
	}

	if j.Status.StartTime != nil {
		t := j.Status.StartTime.Time
		job.StartTime = &t
	}

	if j.Status.CompletionTime != nil {
		t := j.Status.CompletionTime.Time
		job.CompletionTime = &t

		if job.StartTime != nil {
			job.Duration = t.Sub(*job.StartTime).String()
		}
	}

	// Compute status
	if j.Status.Succeeded > 0 && j.Status.Active == 0 {
		job.Status = "Complete"
	} else if j.Status.Failed > 0 && j.Status.Active == 0 {
		job.Status = "Failed"
	} else if j.Status.Active > 0 {
		job.Status = "Running"
	} else {
		job.Status = "Pending"
	}

	return job
}

func convertCronJob(cj *batchv1.CronJob) *CronJob {
	c := &CronJob{
		Name:       cj.Name,
		Namespace:  cj.Namespace,
		Schedule:   cj.Spec.Schedule,
		Suspend:    cj.Spec.Suspend != nil && *cj.Spec.Suspend,
		ActiveJobs: len(cj.Status.Active),
		Labels:     cj.Labels,
		CreatedAt:  cj.CreationTimestamp.Time,
	}

	if cj.Status.LastScheduleTime != nil {
		t := cj.Status.LastScheduleTime.Time
		c.LastScheduleTime = &t
	}

	if cj.Status.LastSuccessfulTime != nil {
		t := cj.Status.LastSuccessfulTime.Time
		c.LastSuccessfulTime = &t
	}

	return c
}

func convertIngress(ing *networkingv1.Ingress) *Ingress {
	i := &Ingress{
		Name:        ing.Name,
		Namespace:   ing.Namespace,
		Labels:      ing.Labels,
		Annotations: ing.Annotations,
		CreatedAt:   ing.CreationTimestamp.Time,
	}

	if ing.Spec.IngressClassName != nil {
		i.Class = *ing.Spec.IngressClassName
	}

	// Rules
	for _, rule := range ing.Spec.Rules {
		r := IngressRule{
			Host: rule.Host,
		}

		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				p := IngressPath{
					Path:     path.Path,
					PathType: string(*path.PathType),
				}

				if path.Backend.Service != nil {
					p.ServiceName = path.Backend.Service.Name
					if path.Backend.Service.Port.Number > 0 {
						p.ServicePort = fmt.Sprintf("%d", path.Backend.Service.Port.Number)
					} else {
						p.ServicePort = path.Backend.Service.Port.Name
					}
				}

				r.Paths = append(r.Paths, p)
			}
		}

		i.Rules = append(i.Rules, r)
	}

	// TLS
	for _, tls := range ing.Spec.TLS {
		i.TLS = append(i.TLS, IngressTLS{
			Hosts:      tls.Hosts,
			SecretName: tls.SecretName,
		})
	}

	// LoadBalancer IPs
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			i.LoadBalancerIPs = append(i.LoadBalancerIPs, lb.IP)
		}
	}

	return i
}

func convertEvent(e *corev1.Event) *Event {
	event := &Event{
		Name:            e.Name,
		Namespace:       e.Namespace,
		Type:            e.Type,
		Reason:          e.Reason,
		Message:         e.Message,
		ObjectKind:      e.InvolvedObject.Kind,
		ObjectName:      e.InvolvedObject.Name,
		ObjectNamespace: e.InvolvedObject.Namespace,
		SourceComponent: e.Source.Component,
		SourceHost:      e.Source.Host,
		Count:           e.Count,
	}

	if !e.FirstTimestamp.IsZero() {
		event.FirstTimestamp = e.FirstTimestamp.Time
	} else if e.EventTime.Time != (time.Time{}) {
		event.FirstTimestamp = e.EventTime.Time
	}

	if !e.LastTimestamp.IsZero() {
		event.LastTimestamp = e.LastTimestamp.Time
	} else {
		event.LastTimestamp = event.FirstTimestamp
	}

	return event
}

// addQuantities is a simple string concatenation for resource quantities
// In a real implementation, you'd parse and add the quantities properly
func addQuantities(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "+" + b
}
