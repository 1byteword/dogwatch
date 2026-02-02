/**
 * Migration Wizard Widget
 * Step-by-step wizard for importing dashboards and alerts from other platforms
 */
class MigrationWizard extends HTMLElement {
    constructor() {
        super();
        this.currentStep = 1;
        this.totalSteps = 5;
        this.selectedSource = null;
        this.uploadedFile = null;
        this.uploadedData = null;
        this.previewData = null;
        this.importResult = null;
        this.importProgress = 0;
        this.importing = false;
        this.error = null;
        this.reports = [];
        this._progressInterval = null; // Track progress interval for cleanup
        this._mounted = false;
        this._boundEventListeners = [];
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadRecentReports();
    }

    disconnectedCallback() {
        this._mounted = false;
        // Clean up any running intervals to prevent memory leaks
        if (this._progressInterval) {
            clearInterval(this._progressInterval);
            this._progressInterval = null;
        }
        // Clean up event listeners
        this._boundEventListeners.forEach(({ element, event, handler }) => {
            if (element) element.removeEventListener(event, handler);
        });
        this._boundEventListeners = [];
    }

    async loadRecentReports() {
        try {
            const resp = await fetch('/api/migration/reports?limit=5');
            if (resp.ok) {
                this.reports = await resp.json();
                this.render();
            }
        } catch (e) {
            console.error('Failed to load reports:', e);
        }
    }

    setSource(source) {
        this.selectedSource = source;
        this.error = null;
        this.render();
    }

    nextStep() {
        if (this.currentStep < this.totalSteps) {
            this.currentStep++;
            this.render();
        }
    }

    prevStep() {
        if (this.currentStep > 1) {
            this.currentStep--;
            this.error = null;
            this.render();
        }
    }

    goToStep(step) {
        this.currentStep = step;
        this.render();
    }

    reset() {
        this.currentStep = 1;
        this.selectedSource = null;
        this.uploadedFile = null;
        this.uploadedData = null;
        this.previewData = null;
        this.importResult = null;
        this.importProgress = 0;
        this.importing = false;
        this.error = null;
        this.render();
    }

    handleFileSelect(event) {
        const file = event.target.files[0];
        if (file) {
            this.uploadFile(file);
        }
    }

    handleDragOver(event) {
        event.preventDefault();
        event.stopPropagation();
        event.currentTarget.classList.add('dragover');
    }

    handleDragLeave(event) {
        event.preventDefault();
        event.stopPropagation();
        event.currentTarget.classList.remove('dragover');
    }

    handleDrop(event) {
        event.preventDefault();
        event.stopPropagation();
        event.currentTarget.classList.remove('dragover');

        const file = event.dataTransfer.files[0];
        if (file) {
            this.uploadFile(file);
        }
    }

    async uploadFile(file) {
        this.uploadedFile = file;
        this.error = null;

        try {
            const text = await file.text();
            this.uploadedData = text;
            await this.previewImport();
            this.render();
        } catch (e) {
            this.error = 'Failed to read file: ' + e.message;
            this.render();
        }
    }

    async previewImport() {
        try {
            const resp = await fetch('/api/migration/preview', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: this.uploadedData
            });

            if (resp.ok) {
                this.previewData = await resp.json();
            } else {
                const errorText = await resp.text();
                this.error = 'Preview failed: ' + errorText;
            }
        } catch (e) {
            this.error = 'Preview failed: ' + e.message;
        }
    }

    async analyzeFidelity() {
        try {
            const resp = await fetch('/api/migration/fidelity/analyze', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: this.uploadedData
            });

            if (resp.ok) {
                const fidelity = await resp.json();
                if (this.previewData) {
                    this.previewData.fidelity = fidelity;
                }
                this.render();
            }
        } catch (e) {
            console.error('Failed to analyze fidelity:', e);
        }
    }

    async startImport() {
        if (this.importing) return;

        this.importing = true;
        this.importProgress = 0;
        this.error = null;
        this.render();

        const endpoint = this.getImportEndpoint();
        if (!endpoint) {
            this.error = 'Unknown import type';
            this.importing = false;
            this.render();
            return;
        }

        try {
            // Simulate progress updates - track interval for cleanup
            this._progressInterval = setInterval(() => {
                if (this.importProgress < 90) {
                    this.importProgress += 10;
                    this.render();
                }
            }, 200);

            const resp = await fetch(endpoint, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: this.uploadedData
            });

            // Clear progress interval
            if (this._progressInterval) {
                clearInterval(this._progressInterval);
                this._progressInterval = null;
            }
            this.importProgress = 100;

            if (resp.ok) {
                const data = await resp.json();
                // Validate response structure before using
                if (data && typeof data === 'object') {
                    this.importResult = data;
                    this.currentStep = 5; // Go to summary
                } else {
                    this.error = 'Import failed: Invalid response format';
                }
            } else {
                const errorText = await resp.text().catch(() => 'Unknown error');
                this.error = 'Import failed: ' + errorText;
            }
        } catch (e) {
            // Ensure interval is cleared on error
            if (this._progressInterval) {
                clearInterval(this._progressInterval);
                this._progressInterval = null;
            }
            this.error = 'Import failed: ' + e.message;
        } finally {
            this.importing = false;
            this.render();
        }
    }

    getImportEndpoint() {
        if (!this.previewData) return null;

        const format = this.previewData.format || this.selectedSource;
        const hasDashboards = this.previewData.items?.some(i => i.type === 'dashboard');
        const hasAlerts = this.previewData.items?.some(i => i.type === 'alert');

        if (hasDashboards) {
            switch (format) {
                case 'datadog': return '/api/migration/datadog/dashboard';
                case 'grafana': return '/api/migration/grafana/dashboard';
                default: return '/api/migration/datadog/dashboard';
            }
        }

        if (hasAlerts) {
            switch (format) {
                case 'datadog': return '/api/migration/datadog/alerts';
                case 'grafana': return '/api/migration/grafana/alerts';
                case 'prometheus': return '/api/migration/prometheus/alerts';
                default: return '/api/migration/alerts';
            }
        }

        return '/api/migration/preview';
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="migration-wizard">
                <div class="wizard-header">
                    <h2>Migration Wizard</h2>
                    <p class="subtitle">Import dashboards and alerts from other platforms</p>
                </div>

                <div class="progress-steps">
                    ${this.renderProgressSteps()}
                </div>

                <div class="wizard-content">
                    ${this.renderStepContent()}
                </div>

                ${this.error ? `<div class="error-message">${this.escapeHtml(this.error)}</div>` : ''}

                <div class="wizard-actions">
                    ${this.renderActions()}
                </div>

                ${this.reports.length > 0 ? this.renderRecentReports() : ''}
            </div>
        `;

        // Re-attach event listeners for file upload
        this.attachEventListeners();
    }

    attachEventListeners() {
        const dropZone = this.querySelector('.drop-zone');
        if (dropZone) {
            const dragOverHandler = (e) => this.handleDragOver(e);
            const dragLeaveHandler = (e) => this.handleDragLeave(e);
            const dropHandler = (e) => this.handleDrop(e);

            dropZone.addEventListener('dragover', dragOverHandler);
            dropZone.addEventListener('dragleave', dragLeaveHandler);
            dropZone.addEventListener('drop', dropHandler);

            this._boundEventListeners.push(
                { element: dropZone, event: 'dragover', handler: dragOverHandler },
                { element: dropZone, event: 'dragleave', handler: dragLeaveHandler },
                { element: dropZone, event: 'drop', handler: dropHandler }
            );
        }

        const fileInput = this.querySelector('input[type="file"]');
        if (fileInput) {
            const changeHandler = (e) => this.handleFileSelect(e);
            fileInput.addEventListener('change', changeHandler);
            this._boundEventListeners.push({ element: fileInput, event: 'change', handler: changeHandler });
        }
    }

    renderProgressSteps() {
        const steps = [
            { num: 1, label: 'Source' },
            { num: 2, label: 'Upload' },
            { num: 3, label: 'Preview' },
            { num: 4, label: 'Import' },
            { num: 5, label: 'Summary' }
        ];

        return steps.map(step => `
            <div class="step ${step.num === this.currentStep ? 'active' : ''} ${step.num < this.currentStep ? 'completed' : ''}">
                <div class="step-number">${step.num < this.currentStep ? '&#10003;' : step.num}</div>
                <div class="step-label">${step.label}</div>
            </div>
        `).join('<div class="step-connector"></div>');
    }

    renderStepContent() {
        switch (this.currentStep) {
            case 1: return this.renderSourceSelection();
            case 2: return this.renderFileUpload();
            case 3: return this.renderPreview();
            case 4: return this.renderImport();
            case 5: return this.renderSummary();
            default: return '';
        }
    }

    renderSourceSelection() {
        const sources = [
            { id: 'datadog', name: 'Datadog', icon: 'D', color: '#632CA6' },
            { id: 'grafana', name: 'Grafana', icon: 'G', color: '#F46800' },
            { id: 'prometheus', name: 'Prometheus', icon: 'P', color: '#E6522C' }
        ];

        return `
            <div class="source-selection">
                <h3>Select Source Platform</h3>
                <p class="help-text">Choose the observability platform you want to import from</p>

                <div class="source-grid">
                    ${sources.map(source => `
                        <div class="source-card ${this.selectedSource === source.id ? 'selected' : ''}"
                             onclick="this.getRootNode().host.setSource('${source.id}')">
                            <div class="source-icon" style="background: ${source.color}">${source.icon}</div>
                            <div class="source-name">${source.name}</div>
                            <div class="source-desc">Import dashboards and alerts</div>
                        </div>
                    `).join('')}
                </div>

                <div class="supported-formats">
                    <h4>Supported Formats</h4>
                    <ul>
                        <li><strong>Datadog:</strong> Dashboard JSON exports, Monitor JSON exports</li>
                        <li><strong>Grafana:</strong> Dashboard JSON exports, Alert rule exports</li>
                        <li><strong>Prometheus:</strong> Alerting rules (YAML), Recording rules</li>
                    </ul>
                </div>
            </div>
        `;
    }

    renderFileUpload() {
        return `
            <div class="file-upload">
                <h3>Upload Configuration File</h3>
                <p class="help-text">Upload a JSON or YAML file exported from ${this.getSourceName()}</p>

                <div class="drop-zone ${this.uploadedFile ? 'has-file' : ''}">
                    ${this.uploadedFile ? `
                        <div class="file-info">
                            <span class="file-icon">&#128196;</span>
                            <span class="file-name">${this.escapeHtml(this.uploadedFile.name)}</span>
                            <span class="file-size">(${this.formatBytes(this.uploadedFile.size)})</span>
                            <button class="btn-clear" onclick="this.getRootNode().host.clearFile()">x</button>
                        </div>
                    ` : `
                        <div class="drop-content">
                            <span class="upload-icon">&#8593;</span>
                            <span class="drop-text">Drag and drop your file here</span>
                            <span class="or-text">or</span>
                            <label class="btn-browse">
                                Browse Files
                                <input type="file" accept=".json,.yaml,.yml" hidden>
                            </label>
                        </div>
                    `}
                </div>

                <div class="upload-tips">
                    <h4>How to export from ${this.getSourceName()}</h4>
                    ${this.renderExportInstructions()}
                </div>
            </div>
        `;
    }

    clearFile() {
        this.uploadedFile = null;
        this.uploadedData = null;
        this.previewData = null;
        this.error = null;
        this.render();
    }

    getSourceName() {
        switch (this.selectedSource) {
            case 'datadog': return 'Datadog';
            case 'grafana': return 'Grafana';
            case 'prometheus': return 'Prometheus';
            default: return 'the source platform';
        }
    }

    renderExportInstructions() {
        switch (this.selectedSource) {
            case 'datadog':
                return `
                    <ol>
                        <li>Open the dashboard or monitor you want to export</li>
                        <li>Click the gear icon (Settings)</li>
                        <li>Select "Export dashboard JSON" or "Export monitor"</li>
                        <li>Save the downloaded file</li>
                    </ol>
                `;
            case 'grafana':
                return `
                    <ol>
                        <li>Open the dashboard you want to export</li>
                        <li>Click the share icon in the top navigation</li>
                        <li>Go to the "Export" tab</li>
                        <li>Click "Save to file" (enable "Export for sharing externally")</li>
                    </ol>
                `;
            case 'prometheus':
                return `
                    <ol>
                        <li>Locate your Prometheus alerting rules file (usually alert.rules.yml)</li>
                        <li>Copy the file to your local machine</li>
                        <li>Upload the YAML file here</li>
                    </ol>
                `;
            default:
                return '<p>Select a source platform for export instructions</p>';
        }
    }

    renderPreview() {
        if (!this.previewData) {
            return '<div class="loading">Loading preview...</div>';
        }

        const items = this.previewData.items || [];
        const warnings = this.previewData.warnings || [];
        const fidelity = this.previewData.estimated_fidelity || this.previewData.fidelity;

        return `
            <div class="preview-section">
                <h3>Import Preview</h3>
                <p class="help-text">Review what will be imported before proceeding</p>

                <div class="preview-header">
                    <div class="format-badge">${this.previewData.format || 'Unknown'}</div>
                    <div class="item-count">${items.length} item(s) found</div>
                </div>

                ${fidelity ? this.renderFidelityScore(fidelity) : ''}

                <div class="preview-items">
                    <h4>Items to Import</h4>
                    ${items.length === 0 ? '<div class="no-items">No items found in the file</div>' : ''}
                    ${items.map(item => this.renderPreviewItem(item)).join('')}
                </div>

                ${warnings.length > 0 ? `
                    <div class="warnings-section">
                        <h4>Warnings</h4>
                        <ul class="warnings-list">
                            ${warnings.map(w => `<li>${this.escapeHtml(w)}</li>`).join('')}
                        </ul>
                    </div>
                ` : ''}
            </div>
        `;
    }

    renderFidelityScore(fidelity) {
        const overall = fidelity.overall || 0;
        const scoreClass = overall >= 80 ? 'high' : overall >= 50 ? 'medium' : 'low';

        return `
            <div class="fidelity-score">
                <h4>Estimated Conversion Fidelity</h4>
                <div class="score-display ${scoreClass}">
                    <div class="score-value">${overall.toFixed(0)}%</div>
                    <div class="score-label">Overall Score</div>
                </div>
                <div class="score-breakdown">
                    <div class="score-item">
                        <span class="label">Query Fidelity</span>
                        <div class="mini-bar"><div class="fill" style="width: ${fidelity.query_fidelity || 0}%"></div></div>
                        <span class="value">${(fidelity.query_fidelity || 0).toFixed(0)}%</span>
                    </div>
                    <div class="score-item">
                        <span class="label">Widget Fidelity</span>
                        <div class="mini-bar"><div class="fill" style="width: ${fidelity.widget_fidelity || 0}%"></div></div>
                        <span class="value">${(fidelity.widget_fidelity || 0).toFixed(0)}%</span>
                    </div>
                    <div class="score-item">
                        <span class="label">Style Fidelity</span>
                        <div class="mini-bar"><div class="fill" style="width: ${fidelity.style_fidelity || 0}%"></div></div>
                        <span class="value">${(fidelity.style_fidelity || 0).toFixed(0)}%</span>
                    </div>
                </div>
                ${fidelity.suggestions?.length > 0 ? `
                    <div class="suggestions">
                        <strong>Suggestions:</strong>
                        <ul>${fidelity.suggestions.map(s => `<li>${this.escapeHtml(s)}</li>`).join('')}</ul>
                    </div>
                ` : ''}
            </div>
        `;
    }

    renderPreviewItem(item) {
        const icon = item.type === 'dashboard' ? '&#128202;' : '&#128276;';
        const typeLabel = item.type === 'dashboard' ? 'Dashboard' : 'Alert';

        return `
            <div class="preview-item">
                <span class="item-icon">${icon}</span>
                <div class="item-info">
                    <span class="item-name">${this.escapeHtml(item.name)}</span>
                    <span class="item-type">${typeLabel}</span>
                    ${item.widget_count !== undefined ? `<span class="item-detail">${item.widget_count} widgets</span>` : ''}
                    ${item.variable_count !== undefined ? `<span class="item-detail">${item.variable_count} variables</span>` : ''}
                    ${item.alert_type ? `<span class="item-detail">Type: ${item.alert_type}</span>` : ''}
                </div>
                <span class="item-status ${item.convertible !== false ? 'convertible' : 'unsupported'}">
                    ${item.convertible !== false ? 'Ready' : 'Unsupported'}
                </span>
            </div>
        `;
    }

    renderImport() {
        return `
            <div class="import-section">
                <h3>Import in Progress</h3>

                <div class="import-progress">
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${this.importProgress}%"></div>
                    </div>
                    <div class="progress-text">${this.importProgress}% Complete</div>
                </div>

                <div class="import-status">
                    ${this.importing ? `
                        <div class="status-item processing">
                            <span class="spinner"></span>
                            <span>Processing import...</span>
                        </div>
                    ` : `
                        <div class="status-item">
                            <span class="check">&#10003;</span>
                            <span>Ready to import ${this.previewData?.items?.length || 0} item(s)</span>
                        </div>
                    `}
                </div>

                ${!this.importing && this.importProgress === 0 ? `
                    <div class="import-options">
                        <h4>Import Options</h4>
                        <label class="option">
                            <input type="checkbox" id="skip-unsupported" checked>
                            <span>Skip unsupported widgets/features</span>
                        </label>
                        <label class="option">
                            <input type="checkbox" id="enable-alerts">
                            <span>Enable imported alerts immediately</span>
                        </label>
                    </div>
                ` : ''}
            </div>
        `;
    }

    renderSummary() {
        const result = this.importResult || {};
        const success = result.success;
        const dashboard = result.dashboard || {};
        const alerts = result.alerts || {};
        const warnings = result.warnings || [];

        return `
            <div class="summary-section">
                <div class="summary-header ${success ? 'success' : 'error'}">
                    <span class="summary-icon">${success ? '&#10003;' : '&#10007;'}</span>
                    <h3>${success ? 'Import Completed Successfully' : 'Import Failed'}</h3>
                </div>

                <div class="summary-stats">
                    ${dashboard.id ? `
                        <div class="stat-card">
                            <div class="stat-icon">&#128202;</div>
                            <div class="stat-label">Dashboard</div>
                            <div class="stat-value">${this.escapeHtml(dashboard.name || 'Imported')}</div>
                            <div class="stat-detail">
                                ${dashboard.widgets_converted || 0}/${dashboard.widgets_total || 0} widgets
                            </div>
                        </div>
                    ` : ''}
                    ${alerts.total !== undefined ? `
                        <div class="stat-card">
                            <div class="stat-icon">&#128276;</div>
                            <div class="stat-label">Alerts</div>
                            <div class="stat-value">${alerts.imported || 0} imported</div>
                            <div class="stat-detail">
                                ${alerts.failed || 0} failed, ${alerts.total || 0} total
                            </div>
                        </div>
                    ` : ''}
                    ${result.duration ? `
                        <div class="stat-card">
                            <div class="stat-icon">&#128336;</div>
                            <div class="stat-label">Duration</div>
                            <div class="stat-value">${result.duration}</div>
                        </div>
                    ` : ''}
                </div>

                ${warnings.length > 0 ? `
                    <div class="summary-warnings">
                        <h4>Warnings (${warnings.length})</h4>
                        <ul>
                            ${warnings.slice(0, 10).map(w => `<li>${this.escapeHtml(w)}</li>`).join('')}
                            ${warnings.length > 10 ? `<li>...and ${warnings.length - 10} more</li>` : ''}
                        </ul>
                    </div>
                ` : ''}

                ${result.report_id ? `
                    <div class="summary-links">
                        <a href="/api/migration/report/${result.report_id}" target="_blank" class="link">View Full Report</a>
                        ${dashboard.id ? `<a href="/dashboards/${dashboard.id}" target="_blank" class="link">View Dashboard</a>` : ''}
                    </div>
                ` : ''}
            </div>
        `;
    }

    renderActions() {
        const canProceed = this.canProceedToNextStep();

        switch (this.currentStep) {
            case 1:
                return `
                    <button class="btn-secondary" onclick="this.getRootNode().host.reset()">Cancel</button>
                    <button class="btn-primary ${!canProceed ? 'disabled' : ''}"
                            onclick="this.getRootNode().host.nextStep()" ${!canProceed ? 'disabled' : ''}>
                        Next: Upload File
                    </button>
                `;
            case 2:
                return `
                    <button class="btn-secondary" onclick="this.getRootNode().host.prevStep()">Back</button>
                    <button class="btn-primary ${!canProceed ? 'disabled' : ''}"
                            onclick="this.getRootNode().host.nextStep()" ${!canProceed ? 'disabled' : ''}>
                        Next: Preview
                    </button>
                `;
            case 3:
                return `
                    <button class="btn-secondary" onclick="this.getRootNode().host.prevStep()">Back</button>
                    <button class="btn-primary ${!canProceed ? 'disabled' : ''}"
                            onclick="this.getRootNode().host.nextStep()" ${!canProceed ? 'disabled' : ''}>
                        Next: Import
                    </button>
                `;
            case 4:
                return `
                    <button class="btn-secondary" onclick="this.getRootNode().host.prevStep()" ${this.importing ? 'disabled' : ''}>Back</button>
                    <button class="btn-primary ${this.importing ? 'disabled' : ''}"
                            onclick="this.getRootNode().host.startImport()" ${this.importing ? 'disabled' : ''}>
                        ${this.importing ? 'Importing...' : 'Start Import'}
                    </button>
                `;
            case 5:
                return `
                    <button class="btn-primary" onclick="this.getRootNode().host.reset()">
                        Start New Migration
                    </button>
                `;
            default:
                return '';
        }
    }

    canProceedToNextStep() {
        switch (this.currentStep) {
            case 1: return !!this.selectedSource;
            case 2: return !!this.uploadedFile && !!this.previewData;
            case 3: return this.previewData?.items?.length > 0;
            case 4: return !this.importing;
            default: return true;
        }
    }

    renderRecentReports() {
        return `
            <div class="recent-reports">
                <h4>Recent Migrations</h4>
                <div class="reports-list">
                    ${this.reports.map(report => `
                        <div class="report-item">
                            <span class="report-source">${report.source || 'Unknown'}</span>
                            <span class="report-stats">
                                ${report.dashboards_imported || 0} dashboards, ${report.alerts_imported || 0} alerts
                            </span>
                            <span class="report-date">${this.formatDate(report.completed_at)}</span>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    formatDate(timestamp) {
        if (!timestamp) return '';
        const date = new Date(timestamp);
        return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    getStyles() {
        return `
            .migration-wizard {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                padding: 1.5rem;
                max-width: 800px;
                margin: 0 auto;
            }

            .wizard-header {
                text-align: center;
                margin-bottom: 1.5rem;
            }

            .wizard-header h2 {
                margin: 0;
                font-size: 1.5rem;
            }

            .subtitle {
                color: var(--text-muted, #71767b);
                margin-top: 0.5rem;
            }

            .progress-steps {
                display: flex;
                justify-content: center;
                align-items: center;
                margin-bottom: 2rem;
            }

            .step {
                display: flex;
                flex-direction: column;
                align-items: center;
                gap: 0.5rem;
            }

            .step-number {
                width: 32px;
                height: 32px;
                border-radius: 50%;
                background: var(--bg-elevated, #1e2128);
                border: 2px solid var(--border, #2f3336);
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
                font-size: 0.85rem;
            }

            .step.active .step-number {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
                color: white;
            }

            .step.completed .step-number {
                background: var(--success, #00ba7c);
                border-color: var(--success, #00ba7c);
                color: white;
            }

            .step-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .step.active .step-label {
                color: var(--text, #e7e9ea);
            }

            .step-connector {
                width: 60px;
                height: 2px;
                background: var(--border, #2f3336);
                margin: 0 0.5rem;
                margin-bottom: 1.5rem;
            }

            .wizard-content {
                min-height: 400px;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                margin-bottom: 1rem;
            }

            .wizard-content h3 {
                margin: 0 0 0.5rem 0;
            }

            .help-text {
                color: var(--text-muted, #71767b);
                font-size: 0.9rem;
                margin-bottom: 1.5rem;
            }

            /* Source selection */
            .source-grid {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 1rem;
                margin-bottom: 2rem;
            }

            .source-card {
                background: var(--bg-card, #16181c);
                border: 2px solid var(--border, #2f3336);
                border-radius: 8px;
                padding: 1.5rem;
                text-align: center;
                cursor: pointer;
                transition: all 0.2s;
            }

            .source-card:hover {
                border-color: var(--accent, #1d9bf0);
            }

            .source-card.selected {
                border-color: var(--accent, #1d9bf0);
                background: rgba(29, 155, 240, 0.1);
            }

            .source-icon {
                width: 48px;
                height: 48px;
                border-radius: 8px;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 1.5rem;
                font-weight: bold;
                color: white;
                margin: 0 auto 0.75rem;
            }

            .source-name {
                font-weight: 600;
                margin-bottom: 0.25rem;
            }

            .source-desc {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .supported-formats {
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
            }

            .supported-formats h4 {
                margin: 0 0 0.5rem 0;
                font-size: 0.9rem;
            }

            .supported-formats ul {
                margin: 0;
                padding-left: 1.5rem;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .supported-formats li {
                margin-bottom: 0.25rem;
            }

            /* File upload */
            .drop-zone {
                border: 2px dashed var(--border, #2f3336);
                border-radius: 8px;
                padding: 3rem;
                text-align: center;
                cursor: pointer;
                transition: all 0.2s;
            }

            .drop-zone:hover, .drop-zone.dragover {
                border-color: var(--accent, #1d9bf0);
                background: rgba(29, 155, 240, 0.05);
            }

            .drop-zone.has-file {
                border-style: solid;
                border-color: var(--success, #00ba7c);
                padding: 1.5rem;
            }

            .drop-content {
                display: flex;
                flex-direction: column;
                align-items: center;
                gap: 0.75rem;
            }

            .upload-icon {
                font-size: 2.5rem;
                color: var(--text-muted, #71767b);
            }

            .drop-text {
                font-size: 1rem;
                color: var(--text-muted, #71767b);
            }

            .or-text {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .btn-browse {
                background: var(--accent, #1d9bf0);
                color: white;
                padding: 0.5rem 1rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.9rem;
            }

            .file-info {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.5rem;
            }

            .file-icon {
                font-size: 1.5rem;
            }

            .file-name {
                font-weight: 500;
            }

            .file-size {
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }

            .btn-clear {
                background: transparent;
                border: none;
                color: var(--error, #f4212e);
                cursor: pointer;
                font-size: 1rem;
                padding: 0.25rem 0.5rem;
            }

            .upload-tips {
                margin-top: 1.5rem;
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
            }

            .upload-tips h4 {
                margin: 0 0 0.5rem 0;
                font-size: 0.9rem;
            }

            .upload-tips ol {
                margin: 0;
                padding-left: 1.5rem;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .upload-tips li {
                margin-bottom: 0.25rem;
            }

            /* Preview */
            .preview-section h4 {
                margin: 1rem 0 0.5rem 0;
                font-size: 0.9rem;
            }

            .preview-header {
                display: flex;
                align-items: center;
                gap: 1rem;
                margin-bottom: 1rem;
            }

            .format-badge {
                background: var(--accent, #1d9bf0);
                color: white;
                padding: 0.25rem 0.75rem;
                border-radius: 4px;
                font-size: 0.85rem;
                text-transform: capitalize;
            }

            .item-count {
                color: var(--text-muted, #71767b);
                font-size: 0.9rem;
            }

            .fidelity-score {
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
                margin-bottom: 1rem;
            }

            .fidelity-score h4 {
                margin: 0 0 0.75rem 0;
            }

            .score-display {
                text-align: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                margin-bottom: 1rem;
            }

            .score-display.high .score-value { color: var(--success, #00ba7c); }
            .score-display.medium .score-value { color: var(--warning, #ffd400); }
            .score-display.low .score-value { color: var(--error, #f4212e); }

            .score-value {
                font-size: 2.5rem;
                font-weight: 700;
            }

            .score-label {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .score-breakdown {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .score-item {
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }

            .score-item .label {
                flex: 0 0 100px;
                font-size: 0.85rem;
            }

            .mini-bar {
                flex: 1;
                height: 8px;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                overflow: hidden;
            }

            .mini-bar .fill {
                height: 100%;
                background: var(--accent, #1d9bf0);
            }

            .score-item .value {
                flex: 0 0 40px;
                text-align: right;
                font-size: 0.85rem;
            }

            .suggestions {
                margin-top: 1rem;
                font-size: 0.85rem;
            }

            .suggestions ul {
                margin: 0.5rem 0 0;
                padding-left: 1.5rem;
                color: var(--text-muted, #71767b);
            }

            .preview-items {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                padding: 1rem;
            }

            .no-items {
                color: var(--text-muted, #71767b);
                text-align: center;
                padding: 1rem;
            }

            .preview-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                margin-bottom: 0.5rem;
            }

            .item-icon {
                font-size: 1.5rem;
            }

            .item-info {
                flex: 1;
            }

            .item-name {
                font-weight: 500;
                display: block;
            }

            .item-type, .item-detail {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-right: 0.5rem;
            }

            .item-status {
                font-size: 0.75rem;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
            }

            .item-status.convertible {
                background: rgba(0, 186, 124, 0.2);
                color: #00ba7c;
            }

            .item-status.unsupported {
                background: rgba(244, 33, 46, 0.2);
                color: #f4212e;
            }

            .warnings-section {
                margin-top: 1rem;
            }

            .warnings-list {
                margin: 0;
                padding-left: 1.5rem;
                font-size: 0.85rem;
                color: var(--warning, #ffd400);
            }

            /* Import */
            .import-section {
                text-align: center;
            }

            .import-progress {
                margin: 2rem 0;
            }

            .progress-bar {
                height: 12px;
                background: var(--bg-card, #16181c);
                border-radius: 6px;
                overflow: hidden;
            }

            .progress-fill {
                height: 100%;
                background: var(--accent, #1d9bf0);
                transition: width 0.3s;
            }

            .progress-text {
                margin-top: 0.5rem;
                font-size: 0.9rem;
                color: var(--text-muted, #71767b);
            }

            .import-status {
                margin: 2rem 0;
            }

            .status-item {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.5rem;
            }

            .spinner {
                width: 20px;
                height: 20px;
                border: 2px solid var(--border, #2f3336);
                border-top-color: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: spin 1s linear infinite;
            }

            @keyframes spin {
                to { transform: rotate(360deg); }
            }

            .check {
                color: var(--success, #00ba7c);
            }

            .import-options {
                text-align: left;
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
            }

            .import-options h4 {
                margin: 0 0 0.75rem 0;
            }

            .option {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.5rem;
                font-size: 0.9rem;
                cursor: pointer;
            }

            /* Summary */
            .summary-header {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.75rem;
                padding: 1.5rem;
                border-radius: 8px;
                margin-bottom: 1.5rem;
            }

            .summary-header.success {
                background: rgba(0, 186, 124, 0.1);
                color: var(--success, #00ba7c);
            }

            .summary-header.error {
                background: rgba(244, 33, 46, 0.1);
                color: var(--error, #f4212e);
            }

            .summary-icon {
                font-size: 2rem;
            }

            .summary-header h3 {
                margin: 0;
            }

            .summary-stats {
                display: grid;
                grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
                gap: 1rem;
                margin-bottom: 1.5rem;
            }

            .stat-card {
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
                text-align: center;
            }

            .stat-icon {
                font-size: 1.5rem;
                margin-bottom: 0.5rem;
            }

            .stat-label {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .stat-value {
                font-size: 1.1rem;
                font-weight: 600;
                margin: 0.25rem 0;
            }

            .stat-detail {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .summary-warnings {
                background: var(--bg-card, #16181c);
                padding: 1rem;
                border-radius: 8px;
                margin-bottom: 1rem;
            }

            .summary-warnings h4 {
                margin: 0 0 0.5rem 0;
            }

            .summary-warnings ul {
                margin: 0;
                padding-left: 1.5rem;
                font-size: 0.85rem;
                color: var(--warning, #ffd400);
            }

            .summary-links {
                display: flex;
                gap: 1rem;
                justify-content: center;
            }

            .link {
                color: var(--accent, #1d9bf0);
                text-decoration: none;
            }

            .link:hover {
                text-decoration: underline;
            }

            /* Actions */
            .wizard-actions {
                display: flex;
                justify-content: flex-end;
                gap: 0.75rem;
            }

            .btn-secondary, .btn-primary {
                padding: 0.75rem 1.5rem;
                border-radius: 4px;
                font-size: 0.9rem;
                cursor: pointer;
            }

            .btn-secondary {
                background: transparent;
                border: 1px solid var(--border, #2f3336);
                color: var(--text, #e7e9ea);
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                border: none;
                color: white;
            }

            .btn-primary.disabled, .btn-primary:disabled {
                background: var(--border, #2f3336);
                cursor: not-allowed;
            }

            /* Error message */
            .error-message {
                background: rgba(244, 33, 46, 0.1);
                color: var(--error, #f4212e);
                padding: 0.75rem 1rem;
                border-radius: 4px;
                margin-bottom: 1rem;
                font-size: 0.9rem;
            }

            /* Recent reports */
            .recent-reports {
                margin-top: 2rem;
                padding-top: 1.5rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .recent-reports h4 {
                margin: 0 0 0.75rem 0;
                font-size: 0.9rem;
                color: var(--text-muted, #71767b);
            }

            .reports-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .report-item {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.5rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                font-size: 0.85rem;
            }

            .report-source {
                text-transform: capitalize;
                font-weight: 500;
                flex: 0 0 80px;
            }

            .report-stats {
                flex: 1;
                color: var(--text-muted, #71767b);
            }

            .report-date {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            @media (max-width: 600px) {
                .source-grid {
                    grid-template-columns: 1fr;
                }

                .progress-steps {
                    flex-wrap: wrap;
                }

                .step-connector {
                    display: none;
                }
            }
        `;
    }
}

customElements.define('migration-wizard', MigrationWizard);
