package compliance

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReportGenerator generates compliance reports
type ReportGenerator struct {
	collector *EvidenceCollector
	store     *Store
}

// NewReportGenerator creates a new report generator
func NewReportGenerator(collector *EvidenceCollector, store *Store) *ReportGenerator {
	return &ReportGenerator{
		collector: collector,
		store:     store,
	}
}

// GenerateSOC2Report generates a SOC2 compliance report
func (g *ReportGenerator) GenerateSOC2Report(orgID string, period DateRange, generatedBy string) (*ComplianceReport, error) {
	report := &ComplianceReport{
		ID:          uuid.New().String(),
		Type:        ReportTypeSOC2,
		Title:       fmt.Sprintf("SOC2 Type II Compliance Report - %s to %s", period.Start.Format("2006-01-02"), period.End.Format("2006-01-02")),
		Description: "SOC2 Trust Services Criteria compliance evidence report",
		Period:      period,
		GeneratedAt: time.Now(),
		GeneratedBy: generatedBy,
		Status:      StatusDraft,
		Version:     1,
		Summary: ComplianceSummary{
			ControlsByCategory: make(map[string]CategoryStats),
		},
	}

	// Process each SOC2 control
	for _, control := range SOC2Controls {
		section, evidence, err := g.evaluateSOC2Control(orgID, control, period)
		if err != nil {
			// Log error but continue with other controls
			section = &ReportSection{
				ID:          control.ID,
				Control:     control.ID,
				Title:       control.Title,
				Description: control.Description,
				Category:    control.Category,
				Status:      ControlPending,
				Notes:       fmt.Sprintf("Error collecting evidence: %v", err),
			}
		}

		report.Sections = append(report.Sections, *section)
		report.Evidence = append(report.Evidence, evidence...)
	}

	// Calculate summary
	g.calculateSummary(report)

	// Save report
	if g.store != nil {
		if err := g.store.SaveReport(report); err != nil {
			return nil, fmt.Errorf("save report: %w", err)
		}
	}

	return report, nil
}

// evaluateSOC2Control evaluates a single SOC2 control
func (g *ReportGenerator) evaluateSOC2Control(orgID string, control SOC2Control, period DateRange) (*ReportSection, []Evidence, error) {
	section := &ReportSection{
		ID:          control.ID,
		Control:     control.ID,
		Title:       control.Title,
		Description: control.Description,
		Category:    control.Category,
		Status:      ControlPending,
	}

	var allEvidence []Evidence
	var findings []Finding

	if g.collector == nil {
		section.Status = ControlPending
		section.Notes = "Evidence collector not configured"
		return section, allEvidence, nil
	}

	// Collect evidence based on control
	switch control.ID {
	case "CC6.1": // User Access Management
		userReport, evidence, err := g.collector.CollectUserAccessEvidence(orgID, period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		// Evaluate compliance
		if userReport.TotalUsers > 0 {
			section.Status = ControlCompliant
			if userReport.InactiveUsers > userReport.TotalUsers/4 {
				section.Status = ControlPartial
				findings = append(findings, Finding{
					ID:          uuid.New().String(),
					ControlID:   control.ID,
					Severity:    SeverityLow,
					Title:       "High number of inactive users",
					Description: fmt.Sprintf("%d inactive users out of %d total", userReport.InactiveUsers, userReport.TotalUsers),
					Remediation: "Review and remove inactive user accounts",
					Status:      "open",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
			}
		}

	case "CC6.2": // Authentication Controls
		authReport, evidence, err := g.collector.CollectAuthenticationEvidence(orgID, period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		// Check for suspicious activity
		if len(authReport.SuspiciousIPs) > 0 {
			section.Status = ControlPartial
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityMedium,
				Title:       "Suspicious login activity detected",
				Description: fmt.Sprintf("%d IPs with high failed login attempts", len(authReport.SuspiciousIPs)),
				Remediation: "Investigate and potentially block suspicious IPs",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		}
		// Check failed login ratio
		if authReport.TotalLogins > 0 {
			failRate := float64(authReport.FailedLogins) / float64(authReport.TotalLogins) * 100
			if failRate > 20 {
				findings = append(findings, Finding{
					ID:          uuid.New().String(),
					ControlID:   control.ID,
					Severity:    SeverityLow,
					Title:       "High authentication failure rate",
					Description: fmt.Sprintf("%.1f%% of login attempts failed", failRate),
					Remediation: "Review authentication mechanisms and user guidance",
					Status:      "open",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
			}
		}

	case "CC6.3": // Access Removal
		userReport, evidence, err := g.collector.CollectUserAccessEvidence(orgID, period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if len(userReport.AccessChanges) > 0 {
			// Check if deactivations are being logged
			hasDeactivations := false
			for _, change := range userReport.AccessChanges {
				if change.ChangeType == "deactivate" || change.ChangeType == "delete" {
					hasDeactivations = true
					break
				}
			}
			if !hasDeactivations {
				section.Status = ControlPartial
				section.Notes = "No user deactivations found in audit log"
			}
		}

	case "CC7.1": // Infrastructure Monitoring
		alertEvidence, err := g.collector.CollectAlertingEvidence()
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, alertEvidence...)

		section.Status = ControlCompliant
		if len(alertEvidence) == 0 {
			section.Status = ControlPartial
			section.Notes = "No alerting rules configured"
		}

	case "CC7.2": // Incident Detection
		evidence, err := g.collector.CollectSecurityIncidentEvidence(period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if len(evidence) == 0 {
			section.Status = ControlPartial
			section.Notes = "No security detection rules configured"
		}

	case "CC7.3": // Incident Response
		incidentReport, evidence, err := g.collector.CollectIncidentResponseEvidence(period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if incidentReport.TotalIncidents > 0 && incidentReport.AvgResponseTime > 60 {
			section.Status = ControlPartial
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityMedium,
				Title:       "High average incident response time",
				Description: fmt.Sprintf("Average response time is %.1f minutes", incidentReport.AvgResponseTime),
				Remediation: "Review and improve incident response procedures",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		}

	case "CC7.4": // Change Management
		changeReport, evidence, err := g.collector.CollectDeploymentEvidence(period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if len(changeReport.CriticalChanges) > 0 {
			section.Status = ControlPartial
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityMedium,
				Title:       "Failed deployments detected",
				Description: fmt.Sprintf("%d failed or rolled back deployments", len(changeReport.CriticalChanges)),
				Remediation: "Review deployment processes and testing procedures",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		}

	case "CC8.1": // Change Authorization
		auditReport, evidence, err := g.collector.CollectAuditLogEvidence(orgID, period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if auditReport.TotalChanges == 0 {
			section.Status = ControlPartial
			section.Notes = "No configuration changes logged"
		}

	case "A1.1": // System Availability
		availReport, evidence, err := g.collector.CollectSLOComplianceEvidence(period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if availReport.OverallUptime < 99.0 {
			section.Status = ControlPartial
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityHigh,
				Title:       "Availability below target",
				Description: fmt.Sprintf("Overall uptime is %.2f%%", availReport.OverallUptime),
				Remediation: "Investigate availability issues and improve system resilience",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		}
		// Check SLO compliance
		for _, slo := range availReport.SLOCompliance {
			if !slo.Met {
				section.Status = ControlPartial
				findings = append(findings, Finding{
					ID:          uuid.New().String(),
					ControlID:   control.ID,
					Severity:    SeverityMedium,
					Title:       fmt.Sprintf("SLO not met: %s", slo.SLOName),
					Description: fmt.Sprintf("Target: %.2f%%, Actual: %.2f%%", slo.Target, slo.Actual),
					Remediation: "Review and address SLO violations",
					Status:      "open",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				})
			}
		}

	case "A1.2": // Backup and Recovery
		backupReport, evidence, err := g.collector.CollectBackupEvidence(period)
		if err != nil {
			return nil, nil, err
		}
		allEvidence = append(allEvidence, evidence...)

		section.Status = ControlCompliant
		if backupReport.TotalBackups == 0 {
			section.Status = ControlNonCompliant
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityCritical,
				Title:       "No backups found",
				Description: "No backup records found for the reporting period",
				Remediation: "Implement and verify backup procedures immediately",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		} else if backupReport.FailedBackups > 0 {
			section.Status = ControlPartial
			findings = append(findings, Finding{
				ID:          uuid.New().String(),
				ControlID:   control.ID,
				Severity:    SeverityHigh,
				Title:       "Backup verification failures",
				Description: fmt.Sprintf("%d backup verifications failed", backupReport.FailedBackups),
				Remediation: "Investigate and fix backup verification issues",
				Status:      "open",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			})
		}

	default:
		// For controls without specific evidence collection
		section.Status = ControlPending
		section.Notes = "Manual evidence collection required"
	}

	section.Evidence = allEvidence
	section.Findings = findings
	now := time.Now()
	section.LastTested = &now

	return section, allEvidence, nil
}

// calculateSummary calculates the report summary
func (g *ReportGenerator) calculateSummary(report *ComplianceReport) {
	summary := &report.Summary
	summary.TotalControls = len(report.Sections)

	categoryStats := make(map[string]*CategoryStats)

	for _, section := range report.Sections {
		switch section.Status {
		case ControlCompliant:
			summary.CompliantControls++
		case ControlNonCompliant:
			summary.NonCompliant++
		case ControlPartial:
			summary.PartialControls++
		case ControlNotApplicable:
			summary.NotApplicable++
		case ControlPending:
			summary.PendingControls++
		}

		// Count findings
		for _, finding := range section.Findings {
			if finding.Status == "open" || finding.Status == "in_progress" {
				summary.OpenFindings++
			}
			switch finding.Severity {
			case SeverityCritical:
				summary.CriticalFindings++
			case SeverityHigh:
				summary.HighFindings++
			case SeverityMedium:
				summary.MediumFindings++
			case SeverityLow:
				summary.LowFindings++
			}
		}

		// Track by category
		if _, ok := categoryStats[section.Category]; !ok {
			categoryStats[section.Category] = &CategoryStats{}
		}
		categoryStats[section.Category].Total++
		if section.Status == ControlCompliant {
			categoryStats[section.Category].Compliant++
		}
	}

	// Calculate scores
	applicable := summary.TotalControls - summary.NotApplicable - summary.PendingControls
	if applicable > 0 {
		summary.ComplianceScore = float64(summary.CompliantControls) / float64(applicable) * 100
	}

	// Risk score based on findings
	summary.RiskScore = float64(summary.CriticalFindings*25+summary.HighFindings*10+summary.MediumFindings*3+summary.LowFindings) / float64(summary.TotalControls) * 10
	if summary.RiskScore > 100 {
		summary.RiskScore = 100
	}

	// Copy category stats
	for cat, stats := range categoryStats {
		if stats.Total > 0 {
			stats.Score = float64(stats.Compliant) / float64(stats.Total) * 100
		}
		summary.ControlsByCategory[cat] = *stats
	}
}

// GenerateGapAnalysis generates a gap analysis from a report
func (g *ReportGenerator) GenerateGapAnalysis(reportID string) (*GapAnalysis, error) {
	report, err := g.store.GetReport(reportID)
	if err != nil {
		return nil, fmt.Errorf("get report: %w", err)
	}

	analysis := &GapAnalysis{
		ReportID:    reportID,
		GeneratedAt: time.Now(),
	}

	for _, section := range report.Sections {
		if section.Status == ControlNonCompliant || section.Status == ControlPartial {
			gap := Gap{
				ControlID:     section.ID,
				ControlTitle:  section.Title,
				CurrentState:  string(section.Status),
				RequiredState: "compliant",
				Severity:      g.calculateGapSeverity(section),
				Remediation:   section.Remediation,
			}

			if gap.Remediation == "" && len(section.Findings) > 0 {
				gap.Remediation = section.Findings[0].Remediation
			}

			analysis.Gaps = append(analysis.Gaps, gap)
			analysis.TotalGaps++
			if gap.Severity == SeverityCritical {
				analysis.CriticalGaps++
			}
		}
	}

	// Generate priority actions
	for i, gap := range analysis.Gaps {
		if gap.Severity == SeverityCritical || gap.Severity == SeverityHigh {
			analysis.PriorityActions = append(analysis.PriorityActions, PriorityAction{
				Rank:      i + 1,
				ControlID: gap.ControlID,
				Action:    gap.Remediation,
				Impact:    fmt.Sprintf("Addresses %s gap", gap.Severity),
				Effort:    gap.EstimatedEffort,
			})
		}
	}

	// Generate recommendations
	if analysis.CriticalGaps > 0 {
		analysis.Recommendations = append(analysis.Recommendations,
			"Address critical control gaps immediately as they pose significant compliance risk")
	}
	if report.Summary.PendingControls > 0 {
		analysis.Recommendations = append(analysis.Recommendations,
			"Complete evidence collection for pending controls")
	}
	if report.Summary.OpenFindings > 5 {
		analysis.Recommendations = append(analysis.Recommendations,
			"Prioritize remediation of open findings")
	}

	return analysis, nil
}

// calculateGapSeverity determines gap severity based on section
func (g *ReportGenerator) calculateGapSeverity(section ReportSection) FindingSeverity {
	// Check findings for highest severity
	highestSeverity := SeverityInfo
	for _, finding := range section.Findings {
		if severityRank(finding.Severity) > severityRank(highestSeverity) {
			highestSeverity = finding.Severity
		}
	}

	if highestSeverity != SeverityInfo {
		return highestSeverity
	}

	// Default based on control category
	if section.Status == ControlNonCompliant {
		return SeverityHigh
	}
	return SeverityMedium
}

func severityRank(s FindingSeverity) int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}
