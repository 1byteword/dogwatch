/**
 * Cost Dashboard Widget
 * "You'd pay $X on Datadog" visualization
 */
class CostDashboard extends HTMLElement {
    constructor() {
        super();
        this.estimate = null;
        this.recommendations = [];
        this.loading = true;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.render();

        try {
            const [estimateResp, recsResp] = await Promise.all([
                fetch('/api/cost/estimate'),
                fetch('/api/cost/recommendations')
            ]);

            if (estimateResp.ok) this.estimate = await estimateResp.json();
            if (recsResp.ok) this.recommendations = await recsResp.json() || [];
        } catch (e) {
            console.error('Failed to load cost data:', e);
            this._showError('Failed to load cost data', e.message);
        } finally {
            this.loading = false;
            this.render();
            this._setupEventListeners();
        }
    }

    _showError(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'error', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.error(message, title);
        }
    }

    _setupEventListeners() {
        // Set up click handler for refresh button (safer than inline onclick with getRootNode)
        const refreshBtn = this.querySelector('#cost-refresh-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => this.loadData());
        }
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-dashboard">
                    <div class="cost-header">
                        <span class="title-icon">💰</span>
                        <span>Cost Intelligence</span>
                    </div>
                    <div class="loading">Loading cost data...</div>
                </div>
            `;
            return;
        }

        const estimate = this.estimate || {};
        const datadog = estimate.datadog || {};
        const newrelic = estimate.newrelic || {};
        const splunk = estimate.splunk || {};
        const savings = this.calculateSavings();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cost-dashboard">
                <div class="cost-header">
                    <div class="header-title">
                        <span class="title-icon">💰</span>
                        <span>Cost Intelligence</span>
                    </div>
                    <button class="btn-refresh" id="cost-refresh-btn">↻</button>
                </div>

                <div class="savings-banner">
                    <div class="savings-text">
                        <span class="savings-label">Your estimated savings</span>
                        <span class="savings-value">${this.formatCurrency(savings.total)}<span class="period">/month</span></span>
                    </div>
                    <div class="savings-subtext">
                        vs. commercial alternatives
                    </div>
                </div>

                <div class="comparison-grid">
                    <div class="comparison-card dogwatch">
                        <div class="card-header">
                            <span class="provider-icon">🐕</span>
                            <span class="provider-name">dogwatch</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">$0</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-subtitle">Self-hosted, unlimited</div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">🐕</span>
                            <span class="provider-name">Datadog</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(datadog.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${datadog.hosts ? `<div class="breakdown-item">Infrastructure: ${this.formatCurrency(datadog.hosts)}</div>` : ''}
                            ${datadog.apm ? `<div class="breakdown-item">APM: ${this.formatCurrency(datadog.apm)}</div>` : ''}
                            ${datadog.logs ? `<div class="breakdown-item">Logs: ${this.formatCurrency(datadog.logs)}</div>` : ''}
                        </div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">📊</span>
                            <span class="provider-name">New Relic</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(newrelic.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${newrelic.users ? `<div class="breakdown-item">Users: ${this.formatCurrency(newrelic.users)}</div>` : ''}
                            ${newrelic.data ? `<div class="breakdown-item">Data Ingest: ${this.formatCurrency(newrelic.data)}</div>` : ''}
                        </div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">🔍</span>
                            <span class="provider-name">Splunk</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(splunk.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${splunk.workload ? `<div class="breakdown-item">Workload: ${this.formatCurrency(splunk.workload)}</div>` : ''}
                            ${splunk.ingest ? `<div class="breakdown-item">Ingest: ${this.formatCurrency(splunk.ingest)}</div>` : ''}
                        </div>
                    </div>
                </div>

                <div class="usage-stats">
                    <h3>Current Usage</h3>
                    <div class="stats-grid">
                        <div class="stat-item">
                            <span class="stat-value">${estimate.metrics_count?.toLocaleString() || 0}</span>
                            <span class="stat-label">Custom Metrics</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${this.formatBytes(estimate.logs_gb || 0)}</span>
                            <span class="stat-label">Logs/day</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${(estimate.spans_million || 0).toFixed(1)}M</span>
                            <span class="stat-label">Spans/month</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${estimate.hosts || 0}</span>
                            <span class="stat-label">Hosts</span>
                        </div>
                    </div>
                </div>

                ${this.recommendations.length > 0 ? `
                    <div class="recommendations">
                        <h3>Optimization Recommendations</h3>
                        <div class="recs-list">
                            ${this.recommendations.slice(0, 3).map(r => `
                                <div class="rec-item ${r.priority || 'medium'}">
                                    <div class="rec-icon">${this.getRecIcon(r.type)}</div>
                                    <div class="rec-content">
                                        <div class="rec-title">${this.escapeHtml(r.title)}</div>
                                        <div class="rec-desc">${this.escapeHtml(r.description)}</div>
                                        ${r.estimated_savings ? `<div class="rec-savings">Save ~${this.formatCurrency(r.estimated_savings)}/mo</div>` : ''}
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    calculateSavings() {
        if (!this.estimate) return { total: 0 };

        const datadog = this.estimate.datadog?.total || 0;
        const newrelic = this.estimate.newrelic?.total || 0;
        const splunk = this.estimate.splunk?.total || 0;

        // Average of the three
        const avg = (datadog + newrelic + splunk) / 3;

        return {
            total: avg,
            datadog,
            newrelic,
            splunk
        };
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(0)}k`;
        return `$${amount.toFixed(0)}`;
    }

    formatBytes(gb) {
        if (!gb || gb === 0) return '0 GB';
        if (gb >= 1000) return `${(gb / 1000).toFixed(1)} TB`;
        return `${gb.toFixed(1)} GB`;
    }

    getRecIcon(type) {
        switch (type) {
            case 'cardinality': return '📊';
            case 'retention': return '🗄️';
            case 'sampling': return '🎯';
            case 'aggregation': return '📈';
            default: return '💡';
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .cost-dashboard {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .cost-header {
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

            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .savings-banner {
                background: linear-gradient(135deg, #00ba7c 0%, #00a36c 100%);
                padding: 1.25rem;
                text-align: center;
                color: white;
            }

            .savings-label {
                display: block;
                font-size: 0.85rem;
                opacity: 0.9;
                margin-bottom: 0.25rem;
            }

            .savings-value {
                font-size: 2.5rem;
                font-weight: 700;
            }

            .savings-value .period {
                font-size: 1rem;
                font-weight: 400;
                opacity: 0.8;
            }

            .savings-subtext {
                font-size: 0.8rem;
                opacity: 0.8;
                margin-top: 0.25rem;
            }

            .comparison-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.5rem;
                padding: 1rem;
            }

            .comparison-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 0.75rem;
                border: 1px solid var(--border, #2f3336);
            }

            .comparison-card.dogwatch {
                background: linear-gradient(135deg, rgba(0, 186, 124, 0.1) 0%, rgba(0, 163, 108, 0.1) 100%);
                border-color: var(--success, #00ba7c);
            }

            .card-header {
                display: flex;
                align-items: center;
                gap: 0.4rem;
                margin-bottom: 0.5rem;
            }

            .provider-icon { font-size: 1rem; }
            .provider-name { font-size: 0.8rem; font-weight: 500; }

            .card-price {
                margin-bottom: 0.25rem;
            }

            .price-value {
                font-size: 1.25rem;
                font-weight: 600;
            }

            .price-period {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .card-subtitle {
                font-size: 0.7rem;
                color: var(--success, #00ba7c);
            }

            .card-breakdown {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .breakdown-item {
                padding: 0.1rem 0;
            }

            .usage-stats {
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .usage-stats h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .stats-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.75rem;
            }

            .stat-item {
                text-align: center;
            }

            .stat-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .recommendations {
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
                flex: 1;
                overflow-y: auto;
            }

            .recommendations h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .recs-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .rec-item {
                display: flex;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .rec-item.high { border-left-color: var(--error, #f4212e); }
            .rec-item.medium { border-left-color: var(--warning, #ffd400); }
            .rec-item.low { border-left-color: var(--success, #00ba7c); }

            .rec-icon { font-size: 1.25rem; }

            .rec-content { flex: 1; }

            .rec-title {
                font-weight: 500;
                font-size: 0.85rem;
                margin-bottom: 0.25rem;
            }

            .rec-desc {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .rec-savings {
                font-size: 0.75rem;
                color: var(--success, #00ba7c);
                margin-top: 0.25rem;
            }

            @media (max-width: 800px) {
                .comparison-grid, .stats-grid {
                    grid-template-columns: repeat(2, 1fr);
                }
            }
        `;
    }
}

customElements.define('cost-dashboard', CostDashboard);
