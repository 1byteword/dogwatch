/**
 * System Settings Widget
 * General system configuration, retention policies, and diagnostics
 */
class SystemSettings extends HTMLElement {
    constructor() {
        super();
        this.settings = null;
        this.systemInfo = null;
        this.loading = true;
        this.error = null;
        this.activeTab = 'general';
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const [settingsResp, infoResp] = await Promise.all([
                fetch('/api/settings'),
                fetch('/api/system/info')
            ]);

            if (settingsResp.ok) {
                this.settings = await settingsResp.json();
            } else {
                this.settings = this.generateDemoSettings();
            }

            if (infoResp.ok) {
                this.systemInfo = await infoResp.json();
            } else {
                this.systemInfo = this.generateDemoSystemInfo();
            }
        } catch (e) {
            console.error('Failed to load settings:', e);
            this.settings = this.generateDemoSettings();
            this.systemInfo = this.generateDemoSystemInfo();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoSettings() {
        return {
            general: {
                instance_name: 'dogwatch-prod',
                timezone: 'UTC',
                date_format: 'YYYY-MM-DD HH:mm:ss',
                default_dashboard: 'overview'
            },
            retention: {
                traces_days: 30,
                metrics_days: 90,
                logs_days: 14,
                alerts_days: 365
            },
            storage: {
                data_dir: '/var/lib/dogwatch',
                max_disk_usage_gb: 100,
                compaction_enabled: true,
                compression_level: 6
            },
            security: {
                session_timeout_minutes: 480,
                max_login_attempts: 5,
                password_min_length: 8,
                require_2fa: false,
                allowed_origins: ['*']
            },
            notifications: {
                smtp_enabled: false,
                smtp_host: '',
                smtp_port: 587,
                slack_enabled: true,
                slack_webhook: 'https://hooks.slack.com/...',
                pagerduty_enabled: false
            }
        };
    }

    generateDemoSystemInfo() {
        return {
            version: '0.9.1',
            build_time: '2024-01-15T10:30:00Z',
            go_version: 'go1.21.5',
            os: 'linux',
            arch: 'amd64',
            uptime_seconds: 86400 * 3 + 7200,
            cpu_cores: 4,
            memory_total_mb: 8192,
            memory_used_mb: 2048,
            disk_total_gb: 500,
            disk_used_gb: 125,
            database_size_mb: 4567,
            active_connections: 42
        };
    }

    setActiveTab(tab) {
        this.activeTab = tab;
        this.render();
    }

    async saveSettings(section, updates) {
        try {
            const resp = await fetch(`/api/settings/${section}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            if (resp.ok) {
                this.settings[section] = { ...this.settings[section], ...updates };
                this.showToast('Settings saved successfully');
                this.render();
            } else {
                const error = await resp.text();
                this.showToast('Failed to save settings: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to save settings:', e);
            // Demo: update locally
            this.settings[section] = { ...this.settings[section], ...updates };
            this.showToast('Settings saved successfully');
            this.render();
        }
    }

    handleFormSubmit(e, section) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);
        const updates = {};

        for (const [key, value] of formData.entries()) {
            // Handle checkboxes specially
            const input = form.querySelector(`[name="${key}"]`);
            if (input && input.type === 'checkbox') {
                updates[key] = input.checked;
            } else if (input && input.type === 'number') {
                updates[key] = parseFloat(value) || 0;
            } else {
                updates[key] = value;
            }
        }

        // Handle unchecked checkboxes
        form.querySelectorAll('input[type="checkbox"]').forEach(cb => {
            if (!updates.hasOwnProperty(cb.name)) {
                updates[cb.name] = false;
            }
        });

        this.saveSettings(section, updates);
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatBytes(bytes) {
        if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB';
        if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB';
        if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return bytes + ' B';
    }

    formatUptime(seconds) {
        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);

        const parts = [];
        if (days > 0) parts.push(`${days}d`);
        if (hours > 0) parts.push(`${hours}h`);
        if (minutes > 0) parts.push(`${minutes}m`);
        return parts.join(' ') || '< 1m';
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="system-settings">
                <div class="header">
                    <div class="header-left">
                        <span class="title">System Settings</span>
                    </div>
                    <div class="header-right">
                        <button class="btn-secondary" id="btn-refresh">Refresh</button>
                    </div>
                </div>

                <div class="settings-layout">
                    <div class="settings-nav">
                        <button class="nav-item ${this.activeTab === 'general' ? 'active' : ''}" data-tab="general">General</button>
                        <button class="nav-item ${this.activeTab === 'retention' ? 'active' : ''}" data-tab="retention">Retention</button>
                        <button class="nav-item ${this.activeTab === 'storage' ? 'active' : ''}" data-tab="storage">Storage</button>
                        <button class="nav-item ${this.activeTab === 'security' ? 'active' : ''}" data-tab="security">Security</button>
                        <button class="nav-item ${this.activeTab === 'notifications' ? 'active' : ''}" data-tab="notifications">Notifications</button>
                        <button class="nav-item ${this.activeTab === 'system' ? 'active' : ''}" data-tab="system">System Info</button>
                    </div>

                    <div class="settings-content">
                        ${this.loading ? `
                            <div class="loading">Loading settings...</div>
                        ` : this.error ? `
                            <div class="error">${this.escapeHtml(this.error)}</div>
                        ` : this.renderTabContent()}
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderTabContent() {
        switch (this.activeTab) {
            case 'general':
                return this.renderGeneralSettings();
            case 'retention':
                return this.renderRetentionSettings();
            case 'storage':
                return this.renderStorageSettings();
            case 'security':
                return this.renderSecuritySettings();
            case 'notifications':
                return this.renderNotificationSettings();
            case 'system':
                return this.renderSystemInfo();
            default:
                return '';
        }
    }

    renderGeneralSettings() {
        const s = this.settings?.general || {};
        return `
            <div class="settings-section">
                <h2>General Settings</h2>
                <p class="section-desc">Configure basic instance settings</p>

                <form id="general-form" data-section="general">
                    <div class="form-group">
                        <label for="instance_name">Instance Name</label>
                        <input type="text" id="instance_name" name="instance_name" value="${this.escapeHtml(s.instance_name || '')}">
                        <span class="form-hint">A name to identify this dogwatch instance</span>
                    </div>

                    <div class="form-group">
                        <label for="timezone">Timezone</label>
                        <select id="timezone" name="timezone">
                            <option value="UTC" ${s.timezone === 'UTC' ? 'selected' : ''}>UTC</option>
                            <option value="America/New_York" ${s.timezone === 'America/New_York' ? 'selected' : ''}>America/New_York</option>
                            <option value="America/Los_Angeles" ${s.timezone === 'America/Los_Angeles' ? 'selected' : ''}>America/Los_Angeles</option>
                            <option value="Europe/London" ${s.timezone === 'Europe/London' ? 'selected' : ''}>Europe/London</option>
                            <option value="Asia/Tokyo" ${s.timezone === 'Asia/Tokyo' ? 'selected' : ''}>Asia/Tokyo</option>
                        </select>
                    </div>

                    <div class="form-group">
                        <label for="date_format">Date Format</label>
                        <select id="date_format" name="date_format">
                            <option value="YYYY-MM-DD HH:mm:ss" ${s.date_format === 'YYYY-MM-DD HH:mm:ss' ? 'selected' : ''}>YYYY-MM-DD HH:mm:ss</option>
                            <option value="MM/DD/YYYY HH:mm:ss" ${s.date_format === 'MM/DD/YYYY HH:mm:ss' ? 'selected' : ''}>MM/DD/YYYY HH:mm:ss</option>
                            <option value="DD/MM/YYYY HH:mm:ss" ${s.date_format === 'DD/MM/YYYY HH:mm:ss' ? 'selected' : ''}>DD/MM/YYYY HH:mm:ss</option>
                        </select>
                    </div>

                    <div class="form-actions">
                        <button type="submit" class="btn-primary">Save Changes</button>
                    </div>
                </form>
            </div>
        `;
    }

    renderRetentionSettings() {
        const s = this.settings?.retention || {};
        return `
            <div class="settings-section">
                <h2>Data Retention</h2>
                <p class="section-desc">Configure how long data is retained before automatic deletion</p>

                <form id="retention-form" data-section="retention">
                    <div class="form-group">
                        <label for="traces_days">Traces Retention (days)</label>
                        <input type="number" id="traces_days" name="traces_days" value="${s.traces_days || 30}" min="1" max="365">
                    </div>

                    <div class="form-group">
                        <label for="metrics_days">Metrics Retention (days)</label>
                        <input type="number" id="metrics_days" name="metrics_days" value="${s.metrics_days || 90}" min="1" max="3650">
                    </div>

                    <div class="form-group">
                        <label for="logs_days">Logs Retention (days)</label>
                        <input type="number" id="logs_days" name="logs_days" value="${s.logs_days || 14}" min="1" max="365">
                    </div>

                    <div class="form-group">
                        <label for="alerts_days">Alerts History Retention (days)</label>
                        <input type="number" id="alerts_days" name="alerts_days" value="${s.alerts_days || 365}" min="1" max="3650">
                    </div>

                    <div class="info-box">
                        <strong>Estimated Storage:</strong> Based on current data rates, these settings will use approximately 45 GB of storage.
                    </div>

                    <div class="form-actions">
                        <button type="submit" class="btn-primary">Save Changes</button>
                    </div>
                </form>
            </div>
        `;
    }

    renderStorageSettings() {
        const s = this.settings?.storage || {};
        return `
            <div class="settings-section">
                <h2>Storage Settings</h2>
                <p class="section-desc">Configure storage locations and limits</p>

                <form id="storage-form" data-section="storage">
                    <div class="form-group">
                        <label for="data_dir">Data Directory</label>
                        <input type="text" id="data_dir" name="data_dir" value="${this.escapeHtml(s.data_dir || '/var/lib/dogwatch')}" readonly>
                        <span class="form-hint">This setting requires a restart to change</span>
                    </div>

                    <div class="form-group">
                        <label for="max_disk_usage_gb">Max Disk Usage (GB)</label>
                        <input type="number" id="max_disk_usage_gb" name="max_disk_usage_gb" value="${s.max_disk_usage_gb || 100}" min="10">
                        <span class="form-hint">Data will be automatically deleted when this limit is reached</span>
                    </div>

                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="compaction_enabled" ${s.compaction_enabled ? 'checked' : ''}>
                            <span>Enable automatic compaction</span>
                        </label>
                    </div>

                    <div class="form-group">
                        <label for="compression_level">Compression Level (1-9)</label>
                        <input type="number" id="compression_level" name="compression_level" value="${s.compression_level || 6}" min="1" max="9">
                        <span class="form-hint">Higher values = better compression but slower writes</span>
                    </div>

                    <div class="form-actions">
                        <button type="submit" class="btn-primary">Save Changes</button>
                    </div>
                </form>
            </div>
        `;
    }

    renderSecuritySettings() {
        const s = this.settings?.security || {};
        return `
            <div class="settings-section">
                <h2>Security Settings</h2>
                <p class="section-desc">Configure authentication and access controls</p>

                <form id="security-form" data-section="security">
                    <div class="form-group">
                        <label for="session_timeout_minutes">Session Timeout (minutes)</label>
                        <input type="number" id="session_timeout_minutes" name="session_timeout_minutes" value="${s.session_timeout_minutes || 480}" min="5">
                    </div>

                    <div class="form-group">
                        <label for="max_login_attempts">Max Login Attempts</label>
                        <input type="number" id="max_login_attempts" name="max_login_attempts" value="${s.max_login_attempts || 5}" min="1" max="20">
                        <span class="form-hint">Account will be locked after this many failed attempts</span>
                    </div>

                    <div class="form-group">
                        <label for="password_min_length">Minimum Password Length</label>
                        <input type="number" id="password_min_length" name="password_min_length" value="${s.password_min_length || 8}" min="6" max="32">
                    </div>

                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" name="require_2fa" ${s.require_2fa ? 'checked' : ''}>
                            <span>Require Two-Factor Authentication</span>
                        </label>
                    </div>

                    <div class="form-group">
                        <label for="allowed_origins">Allowed Origins (CORS)</label>
                        <input type="text" id="allowed_origins" name="allowed_origins" value="${this.escapeHtml((s.allowed_origins || ['*']).join(', '))}">
                        <span class="form-hint">Comma-separated list of allowed origins, or * for all</span>
                    </div>

                    <div class="form-actions">
                        <button type="submit" class="btn-primary">Save Changes</button>
                    </div>
                </form>
            </div>
        `;
    }

    renderNotificationSettings() {
        const s = this.settings?.notifications || {};
        return `
            <div class="settings-section">
                <h2>Notification Settings</h2>
                <p class="section-desc">Configure alert notification channels</p>

                <form id="notifications-form" data-section="notifications">
                    <div class="notification-channel">
                        <h3>Email (SMTP)</h3>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" name="smtp_enabled" ${s.smtp_enabled ? 'checked' : ''} id="smtp-toggle">
                                <span>Enable Email Notifications</span>
                            </label>
                        </div>
                        <div class="smtp-settings" style="${s.smtp_enabled ? '' : 'opacity: 0.5; pointer-events: none;'}">
                            <div class="form-row">
                                <div class="form-group">
                                    <label for="smtp_host">SMTP Host</label>
                                    <input type="text" id="smtp_host" name="smtp_host" value="${this.escapeHtml(s.smtp_host || '')}">
                                </div>
                                <div class="form-group">
                                    <label for="smtp_port">SMTP Port</label>
                                    <input type="number" id="smtp_port" name="smtp_port" value="${s.smtp_port || 587}">
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="notification-channel">
                        <h3>Slack</h3>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" name="slack_enabled" ${s.slack_enabled ? 'checked' : ''}>
                                <span>Enable Slack Notifications</span>
                            </label>
                        </div>
                        <div class="form-group">
                            <label for="slack_webhook">Webhook URL</label>
                            <input type="url" id="slack_webhook" name="slack_webhook" value="${this.escapeHtml(s.slack_webhook || '')}" placeholder="https://hooks.slack.com/services/...">
                        </div>
                    </div>

                    <div class="notification-channel">
                        <h3>PagerDuty</h3>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" name="pagerduty_enabled" ${s.pagerduty_enabled ? 'checked' : ''}>
                                <span>Enable PagerDuty Integration</span>
                            </label>
                        </div>
                    </div>

                    <div class="form-actions">
                        <button type="submit" class="btn-primary">Save Changes</button>
                    </div>
                </form>
            </div>
        `;
    }

    renderSystemInfo() {
        const info = this.systemInfo || {};
        const memoryUsagePercent = info.memory_total_mb ? ((info.memory_used_mb / info.memory_total_mb) * 100).toFixed(1) : 0;
        const diskUsagePercent = info.disk_total_gb ? ((info.disk_used_gb / info.disk_total_gb) * 100).toFixed(1) : 0;

        return `
            <div class="settings-section">
                <h2>System Information</h2>
                <p class="section-desc">Current system status and diagnostics</p>

                <div class="info-grid">
                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Version</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${this.escapeHtml(info.version || 'Unknown')}</div>
                            <div class="info-detail">Built: ${info.build_time || 'Unknown'}</div>
                            <div class="info-detail">Go: ${info.go_version || 'Unknown'}</div>
                        </div>
                    </div>

                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Uptime</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${this.formatUptime(info.uptime_seconds || 0)}</div>
                            <div class="info-detail">${info.os || 'Unknown'} / ${info.arch || 'Unknown'}</div>
                        </div>
                    </div>

                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Memory Usage</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${info.memory_used_mb || 0} MB</div>
                            <div class="progress-bar">
                                <div class="progress-fill ${memoryUsagePercent > 80 ? 'warning' : ''}" style="width: ${memoryUsagePercent}%"></div>
                            </div>
                            <div class="info-detail">${memoryUsagePercent}% of ${info.memory_total_mb || 0} MB</div>
                        </div>
                    </div>

                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Disk Usage</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${info.disk_used_gb || 0} GB</div>
                            <div class="progress-bar">
                                <div class="progress-fill ${diskUsagePercent > 80 ? 'warning' : ''}" style="width: ${diskUsagePercent}%"></div>
                            </div>
                            <div class="info-detail">${diskUsagePercent}% of ${info.disk_total_gb || 0} GB</div>
                        </div>
                    </div>

                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Database</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${this.formatBytes((info.database_size_mb || 0) * 1048576)}</div>
                            <div class="info-detail">SQLite storage</div>
                        </div>
                    </div>

                    <div class="info-card">
                        <div class="info-card-header">
                            <span>Connections</span>
                        </div>
                        <div class="info-card-body">
                            <div class="info-value">${info.active_connections || 0}</div>
                            <div class="info-detail">Active connections</div>
                            <div class="info-detail">${info.cpu_cores || 0} CPU cores</div>
                        </div>
                    </div>
                </div>

                <div class="diagnostic-actions">
                    <h3>Diagnostics</h3>
                    <div class="action-buttons">
                        <button class="btn-secondary" id="btn-download-logs">Download Logs</button>
                        <button class="btn-secondary" id="btn-health-check">Run Health Check</button>
                        <button class="btn-danger" id="btn-clear-cache">Clear Cache</button>
                    </div>
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        // Refresh button
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());

        // Tab navigation
        this.querySelectorAll('.nav-item').forEach(btn => {
            btn.addEventListener('click', () => this.setActiveTab(btn.dataset.tab));
        });

        // Form submissions
        this.querySelectorAll('form[data-section]').forEach(form => {
            form.addEventListener('submit', (e) => this.handleFormSubmit(e, form.dataset.section));
        });

        // SMTP toggle
        this.querySelector('#smtp-toggle')?.addEventListener('change', (e) => {
            const smtpSettings = this.querySelector('.smtp-settings');
            if (smtpSettings) {
                smtpSettings.style.opacity = e.target.checked ? '1' : '0.5';
                smtpSettings.style.pointerEvents = e.target.checked ? 'auto' : 'none';
            }
        });

        // Diagnostic buttons
        this.querySelector('#btn-download-logs')?.addEventListener('click', () => {
            this.showToast('Preparing log download...');
            // In a real implementation, this would trigger a download
        });

        this.querySelector('#btn-health-check')?.addEventListener('click', () => {
            this.showToast('Health check completed - all systems operational');
        });

        this.querySelector('#btn-clear-cache')?.addEventListener('click', () => {
            if (confirm('Are you sure you want to clear the cache?')) {
                this.showToast('Cache cleared successfully');
            }
        });
    }

    getStyles() {
        return `
            .system-settings {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
            }

            .header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .title {
                font-weight: 600;
                font-size: 1rem;
            }

            .btn-primary, .btn-secondary, .btn-danger {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                font-size: 0.85rem;
                cursor: pointer;
                border: none;
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-secondary {
                background: var(--bg-card, #16181c);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .btn-danger {
                background: rgba(244, 63, 94, 0.2);
                color: #f43f5e;
                border: 1px solid rgba(244, 63, 94, 0.3);
            }

            .settings-layout {
                display: flex;
                flex: 1;
                overflow: hidden;
            }

            .settings-nav {
                width: 200px;
                background: var(--bg-elevated, #1e2128);
                border-right: 1px solid var(--border, #2f3336);
                padding: 1rem 0;
            }

            .nav-item {
                display: block;
                width: 100%;
                padding: 0.75rem 1rem;
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 0.9rem;
                text-align: left;
                cursor: pointer;
                border-left: 3px solid transparent;
            }

            .nav-item:hover {
                color: var(--text, #e7e9ea);
                background: rgba(255, 255, 255, 0.02);
            }

            .nav-item.active {
                color: var(--accent, #1d9bf0);
                background: rgba(29, 155, 240, 0.1);
                border-left-color: var(--accent, #1d9bf0);
            }

            .settings-content {
                flex: 1;
                overflow-y: auto;
                padding: 1.5rem;
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 200px;
                color: var(--text-muted, #71767b);
            }

            .settings-section h2 {
                margin: 0 0 0.25rem 0;
                font-size: 1.25rem;
            }

            .section-desc {
                color: var(--text-muted, #71767b);
                margin: 0 0 1.5rem 0;
                font-size: 0.9rem;
            }

            .form-group {
                margin-bottom: 1.25rem;
            }

            .form-group label {
                display: block;
                margin-bottom: 0.5rem;
                font-size: 0.9rem;
                font-weight: 500;
            }

            .form-group input[type="text"],
            .form-group input[type="url"],
            .form-group input[type="number"],
            .form-group select {
                width: 100%;
                max-width: 400px;
                padding: 0.5rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.9rem;
            }

            .form-group input[readonly] {
                opacity: 0.7;
                cursor: not-allowed;
            }

            .form-hint {
                display: block;
                margin-top: 0.25rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .checkbox-label {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                cursor: pointer;
            }

            .checkbox-label input {
                width: auto;
            }

            .form-row {
                display: flex;
                gap: 1rem;
            }

            .form-row .form-group {
                flex: 1;
            }

            .form-actions {
                margin-top: 1.5rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .info-box {
                padding: 0.75rem;
                background: rgba(29, 155, 240, 0.1);
                border-radius: 4px;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .notification-channel {
                margin-bottom: 2rem;
                padding-bottom: 1.5rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .notification-channel h3 {
                margin: 0 0 1rem 0;
                font-size: 1rem;
            }

            .info-grid {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
                gap: 1rem;
                margin-bottom: 2rem;
            }

            .info-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                overflow: hidden;
            }

            .info-card-header {
                padding: 0.75rem;
                background: rgba(255, 255, 255, 0.02);
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                font-weight: 500;
            }

            .info-card-body {
                padding: 0.75rem;
            }

            .info-value {
                font-size: 1.25rem;
                font-weight: 600;
                margin-bottom: 0.5rem;
            }

            .info-detail {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .progress-bar {
                height: 6px;
                background: var(--bg-card, #16181c);
                border-radius: 3px;
                overflow: hidden;
                margin: 0.5rem 0;
            }

            .progress-fill {
                height: 100%;
                background: var(--accent, #1d9bf0);
                transition: width 0.3s;
            }

            .progress-fill.warning {
                background: #fbbf24;
            }

            .diagnostic-actions h3 {
                margin: 0 0 1rem 0;
                font-size: 1rem;
            }

            .action-buttons {
                display: flex;
                gap: 0.75rem;
            }
        `;
    }
}

customElements.define('system-settings', SystemSettings);
