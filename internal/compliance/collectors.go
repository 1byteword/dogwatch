package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"dogwatch/internal/alerting"
	"dogwatch/internal/audit"
	"dogwatch/internal/backup"
	"dogwatch/internal/deploys"
	"dogwatch/internal/incidents"
	"dogwatch/internal/rbac"
	"dogwatch/internal/security"
	"dogwatch/internal/slo"

	"github.com/google/uuid"
)

// EvidenceCollector collects evidence from various dogwatch subsystems
type EvidenceCollector struct {
	rbacStore      *rbac.Store
	auditStore     *audit.Store
	securityStore  *security.Store
	sloStore       *slo.Store
	deploysStore   *deploys.Store
	incidentStore  *incidents.Store
	alertManager   *alerting.AlertManager
	backupDataDir  string
}

// NewEvidenceCollector creates a new evidence collector
func NewEvidenceCollector(opts CollectorOptions) *EvidenceCollector {
	return &EvidenceCollector{
		rbacStore:     opts.RBACStore,
		auditStore:    opts.AuditStore,
		securityStore: opts.SecurityStore,
		sloStore:      opts.SLOStore,
		deploysStore:  opts.DeploysStore,
		incidentStore: opts.IncidentStore,
		alertManager:  opts.AlertManager,
		backupDataDir: opts.BackupDataDir,
	}
}

// CollectorOptions contains stores needed for evidence collection
type CollectorOptions struct {
	RBACStore     *rbac.Store
	AuditStore    *audit.Store
	SecurityStore *security.Store
	SLOStore      *slo.Store
	DeploysStore  *deploys.Store
	IncidentStore *incidents.Store
	AlertManager  *alerting.AlertManager
	BackupDataDir string
}

// CollectUserAccessEvidence collects user access evidence for CC6.1
func (c *EvidenceCollector) CollectUserAccessEvidence(orgID string, period DateRange) (*UserAccessReport, []Evidence, error) {
	report := &UserAccessReport{
		GeneratedAt: time.Now(),
		Period:      period,
		UsersByRole: make(map[string]int),
	}

	var evidenceList []Evidence

	if c.rbacStore == nil {
		return report, evidenceList, nil
	}

	// Get all users
	users, err := c.rbacStore.ListUsers(orgID)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}

	report.TotalUsers = len(users)
	activeCount := 0
	inactiveCount := 0

	for _, user := range users {
		report.UsersByRole[string(user.Role)]++
		if user.IsActive {
			activeCount++
		} else {
			inactiveCount++
		}
	}
	report.ActiveUsers = activeCount
	report.InactiveUsers = inactiveCount

	// Create evidence for user list
	userData := make([]map[string]interface{}, len(users))
	for i, user := range users {
		userData[i] = map[string]interface{}{
			"id":         user.ID,
			"email":      user.Email,
			"name":       user.Name,
			"role":       user.Role,
			"is_active":  user.IsActive,
			"created_at": user.CreatedAt,
			"teams":      user.TeamIDs,
		}
	}

	userEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceReport,
		Title:       "User Access List",
		Description: fmt.Sprintf("Complete list of %d users with their roles and access levels", len(users)),
		Data:        userData,
		DataSummary: fmt.Sprintf("%d total users, %d active, %d inactive", report.TotalUsers, report.ActiveUsers, report.InactiveUsers),
		CollectedAt: time.Now(),
		Source:      "rbac_store",
		Hash:        hashData(userData),
	}
	evidenceList = append(evidenceList, userEvidence)

	// Get teams
	teams, err := c.rbacStore.ListTeams(orgID)
	if err == nil && len(teams) > 0 {
		teamData := make([]map[string]interface{}, len(teams))
		for i, team := range teams {
			teamData[i] = map[string]interface{}{
				"id":          team.ID,
				"name":        team.Name,
				"description": team.Description,
				"members":     team.MemberIDs,
				"created_at":  team.CreatedAt,
			}
		}

		teamEvidence := Evidence{
			ID:          uuid.New().String(),
			Type:        EvidenceReport,
			Title:       "Team Membership Report",
			Description: fmt.Sprintf("List of %d teams with member assignments", len(teams)),
			Data:        teamData,
			DataSummary: fmt.Sprintf("%d teams configured", len(teams)),
			CollectedAt: time.Now(),
			Source:      "rbac_store",
			Hash:        hashData(teamData),
		}
		evidenceList = append(evidenceList, teamEvidence)
	}

	// Get API keys
	apiKeys, err := c.rbacStore.ListAPIKeys(orgID)
	if err == nil && len(apiKeys) > 0 {
		apiKeyData := make([]map[string]interface{}, len(apiKeys))
		for i, key := range apiKeys {
			apiKeyData[i] = map[string]interface{}{
				"id":           key.ID,
				"name":         key.Name,
				"user_id":      key.UserID,
				"key_prefix":   key.KeyPrefix,
				"permissions":  key.Permissions,
				"is_active":    key.IsActive,
				"created_at":   key.CreatedAt,
				"last_used_at": key.LastUsedAt,
			}
		}

		apiKeyEvidence := Evidence{
			ID:          uuid.New().String(),
			Type:        EvidenceReport,
			Title:       "API Key Inventory",
			Description: fmt.Sprintf("List of %d API keys with permissions", len(apiKeys)),
			Data:        apiKeyData,
			DataSummary: fmt.Sprintf("%d API keys configured", len(apiKeys)),
			CollectedAt: time.Now(),
			Source:      "rbac_store",
			Hash:        hashData(apiKeyData),
		}
		evidenceList = append(evidenceList, apiKeyEvidence)
	}

	// Get access changes from audit log
	if c.auditStore != nil {
		changes, err := c.collectAccessChanges(period)
		if err == nil {
			report.AccessChanges = changes
		}
	}

	return report, evidenceList, nil
}

// CollectAuthenticationEvidence collects authentication logs for CC6.2
func (c *EvidenceCollector) CollectAuthenticationEvidence(orgID string, period DateRange) (*AuthenticationReport, []Evidence, error) {
	report := &AuthenticationReport{
		GeneratedAt:  time.Now(),
		Period:       period,
		LoginsByHour: make(map[int]int),
		LoginsByDay:  make(map[string]int),
	}

	var evidenceList []Evidence

	if c.auditStore == nil {
		return report, evidenceList, nil
	}

	// Query login events
	loginLogs, err := c.auditStore.List(audit.QueryOptions{
		OrgID:     orgID,
		Action:    audit.ActionLogin,
		StartTime: &period.Start,
		EndTime:   &period.End,
		Limit:     10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("list login logs: %w", err)
	}

	// Analyze login patterns
	uniqueUsers := make(map[string]bool)
	ipStats := make(map[string]*IPStats)
	suspiciousThreshold := 10 // Failed logins threshold

	for _, log := range loginLogs {
		report.TotalLogins++

		if log.Outcome == audit.OutcomeSuccess {
			report.SuccessLogins++
		} else {
			report.FailedLogins++
		}

		uniqueUsers[log.UserID] = true
		report.LoginsByHour[log.Timestamp.Hour()]++
		report.LoginsByDay[log.Timestamp.Format("2006-01-02")]++

		// Track IP statistics
		if log.UserIP != "" {
			if _, exists := ipStats[log.UserIP]; !exists {
				ipStats[log.UserIP] = &IPStats{IP: log.UserIP}
			}
			ipStats[log.UserIP].Count++
			if log.Outcome == audit.OutcomeSuccess {
				ipStats[log.UserIP].Success++
			} else {
				ipStats[log.UserIP].Failed++
			}
		}
	}

	report.UniqueUsers = len(uniqueUsers)

	// Build top IPs and suspicious IPs lists
	for _, stats := range ipStats {
		report.TopIPs = append(report.TopIPs, *stats)

		if stats.Failed >= suspiciousThreshold {
			report.SuspiciousIPs = append(report.SuspiciousIPs, SuspiciousIP{
				IP:           stats.IP,
				FailedLogins: stats.Failed,
				Reason:       fmt.Sprintf("High number of failed login attempts (%d)", stats.Failed),
			})
		}
	}

	// Create evidence
	loginData := make([]map[string]interface{}, len(loginLogs))
	for i, log := range loginLogs {
		loginData[i] = map[string]interface{}{
			"timestamp":  log.Timestamp,
			"user_id":    log.UserID,
			"user_email": log.UserEmail,
			"ip":         log.UserIP,
			"user_agent": log.UserAgent,
			"outcome":    log.Outcome,
		}
	}

	loginEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceLog,
		Title:       "Authentication Log",
		Description: fmt.Sprintf("Authentication events from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        loginData,
		DataSummary: fmt.Sprintf("%d logins (%d successful, %d failed) from %d unique users", report.TotalLogins, report.SuccessLogins, report.FailedLogins, report.UniqueUsers),
		CollectedAt: time.Now(),
		Source:      "audit_store",
		Hash:        hashData(loginData),
	}
	evidenceList = append(evidenceList, loginEvidence)

	return report, evidenceList, nil
}

// CollectSecurityIncidentEvidence collects security alerts for CC7.2
func (c *EvidenceCollector) CollectSecurityIncidentEvidence(period DateRange) ([]Evidence, error) {
	var evidenceList []Evidence

	if c.securityStore == nil {
		return evidenceList, nil
	}

	// Get security statistics (hours-based)
	hours := int(period.End.Sub(period.Start).Hours())
	if hours < 1 {
		hours = 24
	}
	stats, err := c.securityStore.GetStats(hours)
	if err != nil {
		return nil, fmt.Errorf("get security stats: %w", err)
	}

	// Create evidence for security summary
	statsEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceReport,
		Title:       "Security Alert Summary",
		Description: fmt.Sprintf("Security alert statistics from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        stats,
		DataSummary: fmt.Sprintf("%d total events, %d alerts, %d critical", stats.TotalEvents, stats.TotalAlerts, stats.CriticalAlerts),
		CollectedAt: time.Now(),
		Source:      "security_store",
		Hash:        hashData(stats),
	}
	evidenceList = append(evidenceList, statsEvidence)

	// Get security alerts
	alerts, err := c.securityStore.ListAlerts(security.AlertFilter{
		StartTime: period.Start,
		EndTime:   period.End,
		Limit:     1000,
	})
	if err == nil && len(alerts) > 0 {
		alertData := make([]map[string]interface{}, len(alerts))
		for i, alert := range alerts {
			alertData[i] = map[string]interface{}{
				"id":        alert.ID,
				"timestamp": alert.DetectedAt,
				"severity":  alert.Severity,
				"title":     alert.Title,
				"rule_name": alert.RuleName,
				"status":    alert.State,
			}
		}

		alertEvidence := Evidence{
			ID:          uuid.New().String(),
			Type:        EvidenceLog,
			Title:       "Security Alerts",
			Description: fmt.Sprintf("Security alerts detected during the period"),
			Data:        alertData,
			DataSummary: fmt.Sprintf("%d security alerts", len(alerts)),
			CollectedAt: time.Now(),
			Source:      "security_store",
			Hash:        hashData(alertData),
		}
		evidenceList = append(evidenceList, alertEvidence)
	}

	return evidenceList, nil
}

// CollectIncidentResponseEvidence collects incident data for CC7.3
func (c *EvidenceCollector) CollectIncidentResponseEvidence(period DateRange) (*IncidentReport, []Evidence, error) {
	report := &IncidentReport{
		GeneratedAt:         time.Now(),
		Period:              period,
		IncidentsByPriority: make(map[string]int),
	}

	var evidenceList []Evidence

	if c.incidentStore == nil {
		return report, evidenceList, nil
	}

	// Get incidents
	incidentList, err := c.incidentStore.ListIncidents("all", 500)
	if err != nil {
		return nil, nil, fmt.Errorf("list incidents: %w", err)
	}

	var totalResponseTime float64
	var totalResolutionTime float64
	var responseCount int
	var resolutionCount int

	for _, inc := range incidentList {
		// Filter by period
		if inc.CreatedAt.Before(period.Start) || inc.CreatedAt.After(period.End) {
			continue
		}

		report.TotalIncidents++
		report.IncidentsByPriority[string(inc.Severity)]++

		if inc.Status == incidents.StatusResolved {
			report.ClosedIncidents++
			if inc.ResolvedAt != nil {
				resolutionTime := inc.ResolvedAt.Sub(inc.CreatedAt).Hours()
				totalResolutionTime += resolutionTime
				resolutionCount++
			}
		} else {
			report.OpenIncidents++
		}

		// Calculate response time (time to first acknowledgment)
		if inc.AckedAt != nil {
			responseTime := inc.AckedAt.Sub(inc.CreatedAt).Minutes()
			totalResponseTime += responseTime
			responseCount++
		}

		// Add security-related incidents (critical/high severity)
		if inc.Severity == incidents.SeverityCritical || inc.Severity == incidents.SeverityHigh {
			secInc := SecurityIncident{
				ID:          inc.ID,
				Title:       inc.Title,
				Priority:    string(inc.Severity),
				Status:      string(inc.Status),
				CreatedAt:   inc.CreatedAt,
				ResolvedAt:  inc.ResolvedAt,
				Description: inc.Description,
			}
			if inc.AckedAt != nil {
				secInc.ResponseTime = inc.AckedAt.Sub(inc.CreatedAt).Minutes()
			}
			report.SecurityIncidents = append(report.SecurityIncidents, secInc)
		}
	}

	if responseCount > 0 {
		report.AvgResponseTime = totalResponseTime / float64(responseCount)
	}
	if resolutionCount > 0 {
		report.AvgResolutionTime = totalResolutionTime / float64(resolutionCount)
	}

	// Create evidence
	incidentData := map[string]interface{}{
		"total_incidents":        report.TotalIncidents,
		"open_incidents":         report.OpenIncidents,
		"closed_incidents":       report.ClosedIncidents,
		"avg_response_time_min":  report.AvgResponseTime,
		"avg_resolution_time_hr": report.AvgResolutionTime,
		"by_priority":            report.IncidentsByPriority,
		"security_incidents":     report.SecurityIncidents,
	}

	incidentEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceReport,
		Title:       "Incident Response Summary",
		Description: fmt.Sprintf("Incident response metrics from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        incidentData,
		DataSummary: fmt.Sprintf("%d incidents, avg response: %.1f min, avg resolution: %.1f hr", report.TotalIncidents, report.AvgResponseTime, report.AvgResolutionTime),
		CollectedAt: time.Now(),
		Source:      "incident_store",
		Hash:        hashData(incidentData),
	}
	evidenceList = append(evidenceList, incidentEvidence)

	return report, evidenceList, nil
}

// CollectDeploymentEvidence collects deployment logs for CC7.4
func (c *EvidenceCollector) CollectDeploymentEvidence(period DateRange) (*ConfigChangeReport, []Evidence, error) {
	report := &ConfigChangeReport{
		GeneratedAt:   time.Now(),
		Period:        period,
		ChangesByType: make(map[string]int),
		ChangesByUser: make(map[string]int),
	}

	var evidenceList []Evidence

	if c.deploysStore == nil {
		return report, evidenceList, nil
	}

	// Get deployments
	deployments, err := c.deploysStore.ListByTimeRange(period.Start, period.End)
	if err != nil {
		return nil, nil, fmt.Errorf("list deployments: %w", err)
	}

	for _, dep := range deployments {
		report.TotalChanges++
		report.ChangesByUser[dep.User]++

		change := ConfigChange{
			Timestamp:    dep.Timestamp,
			UserID:       dep.User,
			UserEmail:    dep.User,
			ResourceType: "deployment",
			ResourceID:   dep.ID,
			Action:       "deploy",
			After:        fmt.Sprintf("Version: %s, Commit: %s", dep.Version, dep.CommitSHA),
		}

		// Mark failed deployments as critical
		if dep.Status == "failed" || dep.Status == "rolled_back" {
			change.IsCritical = true
			report.CriticalChanges = append(report.CriticalChanges, change)
		}

		report.AllChanges = append(report.AllChanges, change)
	}

	// Create evidence
	deployData := make([]map[string]interface{}, len(deployments))
	for i, dep := range deployments {
		deployData[i] = map[string]interface{}{
			"id":          dep.ID,
			"service":     dep.Service,
			"version":     dep.Version,
			"environment": dep.Environment,
			"timestamp":   dep.Timestamp,
			"user":        dep.User,
			"commit_sha":  dep.CommitSHA,
			"status":      dep.Status,
		}
	}

	deployEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceLog,
		Title:       "Deployment Audit Log",
		Description: fmt.Sprintf("Deployment events from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        deployData,
		DataSummary: fmt.Sprintf("%d deployments", len(deployments)),
		CollectedAt: time.Now(),
		Source:      "deploys_store",
		Hash:        hashData(deployData),
	}
	evidenceList = append(evidenceList, deployEvidence)

	return report, evidenceList, nil
}

// CollectAuditLogEvidence collects comprehensive audit logs for CC8.1
func (c *EvidenceCollector) CollectAuditLogEvidence(orgID string, period DateRange) (*ConfigChangeReport, []Evidence, error) {
	report := &ConfigChangeReport{
		GeneratedAt:   time.Now(),
		Period:        period,
		ChangesByType: make(map[string]int),
		ChangesByUser: make(map[string]int),
	}

	var evidenceList []Evidence

	if c.auditStore == nil {
		return report, evidenceList, nil
	}

	// Get all audit logs for the period
	logs, err := c.auditStore.List(audit.QueryOptions{
		OrgID:     orgID,
		StartTime: &period.Start,
		EndTime:   &period.End,
		Limit:     10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("list audit logs: %w", err)
	}

	for _, log := range logs {
		report.TotalChanges++
		report.ChangesByType[string(log.ResourceType)]++
		report.ChangesByUser[log.UserEmail]++

		change := ConfigChange{
			Timestamp:    log.Timestamp,
			UserID:       log.UserID,
			UserEmail:    log.UserEmail,
			ResourceType: string(log.ResourceType),
			ResourceID:   log.ResourceID,
			Action:       string(log.Action),
		}

		// Mark certain changes as critical
		if log.ResourceType == audit.ResourceSettings ||
			log.ResourceType == audit.ResourceOrg ||
			(log.Action == audit.ActionDelete && log.ResourceType == audit.ResourceUser) {
			change.IsCritical = true
			report.CriticalChanges = append(report.CriticalChanges, change)
		}

		report.AllChanges = append(report.AllChanges, change)
	}

	// Create evidence
	auditData := make([]map[string]interface{}, len(logs))
	for i, log := range logs {
		auditData[i] = map[string]interface{}{
			"id":            log.ID,
			"timestamp":     log.Timestamp,
			"user_id":       log.UserID,
			"user_email":    log.UserEmail,
			"action":        log.Action,
			"resource_type": log.ResourceType,
			"resource_id":   log.ResourceID,
			"outcome":       log.Outcome,
		}
	}

	auditEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceLog,
		Title:       "Comprehensive Audit Log",
		Description: fmt.Sprintf("All audit events from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        auditData,
		DataSummary: fmt.Sprintf("%d audit events", len(logs)),
		CollectedAt: time.Now(),
		Source:      "audit_store",
		Hash:        hashData(auditData),
	}
	evidenceList = append(evidenceList, auditEvidence)

	return report, evidenceList, nil
}

// CollectSLOComplianceEvidence collects SLO data for A1.1
func (c *EvidenceCollector) CollectSLOComplianceEvidence(period DateRange) (*SystemAvailabilityReport, []Evidence, error) {
	report := &SystemAvailabilityReport{
		GeneratedAt:     time.Now(),
		Period:          period,
		UptimeByService: make(map[string]float64),
	}

	var evidenceList []Evidence

	if c.sloStore == nil {
		return report, evidenceList, nil
	}

	// Get all SLOs
	slos, err := c.sloStore.ListSLOs()
	if err != nil {
		return nil, nil, fmt.Errorf("list SLOs: %w", err)
	}

	var totalUptime float64
	var sloCount int

	for _, s := range slos {
		if !s.Enabled {
			continue
		}

		// Get SLO snapshots for the period
		snapshots, err := c.sloStore.GetSnapshots(s.ID, period.End.Sub(period.Start), 100)
		if err != nil || len(snapshots) == 0 {
			continue
		}

		// Calculate average compliance
		var avgValue float64
		for _, snap := range snapshots {
			avgValue += snap.CurrentValue
		}
		avgValue /= float64(len(snapshots))

		compliance := SLOCompliance{
			SLOID:      s.ID,
			SLOName:    s.Name,
			Target:     s.Target,
			Actual:     avgValue,
			Met:        avgValue >= s.Target,
			BudgetUsed: ((100 - avgValue) / (100 - s.Target)) * 100,
		}
		report.SLOCompliance = append(report.SLOCompliance, compliance)

		if s.Type == slo.SLOAvailability {
			totalUptime += avgValue
			sloCount++
			report.UptimeByService[s.ServiceID] = avgValue
		}
	}

	if sloCount > 0 {
		report.OverallUptime = totalUptime / float64(sloCount)
	}

	// Create evidence
	sloData := map[string]interface{}{
		"overall_uptime":  report.OverallUptime,
		"slo_compliance":  report.SLOCompliance,
		"by_service":      report.UptimeByService,
	}

	sloEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceMetric,
		Title:       "SLO Compliance Report",
		Description: fmt.Sprintf("SLO compliance data from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        sloData,
		DataSummary: fmt.Sprintf("%.2f%% overall uptime, %d SLOs tracked", report.OverallUptime, len(report.SLOCompliance)),
		CollectedAt: time.Now(),
		Source:      "slo_store",
		Hash:        hashData(sloData),
	}
	evidenceList = append(evidenceList, sloEvidence)

	return report, evidenceList, nil
}

// CollectBackupEvidence collects backup data for A1.2
func (c *EvidenceCollector) CollectBackupEvidence(period DateRange) (*BackupReport, []Evidence, error) {
	report := &BackupReport{
		GeneratedAt: time.Now(),
		Period:      period,
	}

	var evidenceList []Evidence

	if c.backupDataDir == "" {
		return report, evidenceList, nil
	}

	// List backups
	backups, err := backup.ListBackups(c.backupDataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("list backups: %w", err)
	}

	for _, b := range backups {
		// Filter by period
		if b.ModTime.Before(period.Start) || b.ModTime.After(period.End) {
			continue
		}

		report.TotalBackups++
		report.BackupSize += b.Size

		record := BackupRecord{
			ID:        b.Path,
			Timestamp: b.ModTime,
			Size:      b.Size,
			Status:    "success",
		}

		// Verify backup integrity
		verifyResult, err := backup.Verify(b.Path)
		if err == nil {
			record.Verified = verifyResult.Valid
			if verifyResult.Valid {
				report.SuccessfulBackups++
			} else {
				report.FailedBackups++
				record.Status = "failed"
			}
		}

		report.Backups = append(report.Backups, record)

		// Track most recent backup
		if report.LastBackup == nil || b.ModTime.After(*report.LastBackup) {
			report.LastBackup = &b.ModTime
		}
	}

	// Create evidence
	backupData := map[string]interface{}{
		"total_backups":      report.TotalBackups,
		"successful_backups": report.SuccessfulBackups,
		"failed_backups":     report.FailedBackups,
		"last_backup":        report.LastBackup,
		"total_size_bytes":   report.BackupSize,
		"backups":            report.Backups,
	}

	backupEvidence := Evidence{
		ID:          uuid.New().String(),
		Type:        EvidenceReport,
		Title:       "Backup Verification Report",
		Description: fmt.Sprintf("Backup status from %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Data:        backupData,
		DataSummary: fmt.Sprintf("%d backups (%d verified), last backup: %s", report.TotalBackups, report.SuccessfulBackups, formatTime(report.LastBackup)),
		CollectedAt: time.Now(),
		Source:      "backup_system",
		Hash:        hashData(backupData),
	}
	evidenceList = append(evidenceList, backupEvidence)

	return report, evidenceList, nil
}

// CollectAlertingEvidence collects alert rule configuration evidence
func (c *EvidenceCollector) CollectAlertingEvidence() ([]Evidence, error) {
	var evidenceList []Evidence

	if c.alertManager == nil {
		return evidenceList, nil
	}

	// Get alert rules from alerting store
	rules, err := c.alertManager.Store.ListRules()
	if err != nil {
		return evidenceList, nil
	}

	if len(rules) > 0 {
		ruleData := make([]map[string]interface{}, len(rules))
		for i, rule := range rules {
			ruleData[i] = map[string]interface{}{
				"id":         rule.ID,
				"name":       rule.Name,
				"type":       rule.Type,
				"enabled":    rule.Enabled,
				"severity":   rule.Labels["severity"],
				"condition":  rule.Condition,
				"threshold":  rule.Threshold,
				"created_at": rule.CreatedAt,
			}
		}

		ruleEvidence := Evidence{
			ID:          uuid.New().String(),
			Type:        EvidenceConfig,
			Title:       "Alert Rule Configuration",
			Description: "Configured alert rules for system monitoring",
			Data:        ruleData,
			DataSummary: fmt.Sprintf("%d alert rules configured", len(rules)),
			CollectedAt: time.Now(),
			Source:      "alert_manager",
			Hash:        hashData(ruleData),
		}
		evidenceList = append(evidenceList, ruleEvidence)
	}

	return evidenceList, nil
}

// Helper functions

func (c *EvidenceCollector) collectAccessChanges(period DateRange) ([]AccessChange, error) {
	var changes []AccessChange

	if c.auditStore == nil {
		return changes, nil
	}

	// Get user-related audit events
	logs, err := c.auditStore.List(audit.QueryOptions{
		ResourceType: audit.ResourceUser,
		StartTime:    &period.Start,
		EndTime:      &period.End,
		Limit:        1000,
	})
	if err != nil {
		return nil, err
	}

	for _, log := range logs {
		change := AccessChange{
			Timestamp:  log.Timestamp,
			UserID:     log.ResourceID,
			UserEmail:  log.ResourceName,
			ChangeType: string(log.Action),
			Details:    fmt.Sprintf("%s %s", log.Action, log.ResourceType),
			ChangedBy:  log.UserEmail,
		}
		changes = append(changes, change)
	}

	return changes, nil
}

func hashData(data interface{}) string {
	jsonBytes, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:])
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Format("2006-01-02 15:04:05")
}
