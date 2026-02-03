package scripts

// K8sScripts contains Kubernetes analysis scripts
var K8sScripts = []*Script{
	{
		Name:        "k8s/pod_restarts",
		Category:    "k8s",
		Title:       "Pod Restart Analysis",
		Description: "Find pods with high restart counts",
		Query: `
			SELECT
				name,
				value as restarts,
				namespace,
				timestamp
			FROM metrics
			WHERE name = 'kube_pod_container_status_restarts_total'
			  AND value > {{.threshold}}
			ORDER BY value DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "threshold", Type: "int", Default: 3, Description: "Minimum restart count"},
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "name", Type: "string"},
			{Name: "restarts", Type: "int"},
			{Name: "namespace", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "k8s/resource_usage",
		Category:    "k8s",
		Title:       "Kubernetes Resource Usage",
		Description: "Analyze CPU and memory usage across pods",
		Query: `
			SELECT
				pod,
				container,
				namespace,
				avg(value) as avg_usage,
				max(value) as max_usage
			FROM metrics
			WHERE name IN ('container_cpu_usage_seconds_total', 'container_memory_usage_bytes')
			GROUP BY pod, container, namespace
			ORDER BY avg_usage DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "pod", Type: "string"},
			{Name: "container", Type: "string"},
			{Name: "namespace", Type: "string"},
			{Name: "avg_usage", Type: "float"},
			{Name: "max_usage", Type: "float"},
		},
	},
	{
		Name:        "k8s/failing_pods",
		Category:    "k8s",
		Title:       "Failing Pods",
		Description: "Find pods in failed or error states",
		Query: `
			SELECT
				name as pod,
				namespace,
				value as status,
				timestamp
			FROM metrics
			WHERE name = 'kube_pod_status_phase'
			  AND (phase = 'Failed' OR phase = 'Unknown')
			ORDER BY timestamp DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 20, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "pod", Type: "string"},
			{Name: "namespace", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "k8s/node_health",
		Category:    "k8s",
		Title:       "Node Health Status",
		Description: "Analyze Kubernetes node health conditions",
		Query: `
			SELECT
				node,
				condition,
				value as status,
				timestamp
			FROM metrics
			WHERE name = 'kube_node_status_condition'
			ORDER BY timestamp DESC
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "node", Type: "string"},
			{Name: "condition", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "timestamp", Type: "string"},
		},
	},
	{
		Name:        "k8s/deployment_status",
		Category:    "k8s",
		Title:       "Deployment Status",
		Description: "Check deployment replica status and availability",
		Query: `
			SELECT
				deployment,
				namespace,
				max(CASE WHEN name LIKE '%desired%' THEN value ELSE 0 END) as desired,
				max(CASE WHEN name LIKE '%available%' THEN value ELSE 0 END) as available,
				max(CASE WHEN name LIKE '%unavailable%' THEN value ELSE 0 END) as unavailable
			FROM metrics
			WHERE name LIKE 'kube_deployment_status%'
			GROUP BY deployment, namespace
			ORDER BY unavailable DESC, deployment
			LIMIT {{.limit}}
		`,
		Parameters: []Parameter{
			{Name: "limit", Type: "int", Default: 50, Description: "Maximum results to return"},
		},
		Columns: []Column{
			{Name: "deployment", Type: "string"},
			{Name: "namespace", Type: "string"},
			{Name: "desired", Type: "int"},
			{Name: "available", Type: "int"},
			{Name: "unavailable", Type: "int"},
		},
	},
}
