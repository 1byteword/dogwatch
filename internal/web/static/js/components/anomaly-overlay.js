/**
 * Anomaly Overlay Component
 * Automatic highlighting of anomalous regions on charts
 */
class AnomalyOverlay extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
        this.anomalies = [];
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
        return ['metric', 'service', 'time-range', 'sensitivity'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            if (name === 'sensitivity') {
                this.detectAnomalies();
            } else {
                this.loadData();
            }
        }
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }
    get sensitivity() { return parseFloat(this.getAttribute('sensitivity')) || 2.0; }

    render() {
        this.innerHTML = `
            <style>
                .anomaly-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .anomaly-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .anomaly-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .anomaly-badge {
                    background: #f43f5e;
                    color: white;
                    padding: 0.2rem 0.5rem;
                    border-radius: 10px;
                    font-size: 0.7rem;
                    font-weight: 600;
                }
                .anomaly-controls {
                    display: flex;
                    gap: 0.5rem;
                    align-items: center;
                }
                .anomaly-controls label {
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                }
                .anomaly-controls input[type="range"] {
                    width: 80px;
                }
                .anomaly-body {
                    flex: 1;
                    padding: 1rem;
                    min-height: 200px;
                }
                .anomaly-chart {
                    width: 100%;
                    height: 100%;
                }
                .anomaly-list {
                    max-height: 150px;
                    overflow-y: auto;
                    border-top: 1px solid var(--border-color, #2f3336);
                }
                .anomaly-item {
                    display: flex;
                    align-items: center;
                    gap: 0.75rem;
                    padding: 0.75rem 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    cursor: pointer;
                    transition: background 0.15s ease;
                }
                .anomaly-item:hover {
                    background: var(--bg-elevated, #1a1f2e);
                }
                .anomaly-item:last-child {
                    border-bottom: none;
                }
                .anomaly-dot {
                    width: 10px;
                    height: 10px;
                    border-radius: 50%;
                    flex-shrink: 0;
                }
                .anomaly-dot.high { background: #f43f5e; }
                .anomaly-dot.medium { background: #f59e0b; }
                .anomaly-dot.low { background: #3b82f6; }
                .anomaly-info {
                    flex: 1;
                    min-width: 0;
                }
                .anomaly-time {
                    font-size: 0.8rem;
                    font-weight: 600;
                }
                .anomaly-desc {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .anomaly-score {
                    font-size: 0.8rem;
                    font-weight: 600;
                    color: var(--text-muted, #71767b);
                }
                .anomaly-chart canvas {
                    width: 100% !important;
                    height: 100% !important;
                }
                .anomaly-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="anomaly-container">
                <div class="anomaly-header">
                    <div class="anomaly-title">
                        <span>&#128270;</span>
                        <span>Anomaly Detection</span>
                        <span class="anomaly-badge" id="count">0</span>
                    </div>
                    <div class="anomaly-controls">
                        <label>Sensitivity:</label>
                        <input type="range" id="sensitivity" min="1" max="4" step="0.5" value="${this.sensitivity}">
                    </div>
                </div>
                <div class="anomaly-body">
                    <canvas class="anomaly-chart" id="chart"></canvas>
                </div>
                <div class="anomaly-list" id="list"></div>
            </div>
        `;

        this.querySelector('#sensitivity')?.addEventListener('input', (e) => {
            this.setAttribute('sensitivity', e.target.value);
            this.detectAnomalies();
        });
    }

    async loadData() {
        try {
            const params = new URLSearchParams({
                metric: this.metric,
                range: this.timeRange
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/metrics/timeseries?${params}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.detectAnomalies();
        } catch (e) {
            this.data = this.generateDemoData();
            this.detectAnomalies();
        }
    }

    generateDemoData() {
        const points = 60;
        const data = [];
        const now = Date.now();

        for (let i = 0; i < points; i++) {
            let value = 50 + Math.random() * 20;

            // Add anomalies
            if (i === 15) value = 150;
            if (i === 16) value = 140;
            if (i === 35) value = 120;
            if (i === 50) value = 5;

            data.push({
                timestamp: now - (points - i) * 60000,
                value
            });
        }

        return { data };
    }

    detectAnomalies() {
        if (!this.data?.data) return;

        const values = this.data.data.map(d => d.value);

        // Calculate mean and std
        const mean = values.reduce((a, b) => a + b, 0) / values.length;
        const std = Math.sqrt(values.reduce((sum, v) => sum + Math.pow(v - mean, 2), 0) / values.length);

        // Detect anomalies based on z-score
        const anomalies = [];
        const threshold = this.sensitivity;

        this.data.data.forEach((d, i) => {
            const zScore = Math.abs((d.value - mean) / std);
            if (zScore > threshold) {
                anomalies.push({
                    index: i,
                    timestamp: d.timestamp,
                    value: d.value,
                    zScore,
                    severity: zScore > threshold * 1.5 ? 'high' : zScore > threshold * 1.2 ? 'medium' : 'low',
                    direction: d.value > mean ? 'spike' : 'drop'
                });
            }
        });

        this.anomalies = anomalies;
        this.renderChart();
        this.renderList();
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.data?.data) return;

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
        const data = this.data.data;

        // Create anomaly highlighting dataset
        const anomalyData = data.map((d, i) => {
            const anomaly = this.anomalies.find(a => a.index === i);
            return anomaly ? d.value : null;
        });

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(d => new Date(d.timestamp)),
                datasets: [
                    {
                        label: 'Metric',
                        data: data.map(d => d.value),
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                    },
                    {
                        label: 'Anomaly',
                        data: anomalyData,
                        borderColor: '#f43f5e',
                        backgroundColor: '#f43f5e',
                        pointRadius: 8,
                        pointHoverRadius: 10,
                        showLine: false,
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: (ctx) => {
                                if (ctx.datasetIndex === 1) {
                                    const anomaly = this.anomalies.find(a => a.index === ctx.dataIndex);
                                    return anomaly ? `Anomaly: ${ctx.raw.toFixed(1)} (z=${anomaly.zScore.toFixed(2)})` : '';
                                }
                                return `Value: ${ctx.raw.toFixed(1)}`;
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        type: 'time',
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b', maxTicksLimit: 6 }
                    },
                    y: {
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b' }
                    }
                }
            }
        });
    }

    renderList() {
        const list = this.querySelector('#list');
        const countBadge = this.querySelector('#count');

        if (!list) return;

        countBadge.textContent = this.anomalies.length;

        if (this.anomalies.length === 0) {
            list.innerHTML = '<div style="padding: 1rem; text-align: center; color: var(--text-muted);">No anomalies detected</div>';
            return;
        }

        list.innerHTML = this.anomalies.map(a => `
            <div class="anomaly-item" data-index="${a.index}">
                <div class="anomaly-dot ${a.severity}"></div>
                <div class="anomaly-info">
                    <div class="anomaly-time">${new Date(a.timestamp).toLocaleTimeString()}</div>
                    <div class="anomaly-desc">${a.direction === 'spike' ? 'Spike' : 'Drop'}: ${a.value.toFixed(1)} (${a.severity})</div>
                </div>
                <div class="anomaly-score">z=${a.zScore.toFixed(1)}</div>
            </div>
        `).join('');

        // Click to highlight on chart
        list.querySelectorAll('.anomaly-item').forEach(item => {
            item.addEventListener('click', () => {
                const index = parseInt(item.dataset.index);
                // Could zoom to this point on the chart
                this.dispatchEvent(new CustomEvent('anomaly-click', {
                    detail: this.anomalies.find(a => a.index === index)
                }));
            });
        });
    }
}

customElements.define('anomaly-overlay', AnomalyOverlay);
