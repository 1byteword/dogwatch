/**
 * Cost Dashboard Widget
 * "You'd pay $X on Datadog" visualization
 */
class CostDashboard extends HTMLElement {
    constructor() {
        super();
        this.comparison = null;
        this.loading = true;
        this.error = null;
        this._mounted = false;
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        this._mounted = false;
    }

    async loadData() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/cost/estimate');
            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            this.comparison = await resp.json();
        } catch (e) {
            console.error('Failed to load cost data:', e);
            this.error = e.message;
        } finally {
            this.loading = false;
            this.render();
        }
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-dashboard">
                    <div class="cost-header">
                        <div class="header-title">
                            <span class="title-icon">&#128176;</span>
                            <span>Cost Intelligence</span>
                        </div>
                    </div>
                    <div class="loading">Loading cost data...</div>
                </div>
            `;
            return;
        }

        if (this.error) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-dashboard">
                    <div class="cost-header">
                        <div class="header-title">
                            <span class="title-icon">&#128176;</span>
                            <span>Cost Intelligence</span>
                        </div>
                        <button class="btn-refresh" id="btn-refresh">Retry</button>
                    </div>
                    <div class="error">Failed to load cost data: ${this.escapeHtml(this.error)}</div>
                </div>
            `;
            this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());
            return;
        }

        const estimates = this.comparison?.estimates || {};
        const usage = this.comparison?.usage || {};
        const savings = this.comparison?.dogwatch_savings || {};

        const datadog = estimates.datadog || {};
        const newrelic = estimates.newrelic || {};
        const splunk = estimates.splunk || {};

        const totalSavings = (datadog.total_monthly || 0) + (newrelic.total_monthly || 0) + (splunk.total_monthly || 0);
        const avgSavings = totalSavings / 3;

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cost-dashboard">
                <div class="cost-header">
                    <div class="header-title">
                        <span class="title-icon">&#128176;</span>
                        <span>Cost Intelligence</span>
                    </div>
                    <button class="btn-refresh" id="btn-refresh">&#8635; Refresh</button>
                </div>

                <div class="savings-banner">
                    <div class="savings-text">
                        <span class="savings-label">Your estimated savings with dogwatch</span>
                        <span class="savings-value">${this.formatCurrency(avgSavings)}<span class="period">/month</span></span>
                    </div>
                    <div class="savings-subtext">
                        ${this.formatCurrency(avgSavings * 12)}/year vs. average commercial platform cost
                    </div>
                </div>

                <div class="comparison-grid">
                    <div class="comparison-card dogwatch">
                        <div class="card-header">
                            <span class="provider-icon">&#128021;</span>
                            <span class="provider-name">dogwatch</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">$0</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-subtitle">Self-hosted, unlimited data</div>
                    </div>

                    ${this.renderVendorCard('Datadog', datadog, '&#128054;')}
                    ${this.renderVendorCard('New Relic', newrelic, '&#128202;')}
                    ${this.renderVendorCard('Splunk', splunk, '&#128269;')}
                </div>

                <div class="usage-section">
                    <h3>Current Usage</h3>
                    <div class="usage-grid">
                        <div class="usage-item">
                            <span class="usage-value">${usage.host_count || 0}</span>
                            <span class="usage-label">Hosts</span>
                        </div>
                        <div class="usage-item">
                            <span class="usage-value">${usage.apm_host_count || 0}</span>
                            <span class="usage-label">APM Hosts</span>
                        </div>
                        <div class="usage-item">
                            <span class="usage-value">${this.formatNumber(usage.spans_per_month || 0)}</span>
                            <span class="usage-label">Spans/month</span>
                        </div>
                        <div class="usage-item">
                            <span class="usage-value">${(usage.logs_gb_per_month || 0).toFixed(1)} GB</span>
                            <span class="usage-label">Logs/month</span>
                        </div>
                        <div class="usage-item">
                            <span class="usage-value">${usage.custom_metrics_count || 0}</span>
                            <span class="usage-label">Custom Metrics</span>
                        </div>
                        <div class="usage-item">
                            <span class="usage-value">${usage.container_count || 0}</span>
                            <span class="usage-label">Containers</span>
                        </div>
                    </div>
                </div>

                <div class="annual-section">
                    <h3>Annual Comparison</h3>
                    <div class="annual-grid">
                        <div class="annual-item dogwatch-annual">
                            <div class="annual-vendor">dogwatch</div>
                            <div class="annual-value">$0</div>
                            <div class="annual-bar" style="width: 5%"></div>
                        </div>
                        ${this.renderAnnualBar('Datadog', datadog.total_annual || 0, Math.max(datadog.total_annual || 0, newrelic.total_annual || 0, splunk.total_annual || 0))}
                        ${this.renderAnnualBar('New Relic', newrelic.total_annual || 0, Math.max(datadog.total_annual || 0, newrelic.total_annual || 0, splunk.total_annual || 0))}
                        ${this.renderAnnualBar('Splunk', splunk.total_annual || 0, Math.max(datadog.total_annual || 0, newrelic.total_annual || 0, splunk.total_annual || 0))}
                    </div>
                </div>

                <div class="notes-section">
                    <h3>Pricing Notes</h3>
                    <ul class="notes-list">
                        <li>Estimates based on publicly available pricing (as of 2024)</li>
                        <li>Actual costs may vary based on contracts and volume discounts</li>
                        <li>dogwatch is self-hosted: you pay only for infrastructure</li>
                        ${(datadog.notes || []).map(n => `<li><strong>Datadog:</strong> ${this.escapeHtml(n)}</li>`).join('')}
                        ${(newrelic.notes || []).map(n => `<li><strong>New Relic:</strong> ${this.escapeHtml(n)}</li>`).join('')}
                        ${(splunk.notes || []).map(n => `<li><strong>Splunk:</strong> ${this.escapeHtml(n)}</li>`).join('')}
                    </ul>
                </div>
            </div>
        `;

        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());
    }

    renderVendorCard(name, estimate, icon) {
        const breakdown = estimate.breakdown || {};
        const breakdownItems = Object.entries(breakdown)
            .filter(([_, v]) => v > 0)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 4);

        return `
            <div class="comparison-card">
                <div class="card-header">
                    <span class="provider-icon">${icon}</span>
                    <span class="provider-name">${this.escapeHtml(name)}</span>
                </div>
                <div class="card-price">
                    <span class="price-value">${this.formatCurrency(estimate.total_monthly || 0)}</span>
                    <span class="price-period">/month</span>
                </div>
                <div class="card-breakdown">
                    ${breakdownItems.map(([k, v]) => `
                        <div class="breakdown-item">
                            <span class="breakdown-label">${this.formatBreakdownLabel(k)}</span>
                            <span class="breakdown-value">${this.formatCurrency(v)}</span>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderAnnualBar(name, amount, max) {
        const pct = max > 0 ? Math.max(5, (amount / max) * 100) : 5;
        return `
            <div class="annual-item">
                <div class="annual-vendor">${this.escapeHtml(name)}</div>
                <div class="annual-value">${this.formatCurrency(amount)}</div>
                <div class="annual-bar" style="width: ${pct}%"></div>
            </div>
        `;
    }

    formatBreakdownLabel(key) {
        return key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(1)}k`;
        return `$${Math.round(amount).toLocaleString()}`;
    }

    formatNumber(num) {
        if (!num || num === 0) return '0';
        if (num >= 1000000000) return `${(num / 1000000000).toFixed(1)}B`;
        if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
        if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
        return num.toLocaleString();
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .cost-dashboard {
                background: var(--bg-card, #16181c);
                min-height: 100%;
                display: flex;
                flex-direction: column;
            }

            .cost-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .title-icon { font-size: 1.25rem; }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.75rem;
                cursor: pointer;
                font-size: 0.85rem;
            }

            .btn-refresh:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .savings-banner {
                background: linear-gradient(135deg, #00ba7c 0%, #00875a 100%);
                padding: 1.5rem;
                text-align: center;
                color: white;
            }

            .savings-label {
                display: block;
                font-size: 0.9rem;
                opacity: 0.9;
                margin-bottom: 0.5rem;
            }

            .savings-value {
                font-size: 3rem;
                font-weight: 700;
                line-height: 1.1;
            }

            .savings-value .period {
                font-size: 1.25rem;
                font-weight: 400;
                opacity: 0.8;
            }

            .savings-subtext {
                font-size: 0.85rem;
                opacity: 0.85;
                margin-top: 0.5rem;
            }

            .comparison-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 1rem;
                padding: 1.5rem;
            }

            .comparison-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                border: 1px solid var(--border, #2f3336);
            }

            .comparison-card.dogwatch {
                background: linear-gradient(135deg, rgba(0, 186, 124, 0.15) 0%, rgba(0, 135, 90, 0.15) 100%);
                border-color: var(--success, #00ba7c);
            }

            .card-header {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.75rem;
            }

            .provider-icon { font-size: 1.25rem; }
            .provider-name { font-size: 0.9rem; font-weight: 600; }

            .card-price {
                margin-bottom: 0.75rem;
            }

            .price-value {
                font-size: 1.75rem;
                font-weight: 700;
            }

            .price-period {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .card-subtitle {
                font-size: 0.8rem;
                color: var(--success, #00ba7c);
            }

            .card-breakdown {
                border-top: 1px solid var(--border, #2f3336);
                padding-top: 0.75rem;
                margin-top: 0.75rem;
            }

            .breakdown-item {
                display: flex;
                justify-content: space-between;
                font-size: 0.8rem;
                padding: 0.25rem 0;
                color: var(--text-muted, #71767b);
            }

            .breakdown-value {
                color: var(--text, #e7e9ea);
            }

            .usage-section, .annual-section, .notes-section {
                padding: 1.5rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .usage-section h3, .annual-section h3, .notes-section h3 {
                font-size: 0.9rem;
                font-weight: 600;
                margin: 0 0 1rem 0;
                color: var(--text-muted, #71767b);
            }

            .usage-grid {
                display: grid;
                grid-template-columns: repeat(6, 1fr);
                gap: 1rem;
            }

            .usage-item {
                text-align: center;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
            }

            .usage-value {
                display: block;
                font-size: 1.5rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }

            .usage-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .annual-grid {
                display: flex;
                flex-direction: column;
                gap: 0.75rem;
            }

            .annual-item {
                display: grid;
                grid-template-columns: 100px 100px 1fr;
                align-items: center;
                gap: 1rem;
            }

            .annual-item.dogwatch-annual .annual-bar {
                background: var(--success, #00ba7c);
            }

            .annual-vendor {
                font-size: 0.9rem;
                font-weight: 500;
            }

            .annual-value {
                font-size: 0.9rem;
                color: var(--text-muted, #71767b);
            }

            .annual-bar {
                height: 24px;
                background: var(--accent, #1d9bf0);
                border-radius: 4px;
                min-width: 20px;
            }

            .notes-list {
                list-style: none;
                padding: 0;
                margin: 0;
            }

            .notes-list li {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                padding: 0.35rem 0;
                padding-left: 1rem;
                position: relative;
            }

            .notes-list li::before {
                content: "\\2022";
                position: absolute;
                left: 0;
                color: var(--accent, #1d9bf0);
            }

            @media (max-width: 1200px) {
                .comparison-grid {
                    grid-template-columns: repeat(2, 1fr);
                }
                .usage-grid {
                    grid-template-columns: repeat(3, 1fr);
                }
            }

            @media (max-width: 768px) {
                .comparison-grid {
                    grid-template-columns: 1fr;
                }
                .usage-grid {
                    grid-template-columns: repeat(2, 1fr);
                }
                .annual-item {
                    grid-template-columns: 80px 80px 1fr;
                }
            }
        `;
    }
}

customElements.define('cost-dashboard', CostDashboard);
