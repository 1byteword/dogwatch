/**
 * Cost Calculator Widget
 * What-if scenarios for cost estimation
 */
class CostCalculator extends HTMLElement {
    constructor() {
        super();
        this.result = null;
        this.loading = false;
        this.error = null;
        this.params = {
            hosts: 10,
            containers: 50,
            custom_metrics: 500,
            logs_gb_per_day: 10,
            spans_per_second: 100
        };
    }

    connectedCallback() {
        this.render();
        this.calculate();
    }

    async calculate() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const query = new URLSearchParams(this.params).toString();
            const resp = await fetch(`/api/cost/quick?${query}`);
            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            this.result = await resp.json();
        } catch (e) {
            console.error('Failed to calculate cost:', e);
            this.error = e.message;
        } finally {
            this.loading = false;
            this.render();
        }
    }

    updateParam(name, value) {
        this.params[name] = parseFloat(value) || 0;
    }

    render() {
        const estimates = this.result?.estimates || {};
        const datadog = estimates.datadog || {};
        const newrelic = estimates.newrelic || {};
        const splunk = estimates.splunk || {};

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cost-calculator">
                <div class="calc-header">
                    <div class="header-title">
                        <span class="title-icon">&#128425;</span>
                        <span>Cost Calculator</span>
                    </div>
                    <p class="header-desc">Estimate what your observability would cost on commercial platforms</p>
                </div>

                <div class="calc-content">
                    <div class="calc-inputs">
                        <h3>Your Infrastructure</h3>

                        <div class="input-group">
                            <label for="hosts">Hosts / Servers</label>
                            <input type="range" id="hosts" name="hosts" min="1" max="500" value="${this.params.hosts}">
                            <span class="input-value">${this.params.hosts}</span>
                        </div>

                        <div class="input-group">
                            <label for="containers">Containers</label>
                            <input type="range" id="containers" name="containers" min="0" max="2000" value="${this.params.containers}">
                            <span class="input-value">${this.params.containers}</span>
                        </div>

                        <div class="input-group">
                            <label for="custom_metrics">Custom Metrics</label>
                            <input type="range" id="custom_metrics" name="custom_metrics" min="0" max="10000" step="100" value="${this.params.custom_metrics}">
                            <span class="input-value">${this.params.custom_metrics.toLocaleString()}</span>
                        </div>

                        <div class="input-group">
                            <label for="logs_gb_per_day">Logs (GB/day)</label>
                            <input type="range" id="logs_gb_per_day" name="logs_gb_per_day" min="0" max="500" value="${this.params.logs_gb_per_day}">
                            <span class="input-value">${this.params.logs_gb_per_day} GB</span>
                        </div>

                        <div class="input-group">
                            <label for="spans_per_second">Spans/second (APM)</label>
                            <input type="range" id="spans_per_second" name="spans_per_second" min="0" max="10000" step="10" value="${this.params.spans_per_second}">
                            <span class="input-value">${this.params.spans_per_second.toLocaleString()}</span>
                        </div>

                        <button class="btn-calculate" id="btn-calculate">
                            ${this.loading ? 'Calculating...' : 'Calculate Costs'}
                        </button>
                    </div>

                    <div class="calc-results">
                        <h3>Estimated Monthly Costs</h3>

                        ${this.loading ? `
                            <div class="loading">Calculating...</div>
                        ` : this.error ? `
                            <div class="error">${this.escapeHtml(this.error)}</div>
                        ` : `
                            <div class="result-card dogwatch">
                                <div class="result-vendor">
                                    <span class="vendor-icon">&#128021;</span>
                                    <span>dogwatch</span>
                                </div>
                                <div class="result-price">$0</div>
                                <div class="result-note">Self-hosted, unlimited</div>
                            </div>

                            <div class="result-card">
                                <div class="result-vendor">
                                    <span class="vendor-icon">&#128054;</span>
                                    <span>Datadog</span>
                                </div>
                                <div class="result-price">${this.formatCurrency(datadog.total_monthly || 0)}</div>
                                <div class="result-annual">${this.formatCurrency(datadog.total_annual || 0)}/year</div>
                                ${this.renderBreakdown(datadog.breakdown)}
                            </div>

                            <div class="result-card">
                                <div class="result-vendor">
                                    <span class="vendor-icon">&#128202;</span>
                                    <span>New Relic</span>
                                </div>
                                <div class="result-price">${this.formatCurrency(newrelic.total_monthly || 0)}</div>
                                <div class="result-annual">${this.formatCurrency(newrelic.total_annual || 0)}/year</div>
                                ${this.renderBreakdown(newrelic.breakdown)}
                            </div>

                            <div class="result-card">
                                <div class="result-vendor">
                                    <span class="vendor-icon">&#128269;</span>
                                    <span>Splunk</span>
                                </div>
                                <div class="result-price">${this.formatCurrency(splunk.total_monthly || 0)}</div>
                                <div class="result-annual">${this.formatCurrency(splunk.total_annual || 0)}/year</div>
                                ${this.renderBreakdown(splunk.breakdown)}
                            </div>

                            <div class="total-savings">
                                <div class="savings-label">Total Annual Savings with dogwatch</div>
                                <div class="savings-value">${this.formatCurrency(this.getTotalAnnualSavings())}</div>
                            </div>
                        `}
                    </div>
                </div>

                <div class="calc-presets">
                    <h3>Quick Presets</h3>
                    <div class="preset-buttons">
                        <button class="preset-btn" data-preset="startup">Startup</button>
                        <button class="preset-btn" data-preset="growth">Growth</button>
                        <button class="preset-btn" data-preset="enterprise">Enterprise</button>
                        <button class="preset-btn" data-preset="scale">Scale</button>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderBreakdown(breakdown) {
        if (!breakdown) return '';
        const items = Object.entries(breakdown)
            .filter(([_, v]) => v > 0)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 3);

        if (items.length === 0) return '';

        return `
            <div class="result-breakdown">
                ${items.map(([k, v]) => `
                    <div class="breakdown-row">
                        <span>${this.formatLabel(k)}</span>
                        <span>${this.formatCurrency(v)}</span>
                    </div>
                `).join('')}
            </div>
        `;
    }

    getTotalAnnualSavings() {
        const estimates = this.result?.estimates || {};
        return (estimates.datadog?.total_annual || 0) +
               (estimates.newrelic?.total_annual || 0) +
               (estimates.splunk?.total_annual || 0);
    }

    applyPreset(preset) {
        const presets = {
            startup: { hosts: 5, containers: 20, custom_metrics: 100, logs_gb_per_day: 2, spans_per_second: 50 },
            growth: { hosts: 25, containers: 100, custom_metrics: 500, logs_gb_per_day: 20, spans_per_second: 200 },
            enterprise: { hosts: 100, containers: 500, custom_metrics: 2000, logs_gb_per_day: 100, spans_per_second: 1000 },
            scale: { hosts: 500, containers: 2000, custom_metrics: 10000, logs_gb_per_day: 500, spans_per_second: 5000 }
        };

        if (presets[preset]) {
            this.params = { ...presets[preset] };
            this.calculate();
        }
    }

    attachEventListeners() {
        // Range inputs
        this.querySelectorAll('input[type="range"]').forEach(input => {
            input.addEventListener('input', (e) => {
                this.updateParam(e.target.name, e.target.value);
                const valueSpan = e.target.nextElementSibling;
                if (valueSpan) {
                    let display = parseFloat(e.target.value).toLocaleString();
                    if (e.target.name === 'logs_gb_per_day') display += ' GB';
                    valueSpan.textContent = display;
                }
            });
        });

        // Calculate button
        this.querySelector('#btn-calculate')?.addEventListener('click', () => this.calculate());

        // Preset buttons
        this.querySelectorAll('.preset-btn').forEach(btn => {
            btn.addEventListener('click', () => this.applyPreset(btn.dataset.preset));
        });
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(1)}k`;
        return `$${Math.round(amount).toLocaleString()}`;
    }

    formatLabel(key) {
        return key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .cost-calculator {
                background: var(--bg-card, #16181c);
                min-height: 100%;
            }

            .calc-header {
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                margin-bottom: 0.25rem;
            }

            .title-icon { font-size: 1.25rem; }

            .header-desc {
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
                margin: 0;
            }

            .calc-content {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 1.5rem;
                padding: 1.5rem;
            }

            .calc-inputs h3, .calc-results h3, .calc-presets h3 {
                font-size: 0.9rem;
                font-weight: 600;
                margin: 0 0 1rem 0;
                color: var(--text-muted, #71767b);
            }

            .input-group {
                margin-bottom: 1.25rem;
            }

            .input-group label {
                display: block;
                font-size: 0.85rem;
                margin-bottom: 0.5rem;
                color: var(--text, #e7e9ea);
            }

            .input-group input[type="range"] {
                width: calc(100% - 80px);
                height: 6px;
                -webkit-appearance: none;
                background: var(--bg-elevated, #1e2128);
                border-radius: 3px;
                outline: none;
            }

            .input-group input[type="range"]::-webkit-slider-thumb {
                -webkit-appearance: none;
                width: 18px;
                height: 18px;
                background: var(--accent, #1d9bf0);
                border-radius: 50%;
                cursor: pointer;
            }

            .input-value {
                display: inline-block;
                width: 70px;
                text-align: right;
                font-size: 0.9rem;
                color: var(--accent, #1d9bf0);
                font-weight: 500;
            }

            .btn-calculate {
                width: 100%;
                padding: 0.75rem;
                background: var(--accent, #1d9bf0);
                color: white;
                border: none;
                border-radius: 6px;
                font-size: 0.9rem;
                font-weight: 500;
                cursor: pointer;
                margin-top: 1rem;
            }

            .btn-calculate:hover {
                background: #1a8cd8;
            }

            .loading, .error {
                padding: 2rem;
                text-align: center;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .result-card {
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                padding: 1rem;
                margin-bottom: 0.75rem;
            }

            .result-card.dogwatch {
                background: linear-gradient(135deg, rgba(0, 186, 124, 0.15) 0%, rgba(0, 135, 90, 0.15) 100%);
                border-color: var(--success, #00ba7c);
            }

            .result-vendor {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 500;
                margin-bottom: 0.5rem;
            }

            .vendor-icon { font-size: 1.25rem; }

            .result-price {
                font-size: 1.75rem;
                font-weight: 700;
            }

            .result-card.dogwatch .result-price {
                color: var(--success, #00ba7c);
            }

            .result-annual {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            .result-note {
                font-size: 0.8rem;
                color: var(--success, #00ba7c);
            }

            .result-breakdown {
                margin-top: 0.75rem;
                padding-top: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .breakdown-row {
                display: flex;
                justify-content: space-between;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                padding: 0.2rem 0;
            }

            .total-savings {
                background: linear-gradient(135deg, #00ba7c 0%, #00875a 100%);
                border-radius: 8px;
                padding: 1rem;
                text-align: center;
                color: white;
                margin-top: 1rem;
            }

            .total-savings .savings-label {
                font-size: 0.85rem;
                opacity: 0.9;
            }

            .total-savings .savings-value {
                font-size: 2rem;
                font-weight: 700;
                margin-top: 0.25rem;
            }

            .calc-presets {
                padding: 1.5rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .preset-buttons {
                display: flex;
                gap: 0.75rem;
                flex-wrap: wrap;
            }

            .preset-btn {
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 6px;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
                cursor: pointer;
            }

            .preset-btn:hover {
                background: var(--bg-card, #16181c);
                border-color: var(--accent, #1d9bf0);
            }

            @media (max-width: 900px) {
                .calc-content {
                    grid-template-columns: 1fr;
                }
            }
        `;
    }
}

customElements.define('cost-calculator', CostCalculator);
