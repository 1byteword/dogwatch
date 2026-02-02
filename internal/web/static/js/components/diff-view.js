/**
 * Diff View Component
 * Side-by-side comparison of two time periods
 */
class DiffView extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.charts = [];
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Handle resize
        this.resizeObserver = new ResizeObserver(() => {
            this.charts.forEach(c => c.resize());
        });
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        this.charts.forEach(c => c.destroy());
        if (this.resizeObserver) this.resizeObserver.disconnect();
    }

    static get observedAttributes() {
        return ['metric', 'period1', 'period2'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            this.loadData();
        }
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get period1() { return this.getAttribute('period1') || 'today'; }
    get period2() { return this.getAttribute('period2') || 'yesterday'; }

    render() {
        this.innerHTML = `
            <style>
                .diff-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .diff-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .diff-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .diff-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .diff-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .diff-summary {
                    display: flex;
                    gap: 2rem;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .diff-stat {
                    flex: 1;
                    text-align: center;
                    padding: 0.75rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-radius: 6px;
                }
                .diff-stat-label {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .diff-stat-value {
                    font-size: 1.25rem;
                    font-weight: 700;
                    margin-top: 0.25rem;
                }
                .diff-stat-change {
                    font-size: 0.8rem;
                    margin-top: 0.25rem;
                }
                .diff-stat-change.positive { color: #22c55e; }
                .diff-stat-change.negative { color: #f43f5e; }
                .diff-stat-change.neutral { color: var(--text-muted, #71767b); }
                .diff-charts {
                    flex: 1;
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: 1px;
                    background: var(--border-color, #2f3336);
                    min-height: 200px;
                }
                .diff-chart-panel {
                    background: var(--bg-card, #16181c);
                    padding: 1rem;
                    display: flex;
                    flex-direction: column;
                }
                .diff-chart-title {
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.5rem;
                    text-align: center;
                }
                .diff-chart {
                    flex: 1;
                }
                .diff-chart canvas {
                    width: 100% !important;
                    height: 100% !important;
                }
                .diff-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    grid-column: span 2;
                }
            </style>
            <div class="diff-container">
                <div class="diff-header">
                    <div class="diff-title">Period Comparison</div>
                    <div class="diff-controls">
                        <select id="period1-select">
                            <option value="today">Today</option>
                            <option value="this-week">This Week</option>
                            <option value="this-month">This Month</option>
                        </select>
                        <span style="color: var(--text-muted)">vs</span>
                        <select id="period2-select">
                            <option value="yesterday">Yesterday</option>
                            <option value="last-week">Last Week</option>
                            <option value="last-month">Last Month</option>
                        </select>
                    </div>
                </div>
                <div class="diff-summary">
                    <div class="diff-stat">
                        <div class="diff-stat-label">Average Latency</div>
                        <div class="diff-stat-value" id="stat-latency">-</div>
                        <div class="diff-stat-change" id="stat-latency-change">-</div>
                    </div>
                    <div class="diff-stat">
                        <div class="diff-stat-label">Error Rate</div>
                        <div class="diff-stat-value" id="stat-errors">-</div>
                        <div class="diff-stat-change" id="stat-errors-change">-</div>
                    </div>
                    <div class="diff-stat">
                        <div class="diff-stat-label">Throughput</div>
                        <div class="diff-stat-value" id="stat-throughput">-</div>
                        <div class="diff-stat-change" id="stat-throughput-change">-</div>
                    </div>
                </div>
                <div class="diff-charts">
                    <div class="diff-chart-panel">
                        <div class="diff-chart-title" id="title1">Period 1</div>
                        <canvas class="diff-chart" id="chart1"></canvas>
                    </div>
                    <div class="diff-chart-panel">
                        <div class="diff-chart-title" id="title2">Period 2</div>
                        <canvas class="diff-chart" id="chart2"></canvas>
                    </div>
                </div>
            </div>
        `;

        this.querySelector('#period1-select')?.addEventListener('change', (e) => {
            this.setAttribute('period1', e.target.value);
            this.loadData();
        });

        this.querySelector('#period2-select')?.addEventListener('change', (e) => {
            this.setAttribute('period2', e.target.value);
            this.loadData();
        });
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/compare?period1=${this.period1}&period2=${this.period2}&metric=${this.metric}`);
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
        const generateSeries = (baseValue, variance) => {
            const points = 24;
            return Array.from({ length: points }, (_, i) => ({
                timestamp: Date.now() - (points - i) * 3600000,
                value: baseValue + (Math.random() - 0.5) * variance
            }));
        };

        return {
            period1: {
                label: 'Today',
                series: generateSeries(65, 30),
                stats: { latency: 65, errors: 0.8, throughput: 1250 }
            },
            period2: {
                label: 'Yesterday',
                series: generateSeries(55, 25),
                stats: { latency: 55, errors: 0.5, throughput: 1100 }
            }
        };
    }

    async updateDisplay() {
        if (!this.data) return;

        const { period1, period2 } = this.data;

        // Update titles
        this.querySelector('#title1').textContent = period1.label;
        this.querySelector('#title2').textContent = period2.label;

        // Update stats with diff
        this.updateStat('latency', period1.stats.latency, period2.stats.latency, 'ms', true);
        this.updateStat('errors', period1.stats.errors, period2.stats.errors, '%', true);
        this.updateStat('throughput', period1.stats.throughput, period2.stats.throughput, '/s', false);

        // Render charts
        await this.renderCharts();
    }

    updateStat(name, current, previous, unit, lowerIsBetter) {
        const valueEl = this.querySelector(`#stat-${name}`);
        const changeEl = this.querySelector(`#stat-${name}-change`);

        valueEl.textContent = current.toFixed(1) + unit;

        const diff = current - previous;
        const pct = ((diff / previous) * 100).toFixed(1);

        if (Math.abs(diff) < 0.1) {
            changeEl.textContent = 'No change';
            changeEl.className = 'diff-stat-change neutral';
        } else if (diff > 0) {
            changeEl.textContent = `+${pct}% vs previous`;
            changeEl.className = `diff-stat-change ${lowerIsBetter ? 'negative' : 'positive'}`;
        } else {
            changeEl.textContent = `${pct}% vs previous`;
            changeEl.className = `diff-stat-change ${lowerIsBetter ? 'positive' : 'negative'}`;
        }
    }

    async renderCharts() {
        if (!window.Chart) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['chart', 'chart-date']);
            } else {
                console.error('Chart.js not available');
                return;
            }
        }

        this.charts.forEach(c => c.destroy());
        this.charts = [];

        const { period1, period2 } = this.data;

        if (!period1?.series || !period2?.series) {
            return;
        }

        [
            { canvas: '#chart1', data: period1, color: '#3b82f6' },
            { canvas: '#chart2', data: period2, color: '#22c55e' }
        ].forEach(({ canvas, data, color }) => {
            const el = this.querySelector(canvas);
            if (!el) return;

            const chart = new Chart(el.getContext('2d'), {
                type: 'line',
                data: {
                    labels: data.series.map(d => new Date(d.timestamp)),
                    datasets: [{
                        data: data.series.map(d => d.value),
                        borderColor: color,
                        backgroundColor: color + '20',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: { legend: { display: false } },
                    scales: {
                        x: {
                            type: 'time',
                            display: false,
                        },
                        y: {
                            grid: { color: 'rgba(255,255,255,0.05)' },
                            ticks: { color: '#71767b' }
                        }
                    }
                }
            });

            this.charts.push(chart);
        });
    }
}

customElements.define('diff-view', DiffView);
