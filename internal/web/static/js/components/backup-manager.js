/**
 * Backup Manager Widget
 * Manage backups, restore, scheduler, and retention policies
 */
class BackupManager extends HTMLElement {
    constructor() {
        super();
        this.backups = [];
        this.schedulerStatus = null;
        this.retentionPolicy = null;
        this.loading = true;
        this.error = null;
        this.refreshInterval = null;
        this.activeOperation = null; // Track ongoing operations
    }

    connectedCallback() {
        this.render();
        this.loadData();
        // Refresh every 30 seconds
        this.refreshInterval = setInterval(() => this.loadData(true), 30000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    async loadData(silent = false) {
        if (!silent) {
            this.loading = true;
            this.error = null;
            this.render();
        }

        try {
            const [backupsResp, schedulerResp, retentionResp] = await Promise.all([
                fetch('/api/backup/list'),
                fetch('/api/backup/scheduler'),
                fetch('/api/backup/retention')
            ]);

            if (backupsResp.ok) {
                this.backups = await backupsResp.json() || [];
            } else if (backupsResp.status === 503) {
                this.backups = [];
            } else {
                throw new Error('Failed to load backups');
            }

            if (schedulerResp.ok) {
                this.schedulerStatus = await schedulerResp.json();
            }

            if (retentionResp.ok) {
                this.retentionPolicy = await retentionResp.json();
            }
        } catch (e) {
            console.error('Failed to load backup data:', e);
            if (!silent) {
                this.error = e.message;
            }
        } finally {
            this.loading = false;
            this.render();
        }
    }

    async createBackup() {
        this.activeOperation = 'creating';
        this.render();

        try {
            const resp = await fetch('/api/backup', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ compress: true })
            });

            if (!resp.ok) {
                const error = await resp.text();
                throw new Error(error);
            }

            const result = await resp.json();
            this.showToast(`Backup created: ${result.size_human}`, 'success');
            await this.loadData(true);
        } catch (e) {
            console.error('Failed to create backup:', e);
            this.showToast('Failed to create backup: ' + e.message, 'error');
        } finally {
            this.activeOperation = null;
            this.render();
        }
    }

    async verifyBackup(path) {
        this.activeOperation = 'verifying';
        this.render();

        try {
            const resp = await fetch('/api/backup/verify', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ backup_path: path })
            });

            if (!resp.ok) {
                const error = await resp.text();
                throw new Error(error);
            }

            const result = await resp.json();
            if (result.valid) {
                this.showToast(`Backup verified: ${result.file_count} files, integrity OK`, 'success');
            } else {
                this.showToast('Backup verification failed: ' + (result.error || 'Unknown error'), 'error');
            }
        } catch (e) {
            console.error('Failed to verify backup:', e);
            this.showToast('Failed to verify backup: ' + e.message, 'error');
        } finally {
            this.activeOperation = null;
            this.render();
        }
    }

    async restoreBackup(path) {
        const filename = path.split('/').pop();
        if (!confirm(`Are you sure you want to restore from "${filename}"?\n\nThis will replace current data. A restart will be required after restore.`)) {
            return;
        }

        this.activeOperation = 'restoring';
        this.render();

        try {
            const resp = await fetch('/api/restore', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ backup_path: path, force: true })
            });

            if (!resp.ok) {
                const error = await resp.text();
                throw new Error(error);
            }

            const result = await resp.json();
            this.showToast(`Restore complete: ${result.file_count} files restored. Please restart dogwatch.`, 'success');
        } catch (e) {
            console.error('Failed to restore backup:', e);
            this.showToast('Failed to restore backup: ' + e.message, 'error');
        } finally {
            this.activeOperation = null;
            this.render();
        }
    }

    downloadBackup(filename) {
        window.location.href = `/api/backup/download/${encodeURIComponent(filename)}`;
    }

    async deleteBackup(path) {
        const filename = path.split('/').pop();
        if (!confirm(`Delete backup "${filename}"?`)) {
            return;
        }

        // Use retention API to delete by applying a policy that excludes this file
        // For now, show a message that manual deletion is needed
        this.showToast('Manual deletion: Remove file from data directory', 'info');
    }

    async toggleScheduler() {
        const action = this.schedulerStatus?.running ? 'stop' : 'start';

        try {
            const resp = await fetch(`/api/backup/scheduler?action=${action}`, {
                method: 'POST'
            });

            if (!resp.ok) {
                const error = await resp.text();
                throw new Error(error);
            }

            this.showToast(`Scheduler ${action === 'start' ? 'started' : 'stopped'}`, 'success');
            await this.loadData(true);
        } catch (e) {
            console.error('Failed to toggle scheduler:', e);
            this.showToast('Failed to toggle scheduler: ' + e.message, 'error');
        }
    }

    async updateSchedulerConfig(config) {
        try {
            const resp = await fetch('/api/backup/scheduler', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });

            if (!resp.ok) {
                const error = await resp.text();
                throw new Error(error);
            }

            this.showToast('Scheduler configuration updated', 'success');
            this.closeModal();
            await this.loadData(true);
        } catch (e) {
            console.error('Failed to update scheduler:', e);
            this.showToast('Failed to update scheduler: ' + e.message, 'error');
        }
    }

    showConfigModal() {
        const modal = this.querySelector('#config-modal');
        if (modal) {
            modal.style.display = 'flex';
        }
    }

    closeModal() {
        const modal = this.querySelector('#config-modal');
        if (modal) {
            modal.style.display = 'none';
        }
    }

    handleConfigSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        const config = {
            enabled: form.querySelector('[name="enabled"]').checked,
            interval: formData.get('interval'),
            max_backups: parseInt(formData.get('max_backups'), 10) || 10,
            max_age_days: parseInt(formData.get('max_age_days'), 10) || 30
        };

        this.updateSchedulerConfig(config);
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatSize(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB';
        if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB';
        if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return bytes + ' B';
    }

    formatDate(dateStr) {
        if (!dateStr) return 'Never';
        const date = new Date(dateStr);
        if (isNaN(date.getTime())) return dateStr;
        return date.toLocaleString();
    }

    formatDuration(durationNs) {
        if (!durationNs) return '';
        const ms = durationNs / 1000000;
        if (ms < 1000) return Math.round(ms) + 'ms';
        const sec = ms / 1000;
        if (sec < 60) return sec.toFixed(1) + 's';
        const min = sec / 60;
        return min.toFixed(1) + 'm';
    }

    formatInterval(intervalNs) {
        if (!intervalNs) return 'N/A';
        const hours = intervalNs / (1000000000 * 3600);
        if (hours >= 24) {
            const days = Math.round(hours / 24);
            return days === 1 ? 'Every day' : `Every ${days} days`;
        }
        return hours === 1 ? 'Every hour' : `Every ${Math.round(hours)} hours`;
    }

    formatTimeUntil(dateStr) {
        if (!dateStr) return '';
        const date = new Date(dateStr);
        if (isNaN(date.getTime())) return '';
        const now = new Date();
        const diff = date - now;
        if (diff <= 0) return 'now';

        const hours = Math.floor(diff / 3600000);
        const minutes = Math.floor((diff % 3600000) / 60000);

        if (hours > 0) {
            return `in ${hours}h ${minutes}m`;
        }
        return `in ${minutes}m`;
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
            <div class="backup-manager">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Backups</span>
                    </div>
                    <div class="header-right">
                        <button class="btn-primary" id="btn-create" ${this.activeOperation ? 'disabled' : ''}>
                            ${this.activeOperation === 'creating' ? 'Creating...' : 'Create Backup Now'}
                        </button>
                    </div>
                </div>

                ${this.loading ? `
                    <div class="loading">Loading backup data...</div>
                ` : this.error ? `
                    <div class="error">${this.escapeHtml(this.error)}</div>
                ` : `
                    <div class="content">
                        ${this.renderSchedulerStatus()}
                        ${this.renderBackupList()}
                    </div>
                `}

                ${this.renderConfigModal()}
            </div>
        `;

        this.attachEventListeners();
    }

    renderSchedulerStatus() {
        const status = this.schedulerStatus || {};
        const running = status.running;
        const enabled = status.enabled;
        const nextRun = status.next_run;
        const interval = status.interval;
        const retention = this.retentionPolicy || {};

        return `
            <div class="scheduler-section">
                <div class="section-header">
                    <h3>Scheduler</h3>
                </div>
                <div class="scheduler-grid">
                    <div class="scheduler-stat">
                        <div class="stat-label">Status</div>
                        <div class="stat-value">
                            <span class="status-dot ${running ? 'running' : 'stopped'}"></span>
                            ${running ? 'Running' : (enabled ? 'Stopped' : 'Disabled')}
                        </div>
                    </div>
                    <div class="scheduler-stat">
                        <div class="stat-label">Next Backup</div>
                        <div class="stat-value">
                            ${running && nextRun ? `${this.formatDate(nextRun)} (${this.formatTimeUntil(nextRun)})` : 'N/A'}
                        </div>
                    </div>
                    <div class="scheduler-stat">
                        <div class="stat-label">Interval</div>
                        <div class="stat-value">${this.formatInterval(interval)}</div>
                    </div>
                    <div class="scheduler-stat">
                        <div class="stat-label">Retention</div>
                        <div class="stat-value">
                            ${retention.max_backups || 10} backups / ${Math.round((retention.max_age || 2592000000000000) / (24 * 3600 * 1000000000))} days
                        </div>
                    </div>
                </div>
                <div class="scheduler-actions">
                    <button class="btn-secondary" id="btn-toggle-scheduler">
                        ${running ? 'Stop Scheduler' : 'Start Scheduler'}
                    </button>
                    <button class="btn-secondary" id="btn-configure">Configure</button>
                </div>
                ${this.renderRecentRuns()}
            </div>
        `;
    }

    renderRecentRuns() {
        const runs = this.schedulerStatus?.recent_runs || [];
        if (runs.length === 0) return '';

        const recentRuns = runs.slice(-5).reverse();

        return `
            <div class="recent-runs">
                <div class="runs-header">Recent Backup History</div>
                <div class="runs-list">
                    ${recentRuns.map(run => `
                        <div class="run-item ${run.success ? 'success' : 'failed'}">
                            <span class="run-status">${run.success ? '&#10003;' : '&#10007;'}</span>
                            <span class="run-time">${this.formatDate(run.started_at)}</span>
                            ${run.success ? `
                                <span class="run-size">${this.formatSize(run.size)}</span>
                                <span class="run-files">${run.file_count} files</span>
                            ` : `
                                <span class="run-error">${this.escapeHtml(run.error)}</span>
                            `}
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderBackupList() {
        const backups = this.backups || [];

        return `
            <div class="backups-section">
                <div class="section-header">
                    <h3>Available Backups</h3>
                    <span class="backup-count">${backups.length} backup${backups.length !== 1 ? 's' : ''}</span>
                </div>
                ${backups.length === 0 ? `
                    <div class="empty-state">
                        <p>No backups found</p>
                        <p class="empty-hint">Create your first backup using the button above</p>
                    </div>
                ` : `
                    <div class="backup-list">
                        ${backups.map(backup => this.renderBackupItem(backup)).join('')}
                    </div>
                `}
            </div>
        `;
    }

    renderBackupItem(backup) {
        const filename = backup.path?.split('/').pop() || backup.filename || 'unknown';
        const verified = backup.verified;

        return `
            <div class="backup-item">
                <div class="backup-main">
                    <div class="backup-icon">${verified ? '&#10003;' : '&#128230;'}</div>
                    <div class="backup-info">
                        <div class="backup-name">${this.escapeHtml(filename)}</div>
                        <div class="backup-meta">
                            ${backup.size_human || this.formatSize(backup.size)} &middot;
                            ${backup.file_count || 0} files &middot;
                            ${this.formatDate(backup.created_at || backup.modified_at)}
                        </div>
                    </div>
                </div>
                <div class="backup-actions">
                    <button class="btn-small" data-action="download" data-filename="${this.escapeHtml(filename)}" title="Download">
                        Download
                    </button>
                    <button class="btn-small" data-action="verify" data-path="${this.escapeHtml(backup.path)}" title="Verify integrity" ${this.activeOperation ? 'disabled' : ''}>
                        ${this.activeOperation === 'verifying' ? '...' : 'Verify'}
                    </button>
                    <button class="btn-small btn-warning" data-action="restore" data-path="${this.escapeHtml(backup.path)}" title="Restore from backup" ${this.activeOperation ? 'disabled' : ''}>
                        ${this.activeOperation === 'restoring' ? '...' : 'Restore'}
                    </button>
                </div>
            </div>
        `;
    }

    renderConfigModal() {
        const status = this.schedulerStatus || {};
        const retention = this.retentionPolicy || {};
        const intervalHours = Math.round((status.interval || 86400000000000) / (3600 * 1000000000));

        return `
            <div class="modal" id="config-modal" style="display: none;">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3>Scheduler Configuration</h3>
                        <button class="modal-close" id="btn-modal-close">&times;</button>
                    </div>
                    <form id="config-form">
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" name="enabled" ${status.enabled ? 'checked' : ''}>
                                <span>Enable automatic backups</span>
                            </label>
                        </div>

                        <div class="form-group">
                            <label for="interval">Backup Interval</label>
                            <select name="interval" id="interval">
                                <option value="1h" ${intervalHours === 1 ? 'selected' : ''}>Every hour</option>
                                <option value="6h" ${intervalHours === 6 ? 'selected' : ''}>Every 6 hours</option>
                                <option value="12h" ${intervalHours === 12 ? 'selected' : ''}>Every 12 hours</option>
                                <option value="24h" ${intervalHours === 24 ? 'selected' : ''}>Every 24 hours</option>
                                <option value="168h" ${intervalHours === 168 ? 'selected' : ''}>Weekly</option>
                            </select>
                        </div>

                        <div class="form-group">
                            <label for="max_backups">Maximum Backups to Keep</label>
                            <input type="number" name="max_backups" id="max_backups"
                                value="${retention.max_backups || 10}" min="1" max="100">
                            <span class="form-hint">Oldest backups will be deleted when limit is reached</span>
                        </div>

                        <div class="form-group">
                            <label for="max_age_days">Maximum Age (days)</label>
                            <input type="number" name="max_age_days" id="max_age_days"
                                value="${Math.round((retention.max_age || 2592000000000000) / (24 * 3600 * 1000000000))}" min="1" max="365">
                            <span class="form-hint">Backups older than this will be deleted</span>
                        </div>

                        <div class="form-actions">
                            <button type="button" class="btn-secondary" id="btn-cancel">Cancel</button>
                            <button type="submit" class="btn-primary">Save Configuration</button>
                        </div>
                    </form>
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        // Create backup button
        this.querySelector('#btn-create')?.addEventListener('click', () => this.createBackup());

        // Toggle scheduler
        this.querySelector('#btn-toggle-scheduler')?.addEventListener('click', () => this.toggleScheduler());

        // Configure button
        this.querySelector('#btn-configure')?.addEventListener('click', () => this.showConfigModal());

        // Modal close buttons
        this.querySelector('#btn-modal-close')?.addEventListener('click', () => this.closeModal());
        this.querySelector('#btn-cancel')?.addEventListener('click', () => this.closeModal());

        // Config form submit
        this.querySelector('#config-form')?.addEventListener('submit', (e) => this.handleConfigSubmit(e));

        // Click outside modal to close
        this.querySelector('#config-modal')?.addEventListener('click', (e) => {
            if (e.target.id === 'config-modal') {
                this.closeModal();
            }
        });

        // Backup action buttons
        this.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', () => {
                const action = btn.dataset.action;
                const path = btn.dataset.path;
                const filename = btn.dataset.filename;

                switch (action) {
                    case 'download':
                        this.downloadBackup(filename);
                        break;
                    case 'verify':
                        this.verifyBackup(path);
                        break;
                    case 'restore':
                        this.restoreBackup(path);
                        break;
                }
            });
        });
    }

    getStyles() {
        return `
            .backup-manager {
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

            .btn-primary, .btn-secondary, .btn-small, .btn-warning {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                font-size: 0.85rem;
                cursor: pointer;
                border: none;
                transition: all 0.15s ease;
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-primary:hover:not(:disabled) {
                background: #1a8cd8;
            }

            .btn-primary:disabled {
                opacity: 0.6;
                cursor: not-allowed;
            }

            .btn-secondary {
                background: var(--bg-card, #16181c);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .btn-secondary:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .btn-small {
                padding: 0.35rem 0.75rem;
                font-size: 0.8rem;
                background: var(--bg-elevated, #1e2128);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .btn-small:hover:not(:disabled) {
                background: var(--bg-card, #16181c);
            }

            .btn-small:disabled {
                opacity: 0.5;
                cursor: not-allowed;
            }

            .btn-warning {
                background: rgba(251, 191, 36, 0.1);
                color: #fbbf24;
                border: 1px solid rgba(251, 191, 36, 0.3);
            }

            .btn-warning:hover:not(:disabled) {
                background: rgba(251, 191, 36, 0.2);
            }

            .content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 200px;
                color: var(--text-muted, #71767b);
            }

            .error {
                color: var(--error, #f4212e);
            }

            /* Scheduler Section */
            .scheduler-section {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                margin-bottom: 1rem;
            }

            .section-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 1rem;
            }

            .section-header h3 {
                margin: 0;
                font-size: 0.95rem;
                font-weight: 600;
            }

            .scheduler-grid {
                display: grid;
                grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
                gap: 1rem;
                margin-bottom: 1rem;
            }

            .scheduler-stat {
                background: var(--bg-card, #16181c);
                padding: 0.75rem;
                border-radius: 6px;
            }

            .stat-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }

            .stat-value {
                font-size: 0.9rem;
                font-weight: 500;
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }

            .status-dot {
                width: 8px;
                height: 8px;
                border-radius: 50%;
            }

            .status-dot.running {
                background: var(--success, #00ba7c);
                box-shadow: 0 0 6px var(--success, #00ba7c);
            }

            .status-dot.stopped {
                background: var(--text-muted, #71767b);
            }

            .scheduler-actions {
                display: flex;
                gap: 0.5rem;
            }

            /* Recent Runs */
            .recent-runs {
                margin-top: 1rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .runs-header {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .runs-list {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }

            .run-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                font-size: 0.8rem;
            }

            .run-status {
                font-size: 0.9rem;
            }

            .run-item.success .run-status {
                color: var(--success, #00ba7c);
            }

            .run-item.failed .run-status {
                color: var(--error, #f4212e);
            }

            .run-time {
                color: var(--text-muted, #71767b);
            }

            .run-size, .run-files {
                color: var(--text, #e7e9ea);
            }

            .run-error {
                color: var(--error, #f4212e);
                flex: 1;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            /* Backups Section */
            .backups-section {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .backup-count {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .empty-state {
                text-align: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            .empty-hint {
                font-size: 0.85rem;
                margin-top: 0.5rem;
            }

            .backup-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .backup-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem;
                background: var(--bg-card, #16181c);
                border-radius: 6px;
                border: 1px solid var(--border, #2f3336);
            }

            .backup-main {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                flex: 1;
                min-width: 0;
            }

            .backup-icon {
                font-size: 1.25rem;
                color: var(--text-muted, #71767b);
            }

            .backup-info {
                flex: 1;
                min-width: 0;
            }

            .backup-name {
                font-weight: 500;
                font-size: 0.9rem;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .backup-meta {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            .backup-actions {
                display: flex;
                gap: 0.5rem;
                flex-shrink: 0;
            }

            /* Modal */
            .modal {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.6);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 1000;
            }

            .modal-content {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                width: 100%;
                max-width: 450px;
                max-height: 90vh;
                overflow-y: auto;
            }

            .modal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .modal-header h3 {
                margin: 0;
                font-size: 1rem;
            }

            .modal-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.5rem;
                cursor: pointer;
                padding: 0;
                line-height: 1;
            }

            .modal-close:hover {
                color: var(--text, #e7e9ea);
            }

            #config-form {
                padding: 1rem;
            }

            .form-group {
                margin-bottom: 1rem;
            }

            .form-group label {
                display: block;
                margin-bottom: 0.5rem;
                font-size: 0.9rem;
                font-weight: 500;
            }

            .form-group input[type="number"],
            .form-group select {
                width: 100%;
                padding: 0.5rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.9rem;
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

            .form-actions {
                display: flex;
                justify-content: flex-end;
                gap: 0.5rem;
                margin-top: 1.5rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }
        `;
    }
}

customElements.define('backup-manager', BackupManager);
