package compliance

// SOC2Control represents a SOC2 Trust Service Criteria control
type SOC2Control struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Criteria    []string `json:"criteria"`
	DogwatchCapabilities []string `json:"dogwatch_capabilities"`
	EvidenceSources []string `json:"evidence_sources"`
}

// SOC2Category represents a Trust Service Category
type SOC2Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SOC2Categories defines all Trust Service Categories
var SOC2Categories = []SOC2Category{
	{
		ID:          "CC",
		Name:        "Common Criteria",
		Description: "Common criteria related to information and systems",
	},
	{
		ID:          "A",
		Name:        "Availability",
		Description: "The system is available for operation and use as committed or agreed",
	},
	{
		ID:          "C",
		Name:        "Confidentiality",
		Description: "Information designated as confidential is protected as committed or agreed",
	},
	{
		ID:          "PI",
		Name:        "Processing Integrity",
		Description: "System processing is complete, valid, accurate, timely, and authorized",
	},
	{
		ID:          "P",
		Name:        "Privacy",
		Description: "Personal information is collected, used, retained, disclosed, and disposed of in conformity with commitments",
	},
}

// SOC2Controls defines the SOC2 controls mapped to dogwatch capabilities
var SOC2Controls = []SOC2Control{
	// CC6 - Logical and Physical Access Controls
	{
		ID:          "CC6.1",
		Category:    "CC6 - Logical and Physical Access",
		Title:       "User Access Management",
		Description: "The entity implements logical access security software, infrastructure, and architectures over protected information assets to protect them from security events.",
		Criteria: []string{
			"Identifies and manages the inventory of information assets",
			"Restricts logical access to information assets",
			"Identifies and authenticates users",
			"Considers network segmentation",
			"Manages points of access",
			"Restricts access to information assets",
			"Manages identification credentials",
			"Uses encryption to protect data",
		},
		DogwatchCapabilities: []string{
			"RBAC system with role-based permissions",
			"User management with email/password authentication",
			"Session management with expiration",
			"API key management with scoped permissions",
			"Team-based access control",
		},
		EvidenceSources: []string{
			"rbac_users",
			"rbac_roles",
			"rbac_teams",
			"rbac_api_keys",
		},
	},
	{
		ID:          "CC6.2",
		Category:    "CC6 - Logical and Physical Access",
		Title:       "Authentication Controls",
		Description: "Prior to issuing system credentials and granting system access, the entity registers and authorizes new internal and external users.",
		Criteria: []string{
			"Registers authorized users",
			"Identifies and authenticates users",
			"Validates authentication credentials",
			"Controls authentication credential distribution",
			"Removes access that is no longer required",
		},
		DogwatchCapabilities: []string{
			"Login/logout audit logging",
			"Session tracking with IP and user agent",
			"Failed login attempt tracking",
			"SSO/OAuth2/SAML integration",
		},
		EvidenceSources: []string{
			"audit_logins",
			"audit_sessions",
			"sso_providers",
		},
	},
	{
		ID:          "CC6.3",
		Category:    "CC6 - Logical and Physical Access",
		Title:       "Access Removal",
		Description: "The entity removes access to protected information assets when appropriate.",
		Criteria: []string{
			"Disables access when no longer required",
			"Removes access for terminated personnel",
			"Reviews and removes unnecessary access",
		},
		DogwatchCapabilities: []string{
			"User deactivation tracking",
			"Access removal audit logs",
			"Session invalidation",
		},
		EvidenceSources: []string{
			"audit_user_changes",
			"audit_deactivations",
		},
	},
	// CC7 - System Operations
	{
		ID:          "CC7.1",
		Category:    "CC7 - System Operations",
		Title:       "Infrastructure Monitoring",
		Description: "To meet its objectives, the entity uses detection and monitoring procedures to identify changes to configurations that result in new vulnerabilities.",
		Criteria: []string{
			"Detects unknown or unauthorized components",
			"Conducts vulnerability scans",
			"Implements detection policies and procedures",
			"Monitors system components",
		},
		DogwatchCapabilities: []string{
			"eBPF-based system monitoring",
			"Real-time metrics collection",
			"Anomaly detection",
			"Alert rules and thresholds",
			"Custom metrics monitoring",
		},
		EvidenceSources: []string{
			"metrics_summary",
			"anomaly_detections",
			"alert_rules",
		},
	},
	{
		ID:          "CC7.2",
		Category:    "CC7 - System Operations",
		Title:       "Incident Detection",
		Description: "The entity monitors system components and the operation of those components for anomalies that are indicative of malicious acts, natural disasters, and errors.",
		Criteria: []string{
			"Implements detection policies",
			"Designs detection measures",
			"Implements filters to analyze anomalies",
			"Monitors the scope of detection activities",
		},
		DogwatchCapabilities: []string{
			"Security alert detection",
			"Threat detection rules",
			"MITRE ATT&CK mapping",
			"Anomaly detection service",
		},
		EvidenceSources: []string{
			"security_alerts",
			"security_rules",
			"anomaly_alerts",
		},
	},
	{
		ID:          "CC7.3",
		Category:    "CC7 - System Operations",
		Title:       "Incident Response",
		Description: "The entity evaluates security events to determine whether they could or have resulted in a failure of the entity to meet its objectives.",
		Criteria: []string{
			"Responds to security incidents by defined procedures",
			"Communicates about security incidents",
			"Determines root causes of security incidents",
			"Implements corrective actions",
		},
		DogwatchCapabilities: []string{
			"Incident management system",
			"On-call escalation policies",
			"Incident timelines",
			"Root cause analysis",
		},
		EvidenceSources: []string{
			"incidents",
			"incident_timelines",
			"escalations",
		},
	},
	{
		ID:          "CC7.4",
		Category:    "CC7 - System Operations",
		Title:       "Change Management",
		Description: "The entity responds to identified security incidents by executing a defined incident response program.",
		Criteria: []string{
			"Assigns roles and responsibilities",
			"Contains security incidents",
			"Mitigates ongoing incidents",
			"Ends threats and restores operations",
			"Evaluates incidents for improvements",
		},
		DogwatchCapabilities: []string{
			"Deployment tracking",
			"Change audit logging",
			"Rollback tracking",
		},
		EvidenceSources: []string{
			"deployments",
			"audit_changes",
		},
	},
	// CC8 - Change Management
	{
		ID:          "CC8.1",
		Category:    "CC8 - Change Management",
		Title:       "Change Authorization",
		Description: "The entity authorizes, designs, develops, configures, documents, tests, approves, and implements changes to infrastructure, data, software, and procedures to meet its objectives.",
		Criteria: []string{
			"Manages changes throughout the system lifecycle",
			"Authorizes changes",
			"Designs and develops changes",
			"Documents changes",
			"Tests changes",
			"Approves changes",
			"Deploys changes",
		},
		DogwatchCapabilities: []string{
			"Comprehensive audit logging",
			"Configuration change tracking",
			"Dashboard/rule version history",
		},
		EvidenceSources: []string{
			"audit_all",
			"config_changes",
		},
	},
	// A1 - Availability
	{
		ID:          "A1.1",
		Category:    "A1 - Availability",
		Title:       "System Availability",
		Description: "The entity maintains, monitors, and evaluates current processing capacity and use of system components.",
		Criteria: []string{
			"Manages capacity to meet availability commitments",
			"Identifies environmental threats",
			"Designs facilities to withstand environmental threats",
			"Recovers system within recovery time and point objectives",
		},
		DogwatchCapabilities: []string{
			"Uptime monitoring via synthetics",
			"SLO tracking and error budgets",
			"System metrics collection",
			"Capacity monitoring",
		},
		EvidenceSources: []string{
			"slo_compliance",
			"synthetics_uptime",
			"system_metrics",
		},
	},
	{
		ID:          "A1.2",
		Category:    "A1 - Availability",
		Title:       "Backup and Recovery",
		Description: "The entity authorizes, designs, develops or acquires, implements, operates, approves, maintains, and monitors environmental protections, software, data backup processes, and recovery infrastructure.",
		Criteria: []string{
			"Implements recovery infrastructure",
			"Tests recovery infrastructure",
			"Maintains backup processes",
			"Stores backup offsite",
		},
		DogwatchCapabilities: []string{
			"Automated backup system",
			"Backup verification",
			"Restore testing capability",
			"Backup retention policies",
		},
		EvidenceSources: []string{
			"backup_records",
			"backup_verifications",
		},
	},
	// Additional common controls
	{
		ID:          "CC2.1",
		Category:    "CC2 - Communication and Information",
		Title:       "Information Communication",
		Description: "The entity obtains or generates and uses relevant, quality information to support the functioning of internal control.",
		Criteria: []string{
			"Identifies information requirements",
			"Captures internal and external sources of data",
			"Processes relevant data into information",
		},
		DogwatchCapabilities: []string{
			"Centralized log aggregation",
			"Trace collection and analysis",
			"Metrics aggregation",
		},
		EvidenceSources: []string{
			"logs_summary",
			"traces_summary",
			"metrics_summary",
		},
	},
	{
		ID:          "CC3.1",
		Category:    "CC3 - Risk Assessment",
		Title:       "Risk Identification",
		Description: "The entity specifies objectives with sufficient clarity to enable the identification and assessment of risks relating to objectives.",
		Criteria: []string{
			"Specifies objectives",
			"Identifies and assesses risks",
			"Considers fraud risks",
		},
		DogwatchCapabilities: []string{
			"Threat detection",
			"Security event logging",
			"Risk scoring",
		},
		EvidenceSources: []string{
			"security_summary",
			"risk_assessment",
		},
	},
	{
		ID:          "CC4.1",
		Category:    "CC4 - Monitoring Activities",
		Title:       "Ongoing Monitoring",
		Description: "The entity selects, develops, and performs ongoing and/or separate evaluations to ascertain whether the components of internal control are present and functioning.",
		Criteria: []string{
			"Considers mix of ongoing and separate evaluations",
			"Considers rate of change",
			"Establishes baseline understanding",
			"Uses knowledgeable personnel",
		},
		DogwatchCapabilities: []string{
			"Continuous monitoring",
			"Automated alerting",
			"Compliance dashboards",
		},
		EvidenceSources: []string{
			"alert_summary",
			"compliance_dashboards",
		},
	},
	{
		ID:          "CC5.1",
		Category:    "CC5 - Control Activities",
		Title:       "Control Activities Design",
		Description: "The entity selects and develops control activities that contribute to the mitigation of risks to the achievement of objectives to acceptable levels.",
		Criteria: []string{
			"Integrates with risk assessment",
			"Considers entity-specific factors",
			"Determines relevant business processes",
			"Evaluates technology processes",
		},
		DogwatchCapabilities: []string{
			"Alert rules and policies",
			"Data retention controls",
			"Access control policies",
		},
		EvidenceSources: []string{
			"control_policies",
			"alert_policies",
		},
	},
}

// GetSOC2Control returns a SOC2 control by ID
func GetSOC2Control(id string) *SOC2Control {
	for _, control := range SOC2Controls {
		if control.ID == id {
			return &control
		}
	}
	return nil
}

// GetSOC2ControlsByCategory returns all controls in a category
func GetSOC2ControlsByCategory(category string) []SOC2Control {
	var controls []SOC2Control
	for _, control := range SOC2Controls {
		if control.Category == category {
			controls = append(controls, control)
		}
	}
	return controls
}

// GetSOC2Categories returns unique categories from controls
func GetSOC2Categories() []string {
	seen := make(map[string]bool)
	var categories []string
	for _, control := range SOC2Controls {
		if !seen[control.Category] {
			seen[control.Category] = true
			categories = append(categories, control.Category)
		}
	}
	return categories
}

// MapDogwatchToSOC2 returns which SOC2 controls a dogwatch feature supports
func MapDogwatchToSOC2(capability string) []string {
	var controlIDs []string
	for _, control := range SOC2Controls {
		for _, cap := range control.DogwatchCapabilities {
			if cap == capability {
				controlIDs = append(controlIDs, control.ID)
				break
			}
		}
	}
	return controlIDs
}

// SOC2EvidenceRequirements defines what evidence is needed for each control
type SOC2EvidenceRequirements struct {
	ControlID        string   `json:"control_id"`
	RequiredEvidence []string `json:"required_evidence"`
	OptionalEvidence []string `json:"optional_evidence"`
}

// GetSOC2EvidenceRequirements returns evidence requirements for all controls
func GetSOC2EvidenceRequirements() []SOC2EvidenceRequirements {
	return []SOC2EvidenceRequirements{
		{
			ControlID: "CC6.1",
			RequiredEvidence: []string{
				"User access list with roles",
				"Role permissions matrix",
				"API key inventory",
				"Team membership report",
			},
			OptionalEvidence: []string{
				"Access review documentation",
				"Segregation of duties matrix",
			},
		},
		{
			ControlID: "CC6.2",
			RequiredEvidence: []string{
				"Login audit logs",
				"Authentication method inventory",
				"Session management logs",
			},
			OptionalEvidence: []string{
				"MFA enrollment report",
				"SSO provider configuration",
			},
		},
		{
			ControlID: "CC6.3",
			RequiredEvidence: []string{
				"User deactivation logs",
				"Access removal audit trail",
			},
			OptionalEvidence: []string{
				"Termination checklist evidence",
				"Access review records",
			},
		},
		{
			ControlID: "CC7.1",
			RequiredEvidence: []string{
				"System metrics summary",
				"Alert rule configuration",
				"Monitoring coverage report",
			},
			OptionalEvidence: []string{
				"Anomaly detection configuration",
				"Custom metrics inventory",
			},
		},
		{
			ControlID: "CC7.2",
			RequiredEvidence: []string{
				"Security alert summary",
				"Detection rule inventory",
			},
			OptionalEvidence: []string{
				"Threat intelligence feeds",
				"MITRE mapping documentation",
			},
		},
		{
			ControlID: "CC7.3",
			RequiredEvidence: []string{
				"Incident records",
				"Incident response metrics",
				"Escalation policy documentation",
			},
			OptionalEvidence: []string{
				"Post-incident reviews",
				"Lessons learned documentation",
			},
		},
		{
			ControlID: "CC7.4",
			RequiredEvidence: []string{
				"Deployment audit log",
				"Change records",
			},
			OptionalEvidence: []string{
				"Rollback records",
				"Change approval documentation",
			},
		},
		{
			ControlID: "CC8.1",
			RequiredEvidence: []string{
				"Configuration change audit log",
				"Change authorization records",
			},
			OptionalEvidence: []string{
				"Change management policy",
				"Testing documentation",
			},
		},
		{
			ControlID: "A1.1",
			RequiredEvidence: []string{
				"SLO compliance report",
				"Uptime metrics",
				"Capacity utilization report",
			},
			OptionalEvidence: []string{
				"Synthetic monitoring results",
				"Performance trending data",
			},
		},
		{
			ControlID: "A1.2",
			RequiredEvidence: []string{
				"Backup execution logs",
				"Backup verification results",
			},
			OptionalEvidence: []string{
				"Recovery test results",
				"Backup retention compliance",
			},
		},
	}
}
