/**
 * Synthetics Uptime Widget
 * Shows synthetic monitor status and uptime percentages
 */
class SyntheticsUptime extends HTMLElement {
    constructor() {
        super();
        this.monitors = [];
        this.loading = true;
        this.filter = 'all'; // all, failing, passing
    }

    connectedCallback() {
        this.render();
        this.loadData();
        // Refresh every 60 seconds
        this._refreshInterval = setInterval(() => this.loadData(), 60000);
    }

    disconnectedCallback() {
        if (this._refreshInterval) {
            clearInterval(this._refreshInterval);
            this._refreshInterval = null;
        }
    }

    async loadData() {
        this.loading = true;
        this.render();

        try {
            const resp = await fetch('/api/synthetics/monitors');
            if (resp.ok) {
                this.monitors = await resp.json() || [];
            }
        } catch (e) {
            console.error('Failed to load synthetics data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    setFilter(filter) {
        this.filter = filter;
        this.render();
    }

    getFilteredMonitors() {
        if (this.filter === 'all') return this.monitors;
        if (this.filter === 'failing') return this.monitors.filter(m => m.status === 'failing' || m.status === 'degraded');
        if (this.filter === 'passing') return this.monitors.filter(m => m.status === 'passing');
        return this.monitors;
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="synthetics-uptime">
                    <div class="synth-header">
                        <span class="title-icon">🔍</span>
                        <span>Synthetic Monitors</span>
                    </div>
                    <div class="loading">Loading monitors...</div>
                </div>
            `;
            return;
        }

        const monitors = this.getFilteredMonitors();
        const stats = this.calculateStats();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="synthetics-uptime">
                <div class="synth-header">
                    <div class="header-title">
                        <span class="title-icon">🔍</span>
                        <span>Synthetic Monitors</span>
                    </div>
                    <button class="btn-refresh" onclick="this.getRootNode().host.loadData()">↻</button>
                </div>

                <div class="stats-bar">
                    <div class="stat-item">
                        <span class="stat-value">${stats.total}</span>
                        <span class="stat-label">Total</span>
                    </div>
                    <div class="stat-item passing">
                        <span class="stat-value">${stats.passing}</span>
                        <span class="stat-label">Passing</span>
                    </div>
                    <div class="stat-item failing">
                        <span class="stat-value">${stats.failing}</span>
                        <span class="stat-label">Failing</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${stats.avgUptime.toFixed(1)}%</span>
                        <span class="stat-label">Avg Uptime</span>
                    </div>
                </div>

                <div class="filter-tabs">
                    <button class="tab ${this.filter === 'all' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('all')">All</button>
                    <button class="tab ${this.filter === 'failing' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('failing')">Failing</button>
                    <button class="tab ${this.filter === 'passing' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('passing')">Passing</button>
                </div>

                <div class="monitors-list">
                    ${monitors.length === 0 ? `
                        <div class="empty-state">No monitors match filter</div>
                    ` : monitors.map(m => this.renderMonitor(m)).join('')}
                </div>
            </div>
        `;
    }

    renderMonitor(monitor) {
        const statusClass = monitor.status === 'passing' ? 'passing' :
                           monitor.status === 'degraded' ? 'degraded' : 'failing';
        const statusIcon = monitor.status === 'passing' ? '✓' :
                          monitor.status === 'degraded' ? '!' : '✗';

        return `
            <div class="monitor-item ${statusClass}">
                <div class="monitor-status">
                    <span class="status-icon">${statusIcon}</span>
                </div>
                <div class="monitor-info">
                    <div class="monitor-name">${this.escapeHtml(monitor.name)}</div>
                    <div class="monitor-url">${this.escapeHtml(monitor.url || monitor.endpoint || '')}</div>
                </div>
                <div class="monitor-metrics">
                    <div class="metric">
                        <span class="metric-value">${monitor.uptime?.toFixed(1) || 0}%</span>
                        <span class="metric-label">Uptime</span>
                    </div>
                    <div class="metric">
                        <span class="metric-value">${monitor.latency_ms || 0}ms</span>
                        <span class="metric-label">Latency</span>
                    </div>
                </div>
                <div class="uptime-bar">
                    ${this.renderUptimeBar(monitor.history || [])}
                </div>
            </div>
        `;
    }

    renderUptimeBar(history) {
        // Show last 30 checks as small bars
        const checks = history.slice(-30);
        if (checks.length === 0) {
            // Generate placeholder
            return '<div class="uptime-placeholder">No history</div>';
        }

        return `
            <div class="uptime-bars">
                ${checks.map(c => `
                    <div class="uptime-tick ${c.success ? 'up' : 'down'}"
                         title="${c.timestamp ? new Date(c.timestamp).toLocaleString() : ''}: ${c.success ? 'Up' : 'Down'}">
                    </div>
                `).join('')}
            </div>
        `;
    }

    calculateStats() {
        const total = this.monitors.length;
        const passing = this.monitors.filter(m => m.status === 'passing').length;
        const failing = this.monitors.filter(m => m.status === 'failing' || m.status === 'degraded').length;
        const avgUptime = total > 0
            ? this.monitors.reduce((sum, m) => sum + (m.uptime || 0), 0) / total
            : 100;

        return { total, passing, failing, avgUptime };
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .synthetics-uptime {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .synth-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.25rem 0.5rem;
                cursor: pointer;
            }

            .loading, .empty-state {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            .stats-bar {
                display: flex;
                justify-content: space-around;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .stat-item {
                text-align: center;
            }

            .stat-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
            }

            .stat-item.passing .stat-value { color: var(--success, #00ba7c); }
            .stat-item.failing .stat-value { color: var(--error, #f4212e); }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .filter-tabs {
                display: flex;
                gap: 0.5rem;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tab {
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.75rem;
                font-size: 0.8rem;
                cursor: pointer;
            }

            .tab.active {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
                color: white;
            }

            .monitors-list {
                flex: 1;
                overflow-y: auto;
                padding: 0.5rem;
            }

            .monitor-item {
                display: grid;
                grid-template-columns: auto 1fr auto auto;
                gap: 0.75rem;
                align-items: center;
                padding: 0.75rem;
                margin-bottom: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .monitor-item.passing { border-left-color: var(--success, #00ba7c); }
            .monitor-item.degraded { border-left-color: var(--warning, #ffd400); }
            .monitor-item.failing { border-left-color: var(--error, #f4212e); }

            .monitor-status {
                width: 28px;
                height: 28px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.9rem;
                font-weight: bold;
            }

            .monitor-item.passing .monitor-status {
                background: rgba(0, 186, 124, 0.2);
                color: var(--success, #00ba7c);
            }

            .monitor-item.degraded .monitor-status {
                background: rgba(255, 212, 0, 0.2);
                color: var(--warning, #ffd400);
            }

            .monitor-item.failing .monitor-status {
                background: rgba(244, 33, 46, 0.2);
                color: var(--error, #f4212e);
            }

            .monitor-info {
                min-width: 0;
            }

            .monitor-name {
                font-weight: 500;
                font-size: 0.9rem;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .monitor-url {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .monitor-metrics {
                display: flex;
                gap: 1rem;
            }

            .metric {
                text-align: center;
            }

            .metric-value {
                display: block;
                font-size: 0.9rem;
                font-weight: 600;
            }

            .metric-label {
                font-size: 0.65rem;
                color: var(--text-muted, #71767b);
            }

            .uptime-bar {
                width: 120px;
            }

            .uptime-bars {
                display: flex;
                gap: 2px;
                height: 20px;
                align-items: flex-end;
            }

            .uptime-tick {
                flex: 1;
                min-width: 3px;
                height: 100%;
                border-radius: 1px;
            }

            .uptime-tick.up { background: var(--success, #00ba7c); }
            .uptime-tick.down { background: var(--error, #f4212e); }

            .uptime-placeholder {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                text-align: center;
            }

            @media (max-width: 700px) {
                .monitor-item {
                    grid-template-columns: auto 1fr;
                    grid-template-rows: auto auto;
                }

                .monitor-metrics, .uptime-bar {
                    grid-column: 1 / -1;
                }
            }
        `;
    }
}

customElements.define('synthetics-uptime', SyntheticsUptime);
