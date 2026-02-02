/**
 * Hot Reload Manager Widget
 * Manages eBPF probes with reload, rollback, health monitoring,
 * and configuration editing capabilities
 */
class HotreloadManager extends HTMLElement {
    constructor() {
        super();
        this.probes = [];
        this.selectedProbe = null;
        this.stats = null;
        this.refreshInterval = null;
        this._mounted = false;
        this._boundEventListeners = [];
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
        this.refreshInterval = setInterval(() => {
            if (this._mounted) this.loadHealth();
        }, 10000);
    }

    disconnectedCallback() {
        this._mounted = false;
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
        // Clean up event listeners
        this._boundEventListeners.forEach(({ element, event, handler }) => {
            if (element) element.removeEventListener(event, handler);
        });
        this._boundEventListeners = [];
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="hotreload-container">
                <div class="hotreload-header">
                    <div class="hotreload-title">
                        <span class="title-icon">&#9889;</span>
                        <span>Probe Manager</span>
                    </div>
                    <div class="hotreload-controls">
                        <button class="btn-refresh" id="btn-refresh">&#8635; Refresh</button>
                    </div>
                </div>

                <div class="stats-bar">
                    <div class="stat-item">
                        <span class="stat-value" id="stat-probes">-</span>
                        <span class="stat-label">Active Probes</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-reloads">-</span>
                        <span class="stat-label">Total Reloads</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-failures">-</span>
                        <span class="stat-label">Failures</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-rollbacks">-</span>
                        <span class="stat-label">Rollbacks</span>
                    </div>
                </div>

                <div class="hotreload-content">
                    <div class="probe-list" id="probe-list">
                        <div class="loading">Loading probes...</div>
                    </div>
                    <div class="probe-detail" id="probe-detail">
                        <div class="detail-empty">
                            Select a probe to view details
                        </div>
                    </div>
                </div>

                <div class="config-modal" id="config-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span>Edit Probe Configuration</span>
                            <button class="btn-close" id="btn-close-modal">&times;</button>
                        </div>
                        <div class="modal-body">
                            <div class="config-editor">
                                <textarea id="config-editor" placeholder="Loading configuration..."></textarea>
                            </div>
                            <div class="config-format">
                                <label>
                                    <input type="radio" name="config-format" value="json" checked> JSON
                                </label>
                                <label>
                                    <input type="radio" name="config-format" value="yaml"> YAML
                                </label>
                            </div>
                        </div>
                        <div class="modal-actions">
                            <button class="btn-cancel" id="btn-cancel-config">Cancel</button>
                            <button class="btn-save" id="btn-save-config">Save Configuration</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    getStyles() {
        return `
            .hotreload-container {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                position: relative;
            }
            .hotreload-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .hotreload-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                font-size: 0.9rem;
            }
            .title-icon { font-size: 1.1rem; }
            .hotreload-controls {
                display: flex;
                gap: 0.5rem;
            }
            .btn-refresh, .btn-reload, .btn-rollback, .btn-config, .btn-save, .btn-cancel, .btn-close {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
                font-size: 0.8rem;
            }
            .btn-reload {
                background: rgba(29, 155, 240, 0.2);
                color: #1d9bf0;
                border-color: rgba(29, 155, 240, 0.3);
            }
            .btn-rollback {
                background: rgba(251, 191, 36, 0.2);
                color: #fbbf24;
                border-color: rgba(251, 191, 36, 0.3);
            }
            .btn-save {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
            }
            .stats-bar {
                display: flex;
                gap: 1.5rem;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .stat-item {
                display: flex;
                flex-direction: column;
                align-items: center;
            }
            .stat-value {
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }
            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .hotreload-content {
                display: flex;
                flex: 1;
                overflow: hidden;
            }
            .probe-list {
                width: 280px;
                border-right: 1px solid var(--border, #2f3336);
                overflow-y: auto;
            }
            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }
            .probe-item {
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                cursor: pointer;
                transition: background 0.15s ease;
            }
            .probe-item:hover {
                background: rgba(29, 155, 240, 0.08);
            }
            .probe-item.selected {
                background: rgba(29, 155, 240, 0.12);
                border-left: 3px solid var(--accent, #1d9bf0);
            }
            .probe-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.25rem;
            }
            .probe-name {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .probe-status {
                display: inline-flex;
                width: 10px;
                height: 10px;
                border-radius: 50%;
            }
            .probe-status.healthy {
                background: var(--success, #00ba7c);
                box-shadow: 0 0 6px var(--success, #00ba7c);
            }
            .probe-status.degraded {
                background: #fbbf24;
                box-shadow: 0 0 6px #fbbf24;
            }
            .probe-status.failed {
                background: #f43f5e;
                box-shadow: 0 0 6px #f43f5e;
            }
            .probe-meta {
                display: flex;
                gap: 0.75rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }
            .probe-detail {
                flex: 1;
                overflow-y: auto;
            }
            .detail-empty {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }
            .detail-header {
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .detail-title-row {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
            }
            .detail-title {
                font-weight: 600;
                font-size: 1.1rem;
            }
            .detail-actions {
                display: flex;
                gap: 0.5rem;
            }
            .detail-version {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }
            .detail-body {
                padding: 1rem;
            }
            .detail-section {
                margin-bottom: 1.5rem;
            }
            .detail-section-title {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
                margin-bottom: 0.75rem;
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }
            .detail-grid {
                display: grid;
                grid-template-columns: repeat(2, 1fr);
                gap: 0.75rem;
            }
            .detail-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                padding: 0.75rem;
            }
            .detail-card-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }
            .detail-card-value {
                font-size: 1rem;
                font-weight: 500;
            }
            .detail-card-value.success { color: var(--success, #00ba7c); }
            .detail-card-value.warning { color: #fbbf24; }
            .detail-card-value.error { color: #f43f5e; }
            .version-history {
                max-height: 200px;
                overflow-y: auto;
            }
            .version-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.5rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                margin-bottom: 0.5rem;
            }
            .version-item.current {
                border-left: 3px solid var(--accent, #1d9bf0);
            }
            .version-info {
                display: flex;
                flex-direction: column;
            }
            .version-number {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .version-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .version-badge {
                font-size: 0.7rem;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                background: rgba(29, 155, 240, 0.2);
                color: #1d9bf0;
            }
            .config-preview {
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                padding: 0.75rem;
                font-family: monospace;
                font-size: 0.8rem;
                white-space: pre-wrap;
                max-height: 150px;
                overflow-y: auto;
            }
            .config-modal {
                position: absolute;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 200;
            }
            .modal-content {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                width: 500px;
                max-height: 80%;
                display: flex;
                flex-direction: column;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            }
            .modal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 500;
            }
            .modal-body {
                padding: 1rem;
                flex: 1;
                overflow: hidden;
                display: flex;
                flex-direction: column;
            }
            .config-editor {
                flex: 1;
                min-height: 250px;
            }
            .config-editor textarea {
                width: 100%;
                height: 100%;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.75rem;
                color: var(--text, #e7e9ea);
                font-family: monospace;
                font-size: 0.85rem;
                resize: none;
            }
            .config-format {
                display: flex;
                gap: 1rem;
                margin-top: 0.75rem;
                font-size: 0.85rem;
            }
            .config-format label {
                display: flex;
                align-items: center;
                gap: 0.25rem;
                cursor: pointer;
            }
            .modal-actions {
                display: flex;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
                justify-content: flex-end;
            }
            .empty-state {
                text-align: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }
        `;
    }

    setupEventListeners() {
        // Helper to track event listeners for cleanup
        const addListener = (selector, event, handler) => {
            const element = this.querySelector(selector);
            if (element) {
                element.addEventListener(event, handler);
                this._boundEventListeners.push({ element, event, handler });
            }
        };

        // Refresh
        addListener('#btn-refresh', 'click', () => this.loadData());

        // Config modal
        addListener('#btn-close-modal', 'click', () => {
            this.querySelector('#config-modal').style.display = 'none';
        });

        addListener('#btn-cancel-config', 'click', () => {
            this.querySelector('#config-modal').style.display = 'none';
        });

        addListener('#btn-save-config', 'click', () => this.saveConfig());
    }

    async loadData() {
        const listContainer = this.querySelector('#probe-list');
        if (listContainer) {
            listContainer.innerHTML = '<div class="loading">Loading probes...</div>';
        }

        try {
            const [probesResp, statsResp, healthResp] = await Promise.all([
                fetch('/api/probes'),
                fetch('/api/probes/stats'),
                fetch('/api/probes/health')
            ]);

            if (probesResp.ok) {
                this.probes = await probesResp.json();
            } else {
                this.probes = this.generateDemoProbes();
            }

            if (statsResp.ok) {
                this.stats = await statsResp.json();
            } else {
                this.stats = this.generateDemoStats();
            }

            if (healthResp.ok) {
                const health = await healthResp.json();
                this.mergeHealth(health);
            }

            this.updateStats();
            this.renderProbeList();
        } catch (e) {
            console.error('Failed to load probes:', e);
            this.probes = this.generateDemoProbes();
            this.stats = this.generateDemoStats();
            this.updateStats();
            this.renderProbeList();
        }
    }

    async loadHealth() {
        try {
            const resp = await fetch('/api/probes/health');
            if (resp.ok) {
                const health = await resp.json();
                this.mergeHealth(health);
                this.renderProbeList();
                if (this.selectedProbe) {
                    this.renderProbeDetail(this.selectedProbe);
                }
            }
        } catch (e) {
            // Ignore
        }
    }

    mergeHealth(health) {
        if (!Array.isArray(health)) return;
        for (const h of health) {
            const probe = this.probes.find(p => p.name === h.name);
            if (probe) {
                probe.health = h;
            }
        }
    }

    generateDemoProbes() {
        return [
            {
                name: 'http-probe',
                version: { version: '2.4.1', loadedAt: Date.now() - 86400000 },
                health: { status: 'healthy', eventsTotal: 1245678, errorCount: 12 },
                config: { enabled: true, sampleRate: 1.0, bufferSize: 1000, maxEventsPerSec: 100000 },
                versionHistory: [
                    { version: '2.4.1', loadedAt: Date.now() - 86400000 },
                    { version: '2.4.0', loadedAt: Date.now() - 172800000 },
                    { version: '2.3.8', loadedAt: Date.now() - 345600000 }
                ]
            },
            {
                name: 'mysql-probe',
                version: { version: '1.2.0', loadedAt: Date.now() - 43200000 },
                health: { status: 'healthy', eventsTotal: 567890, errorCount: 3 },
                config: { enabled: true, sampleRate: 0.5, bufferSize: 500, maxEventsPerSec: 50000 },
                versionHistory: [
                    { version: '1.2.0', loadedAt: Date.now() - 43200000 },
                    { version: '1.1.9', loadedAt: Date.now() - 259200000 }
                ]
            },
            {
                name: 'redis-probe',
                version: { version: '1.0.5', loadedAt: Date.now() - 7200000 },
                health: { status: 'degraded', eventsTotal: 234567, errorCount: 156 },
                config: { enabled: true, sampleRate: 1.0, bufferSize: 2000, maxEventsPerSec: 200000 },
                versionHistory: [
                    { version: '1.0.5', loadedAt: Date.now() - 7200000 },
                    { version: '1.0.4', loadedAt: Date.now() - 86400000 }
                ]
            },
            {
                name: 'postgres-probe',
                version: { version: '1.3.2', loadedAt: Date.now() - 3600000 },
                health: { status: 'healthy', eventsTotal: 890123, errorCount: 5 },
                config: { enabled: true, sampleRate: 0.8, bufferSize: 1500, maxEventsPerSec: 75000 },
                versionHistory: [
                    { version: '1.3.2', loadedAt: Date.now() - 3600000 }
                ]
            },
            {
                name: 'grpc-probe',
                version: { version: '0.9.1', loadedAt: Date.now() - 1800000 },
                health: { status: 'failed', eventsTotal: 45678, errorCount: 2345 },
                config: { enabled: false, sampleRate: 1.0, bufferSize: 1000, maxEventsPerSec: 100000 },
                versionHistory: [
                    { version: '0.9.1', loadedAt: Date.now() - 1800000 },
                    { version: '0.9.0', loadedAt: Date.now() - 172800000 }
                ]
            }
        ];
    }

    generateDemoStats() {
        return {
            reloadCount: 47,
            failureCount: 3,
            rollbackCount: 2,
            lastReload: Date.now() - 3600000
        };
    }

    updateStats() {
        const stats = this.stats || {};
        const activeProbes = this.probes.filter(p => p.health?.status !== 'failed').length;

        this.querySelector('#stat-probes').textContent = activeProbes;
        this.querySelector('#stat-reloads').textContent = stats.reloadCount || 0;
        this.querySelector('#stat-failures').textContent = stats.failureCount || 0;
        this.querySelector('#stat-rollbacks').textContent = stats.rollbackCount || 0;
    }

    renderProbeList() {
        const container = this.querySelector('#probe-list');
        if (!container) return;

        if (this.probes.length === 0) {
            container.innerHTML = '<div class="empty-state">No probes registered</div>';
            return;
        }

        container.innerHTML = this.probes.map(probe => {
            const isSelected = this.selectedProbe?.name === probe.name;
            const status = probe.health?.status || 'healthy';

            return `
                <div class="probe-item ${isSelected ? 'selected' : ''}" data-probe="${this.escapeHtml(probe.name)}">
                    <div class="probe-header">
                        <span class="probe-name">${this.escapeHtml(probe.name)}</span>
                        <span class="probe-status ${status}"></span>
                    </div>
                    <div class="probe-meta">
                        <span>v${this.escapeHtml(probe.version?.version || '?')}</span>
                        <span>${this.formatNumber(probe.health?.eventsTotal || 0)} events</span>
                    </div>
                </div>
            `;
        }).join('');

        // Add click handlers
        container.querySelectorAll('.probe-item').forEach(el => {
            el.addEventListener('click', () => {
                const probeName = el.dataset.probe;
                const probe = this.probes.find(p => p.name === probeName);
                this.selectProbe(probe);
            });
        });
    }

    selectProbe(probe) {
        this.selectedProbe = probe;
        this.renderProbeList();
        this.renderProbeDetail(probe);
    }

    renderProbeDetail(probe) {
        const container = this.querySelector('#probe-detail');
        if (!container) return;

        if (!probe) {
            container.innerHTML = '<div class="detail-empty">Select a probe to view details</div>';
            return;
        }

        const health = probe.health || {};
        const config = probe.config || {};
        const statusClass = health.status === 'healthy' ? 'success' : health.status === 'degraded' ? 'warning' : 'error';

        container.innerHTML = `
            <div class="detail-header">
                <div class="detail-title-row">
                    <span class="detail-title">${this.escapeHtml(probe.name)}</span>
                    <div class="detail-actions">
                        <button class="btn-reload" id="btn-reload">&#8635; Reload</button>
                        <button class="btn-rollback" id="btn-rollback">&#8630; Rollback</button>
                        <button class="btn-config" id="btn-edit-config">&#9881; Config</button>
                    </div>
                </div>
                <div class="detail-version">
                    Version ${this.escapeHtml(probe.version?.version || '?')} -
                    Loaded ${this.formatRelativeTime(probe.version?.loadedAt)}
                </div>
            </div>

            <div class="detail-body">
                <div class="detail-section">
                    <div class="detail-section-title">
                        <span>&#128200;</span> Health & Metrics
                    </div>
                    <div class="detail-grid">
                        <div class="detail-card">
                            <div class="detail-card-label">Status</div>
                            <div class="detail-card-value ${statusClass}">
                                ${(health.status || 'unknown').toUpperCase()}
                            </div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Events Processed</div>
                            <div class="detail-card-value">${this.formatNumber(health.eventsTotal || 0)}</div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Error Count</div>
                            <div class="detail-card-value ${health.errorCount > 100 ? 'error' : ''}">${this.formatNumber(health.errorCount || 0)}</div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Last Check</div>
                            <div class="detail-card-value">${this.formatRelativeTime(health.lastCheck)}</div>
                        </div>
                    </div>
                </div>

                <div class="detail-section">
                    <div class="detail-section-title">
                        <span>&#9881;</span> Configuration
                    </div>
                    <div class="detail-grid">
                        <div class="detail-card">
                            <div class="detail-card-label">Enabled</div>
                            <div class="detail-card-value ${config.enabled ? 'success' : 'error'}">
                                ${config.enabled ? 'Yes' : 'No'}
                            </div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Sample Rate</div>
                            <div class="detail-card-value">${((config.sampleRate || 1) * 100).toFixed(0)}%</div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Buffer Size</div>
                            <div class="detail-card-value">${config.bufferSize || 1000}</div>
                        </div>
                        <div class="detail-card">
                            <div class="detail-card-label">Max Events/sec</div>
                            <div class="detail-card-value">${this.formatNumber(config.maxEventsPerSec || 100000)}</div>
                        </div>
                    </div>
                    <div class="config-preview" style="margin-top: 0.75rem;">
${JSON.stringify(config, null, 2)}
                    </div>
                </div>

                <div class="detail-section">
                    <div class="detail-section-title">
                        <span>&#128197;</span> Version History
                    </div>
                    <div class="version-history">
                        ${(probe.versionHistory || []).map((v, i) => `
                            <div class="version-item ${i === 0 ? 'current' : ''}">
                                <div class="version-info">
                                    <span class="version-number">v${this.escapeHtml(v.version)}</span>
                                    <span class="version-time">${this.formatRelativeTime(v.loadedAt)}</span>
                                </div>
                                ${i === 0 ? '<span class="version-badge">Current</span>' : ''}
                            </div>
                        `).join('')}
                    </div>
                </div>
            </div>
        `;

        // Add action handlers
        container.querySelector('#btn-reload')?.addEventListener('click', () => this.reloadProbe(probe.name));
        container.querySelector('#btn-rollback')?.addEventListener('click', () => this.rollbackProbe(probe.name));
        container.querySelector('#btn-edit-config')?.addEventListener('click', () => this.openConfigEditor(probe));
    }

    async reloadProbe(name) {
        const btn = this.querySelector('#btn-reload');
        if (btn) {
            btn.textContent = 'Reloading...';
            btn.disabled = true;
        }

        try {
            const resp = await fetch(`/api/probes/${encodeURIComponent(name)}/reload`, {
                method: 'POST'
            });

            if (resp.ok) {
                await this.loadData();
                if (this.selectedProbe?.name === name) {
                    const probe = this.probes.find(p => p.name === name);
                    this.selectProbe(probe);
                }
            } else {
                alert('Reload failed: ' + (await resp.text()));
            }
        } catch (e) {
            console.error('Failed to reload probe:', e);
            // Demo: update locally
            const probe = this.probes.find(p => p.name === name);
            if (probe) {
                const newVersion = this.incrementVersion(probe.version?.version || '1.0.0');
                probe.versionHistory = [{ version: newVersion, loadedAt: Date.now() }, ...(probe.versionHistory || [])];
                probe.version = { version: newVersion, loadedAt: Date.now() };
                this.selectProbe(probe);
            }
        } finally {
            if (btn) {
                btn.innerHTML = '&#8635; Reload';
                btn.disabled = false;
            }
        }
    }

    async rollbackProbe(name) {
        const probe = this.probes.find(p => p.name === name);
        if (!probe || !probe.versionHistory || probe.versionHistory.length < 2) {
            alert('No previous version available for rollback');
            return;
        }

        if (!confirm(`Rollback ${name} to version ${probe.versionHistory[1].version}?`)) {
            return;
        }

        const btn = this.querySelector('#btn-rollback');
        if (btn) {
            btn.textContent = 'Rolling back...';
            btn.disabled = true;
        }

        try {
            const resp = await fetch(`/api/probes/${encodeURIComponent(name)}/rollback`, {
                method: 'POST'
            });

            if (resp.ok) {
                await this.loadData();
                if (this.selectedProbe?.name === name) {
                    const updatedProbe = this.probes.find(p => p.name === name);
                    this.selectProbe(updatedProbe);
                }
            } else {
                alert('Rollback failed: ' + (await resp.text()));
            }
        } catch (e) {
            console.error('Failed to rollback probe:', e);
            // Demo: rollback locally
            if (probe.versionHistory.length >= 2) {
                const prevVersion = probe.versionHistory[1];
                probe.version = { ...prevVersion, loadedAt: Date.now() };
                probe.versionHistory = [{ ...prevVersion, loadedAt: Date.now() }, ...probe.versionHistory];
                this.selectProbe(probe);
            }
        } finally {
            if (btn) {
                btn.innerHTML = '&#8630; Rollback';
                btn.disabled = false;
            }
        }
    }

    openConfigEditor(probe) {
        const modal = this.querySelector('#config-modal');
        const editor = this.querySelector('#config-editor');

        if (modal && editor) {
            editor.value = JSON.stringify(probe.config || {}, null, 2);
            modal.style.display = 'flex';
        }
    }

    async saveConfig() {
        if (!this.selectedProbe) return;

        const editor = this.querySelector('#config-editor');
        const format = this.querySelector('input[name="config-format"]:checked')?.value || 'json';

        let config;
        try {
            if (format === 'json') {
                config = JSON.parse(editor.value);
            } else {
                // Simple YAML parsing (would need a real library in production)
                alert('YAML parsing not implemented in this demo');
                return;
            }
        } catch (e) {
            alert('Invalid JSON: ' + e.message);
            return;
        }

        try {
            const resp = await fetch(`/api/probes/${encodeURIComponent(this.selectedProbe.name)}/config`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });

            if (resp.ok) {
                this.querySelector('#config-modal').style.display = 'none';
                await this.loadData();
                const probe = this.probes.find(p => p.name === this.selectedProbe.name);
                this.selectProbe(probe);
            } else {
                alert('Failed to save config: ' + (await resp.text()));
            }
        } catch (e) {
            console.error('Failed to save config:', e);
            // Demo: update locally
            this.selectedProbe.config = config;
            this.querySelector('#config-modal').style.display = 'none';
            this.renderProbeDetail(this.selectedProbe);
        }
    }

    incrementVersion(version) {
        const parts = version.split('.');
        parts[2] = parseInt(parts[2] || 0) + 1;
        return parts.join('.');
    }

    formatNumber(n) {
        if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
        if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
        return n.toString();
    }

    formatRelativeTime(timestamp) {
        if (!timestamp) return 'N/A';
        const diff = Date.now() - timestamp;
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return `${Math.floor(diff / 86400000)}d ago`;
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // Public API
    refresh() {
        this.loadData();
    }
}

customElements.define('hotreload-manager', HotreloadManager);
