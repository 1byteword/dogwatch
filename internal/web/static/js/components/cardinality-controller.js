/**
 * Cardinality Controller Widget
 * Monitors and manages metric cardinality with circuit breaker protection
 */
class CardinalityController extends HTMLElement {
    constructor() {
        super();
        this.dashboard = null;
        this.stats = null;
        this.topMetrics = [];
        this.topLabels = [];
        this.circuitBreaker = null;
        this.quarantine = [];
        this.alerts = [];
        this.historicalData = [];
        this.loading = true;
        this.refreshInterval = null;
        this.chart = null;
        this._mounted = false;
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
        // Auto-refresh every 15 seconds
        this.refreshInterval = setInterval(() => {
            if (this._mounted) this.loadData();
        }, 15000);
    }

    disconnectedCallback() {
        this._mounted = false;
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    async loadData() {
        try {
            const [dashboardResp, metricsResp, labelsResp, cbResp, quarantineResp, alertsResp] = await Promise.all([
                fetch('/api/cardinality/dashboard'),
                fetch('/api/cardinality/controller/metrics?limit=10'),
                fetch('/api/cardinality/controller/labels?limit=10'),
                fetch('/api/cardinality/circuit-breaker'),
                fetch('/api/cardinality/quarantine'),
                fetch('/api/cardinality/alerts?limit=20')
            ]);

            if (dashboardResp.ok) this.dashboard = await dashboardResp.json();
            if (metricsResp.ok) {
                const data = await metricsResp.json();
                this.topMetrics = data.metrics || [];
            }
            if (labelsResp.ok) {
                const data = await labelsResp.json();
                this.topLabels = data.labels || [];
            }
            if (cbResp.ok) this.circuitBreaker = await cbResp.json();
            if (quarantineResp.ok) {
                const data = await quarantineResp.json();
                this.quarantine = data.quarantined || [];
            }
            if (alertsResp.ok) {
                const data = await alertsResp.json();
                this.alerts = data.alerts || [];
            }

            // Extract stats from dashboard if available
            if (this.dashboard) {
                this.stats = this.dashboard.stats;
            }

            // Track historical data for chart
            this.updateHistoricalData();

        } catch (e) {
            console.error('Failed to load cardinality data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    updateHistoricalData() {
        if (!this.stats) return;

        const dataPoint = {
            timestamp: Date.now(),
            totalSeries: this.stats.total_series || 0,
            totalMetrics: this.stats.total_metrics || 0
        };

        this.historicalData.push(dataPoint);

        // Keep last 30 data points (7.5 minutes at 15s intervals)
        if (this.historicalData.length > 30) {
            this.historicalData.shift();
        }
    }

    async resetCircuitBreaker() {
        try {
            const resp = await fetch('/api/cardinality/circuit-breaker/reset', {
                method: 'POST'
            });
            if (resp.ok) {
                this.showToast('Circuit breaker reset successfully');
                this.loadData();
            } else {
                this.showToast('Failed to reset circuit breaker', 'error');
            }
        } catch (e) {
            this.showToast('Error: ' + e.message, 'error');
        }
    }

    async releaseFromQuarantine(metricName) {
        try {
            const resp = await fetch('/api/cardinality/quarantine/' + encodeURIComponent(metricName), {
                method: 'DELETE'
            });
            if (resp.ok) {
                this.showToast('Released ' + metricName + ' from quarantine');
                this.loadData();
            } else {
                this.showToast('Failed to release from quarantine', 'error');
            }
        } catch (e) {
            this.showToast('Error: ' + e.message, 'error');
        }
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatNumber(num) {
        if (!num && num !== 0) return '-';
        if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
        if (num >= 1000) return (num / 1000).toFixed(1) + 'k';
        return num.toString();
    }

    formatRelativeTime(timestamp) {
        if (!timestamp) return '-';
        const d = new Date(timestamp);
        const diff = Date.now() - d.getTime();
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getCircuitBreakerClass() {
        if (!this.circuitBreaker) return 'closed';
        switch (this.circuitBreaker.state) {
            case 'open': return 'open';
            case 'half_open': return 'half-open';
            default: return 'closed';
        }
    }

    getCircuitBreakerLabel() {
        if (!this.circuitBreaker) return 'Closed';
        switch (this.circuitBreaker.state) {
            case 'open': return 'OPEN';
            case 'half_open': return 'Half-Open';
            default: return 'Closed';
        }
    }

    getSeverityClass(severity) {
        switch (severity) {
            case 'critical': return 'critical';
            case 'warning': return 'warning';
            default: return 'info';
        }
    }

    renderSparkline() {
        if (this.historicalData.length < 2) return '';

        const width = 200;
        const height = 40;
        const padding = 2;
        const data = this.historicalData.map(d => d.totalSeries);
        const min = Math.min(...data);
        const max = Math.max(...data) || 1;
        const range = max - min || 1;

        const points = data.map((value, i) => {
            const x = padding + (i / (data.length - 1)) * (width - 2 * padding);
            const y = height - padding - ((value - min) / range) * (height - 2 * padding);
            return `${x},${y}`;
        }).join(' ');

        return `
            <svg width="${width}" height="${height}" class="sparkline">
                <polyline
                    points="${points}"
                    fill="none"
                    stroke="var(--accent, #1d9bf0)"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                />
            </svg>
        `;
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cardinality-controller">
                    <div class="header">
                        <div class="title">Cardinality Controller</div>
                    </div>
                    <div class="loading">Loading cardinality data...</div>
                </div>
            `;
            return;
        }

        const stats = this.stats || {};
        const cb = this.circuitBreaker || {};
        const trends = this.dashboard?.trends || {};

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cardinality-controller">
                <div class="header">
                    <div class="title">Cardinality Controller</div>
                    <div class="header-actions">
                        <button class="btn btn-secondary" onclick="this.getRootNode().host.loadData()">Refresh</button>
                    </div>
                </div>

                <!-- Overview Stats -->
                <div class="overview-section">
                    <div class="stats-row">
                        <div class="stat-card primary">
                            <div class="stat-value">${this.formatNumber(stats.total_series)}</div>
                            <div class="stat-label">Total Series</div>
                            <div class="stat-trend">${this.renderSparkline()}</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${this.formatNumber(stats.total_metrics)}</div>
                            <div class="stat-label">Total Metrics</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${this.formatNumber(stats.total_label_keys)}</div>
                            <div class="stat-label">Label Keys</div>
                        </div>
                        <div class="stat-card ${stats.high_cardinality_metrics > 0 ? 'warning' : ''}">
                            <div class="stat-value">${stats.high_cardinality_metrics || 0}</div>
                            <div class="stat-label">High Cardinality</div>
                        </div>
                    </div>
                </div>

                <!-- Circuit Breaker Status -->
                <div class="circuit-breaker-section">
                    <div class="circuit-breaker ${this.getCircuitBreakerClass()}">
                        <div class="cb-visual">
                            <div class="cb-icon">
                                <div class="cb-ring"></div>
                                <div class="cb-center"></div>
                            </div>
                            <div class="cb-state">${this.getCircuitBreakerLabel()}</div>
                        </div>
                        <div class="cb-info">
                            <div class="cb-stats">
                                <div class="cb-stat">
                                    <span class="cb-stat-label">Total Series:</span>
                                    <span class="cb-stat-value">${this.formatNumber(cb.total_series)}</span>
                                </div>
                                <div class="cb-stat">
                                    <span class="cb-stat-label">Threshold:</span>
                                    <span class="cb-stat-value">${this.formatNumber(cb.threshold)}</span>
                                </div>
                                <div class="cb-stat">
                                    <span class="cb-stat-label">Blocked:</span>
                                    <span class="cb-stat-value">${this.formatNumber(cb.blocked_count)}</span>
                                </div>
                            </div>
                            ${cb.state === 'open' ? `
                                <button class="btn btn-warning btn-sm" onclick="this.getRootNode().host.resetCircuitBreaker()">
                                    Reset Circuit Breaker
                                </button>
                            ` : ''}
                        </div>
                    </div>
                    <div class="cb-progress">
                        <div class="cb-progress-bar" style="width: ${Math.min(100, (cb.total_series / cb.threshold * 100) || 0)}%"></div>
                    </div>
                    <div class="cb-progress-labels">
                        <span>0</span>
                        <span>${this.formatNumber(cb.threshold)}</span>
                    </div>
                </div>

                <!-- Top Offenders -->
                <div class="offenders-section">
                    <div class="section-header">
                        <h3>Top Cardinality Metrics</h3>
                    </div>
                    ${this.topMetrics.length > 0 ? `
                    <div class="offenders-list">
                        ${this.topMetrics.map((m, i) => `
                            <div class="offender-item ${m.quarantined ? 'quarantined' : ''}">
                                <div class="offender-rank">#${i + 1}</div>
                                <div class="offender-info">
                                    <div class="offender-name">${this.escapeHtml(m.name || m.Name)}</div>
                                    <div class="offender-labels">${m.label_count || m.LabelCount || 0} labels</div>
                                </div>
                                <div class="offender-series">
                                    <div class="series-value">${this.formatNumber(m.total_series || m.TotalSeries)}</div>
                                    <div class="series-bar">
                                        <div class="series-fill" style="width: ${Math.min(100, ((m.total_series || m.TotalSeries) / (this.topMetrics[0]?.total_series || this.topMetrics[0]?.TotalSeries || 1)) * 100)}%"></div>
                                    </div>
                                </div>
                                ${m.quarantined || m.Quarantined ? '<span class="badge quarantine">Quarantined</span>' : ''}
                                ${m.blocked || m.Blocked > 0 ? `<span class="badge blocked">${m.blocked || m.Blocked} blocked</span>` : ''}
                            </div>
                        `).join('')}
                    </div>
                    ` : '<div class="empty-state">No metrics recorded yet</div>'}
                </div>

                <!-- Top Labels -->
                <div class="labels-section">
                    <div class="section-header">
                        <h3>Top Cardinality Labels</h3>
                    </div>
                    ${this.topLabels.length > 0 ? `
                    <div class="labels-list">
                        ${this.topLabels.map((l, i) => `
                            <div class="label-item ${l.is_problematic || l.IsProblematic ? 'problematic' : ''}">
                                <div class="label-rank">#${i + 1}</div>
                                <div class="label-info">
                                    <div class="label-key">${this.escapeHtml(l.key || l.Key)}</div>
                                    <div class="label-metrics">${l.metric_count || l.MetricCount || 0} metrics</div>
                                </div>
                                <div class="label-values">
                                    <div class="values-count">${this.formatNumber(l.unique_values || l.UniqueValues)}</div>
                                    <div class="values-label">unique values</div>
                                </div>
                                ${l.is_problematic || l.IsProblematic ? '<span class="badge warning">Problematic</span>' : ''}
                            </div>
                        `).join('')}
                    </div>
                    ` : '<div class="empty-state">No labels recorded yet</div>'}
                </div>

                <!-- Quarantine List -->
                <div class="quarantine-section">
                    <div class="section-header">
                        <h3>Quarantined Metrics (${this.quarantine.length})</h3>
                    </div>
                    ${this.quarantine.length > 0 ? `
                    <div class="quarantine-list">
                        ${this.quarantine.map(q => `
                            <div class="quarantine-item">
                                <div class="quarantine-info">
                                    <div class="quarantine-name">${this.escapeHtml(q.metric_name || q.MetricName)}</div>
                                    <div class="quarantine-reason">${this.escapeHtml(q.reason || q.Reason)}</div>
                                    <div class="quarantine-time">Quarantined ${this.formatRelativeTime(q.quarantined_at || q.QuarantinedAt)}</div>
                                    ${(q.allowed_labels || q.AllowedLabels || []).length > 0 ? `
                                        <div class="quarantine-allowed">Allowed: ${(q.allowed_labels || q.AllowedLabels).join(', ')}</div>
                                    ` : ''}
                                </div>
                                <button class="btn btn-sm btn-secondary" onclick="this.getRootNode().host.releaseFromQuarantine('${this.escapeHtml(q.metric_name || q.MetricName)}')">
                                    Release
                                </button>
                            </div>
                        `).join('')}
                    </div>
                    ` : '<div class="empty-state">No metrics in quarantine</div>'}
                </div>

                <!-- Recent Alerts -->
                <div class="alerts-section">
                    <div class="section-header">
                        <h3>Recent Alerts</h3>
                    </div>
                    ${this.alerts.length > 0 ? `
                    <div class="alerts-list">
                        ${this.alerts.slice(0, 10).map(a => `
                            <div class="alert-item ${this.getSeverityClass(a.severity)}">
                                <div class="alert-icon ${this.getSeverityClass(a.severity)}">!</div>
                                <div class="alert-content">
                                    <div class="alert-type">${this.escapeHtml(a.alert_type)}</div>
                                    <div class="alert-message">${this.escapeHtml(a.message)}</div>
                                    <div class="alert-meta">
                                        ${a.metric_name ? `<span>Metric: ${this.escapeHtml(a.metric_name)}</span>` : ''}
                                        <span>${this.formatRelativeTime(a.timestamp)}</span>
                                    </div>
                                </div>
                                <div class="alert-value">
                                    ${a.current_value ? this.formatNumber(a.current_value) : ''}
                                </div>
                            </div>
                        `).join('')}
                    </div>
                    ` : '<div class="empty-state">No recent alerts</div>'}
                </div>

                <!-- Trends Summary -->
                ${trends.growing_metrics?.length > 0 || trends.stable_metrics > 0 ? `
                <div class="trends-section">
                    <div class="section-header">
                        <h3>Cardinality Trends</h3>
                    </div>
                    <div class="trends-grid">
                        <div class="trend-card growing">
                            <div class="trend-value">${trends.growing_metrics?.length || 0}</div>
                            <div class="trend-label">Growing Rapidly</div>
                        </div>
                        <div class="trend-card stable">
                            <div class="trend-value">${trends.stable_metrics || 0}</div>
                            <div class="trend-label">Stable</div>
                        </div>
                        <div class="trend-card declining">
                            <div class="trend-value">${trends.declining_metrics || 0}</div>
                            <div class="trend-label">Declining</div>
                        </div>
                    </div>
                    ${trends.growing_metrics?.length > 0 ? `
                    <div class="growing-list">
                        <div class="growing-title">Rapidly Growing:</div>
                        <div class="growing-metrics">
                            ${trends.growing_metrics.slice(0, 5).map(m => `
                                <span class="growing-metric">${this.escapeHtml(m)}</span>
                            `).join('')}
                        </div>
                    </div>
                    ` : ''}
                </div>
                ` : ''}

                <!-- Footer Info -->
                <div class="footer-info">
                    <span>Last updated: ${this.formatRelativeTime(stats.last_updated)}</span>
                    <span>Blocked requests: ${this.formatNumber(stats.blocked_requests)}</span>
                    <span>Alerts generated: ${stats.alerts_generated || 0}</span>
                </div>
            </div>
        `;
    }

    getStyles() {
        return `
            .cardinality-controller {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .title {
                font-weight: 600;
                font-size: 1rem;
            }

            .header-actions {
                display: flex;
                gap: 0.5rem;
            }

            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            /* Overview Section */
            .overview-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .stats-row {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.75rem;
            }

            .stat-card {
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 6px;
                text-align: center;
            }

            .stat-card.primary {
                background: linear-gradient(135deg, rgba(29, 155, 240, 0.1) 0%, rgba(29, 155, 240, 0.05) 100%);
                border: 1px solid rgba(29, 155, 240, 0.2);
            }

            .stat-card.warning {
                background: rgba(255, 212, 0, 0.1);
                border: 1px solid rgba(255, 212, 0, 0.2);
            }

            .stat-value {
                font-size: 1.5rem;
                font-weight: 700;
                color: var(--accent, #1d9bf0);
            }

            .stat-card.warning .stat-value {
                color: var(--warning, #ffd400);
            }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            .stat-trend {
                margin-top: 0.5rem;
            }

            .sparkline {
                width: 100%;
                max-width: 200px;
            }

            /* Circuit Breaker Section */
            .circuit-breaker-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .circuit-breaker {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 1rem;
                border-radius: 8px;
                background: var(--bg-elevated, #1e2128);
                border-left: 4px solid var(--success, #00ba7c);
            }

            .circuit-breaker.open {
                border-left-color: var(--error, #f4212e);
                background: rgba(244, 33, 46, 0.05);
            }

            .circuit-breaker.half-open {
                border-left-color: var(--warning, #ffd400);
                background: rgba(255, 212, 0, 0.05);
            }

            .cb-visual {
                display: flex;
                flex-direction: column;
                align-items: center;
                gap: 0.5rem;
            }

            .cb-icon {
                position: relative;
                width: 48px;
                height: 48px;
            }

            .cb-ring {
                position: absolute;
                inset: 0;
                border: 3px solid var(--success, #00ba7c);
                border-radius: 50%;
                animation: pulse 2s infinite;
            }

            .circuit-breaker.open .cb-ring {
                border-color: var(--error, #f4212e);
                animation: none;
            }

            .circuit-breaker.half-open .cb-ring {
                border-color: var(--warning, #ffd400);
                animation: blink 1s infinite;
            }

            .cb-center {
                position: absolute;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                width: 20px;
                height: 20px;
                background: var(--success, #00ba7c);
                border-radius: 50%;
            }

            .circuit-breaker.open .cb-center {
                background: var(--error, #f4212e);
            }

            .circuit-breaker.half-open .cb-center {
                background: var(--warning, #ffd400);
            }

            @keyframes pulse {
                0%, 100% { transform: scale(1); opacity: 1; }
                50% { transform: scale(1.1); opacity: 0.7; }
            }

            @keyframes blink {
                0%, 100% { opacity: 1; }
                50% { opacity: 0.5; }
            }

            .cb-state {
                font-size: 0.75rem;
                font-weight: 600;
                text-transform: uppercase;
            }

            .circuit-breaker.closed .cb-state { color: var(--success, #00ba7c); }
            .circuit-breaker.open .cb-state { color: var(--error, #f4212e); }
            .circuit-breaker.half-open .cb-state { color: var(--warning, #ffd400); }

            .cb-info {
                flex: 1;
            }

            .cb-stats {
                display: flex;
                gap: 1.5rem;
                margin-bottom: 0.5rem;
            }

            .cb-stat {
                display: flex;
                flex-direction: column;
            }

            .cb-stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .cb-stat-value {
                font-size: 0.9rem;
                font-weight: 600;
            }

            .cb-progress {
                height: 6px;
                background: var(--bg-elevated, #1e2128);
                border-radius: 3px;
                margin-top: 0.75rem;
                overflow: hidden;
            }

            .cb-progress-bar {
                height: 100%;
                background: linear-gradient(90deg, var(--success, #00ba7c) 0%, var(--warning, #ffd400) 70%, var(--error, #f4212e) 100%);
                border-radius: 3px;
                transition: width 0.3s ease;
            }

            .cb-progress-labels {
                display: flex;
                justify-content: space-between;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            /* Section Headers */
            .section-header {
                padding: 0.75rem 1rem 0.5rem;
            }

            .section-header h3 {
                font-size: 0.85rem;
                margin: 0;
                color: var(--text-muted, #71767b);
            }

            /* Offenders Section */
            .offenders-section {
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .offenders-list {
                padding: 0 1rem 1rem;
            }

            .offender-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
            }

            .offender-item.quarantined {
                opacity: 0.7;
                border-left: 3px solid var(--warning, #ffd400);
            }

            .offender-rank {
                font-size: 0.75rem;
                font-weight: 600;
                color: var(--text-muted, #71767b);
                width: 24px;
            }

            .offender-info {
                flex: 1;
                min-width: 0;
            }

            .offender-name {
                font-weight: 500;
                font-size: 0.85rem;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .offender-labels {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .offender-series {
                width: 100px;
                text-align: right;
            }

            .series-value {
                font-weight: 600;
                font-size: 0.85rem;
            }

            .series-bar {
                height: 4px;
                background: var(--bg-card, #16181c);
                border-radius: 2px;
                margin-top: 0.25rem;
                overflow: hidden;
            }

            .series-fill {
                height: 100%;
                background: var(--accent, #1d9bf0);
                border-radius: 2px;
            }

            /* Labels Section */
            .labels-section {
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .labels-list {
                padding: 0 1rem 1rem;
            }

            .label-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
            }

            .label-item.problematic {
                border-left: 3px solid var(--error, #f4212e);
            }

            .label-rank {
                font-size: 0.75rem;
                font-weight: 600;
                color: var(--text-muted, #71767b);
                width: 24px;
            }

            .label-info {
                flex: 1;
            }

            .label-key {
                font-weight: 500;
                font-size: 0.85rem;
                font-family: monospace;
            }

            .label-metrics {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .label-values {
                text-align: right;
            }

            .values-count {
                font-weight: 600;
                font-size: 0.85rem;
            }

            .values-label {
                font-size: 0.65rem;
                color: var(--text-muted, #71767b);
            }

            /* Quarantine Section */
            .quarantine-section {
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .quarantine-list {
                padding: 0 1rem 1rem;
            }

            .quarantine-item {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--warning, #ffd400);
                margin-bottom: 0.5rem;
            }

            .quarantine-name {
                font-weight: 500;
                font-size: 0.85rem;
            }

            .quarantine-reason {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .quarantine-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .quarantine-allowed {
                font-size: 0.7rem;
                color: var(--success, #00ba7c);
                margin-top: 0.25rem;
            }

            /* Alerts Section */
            .alerts-section {
                border-bottom: 1px solid var(--border, #2f3336);
                flex: 1;
                overflow-y: auto;
            }

            .alerts-list {
                padding: 0 1rem 1rem;
            }

            .alert-item {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
                border-left: 3px solid var(--border, #2f3336);
            }

            .alert-item.critical {
                border-left-color: var(--error, #f4212e);
            }

            .alert-item.warning {
                border-left-color: var(--warning, #ffd400);
            }

            .alert-item.info {
                border-left-color: var(--accent, #1d9bf0);
            }

            .alert-icon {
                width: 20px;
                height: 20px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 50%;
                font-size: 0.7rem;
                font-weight: 700;
            }

            .alert-icon.critical { background: rgba(244, 33, 46, 0.2); color: var(--error, #f4212e); }
            .alert-icon.warning { background: rgba(255, 212, 0, 0.2); color: var(--warning, #ffd400); }
            .alert-icon.info { background: rgba(29, 155, 240, 0.2); color: var(--accent, #1d9bf0); }

            .alert-content {
                flex: 1;
                min-width: 0;
            }

            .alert-type {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
            }

            .alert-message {
                font-size: 0.8rem;
                margin-top: 0.125rem;
            }

            .alert-meta {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
                display: flex;
                gap: 0.75rem;
            }

            .alert-value {
                font-weight: 600;
                font-size: 0.85rem;
            }

            /* Trends Section */
            .trends-section {
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .trends-grid {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 0.75rem;
                padding: 0 1rem;
            }

            .trend-card {
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                text-align: center;
            }

            .trend-card.growing { border-top: 3px solid var(--error, #f4212e); }
            .trend-card.stable { border-top: 3px solid var(--success, #00ba7c); }
            .trend-card.declining { border-top: 3px solid var(--accent, #1d9bf0); }

            .trend-value {
                font-size: 1.25rem;
                font-weight: 600;
            }

            .trend-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .growing-list {
                padding: 0.75rem 1rem 1rem;
            }

            .growing-title {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .growing-metrics {
                display: flex;
                flex-wrap: wrap;
                gap: 0.5rem;
            }

            .growing-metric {
                font-size: 0.75rem;
                padding: 0.25rem 0.5rem;
                background: rgba(244, 33, 46, 0.1);
                color: var(--error, #f4212e);
                border-radius: 4px;
            }

            /* Badges */
            .badge {
                font-size: 0.65rem;
                padding: 0.125rem 0.375rem;
                border-radius: 4px;
                font-weight: 500;
            }

            .badge.quarantine {
                background: rgba(255, 212, 0, 0.1);
                color: var(--warning, #ffd400);
            }

            .badge.blocked {
                background: rgba(244, 33, 46, 0.1);
                color: var(--error, #f4212e);
            }

            .badge.warning {
                background: rgba(244, 33, 46, 0.1);
                color: var(--error, #f4212e);
            }

            /* Buttons */
            .btn {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                border: none;
                cursor: pointer;
                font-size: 0.8rem;
                font-weight: 500;
                transition: background 0.2s;
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-secondary {
                background: var(--bg-elevated, #1e2128);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .btn-warning {
                background: var(--warning, #ffd400);
                color: #000;
            }

            .btn-sm {
                padding: 0.25rem 0.5rem;
                font-size: 0.7rem;
            }

            .btn:hover {
                opacity: 0.9;
            }

            /* Empty State */
            .empty-state {
                padding: 1rem;
                text-align: center;
                color: var(--text-muted, #71767b);
                font-size: 0.8rem;
            }

            /* Footer */
            .footer-info {
                padding: 0.75rem 1rem;
                display: flex;
                justify-content: space-between;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                background: var(--bg-elevated, #1e2128);
            }

            @media (max-width: 800px) {
                .stats-row {
                    grid-template-columns: repeat(2, 1fr);
                }

                .cb-stats {
                    flex-wrap: wrap;
                }

                .trends-grid {
                    grid-template-columns: 1fr;
                }

                .footer-info {
                    flex-direction: column;
                    gap: 0.25rem;
                }
            }
        `;
    }
}

customElements.define('cardinality-controller', CardinalityController);
