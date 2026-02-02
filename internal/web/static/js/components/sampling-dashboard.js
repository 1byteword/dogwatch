/**
 * Sampling Dashboard Widget
 * Real-time visualization of sampling configuration, stats, and cost tracking
 */
class SamplingDashboard extends HTMLElement {
    constructor() {
        super();
        this.config = null;
        this.stats = null;
        this.rules = [];
        this.adaptiveRate = null;
        this.costData = null;
        this.patterns = null;
        this.bufferedTraces = 0;
        this.loading = true;
        this.error = null;
        this.activeTab = 'overview';
        this.refreshInterval = null;
        this.statsHistory = [];
        this._mounted = false;
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
        // Auto-refresh every 5 seconds
        this.refreshInterval = setInterval(() => {
            if (this._mounted) this.loadData();
        }, 5000);
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
            const [configResp, statsResp, rulesResp, adaptiveResp, costResp, patternsResp, bufferedResp] = await Promise.all([
                fetch('/api/sampling/config'),
                fetch('/api/sampling/stats'),
                fetch('/api/sampling/rules'),
                fetch('/api/sampling/adaptive/rate'),
                fetch('/api/sampling/intelligent/cost'),
                fetch('/api/sampling/intelligent/patterns'),
                fetch('/api/sampling/tail/buffered')
            ]);

            if (configResp.ok) this.config = await configResp.json();
            if (statsResp.ok) this.stats = await statsResp.json();
            if (rulesResp.ok) {
                const rulesData = await rulesResp.json();
                this.rules = rulesData.rules || [];
            }
            if (adaptiveResp.ok) this.adaptiveRate = await adaptiveResp.json();
            if (costResp.ok) this.costData = await costResp.json();
            if (patternsResp.ok) this.patterns = await patternsResp.json();
            if (bufferedResp.ok) {
                const bufferedData = await bufferedResp.json();
                this.bufferedTraces = bufferedData.buffered_traces || 0;
            }

            // Track history for chart
            if (this.stats) {
                this.statsHistory.push({
                    time: Date.now(),
                    kept: this.stats.stats?.TotalKept || 0,
                    dropped: this.stats.stats?.TotalDropped || 0
                });
                // Keep last 60 data points (5 minutes at 5s intervals)
                if (this.statsHistory.length > 60) {
                    this.statsHistory.shift();
                }
            }

            this.error = null;
        } catch (e) {
            console.error('Failed to load sampling data:', e);
            this.error = e.message;
        } finally {
            this.loading = false;
            this.render();
        }
    }

    setActiveTab(tab) {
        this.activeTab = tab;
        this.render();
    }

    async updateConfig(updates) {
        try {
            const newConfig = { ...this.config, ...updates };
            const resp = await fetch('/api/sampling/config', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(newConfig)
            });
            if (resp.ok) {
                this.config = newConfig;
                this.render();
            }
        } catch (e) {
            console.error('Failed to update config:', e);
        }
    }

    async addRule(rule) {
        try {
            const resp = await fetch('/api/sampling/rules', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(rule)
            });
            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to add rule:', e);
        }
    }

    async deleteRule(ruleId) {
        try {
            const resp = await fetch(`/api/sampling/rules/${ruleId}`, {
                method: 'DELETE'
            });
            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to delete rule:', e);
        }
    }

    async flushTailBuffer() {
        try {
            const resp = await fetch('/api/sampling/tail/flush', {
                method: 'POST'
            });
            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to flush buffer:', e);
        }
    }

    async setAdaptiveTarget(targetTps) {
        try {
            const resp = await fetch('/api/sampling/adaptive/rate', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ target_tps: targetTps })
            });
            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to set adaptive target:', e);
        }
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="sampling-dashboard">
                    <div class="header">
                        <span class="title">Sampling Dashboard</span>
                    </div>
                    <div class="loading">Loading sampling data...</div>
                </div>
            `;
            return;
        }

        if (this.error) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="sampling-dashboard">
                    <div class="header">
                        <span class="title">Sampling Dashboard</span>
                    </div>
                    <div class="error">Error: ${this.escapeHtml(this.error)}</div>
                </div>
            `;
            return;
        }

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="sampling-dashboard">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Sampling Dashboard</span>
                        ${this.config?.enabled ? '<span class="status-badge active">Active</span>' : '<span class="status-badge inactive">Inactive</span>'}
                    </div>
                    <button class="btn-refresh" onclick="this.getRootNode().host.loadData()">Refresh</button>
                </div>

                <div class="tabs">
                    <button class="tab ${this.activeTab === 'overview' ? 'active' : ''}" onclick="this.getRootNode().host.setActiveTab('overview')">Overview</button>
                    <button class="tab ${this.activeTab === 'rules' ? 'active' : ''}" onclick="this.getRootNode().host.setActiveTab('rules')">Rules</button>
                    <button class="tab ${this.activeTab === 'stats' ? 'active' : ''}" onclick="this.getRootNode().host.setActiveTab('stats')">Stats</button>
                    <button class="tab ${this.activeTab === 'cost' ? 'active' : ''}" onclick="this.getRootNode().host.setActiveTab('cost')">Cost</button>
                    <button class="tab ${this.activeTab === 'learning' ? 'active' : ''}" onclick="this.getRootNode().host.setActiveTab('learning')">Learning</button>
                </div>

                <div class="tab-content">
                    ${this.renderTabContent()}
                </div>
            </div>
        `;
    }

    renderTabContent() {
        switch (this.activeTab) {
            case 'overview':
                return this.renderOverview();
            case 'rules':
                return this.renderRules();
            case 'stats':
                return this.renderStats();
            case 'cost':
                return this.renderCost();
            case 'learning':
                return this.renderLearning();
            default:
                return '';
        }
    }

    renderOverview() {
        const stats = this.stats?.stats || {};
        const keepRate = this.stats?.keep_rate_pct?.toFixed(1) || '0';
        const dropRate = this.stats?.drop_rate_pct?.toFixed(1) || '0';
        const currentRate = (this.adaptiveRate?.current_rate * 100)?.toFixed(1) || '0';

        return `
            <div class="overview-grid">
                <div class="rate-cards">
                    <div class="rate-card head">
                        <div class="rate-label">Head Sampling</div>
                        <div class="rate-value">${this.config?.head_sampler?.enabled ? 'Enabled' : 'Disabled'}</div>
                        <div class="rate-detail">${this.config?.default_sample_rate ? (this.config.default_sample_rate * 100).toFixed(0) + '% default rate' : ''}</div>
                    </div>
                    <div class="rate-card tail">
                        <div class="rate-label">Tail Sampling</div>
                        <div class="rate-value">${this.config?.tail_sampler?.enabled ? 'Enabled' : 'Disabled'}</div>
                        <div class="rate-detail">${this.bufferedTraces} buffered traces</div>
                        ${this.config?.tail_sampler?.enabled ? `<button class="btn-small" onclick="this.getRootNode().host.flushTailBuffer()">Flush Buffer</button>` : ''}
                    </div>
                    <div class="rate-card adaptive">
                        <div class="rate-label">Adaptive Sampling</div>
                        <div class="rate-value">${this.config?.adaptive_sampler?.enabled ? currentRate + '%' : 'Disabled'}</div>
                        <div class="rate-detail">Target: ${this.config?.adaptive_sampler?.target_traces_per_second || 0} TPS</div>
                    </div>
                </div>

                <div class="stats-summary">
                    <div class="stat-box">
                        <span class="stat-number">${this.formatNumber(stats.TotalProcessed || 0)}</span>
                        <span class="stat-label">Total Processed</span>
                    </div>
                    <div class="stat-box kept">
                        <span class="stat-number">${keepRate}%</span>
                        <span class="stat-label">Keep Rate</span>
                    </div>
                    <div class="stat-box dropped">
                        <span class="stat-number">${dropRate}%</span>
                        <span class="stat-label">Drop Rate</span>
                    </div>
                    <div class="stat-box">
                        <span class="stat-number">${this.rules.length}</span>
                        <span class="stat-label">Active Rules</span>
                    </div>
                </div>

                <div class="service-rates">
                    <h3>Per-Service Rates</h3>
                    ${this.renderServiceRates()}
                </div>
            </div>
        `;
    }

    renderServiceRates() {
        const serviceRates = this.adaptiveRate?.service_rates || {};
        const services = Object.entries(serviceRates);

        if (services.length === 0) {
            return '<div class="no-data">No service-specific rates configured</div>';
        }

        return `
            <div class="service-rates-list">
                ${services.map(([service, rate]) => `
                    <div class="service-rate-item">
                        <span class="service-name">${this.escapeHtml(service)}</span>
                        <div class="rate-bar">
                            <div class="rate-fill" style="width: ${rate * 100}%"></div>
                        </div>
                        <span class="rate-value">${(rate * 100).toFixed(1)}%</span>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderRules() {
        return `
            <div class="rules-section">
                <div class="rules-header">
                    <h3>Sampling Rules</h3>
                    <button class="btn-add" onclick="this.getRootNode().host.showAddRuleDialog()">+ Add Rule</button>
                </div>

                <div class="rules-list">
                    ${this.rules.length === 0 ? '<div class="no-data">No sampling rules configured</div>' : ''}
                    ${this.rules.sort((a, b) => b.priority - a.priority).map(rule => `
                        <div class="rule-card ${rule.enabled ? '' : 'disabled'}">
                            <div class="rule-header">
                                <div class="rule-info">
                                    <span class="rule-name">${this.escapeHtml(rule.name)}</span>
                                    <span class="rule-priority">Priority: ${rule.priority}</span>
                                </div>
                                <div class="rule-actions">
                                    <span class="rule-action ${rule.action === 0 ? 'keep' : 'drop'}">${rule.action === 0 ? 'KEEP' : 'DROP'}</span>
                                    <button class="btn-delete" onclick="this.getRootNode().host.deleteRule('${rule.id}')">Delete</button>
                                </div>
                            </div>
                            <div class="rule-conditions">
                                ${rule.condition.service ? `<span class="condition">Service: ${this.escapeHtml(rule.condition.service)}</span>` : ''}
                                ${rule.condition.operation ? `<span class="condition">Operation: ${this.escapeHtml(rule.condition.operation)}</span>` : ''}
                                ${rule.condition.min_latency_ms ? `<span class="condition">Min Latency: ${rule.condition.min_latency_ms}ms</span>` : ''}
                                ${rule.condition.has_error !== undefined ? `<span class="condition">Has Error: ${rule.condition.has_error}</span>` : ''}
                            </div>
                            ${rule.sample_rate < 1 ? `<div class="rule-rate">Sample Rate: ${(rule.sample_rate * 100).toFixed(0)}%</div>` : ''}
                        </div>
                    `).join('')}
                </div>

                <div id="add-rule-dialog" class="dialog hidden">
                    <div class="dialog-content">
                        <h3>Add Sampling Rule</h3>
                        <form onsubmit="event.preventDefault(); this.getRootNode().host.submitAddRule(this);">
                            <label>Name: <input type="text" name="name" required></label>
                            <label>Priority: <input type="number" name="priority" value="50" min="0" max="100"></label>
                            <label>Service Pattern: <input type="text" name="service" placeholder="*"></label>
                            <label>Operation Pattern: <input type="text" name="operation" placeholder="*"></label>
                            <label>Min Latency (ms): <input type="number" name="min_latency" min="0"></label>
                            <label>Has Error:
                                <select name="has_error">
                                    <option value="">Any</option>
                                    <option value="true">Yes</option>
                                    <option value="false">No</option>
                                </select>
                            </label>
                            <label>Action:
                                <select name="action">
                                    <option value="0">Keep</option>
                                    <option value="1">Drop</option>
                                </select>
                            </label>
                            <label>Sample Rate: <input type="number" name="sample_rate" value="1.0" min="0" max="1" step="0.01"></label>
                            <div class="dialog-actions">
                                <button type="button" onclick="this.getRootNode().host.hideAddRuleDialog()">Cancel</button>
                                <button type="submit">Add Rule</button>
                            </div>
                        </form>
                    </div>
                </div>
            </div>
        `;
    }

    showAddRuleDialog() {
        const dialog = this.querySelector('#add-rule-dialog');
        if (dialog) dialog.classList.remove('hidden');
    }

    hideAddRuleDialog() {
        const dialog = this.querySelector('#add-rule-dialog');
        if (dialog) dialog.classList.add('hidden');
    }

    submitAddRule(form) {
        const formData = new FormData(form);
        const rule = {
            name: formData.get('name'),
            enabled: true,
            priority: parseInt(formData.get('priority')) || 50,
            condition: {},
            action: parseInt(formData.get('action')),
            sample_rate: parseFloat(formData.get('sample_rate')) || 1.0
        };

        if (formData.get('service')) rule.condition.service = formData.get('service');
        if (formData.get('operation')) rule.condition.operation = formData.get('operation');
        if (formData.get('min_latency')) rule.condition.min_latency_ms = parseFloat(formData.get('min_latency'));
        if (formData.get('has_error') !== '') rule.condition.has_error = formData.get('has_error') === 'true';

        this.addRule(rule);
        this.hideAddRuleDialog();
    }

    renderStats() {
        const stats = this.stats?.stats || {};
        const adaptiveStats = this.stats?.adaptive_stats || {};

        return `
            <div class="stats-section">
                <div class="stats-chart">
                    <h3>Sampled vs Dropped (Last 5 min)</h3>
                    <div class="chart-container">
                        ${this.renderStatsChart()}
                    </div>
                </div>

                <div class="stats-details">
                    <div class="stats-group">
                        <h4>Totals</h4>
                        <div class="stat-row"><span>Total Processed:</span><span>${this.formatNumber(stats.TotalProcessed || 0)}</span></div>
                        <div class="stat-row"><span>Total Kept:</span><span>${this.formatNumber(stats.TotalKept || 0)}</span></div>
                        <div class="stat-row"><span>Total Dropped:</span><span>${this.formatNumber(stats.TotalDropped || 0)}</span></div>
                        <div class="stat-row"><span>Buffered Traces:</span><span>${this.formatNumber(stats.BufferedTraces || 0)}</span></div>
                    </div>

                    <div class="stats-group">
                        <h4>Head Sampler</h4>
                        <div class="stat-row"><span>Cached Decisions:</span><span>${this.formatNumber(stats.HeadStats?.CachedDecisions || 0)}</span></div>
                        <div class="stat-row"><span>Rules Matched:</span><span>${this.formatNumber(stats.HeadStats?.RulesMatched || 0)}</span></div>
                    </div>

                    <div class="stats-group">
                        <h4>Tail Sampler</h4>
                        <div class="stat-row"><span>Deferred Spans:</span><span>${this.formatNumber(stats.TailStats?.DeferredSpans || 0)}</span></div>
                        <div class="stat-row"><span>Errors Kept:</span><span>${this.formatNumber(stats.TailStats?.ErrorsKept || 0)}</span></div>
                        <div class="stat-row"><span>High Latency Kept:</span><span>${this.formatNumber(stats.TailStats?.HighLatencyKept || 0)}</span></div>
                    </div>

                    <div class="stats-group">
                        <h4>Adaptive Sampler</h4>
                        <div class="stat-row"><span>Current Rate:</span><span>${((stats.CurrentRate || 0) * 100).toFixed(2)}%</span></div>
                        <div class="stat-row"><span>Adjustments:</span><span>${this.formatNumber(adaptiveStats?.AdjustmentCount || 0)}</span></div>
                    </div>
                </div>
            </div>
        `;
    }

    renderStatsChart() {
        if (this.statsHistory.length < 2) {
            return '<div class="no-data">Collecting data...</div>';
        }

        const width = 400;
        const height = 150;
        const padding = 30;

        // Calculate deltas
        const deltas = [];
        for (let i = 1; i < this.statsHistory.length; i++) {
            deltas.push({
                time: this.statsHistory[i].time,
                kept: this.statsHistory[i].kept - this.statsHistory[i-1].kept,
                dropped: this.statsHistory[i].dropped - this.statsHistory[i-1].dropped
            });
        }

        const maxVal = Math.max(...deltas.map(d => Math.max(d.kept, d.dropped)), 1);
        const xStep = (width - 2 * padding) / (deltas.length - 1 || 1);

        const keptPath = deltas.map((d, i) => {
            const x = padding + i * xStep;
            const y = height - padding - (d.kept / maxVal) * (height - 2 * padding);
            return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
        }).join(' ');

        const droppedPath = deltas.map((d, i) => {
            const x = padding + i * xStep;
            const y = height - padding - (d.dropped / maxVal) * (height - 2 * padding);
            return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
        }).join(' ');

        return `
            <svg width="${width}" height="${height}" class="stats-svg">
                <line x1="${padding}" y1="${height - padding}" x2="${width - padding}" y2="${height - padding}" stroke="#2f3336" />
                <line x1="${padding}" y1="${padding}" x2="${padding}" y2="${height - padding}" stroke="#2f3336" />
                <path d="${keptPath}" fill="none" stroke="#00ba7c" stroke-width="2" />
                <path d="${droppedPath}" fill="none" stroke="#f4212e" stroke-width="2" />
            </svg>
            <div class="chart-legend">
                <span class="legend-item kept"><span class="dot"></span> Kept</span>
                <span class="legend-item dropped"><span class="dot"></span> Dropped</span>
            </div>
        `;
    }

    renderCost() {
        const cost = this.costData || {};
        const budgetPct = cost.budget_used_pct || 0;

        return `
            <div class="cost-section">
                <div class="cost-summary">
                    <div class="cost-card">
                        <div class="cost-label">Daily Cost</div>
                        <div class="cost-value">${this.formatCurrency(cost.daily_cost || 0)}</div>
                    </div>
                    <div class="cost-card">
                        <div class="cost-label">Daily Budget</div>
                        <div class="cost-value">${this.formatCurrency(cost.daily_budget || 0)}</div>
                    </div>
                    <div class="cost-card">
                        <div class="cost-label">Daily Spans</div>
                        <div class="cost-value">${this.formatNumber(cost.daily_spans || 0)}</div>
                    </div>
                    <div class="cost-card">
                        <div class="cost-label">Cost Per Span</div>
                        <div class="cost-value">${cost.cost_per_span ? '$' + cost.cost_per_span.toFixed(6) : '$0'}</div>
                    </div>
                </div>

                <div class="budget-progress">
                    <h3>Budget Usage</h3>
                    <div class="progress-bar">
                        <div class="progress-fill ${budgetPct > 90 ? 'danger' : budgetPct > 70 ? 'warning' : ''}" style="width: ${Math.min(budgetPct, 100)}%"></div>
                    </div>
                    <div class="progress-label">${budgetPct.toFixed(1)}% used (${this.formatCurrency(cost.budget_remaining || 0)} remaining)</div>
                </div>

                <div class="hourly-breakdown">
                    <h3>Hourly Cost Breakdown</h3>
                    <div class="hourly-chart">
                        ${this.renderHourlyCost(cost.hourly_costs || [])}
                    </div>
                </div>
            </div>
        `;
    }

    renderHourlyCost(hourlyCosts) {
        const maxCost = Math.max(...hourlyCosts, 0.01);
        return `
            <div class="hourly-bars">
                ${hourlyCosts.map((cost, hour) => `
                    <div class="hour-bar" title="${hour}:00 - ${this.formatCurrency(cost)}">
                        <div class="bar-fill" style="height: ${(cost / maxCost) * 100}%"></div>
                        <span class="hour-label">${hour}</span>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderLearning() {
        const patterns = this.patterns?.patterns || {};
        const serviceLatency = patterns.service_latency || {};
        const learnedRates = patterns.learned_rates || {};
        const samplesCollected = patterns.samples_collected || 0;

        return `
            <div class="learning-section">
                <div class="learning-status">
                    <h3>Learning Mode Status</h3>
                    <div class="status-info">
                        <span class="status-label">Samples Collected:</span>
                        <span class="status-value">${this.formatNumber(samplesCollected)}</span>
                    </div>
                    <div class="status-info">
                        <span class="status-label">Services Learned:</span>
                        <span class="status-value">${Object.keys(serviceLatency).length}</span>
                    </div>
                </div>

                <div class="learned-patterns">
                    <h3>Service Latency Patterns</h3>
                    <div class="patterns-list">
                        ${Object.keys(serviceLatency).length === 0 ? '<div class="no-data">No patterns learned yet</div>' : ''}
                        ${Object.entries(serviceLatency).map(([service, stats]) => `
                            <div class="pattern-card">
                                <div class="pattern-service">${this.escapeHtml(service)}</div>
                                <div class="pattern-stats">
                                    <span>Mean: ${stats.mean?.toFixed(2) || 0}ms</span>
                                    <span>StdDev: ${stats.std_dev?.toFixed(2) || 0}ms</span>
                                    <span>Count: ${this.formatNumber(stats.count || 0)}</span>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>

                <div class="suggested-rates">
                    <h3>Suggested Sampling Rates</h3>
                    <div class="rates-list">
                        ${Object.keys(learnedRates).length === 0 ? '<div class="no-data">No rate suggestions yet</div>' : ''}
                        ${Object.entries(learnedRates).map(([service, rate]) => `
                            <div class="rate-suggestion">
                                <span class="service">${this.escapeHtml(service)}</span>
                                <span class="suggested-rate">${(rate * 100).toFixed(1)}%</span>
                            </div>
                        `).join('')}
                    </div>
                </div>
            </div>
        `;
    }

    formatNumber(num) {
        if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
        if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
        return num.toString();
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0.00';
        if (amount >= 1000) return '$' + (amount / 1000).toFixed(1) + 'k';
        return '$' + amount.toFixed(2);
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
            .sampling-dashboard {
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

            .header-left {
                display: flex;
                align-items: center;
                gap: 0.75rem;
            }

            .title { font-weight: 600; font-size: 1rem; }

            .status-badge {
                font-size: 0.7rem;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
            }

            .status-badge.active { background: rgba(0, 186, 124, 0.2); color: #00ba7c; }
            .status-badge.inactive { background: rgba(113, 118, 123, 0.2); color: #71767b; }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.25rem 0.75rem;
                cursor: pointer;
            }

            .loading, .error, .no-data {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .tabs {
                display: flex;
                gap: 0.25rem;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tab {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.5rem 1rem;
                cursor: pointer;
                border-radius: 4px;
                font-size: 0.85rem;
            }

            .tab:hover { background: var(--bg-card, #16181c); }
            .tab.active { background: var(--accent, #1d9bf0); color: white; }

            .tab-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            /* Overview styles */
            .overview-grid { display: flex; flex-direction: column; gap: 1rem; }

            .rate-cards {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 0.75rem;
            }

            .rate-card {
                background: var(--bg-elevated, #1e2128);
                padding: 1rem;
                border-radius: 8px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .rate-card.head { border-left-color: var(--accent, #1d9bf0); }
            .rate-card.tail { border-left-color: var(--warning, #ffd400); }
            .rate-card.adaptive { border-left-color: var(--success, #00ba7c); }

            .rate-label { font-size: 0.75rem; color: var(--text-muted, #71767b); }
            .rate-value { font-size: 1.25rem; font-weight: 600; margin: 0.25rem 0; }
            .rate-detail { font-size: 0.75rem; color: var(--text-muted, #71767b); }

            .btn-small {
                font-size: 0.7rem;
                padding: 0.25rem 0.5rem;
                margin-top: 0.5rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                color: var(--text, #e7e9ea);
                border-radius: 4px;
                cursor: pointer;
            }

            .stats-summary {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.75rem;
            }

            .stat-box {
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 8px;
                text-align: center;
            }

            .stat-box.kept { color: var(--success, #00ba7c); }
            .stat-box.dropped { color: var(--error, #f4212e); }

            .stat-number { display: block; font-size: 1.5rem; font-weight: 600; }
            .stat-label { font-size: 0.75rem; color: var(--text-muted, #71767b); }

            .service-rates h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.5rem 0;
            }

            .service-rates-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .service-rate-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
            }

            .service-name { flex: 0 0 120px; font-size: 0.85rem; }

            .rate-bar {
                flex: 1;
                height: 8px;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                overflow: hidden;
            }

            .rate-fill { height: 100%; background: var(--accent, #1d9bf0); }

            /* Rules styles */
            .rules-section { display: flex; flex-direction: column; gap: 1rem; }

            .rules-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
            }

            .rules-header h3 { margin: 0; font-size: 1rem; }

            .btn-add {
                background: var(--accent, #1d9bf0);
                color: white;
                border: none;
                padding: 0.5rem 1rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.85rem;
            }

            .rules-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .rule-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 0.75rem;
                border: 1px solid var(--border, #2f3336);
            }

            .rule-card.disabled { opacity: 0.5; }

            .rule-header {
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
            }

            .rule-name { font-weight: 500; }
            .rule-priority { font-size: 0.75rem; color: var(--text-muted, #71767b); margin-left: 0.5rem; }

            .rule-actions { display: flex; align-items: center; gap: 0.5rem; }

            .rule-action {
                font-size: 0.7rem;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
            }

            .rule-action.keep { background: rgba(0, 186, 124, 0.2); color: #00ba7c; }
            .rule-action.drop { background: rgba(244, 33, 46, 0.2); color: #f4212e; }

            .btn-delete {
                background: transparent;
                border: 1px solid var(--error, #f4212e);
                color: var(--error, #f4212e);
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.7rem;
            }

            .rule-conditions {
                display: flex;
                flex-wrap: wrap;
                gap: 0.5rem;
                margin-top: 0.5rem;
            }

            .condition {
                font-size: 0.75rem;
                background: var(--bg-card, #16181c);
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
            }

            .rule-rate {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.5rem;
            }

            /* Dialog styles */
            .dialog {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 1000;
            }

            .dialog.hidden { display: none; }

            .dialog-content {
                background: var(--bg-card, #16181c);
                padding: 1.5rem;
                border-radius: 8px;
                min-width: 400px;
            }

            .dialog-content h3 { margin: 0 0 1rem 0; }

            .dialog-content form {
                display: flex;
                flex-direction: column;
                gap: 0.75rem;
            }

            .dialog-content label {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
                font-size: 0.85rem;
            }

            .dialog-content input,
            .dialog-content select {
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
            }

            .dialog-actions {
                display: flex;
                justify-content: flex-end;
                gap: 0.5rem;
                margin-top: 1rem;
            }

            .dialog-actions button {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                cursor: pointer;
            }

            .dialog-actions button[type="button"] {
                background: transparent;
                border: 1px solid var(--border, #2f3336);
                color: var(--text, #e7e9ea);
            }

            .dialog-actions button[type="submit"] {
                background: var(--accent, #1d9bf0);
                border: none;
                color: white;
            }

            /* Stats styles */
            .stats-section { display: flex; flex-direction: column; gap: 1rem; }

            .stats-chart h3 { font-size: 0.85rem; margin: 0 0 0.5rem 0; }

            .chart-container {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .stats-svg { display: block; margin: 0 auto; }

            .chart-legend {
                display: flex;
                justify-content: center;
                gap: 1rem;
                margin-top: 0.5rem;
            }

            .legend-item { display: flex; align-items: center; gap: 0.25rem; font-size: 0.75rem; }
            .legend-item .dot { width: 8px; height: 8px; border-radius: 50%; }
            .legend-item.kept .dot { background: #00ba7c; }
            .legend-item.dropped .dot { background: #f4212e; }

            .stats-details {
                display: grid;
                grid-template-columns: repeat(2, 1fr);
                gap: 1rem;
            }

            .stats-group {
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 8px;
            }

            .stats-group h4 {
                margin: 0 0 0.5rem 0;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .stat-row {
                display: flex;
                justify-content: space-between;
                font-size: 0.8rem;
                padding: 0.25rem 0;
            }

            /* Cost styles */
            .cost-section { display: flex; flex-direction: column; gap: 1rem; }

            .cost-summary {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.75rem;
            }

            .cost-card {
                background: var(--bg-elevated, #1e2128);
                padding: 1rem;
                border-radius: 8px;
                text-align: center;
            }

            .cost-label { font-size: 0.75rem; color: var(--text-muted, #71767b); }
            .cost-value { font-size: 1.25rem; font-weight: 600; margin-top: 0.25rem; }

            .budget-progress h3 {
                font-size: 0.85rem;
                margin: 0 0 0.5rem 0;
            }

            .progress-bar {
                height: 12px;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                overflow: hidden;
            }

            .progress-fill {
                height: 100%;
                background: var(--success, #00ba7c);
                transition: width 0.3s;
            }

            .progress-fill.warning { background: var(--warning, #ffd400); }
            .progress-fill.danger { background: var(--error, #f4212e); }

            .progress-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            .hourly-breakdown h3 { font-size: 0.85rem; margin: 0 0 0.5rem 0; }

            .hourly-chart {
                background: var(--bg-elevated, #1e2128);
                padding: 1rem;
                border-radius: 8px;
            }

            .hourly-bars {
                display: flex;
                align-items: flex-end;
                height: 100px;
                gap: 2px;
            }

            .hour-bar {
                flex: 1;
                display: flex;
                flex-direction: column;
                align-items: center;
            }

            .bar-fill {
                width: 100%;
                background: var(--accent, #1d9bf0);
                border-radius: 2px 2px 0 0;
            }

            .hour-label {
                font-size: 0.6rem;
                color: var(--text-muted, #71767b);
                margin-top: 4px;
            }

            /* Learning styles */
            .learning-section { display: flex; flex-direction: column; gap: 1rem; }

            .learning-status {
                background: var(--bg-elevated, #1e2128);
                padding: 1rem;
                border-radius: 8px;
            }

            .learning-status h3 { margin: 0 0 0.5rem 0; font-size: 0.85rem; }

            .status-info {
                display: flex;
                justify-content: space-between;
                font-size: 0.85rem;
                padding: 0.25rem 0;
            }

            .learned-patterns h3, .suggested-rates h3 {
                font-size: 0.85rem;
                margin: 0 0 0.5rem 0;
            }

            .patterns-list, .rates-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .pattern-card {
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 8px;
            }

            .pattern-service { font-weight: 500; margin-bottom: 0.25rem; }

            .pattern-stats {
                display: flex;
                gap: 1rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .rate-suggestion {
                display: flex;
                justify-content: space-between;
                padding: 0.5rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
            }

            .suggested-rate { color: var(--accent, #1d9bf0); font-weight: 500; }

            @media (max-width: 800px) {
                .rate-cards, .stats-summary, .cost-summary, .stats-details {
                    grid-template-columns: repeat(2, 1fr);
                }
            }
        `;
    }
}

customElements.define('sampling-dashboard', SamplingDashboard);
