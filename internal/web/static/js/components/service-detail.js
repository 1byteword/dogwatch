/**
 * Service Detail Widget
 * Single service deep-dive: RED metrics, traces, logs, deploys
 */
class ServiceDetail extends HTMLElement {
    constructor() {
        super();
        this.service = null;
        this.serviceName = '';
        this.metrics = {};
        this.traces = [];
        this.logs = [];
        this.deploys = [];
        this.incidents = [];
        this.activeTab = 'overview';
    }

    static get observedAttributes() {
        return ['service-name'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'service-name' && newValue && newValue !== oldValue) {
            this.serviceName = newValue;
            this.loadServiceData();
        }
    }

    connectedCallback() {
        this.render();
        if (this.serviceName) {
            this.loadServiceData();
        }
    }

    async loadServiceData() {
        if (!this.serviceName) return;

        this.showLoading();

        try {
            const [catalogResp, tracesResp, deploysResp, incidentsResp] = await Promise.all([
                fetch(`/api/catalog/services?name=${encodeURIComponent(this.serviceName)}`),
                fetch(`/api/traces?service=${encodeURIComponent(this.serviceName)}&limit=20`),
                fetch(`/api/deploys?service=${encodeURIComponent(this.serviceName)}&limit=10`),
                fetch(`/api/incidents?service=${encodeURIComponent(this.serviceName)}&limit=5`)
            ]);

            if (catalogResp.ok) {
                const services = await catalogResp.json();
                this.service = services?.find(s => s.name === this.serviceName) || { name: this.serviceName };
            }

            if (tracesResp.ok) this.traces = await tracesResp.json() || [];
            if (deploysResp.ok) this.deploys = await deploysResp.json() || [];
            if (incidentsResp.ok) this.incidents = await incidentsResp.json() || [];

            // Calculate RED metrics from traces
            this.calculateMetrics();

            this.renderContent();
        } catch (e) {
            console.error('Failed to load service data:', e);
            this.showError(e.message);
        }
    }

    calculateMetrics() {
        if (this.traces.length === 0) {
            this.metrics = { rate: 0, errorRate: 0, p50: 0, p95: 0, p99: 0 };
            return;
        }

        const durations = this.traces.map(t => t.duration_ms || 0).sort((a, b) => a - b);
        const errors = this.traces.filter(t => t.has_error).length;

        this.metrics = {
            rate: this.traces.length, // per time window
            errorRate: (errors / this.traces.length) * 100,
            p50: durations[Math.floor(durations.length * 0.5)] || 0,
            p95: durations[Math.floor(durations.length * 0.95)] || 0,
            p99: durations[Math.floor(durations.length * 0.99)] || 0
        };
    }

    setTab(tab) {
        this.activeTab = tab;
        this.querySelectorAll('.tab-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.tab === tab);
        });
        this.renderTabContent();
    }

    showLoading() {
        const content = this.querySelector('#service-content');
        if (content) {
            content.innerHTML = `<div class="loading"><div class="spinner"></div>Loading...</div>`;
        }
    }

    showError(message) {
        const content = this.querySelector('#service-content');
        if (content) {
            content.innerHTML = `<div class="error">Error: ${message}</div>`;
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="service-detail">
                <div class="service-header" id="service-header">
                    <div class="header-loading">Select a service to view details</div>
                </div>
                <div class="service-tabs">
                    <button class="tab-btn active" data-tab="overview" onclick="this.getRootNode().host.setTab('overview')">Overview</button>
                    <button class="tab-btn" data-tab="traces" onclick="this.getRootNode().host.setTab('traces')">Traces</button>
                    <button class="tab-btn" data-tab="logs" onclick="this.getRootNode().host.setTab('logs')">Logs</button>
                    <button class="tab-btn" data-tab="deploys" onclick="this.getRootNode().host.setTab('deploys')">Deploys</button>
                    <button class="tab-btn" data-tab="incidents" onclick="this.getRootNode().host.setTab('incidents')">Incidents</button>
                </div>
                <div class="service-content" id="service-content">
                    <div class="empty-state">
                        <span class="icon">🔧</span>
                        <p>Select a service to view details</p>
                    </div>
                </div>
            </div>
        `;
    }

    renderContent() {
        this.renderHeader();
        this.renderTabContent();
    }

    renderHeader() {
        const header = this.querySelector('#service-header');
        if (!header) return;

        const health = this.getHealthStatus();
        const tier = this.service?.tier || 'unknown';

        header.innerHTML = `
            <div class="header-main">
                <div class="service-icon">${this.getServiceIcon()}</div>
                <div class="service-info">
                    <h1 class="service-name">${this.escapeHtml(this.serviceName)}</h1>
                    <div class="service-meta">
                        ${this.service?.team ? `<span class="meta-item">Team: ${this.escapeHtml(this.service.team)}</span>` : ''}
                        ${this.service?.owner ? `<span class="meta-item">Owner: ${this.escapeHtml(this.service.owner)}</span>` : ''}
                        <span class="meta-item">Tier: ${tier}</span>
                    </div>
                </div>
                <div class="service-status">
                    <span class="health-badge ${health.status}">${health.label}</span>
                </div>
            </div>
            <div class="header-metrics">
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.rate}</span>
                    <span class="metric-label">Requests</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value ${this.metrics.errorRate > 1 ? 'error' : ''}">${this.metrics.errorRate.toFixed(2)}%</span>
                    <span class="metric-label">Error Rate</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p50.toFixed(0)}ms</span>
                    <span class="metric-label">p50 Latency</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p95.toFixed(0)}ms</span>
                    <span class="metric-label">p95 Latency</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p99.toFixed(0)}ms</span>
                    <span class="metric-label">p99 Latency</span>
                </div>
            </div>
        `;
    }

    renderTabContent() {
        const content = this.querySelector('#service-content');
        if (!content) return;

        switch (this.activeTab) {
            case 'overview':
                content.innerHTML = this.renderOverview();
                break;
            case 'traces':
                content.innerHTML = this.renderTraces();
                break;
            case 'logs':
                content.innerHTML = this.renderLogs();
                break;
            case 'deploys':
                content.innerHTML = this.renderDeploys();
                break;
            case 'incidents':
                content.innerHTML = this.renderIncidents();
                break;
        }
    }

    renderOverview() {
        return `
            <div class="overview-grid">
                <div class="overview-card">
                    <h3>Service Info</h3>
                    <div class="info-list">
                        <div class="info-item">
                            <span class="info-label">Repository</span>
                            <span class="info-value">${this.service?.repo_url ? `<a href="${this.escapeHtml(this.service.repo_url)}" target="_blank">${this.escapeHtml(this.service.repo_url)}</a>` : '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Language</span>
                            <span class="info-value">${this.service?.language || '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Framework</span>
                            <span class="info-value">${this.service?.framework || '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Tags</span>
                            <span class="info-value">${this.service?.tags?.join(', ') || '—'}</span>
                        </div>
                    </div>
                </div>
                <div class="overview-card">
                    <h3>Recent Activity</h3>
                    <div class="activity-list">
                        ${this.deploys.slice(0, 3).map(d => `
                            <div class="activity-item">
                                <span class="activity-icon">🚀</span>
                                <span class="activity-text">Deploy: ${this.escapeHtml(d.version || d.commit?.substring(0, 7) || 'unknown')}</span>
                                <span class="activity-time">${this.formatTime(d.deployed_at)}</span>
                            </div>
                        `).join('') || '<div class="empty">No recent deploys</div>'}
                        ${this.incidents.filter(i => i.status !== 'resolved').slice(0, 2).map(i => `
                            <div class="activity-item incident">
                                <span class="activity-icon">🚨</span>
                                <span class="activity-text">${this.escapeHtml(i.title)}</span>
                                <span class="activity-time">${this.formatTime(i.created_at)}</span>
                            </div>
                        `).join('')}
                    </div>
                </div>
                <div class="overview-card full-width">
                    <h3>Dependencies</h3>
                    <div class="deps-list">
                        ${this.service?.dependencies?.map(d => `
                            <span class="dep-badge">${this.escapeHtml(d)}</span>
                        `).join('') || '<span class="empty">No dependencies tracked</span>'}
                    </div>
                </div>
            </div>
        `;
    }

    renderTraces() {
        if (this.traces.length === 0) {
            return `<div class="empty-state"><span class="icon">🔍</span><p>No traces found</p></div>`;
        }

        return `
            <div class="traces-list">
                <div class="list-header">
                    <span class="col-status">Status</span>
                    <span class="col-operation">Operation</span>
                    <span class="col-duration">Duration</span>
                    <span class="col-time">Time</span>
                </div>
                ${this.traces.map(t => `
                    <div class="trace-row ${t.has_error ? 'error' : ''}" onclick="window.open('/traces.html?id=${t.trace_id}', '_blank')">
                        <span class="col-status">
                            <span class="status-dot ${t.has_error ? 'error' : 'ok'}"></span>
                        </span>
                        <span class="col-operation">${this.escapeHtml(t.root_span || 'unknown')}</span>
                        <span class="col-duration">${(t.duration_ms || 0).toFixed(1)}ms</span>
                        <span class="col-time">${this.formatTime(t.start_time)}</span>
                    </div>
                `).join('')}
            </div>
            <div class="list-footer">
                <a href="/traces.html?service=${encodeURIComponent(this.serviceName)}" class="link-more">View all traces →</a>
            </div>
        `;
    }

    renderLogs() {
        return `
            <div class="logs-embed">
                <log-explorer></log-explorer>
            </div>
            <div class="list-footer">
                <a href="/logs.html?service=${encodeURIComponent(this.serviceName)}" class="link-more">Open in Log Explorer →</a>
            </div>
        `;
    }

    renderDeploys() {
        if (this.deploys.length === 0) {
            return `<div class="empty-state"><span class="icon">🚀</span><p>No deploys found</p></div>`;
        }

        return `
            <div class="deploys-list">
                ${this.deploys.map(d => `
                    <div class="deploy-card ${d.status || 'success'}">
                        <div class="deploy-main">
                            <div class="deploy-version">${this.escapeHtml(d.version || d.commit?.substring(0, 7) || 'unknown')}</div>
                            <div class="deploy-meta">
                                <span>by ${this.escapeHtml(d.deployed_by || 'unknown')}</span>
                                <span>${this.formatTime(d.deployed_at)}</span>
                            </div>
                            ${d.message ? `<div class="deploy-message">${this.escapeHtml(d.message)}</div>` : ''}
                        </div>
                        <div class="deploy-status">
                            <span class="status-badge ${d.status || 'success'}">${d.status || 'success'}</span>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderIncidents() {
        if (this.incidents.length === 0) {
            return `<div class="empty-state"><span class="icon">✓</span><p>No incidents</p></div>`;
        }

        return `
            <div class="incidents-list">
                ${this.incidents.map(i => `
                    <div class="incident-card ${i.severity}">
                        <div class="incident-severity">
                            ${this.getSeverityIcon(i.severity)}
                        </div>
                        <div class="incident-main">
                            <div class="incident-title">${this.escapeHtml(i.title)}</div>
                            <div class="incident-meta">
                                <span class="status-badge ${i.status}">${i.status}</span>
                                <span>${this.formatTime(i.created_at)}</span>
                            </div>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    getHealthStatus() {
        if (this.metrics.errorRate > 5) return { status: 'critical', label: 'Critical' };
        if (this.metrics.errorRate > 1) return { status: 'degraded', label: 'Degraded' };
        return { status: 'healthy', label: 'Healthy' };
    }

    getServiceIcon() {
        const type = this.service?.type || 'service';
        switch (type) {
            case 'database': return '🗄️';
            case 'cache': return '⚡';
            case 'queue': return '📬';
            case 'gateway': return '🚪';
            case 'frontend': return '🖥️';
            default: return '🔧';
        }
    }

    getSeverityIcon(severity) {
        switch (severity) {
            case 'critical': return '🔴';
            case 'high': return '🟠';
            case 'medium': return '🟡';
            case 'low': return '🟢';
            default: return '⚪';
        }
    }

    formatTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const now = new Date();
        const diffMs = now - d;

        if (diffMs < 60000) return 'just now';
        if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
        if (diffMs < 86400000) return `${Math.floor(diffMs / 3600000)}h ago`;

        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .service-detail {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .service-header {
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-loading {
                text-align: center;
                color: var(--text-muted, #71767b);
                padding: 1rem;
            }

            .header-main {
                display: flex;
                align-items: center;
                gap: 1rem;
                margin-bottom: 1rem;
            }

            .service-icon {
                font-size: 2rem;
            }

            .service-info { flex: 1; }

            .service-name {
                font-size: 1.25rem;
                font-weight: 600;
                margin: 0 0 0.25rem 0;
            }

            .service-meta {
                display: flex;
                gap: 1rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .health-badge {
                padding: 0.25rem 0.75rem;
                border-radius: 12px;
                font-size: 0.8rem;
                font-weight: 500;
            }

            .health-badge.healthy { background: var(--success, #00ba7c); color: white; }
            .health-badge.degraded { background: var(--warning, #ffd400); color: #1a1a1a; }
            .health-badge.critical { background: var(--error, #f4212e); color: white; }

            .header-metrics {
                display: flex;
                gap: 1rem;
            }

            .metric-card {
                background: var(--bg-card, #16181c);
                padding: 0.75rem 1rem;
                border-radius: 8px;
                text-align: center;
                min-width: 80px;
            }

            .metric-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
            }

            .metric-value.error { color: var(--error, #f4212e); }

            .metric-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .service-tabs {
                display: flex;
                gap: 0.25rem;
                padding: 0 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tab-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.75rem 1rem;
                cursor: pointer;
                border-bottom: 2px solid transparent;
                margin-bottom: -1px;
            }

            .tab-btn:hover { color: var(--text, #e7e9ea); }
            .tab-btn.active {
                color: var(--accent, #1d9bf0);
                border-bottom-color: var(--accent, #1d9bf0);
            }

            .service-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .empty-state, .error {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .spinner {
                width: 24px;
                height: 24px;
                border: 3px solid var(--border, #2f3336);
                border-top-color: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: spin 0.8s linear infinite;
                margin-bottom: 0.5rem;
            }

            @keyframes spin { to { transform: rotate(360deg); } }

            .empty-state .icon { font-size: 2rem; margin-bottom: 0.5rem; }

            /* Overview */
            .overview-grid {
                display: grid;
                grid-template-columns: repeat(2, 1fr);
                gap: 1rem;
            }

            .overview-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .overview-card.full-width { grid-column: span 2; }

            .overview-card h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .info-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .info-item {
                display: flex;
                justify-content: space-between;
                font-size: 0.85rem;
            }

            .info-label { color: var(--text-muted, #71767b); }

            .info-value a { color: var(--accent, #1d9bf0); text-decoration: none; }

            .activity-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .activity-item {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-size: 0.85rem;
            }

            .activity-item.incident { color: var(--error, #f4212e); }

            .activity-time {
                margin-left: auto;
                color: var(--text-muted, #71767b);
                font-size: 0.75rem;
            }

            .deps-list { display: flex; flex-wrap: wrap; gap: 0.5rem; }

            .dep-badge {
                background: var(--bg-card, #16181c);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.8rem;
            }

            /* Traces */
            .traces-list { display: flex; flex-direction: column; }

            .list-header {
                display: flex;
                padding: 0.5rem 0;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .trace-row {
                display: flex;
                padding: 0.75rem 0;
                border-bottom: 1px solid var(--border, #2f3336);
                cursor: pointer;
            }

            .trace-row:hover { background: var(--bg-elevated, #1e2128); }

            .col-status { width: 40px; }
            .col-operation { flex: 1; }
            .col-duration { width: 80px; text-align: right; }
            .col-time { width: 100px; text-align: right; color: var(--text-muted, #71767b); }

            .status-dot {
                display: inline-block;
                width: 8px;
                height: 8px;
                border-radius: 50%;
            }

            .status-dot.ok { background: var(--success, #00ba7c); }
            .status-dot.error { background: var(--error, #f4212e); }

            .list-footer {
                padding: 1rem 0;
                text-align: center;
            }

            .link-more {
                color: var(--accent, #1d9bf0);
                text-decoration: none;
                font-size: 0.85rem;
            }

            /* Deploys */
            .deploys-list { display: flex; flex-direction: column; gap: 0.75rem; }

            .deploy-card {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 3px solid var(--success, #00ba7c);
            }

            .deploy-card.failed { border-left-color: var(--error, #f4212e); }

            .deploy-version { font-weight: 600; margin-bottom: 0.25rem; }

            .deploy-meta {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                display: flex;
                gap: 1rem;
            }

            .deploy-message {
                font-size: 0.85rem;
                margin-top: 0.5rem;
            }

            .status-badge {
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.7rem;
                text-transform: uppercase;
            }

            .status-badge.success { background: var(--success, #00ba7c); color: white; }
            .status-badge.failed { background: var(--error, #f4212e); color: white; }

            /* Incidents */
            .incidents-list { display: flex; flex-direction: column; gap: 0.75rem; }

            .incident-card {
                display: flex;
                gap: 0.75rem;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .incident-card.critical { border-left-color: var(--error, #f4212e); }
            .incident-card.high { border-left-color: #ff7a00; }

            .incident-severity { font-size: 1.25rem; }
            .incident-main { flex: 1; }

            .incident-title { font-weight: 500; margin-bottom: 0.25rem; }

            .incident-meta {
                display: flex;
                gap: 0.75rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .status-badge.active { background: var(--error, #f4212e); color: white; }
            .status-badge.resolved { background: var(--success, #00ba7c); color: white; }

            .logs-embed {
                height: 300px;
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                overflow: hidden;
            }

            .empty { color: var(--text-muted, #71767b); font-size: 0.85rem; }
        `;
    }
}

customElements.define('service-detail', ServiceDetail);
