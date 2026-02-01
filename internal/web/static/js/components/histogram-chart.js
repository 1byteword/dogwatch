/**
 * Histogram Chart Component
 * Shows latency distribution as a bar chart with percentile markers
 */
class HistogramChart extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        if (this.chart) this.chart.destroy();
    }

    static get observedAttributes() {
        return ['metric', 'service', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) this.loadData();
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .histogram-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .histogram-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .histogram-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .histogram-body {
                    flex: 1;
                    padding: 1rem;
                    min-height: 200px;
                }
                .histogram-canvas {
                    width: 100%;
                    height: 100%;
                }
                .histogram-percentiles {
                    display: flex;
                    justify-content: space-around;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                }
                .histogram-percentile {
                    text-align: center;
                }
                .histogram-percentile-label {
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                    text-transform: uppercase;
                }
                .histogram-percentile-value {
                    font-size: 1.1rem;
                    font-weight: 600;
                    margin-top: 0.25rem;
                }
                .histogram-percentile-value.p50 { color: #22c55e; }
                .histogram-percentile-value.p90 { color: #3b82f6; }
                .histogram-percentile-value.p95 { color: #f59e0b; }
                .histogram-percentile-value.p99 { color: #f43f5e; }
            </style>
            <div class="histogram-container">
                <div class="histogram-header">
                    <div class="histogram-title">Latency Distribution</div>
                </div>
                <div class="histogram-body">
                    <canvas class="histogram-canvas" id="chart"></canvas>
                </div>
                <div class="histogram-percentiles">
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P50</div>
                        <div class="histogram-percentile-value p50" id="p50">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P90</div>
                        <div class="histogram-percentile-value p90" id="p90">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P95</div>
                        <div class="histogram-percentile-value p95" id="p95">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P99</div>
                        <div class="histogram-percentile-value p99" id="p99">-</div>
                    </div>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/histogram?metric=${this.metric}&range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.renderChart();
        } catch (e) {
            this.data = this.generateDemoData();
            this.renderChart();
        }
    }

    generateDemoData() {
        // Generate log-normal distribution (realistic for latency)
        const buckets = ['0-5ms', '5-10ms', '10-25ms', '25-50ms', '50-100ms', '100-250ms', '250-500ms', '500ms-1s', '1s+'];
        const counts = [1200, 2800, 4500, 3200, 1800, 800, 300, 100, 50];

        return {
            buckets,
            counts,
            percentiles: { p50: 22, p90: 85, p95: 145, p99: 380 },
            total: counts.reduce((a, b) => a + b, 0)
        };
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.data) return;

        if (!window.Chart && window.LibLoader) {
            await window.LibLoader.load('chart');
        }

        if (this.chart) this.chart.destroy();

        const ctx = canvas.getContext('2d');
        const { buckets, counts, percentiles } = this.data;

        // Create gradient colors
        const colors = counts.map((_, i) => {
            const ratio = i / (counts.length - 1);
            if (ratio < 0.5) return `rgba(34, 197, 94, ${0.6 + ratio * 0.4})`;
            if (ratio < 0.75) return `rgba(59, 130, 246, ${0.6 + (ratio - 0.5) * 0.8})`;
            if (ratio < 0.9) return `rgba(245, 158, 11, ${0.7 + (ratio - 0.75) * 0.6})`;
            return `rgba(244, 63, 94, ${0.8 + (ratio - 0.9) * 0.4})`;
        });

        this.chart = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: buckets,
                datasets: [{
                    data: counts,
                    backgroundColor: colors,
                    borderRadius: 4,
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: (ctx) => {
                                const pct = ((ctx.raw / this.data.total) * 100).toFixed(1);
                                return `${ctx.raw.toLocaleString()} requests (${pct}%)`;
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        grid: { display: false },
                        ticks: { color: '#71767b', font: { size: 10 } }
                    },
                    y: {
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b' }
                    }
                }
            }
        });

        // Update percentiles
        const formatMs = (ms) => ms < 1000 ? `${ms}ms` : `${(ms/1000).toFixed(1)}s`;
        this.querySelector('#p50').textContent = formatMs(percentiles.p50);
        this.querySelector('#p90').textContent = formatMs(percentiles.p90);
        this.querySelector('#p95').textContent = formatMs(percentiles.p95);
        this.querySelector('#p99').textContent = formatMs(percentiles.p99);
    }
}

customElements.define('histogram-chart', HistogramChart);
