/**
 * Error Budget Burndown Component
 * SLO visualization showing budget remaining over time
 */
class ErrorBudget extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Handle resize
        this.resizeObserver = new ResizeObserver(() => {
            if (this.chart) {
                this.chart.resize();
            }
        });
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.chart) this.chart.destroy();
        if (this.resizeObserver) this.resizeObserver.disconnect();
    }

    static get observedAttributes() {
        return ['slo-id', 'window'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            this.loadData();
        }
    }

    get sloId() { return this.getAttribute('slo-id') || ''; }
    get window() { return this.getAttribute('window') || '30d'; }

    render() {
        this.innerHTML = `
            <style>
                .budget-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .budget-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .budget-summary {
                    display: flex;
                    gap: 2rem;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-stat {
                    text-align: center;
                }
                .budget-stat-label {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.25rem;
                }
                .budget-stat-value {
                    font-size: 1.5rem;
                    font-weight: 700;
                }
                .budget-stat-value.good { color: #22c55e; }
                .budget-stat-value.warning { color: #f59e0b; }
                .budget-stat-value.critical { color: #f43f5e; }
                .budget-progress {
                    padding: 0.75rem 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-progress-bar {
                    height: 8px;
                    background: var(--border-color, #2f3336);
                    border-radius: 4px;
                    overflow: hidden;
                }
                .budget-progress-fill {
                    height: 100%;
                    border-radius: 4px;
                    transition: width 0.5s ease;
                }
                .budget-progress-labels {
                    display: flex;
                    justify-content: space-between;
                    margin-top: 0.5rem;
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .budget-chart {
                    flex: 1;
                    padding: 1rem;
                    min-height: 150px;
                }
                .budget-chart canvas {
                    width: 100% !important;
                    height: 100% !important;
                }
                .budget-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
                .budget-footer {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .budget-footer-item {
                    display: flex;
                    gap: 0.5rem;
                }
                .budget-footer-label {
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="budget-container">
                <div class="budget-header">
                    <div class="budget-title">Error Budget - <span id="slo-name">SLO</span></div>
                </div>
                <div class="budget-summary">
                    <div class="budget-stat">
                        <div class="budget-stat-label">Budget Remaining</div>
                        <div class="budget-stat-value" id="remaining">--</div>
                    </div>
                    <div class="budget-stat">
                        <div class="budget-stat-label">Burn Rate</div>
                        <div class="budget-stat-value" id="burn-rate">--</div>
                    </div>
                    <div class="budget-stat">
                        <div class="budget-stat-label">Time Left</div>
                        <div class="budget-stat-value" id="time-left">--</div>
                    </div>
                </div>
                <div class="budget-progress">
                    <div class="budget-progress-bar">
                        <div class="budget-progress-fill" id="progress-fill"></div>
                    </div>
                    <div class="budget-progress-labels">
                        <span>0%</span>
                        <span>Budget consumed</span>
                        <span>100%</span>
                    </div>
                </div>
                <div class="budget-chart">
                    <canvas id="chart"></canvas>
                </div>
                <div class="budget-footer">
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Target:</span>
                        <span id="target">99.9%</span>
                    </div>
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Current:</span>
                        <span id="current">99.85%</span>
                    </div>
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Window:</span>
                        <span id="window">30 days</span>
                    </div>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/slos/${this.sloId}/budget?window=${this.window}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateDisplay();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateDisplay();
        }
    }

    generateDemoData() {
        const days = 30;
        const burndown = [];
        let budget = 100;

        for (let i = 0; i < days; i++) {
            const dailyBurn = Math.random() * 5;
            budget = Math.max(0, budget - dailyBurn);
            burndown.push({
                date: Date.now() - (days - i) * 86400000,
                remaining: budget
            });
        }

        return {
            name: 'API Availability',
            target: 99.9,
            current: 99.85,
            budgetRemaining: budget,
            budgetConsumed: 100 - budget,
            burnRate: 1.2,
            timeToExhaustion: budget / 1.2,
            burndown,
            window: '30d'
        };
    }

    async updateDisplay() {
        if (!this.data) return;

        const { name, target, current, budgetRemaining, budgetConsumed, burnRate, timeToExhaustion, burndown } = this.data;

        // Update text
        this.querySelector('#slo-name').textContent = name;
        this.querySelector('#target').textContent = target + '%';
        this.querySelector('#current').textContent = current + '%';
        this.querySelector('#window').textContent = this.window;

        // Budget remaining with color
        const remainingEl = this.querySelector('#remaining');
        remainingEl.textContent = budgetRemaining.toFixed(1) + '%';
        remainingEl.className = 'budget-stat-value ' +
            (budgetRemaining > 50 ? 'good' : budgetRemaining > 20 ? 'warning' : 'critical');

        // Burn rate
        const burnRateEl = this.querySelector('#burn-rate');
        burnRateEl.textContent = burnRate.toFixed(1) + 'x';
        burnRateEl.className = 'budget-stat-value ' +
            (burnRate < 1 ? 'good' : burnRate < 2 ? 'warning' : 'critical');

        // Time left
        const timeLeftEl = this.querySelector('#time-left');
        if (timeToExhaustion > 30) {
            timeLeftEl.textContent = '>30d';
            timeLeftEl.className = 'budget-stat-value good';
        } else {
            timeLeftEl.textContent = timeToExhaustion.toFixed(0) + 'd';
            timeLeftEl.className = 'budget-stat-value ' +
                (timeToExhaustion > 10 ? 'warning' : 'critical');
        }

        // Progress bar
        const progressFill = this.querySelector('#progress-fill');
        progressFill.style.width = budgetConsumed + '%';
        progressFill.style.background = budgetConsumed < 50 ? '#22c55e' :
            budgetConsumed < 80 ? '#f59e0b' : '#f43f5e';

        // Chart
        await this.renderChart(burndown);
    }

    async renderChart(burndown) {
        const canvas = this.querySelector('#chart');
        if (!canvas || !burndown || burndown.length === 0) return;

        if (!window.Chart) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['chart', 'chart-date']);
            } else {
                console.error('Chart.js not available');
                return;
            }
        }

        if (this.chart) this.chart.destroy();

        const ctx = canvas.getContext('2d');

        // Ideal burndown line
        const idealLine = burndown.map((_, i) =>
            100 - (100 / burndown.length) * i
        );

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: burndown.map(d => new Date(d.date)),
                datasets: [
                    {
                        label: 'Actual',
                        data: burndown.map(d => d.remaining),
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.3,
                    },
                    {
                        label: 'Ideal',
                        data: idealLine,
                        borderColor: '#71767b',
                        borderDash: [5, 5],
                        fill: false,
                        pointRadius: 0,
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'top',
                        labels: { color: '#71767b', boxWidth: 12 }
                    }
                },
                scales: {
                    x: {
                        type: 'time',
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b', maxTicksLimit: 5 }
                    },
                    y: {
                        min: 0,
                        max: 100,
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: {
                            color: '#71767b',
                            callback: v => v + '%'
                        }
                    }
                }
            }
        });
    }
}

customElements.define('error-budget', ErrorBudget);
