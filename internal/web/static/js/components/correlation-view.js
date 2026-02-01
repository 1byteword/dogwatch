/**
 * Correlation View Component
 * Split-pane view with metrics on top and related logs/traces below
 */
class CorrelationView extends HTMLElement {
    constructor() {
        super();
        this.metricsData = null;
        this.correlatedData = null;
        this.selectedTimeRange = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadMetrics();
    }

    disconnectedCallback() {
        if (this.chart) {
            this.chart.destroy();
        }
    }

    static get observedAttributes() {
        return ['metric', 'service', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadMetrics();
        }
    }

    get metric() { return this.getAttribute('metric') || 'http_request_duration_seconds'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .correlation-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .correlation-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .correlation-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .correlation-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .correlation-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .correlation-metrics-panel {
                    height: 200px;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    position: relative;
                }
                .correlation-chart {
                    width: 100%;
                    height: 100%;
                }
                .correlation-hint {
                    position: absolute;
                    top: 0.5rem;
                    right: 1rem;
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-details-panel {
                    flex: 1;
                    display: flex;
                    flex-direction: column;
                    overflow: hidden;
                }
                .correlation-tabs {
                    display: flex;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .correlation-tab {
                    padding: 0.75rem 1rem;
                    cursor: pointer;
                    font-size: 0.85rem;
                    color: var(--text-muted, #71767b);
                    border-bottom: 2px solid transparent;
                    transition: all 0.15s ease;
                }
                .correlation-tab:hover {
                    color: var(--text-primary, #e7e9ea);
                }
                .correlation-tab.active {
                    color: var(--color-info, #1d9bf0);
                    border-bottom-color: var(--color-info, #1d9bf0);
                }
                .correlation-content {
                    flex: 1;
                    overflow: auto;
                    padding: 1rem;
                }
                .correlation-empty {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    text-align: center;
                    gap: 0.5rem;
                }
                .correlation-trace-item, .correlation-log-item {
                    padding: 0.75rem;
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    margin-bottom: 0.5rem;
                    cursor: pointer;
                    transition: all 0.15s ease;
                }
                .correlation-trace-item:hover, .correlation-log-item:hover {
                    border-color: var(--color-info, #1d9bf0);
                    background: var(--bg-elevated, #1a1f2e);
                }
                .correlation-item-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 0.5rem;
                }
                .correlation-item-title {
                    font-weight: 600;
                    font-size: 0.85rem;
                }
                .correlation-item-time {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-item-meta {
                    display: flex;
                    gap: 1rem;
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-log-message {
                    font-family: monospace;
                    font-size: 0.8rem;
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .correlation-badge {
                    display: inline-flex;
                    padding: 0.2rem 0.5rem;
                    border-radius: 4px;
                    font-size: 0.7rem;
                    font-weight: 600;
                }
                .correlation-badge.error {
                    background: rgba(244, 63, 94, 0.2);
                    color: #f43f5e;
                }
                .correlation-badge.warning {
                    background: rgba(245, 158, 11, 0.2);
                    color: #f59e0b;
                }
                .correlation-badge.info {
                    background: rgba(59, 130, 246, 0.2);
                    color: #3b82f6;
                }
                .correlation-selection-info {
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                    display: none;
                }
                .correlation-selection-info.visible {
                    display: block;
                }
            </style>
            <div class="correlation-container">
                <div class="correlation-header">
                    <div class="correlation-title">
                        <span>&#128279;</span>
                        <span>Correlation View</span>
                    </div>
                    <div class="correlation-controls">
                        <select id="metric-select">
                            <option value="http_request_duration_seconds">Request Latency</option>
                            <option value="http_requests_total">Request Rate</option>
                            <option value="http_request_errors_total">Error Rate</option>
                        </select>
                        <select id="time-select">
                            <option value="15m">15 min</option>
                            <option value="1h" selected>1 hour</option>
                            <option value="6h">6 hours</option>
                        </select>
                    </div>
                </div>
                <div class="correlation-metrics-panel">
                    <canvas class="correlation-chart" id="chart"></canvas>
                    <div class="correlation-hint">Click and drag to select a time range</div>
                </div>
                <div class="correlation-selection-info" id="selection-info">
                    Selected: <span id="selection-range"></span> -
                    <span id="selection-count"></span> events found
                </div>
                <div class="correlation-details-panel">
                    <div class="correlation-tabs">
                        <div class="correlation-tab active" data-tab="traces">Traces</div>
                        <div class="correlation-tab" data-tab="logs">Logs</div>
                        <div class="correlation-tab" data-tab="events">Events</div>
                    </div>
                    <div class="correlation-content" id="content">
                        <div class="correlation-empty">
                            <span style="font-size: 2rem;">&#128270;</span>
                            <span>Select a time range on the chart above</span>
                            <span>to see correlated traces and logs</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        // Tab switching
        this.querySelectorAll('.correlation-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.querySelectorAll('.correlation-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.renderCorrelatedData(e.target.dataset.tab);
            });
        });

        // Metric selection
        this.querySelector('#metric-select')?.addEventListener('change', (e) => {
            this.setAttribute('metric', e.target.value);
        });

        // Time range
        this.querySelector('#time-select')?.addEventListener('change', (e) => {
            this.setAttribute('time-range', e.target.value);
        });
    }

    async loadMetrics() {
        try {
            const params = new URLSearchParams({
                metric: this.metric,
                range: this.timeRange
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/metrics/timeseries?${params}`);

            if (!resp.ok) {
                this.metricsData = this.generateDemoMetrics();
            } else {
                this.metricsData = await resp.json();
            }

            this.renderChart();
        } catch (e) {
            console.error('Failed to load metrics:', e);
            this.metricsData = this.generateDemoMetrics();
            this.renderChart();
        }
    }

    generateDemoMetrics() {
        const now = Date.now();
        const points = 60;
        const interval = 60000; // 1 minute
        const data = [];

        for (let i = 0; i < points; i++) {
            const timestamp = now - (points - i) * interval;
            let value = 50 + Math.random() * 30;

            // Add some spikes
            if (i === 25 || i === 26) value += 100;
            if (i === 45) value += 80;

            data.push({ timestamp, value });
        }

        return { data };
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.metricsData) return;

        // Ensure Chart.js is loaded
        if (!window.Chart) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['chart', 'chart-date']);
            }
        }

        if (this.chart) {
            this.chart.destroy();
        }

        const ctx = canvas.getContext('2d');
        const data = this.metricsData.data;

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(d => new Date(d.timestamp)),
                datasets: [{
                    data: data.map(d => d.value),
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0,
                    pointHoverRadius: 4,
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
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
                },
                interaction: {
                    mode: 'nearest',
                    axis: 'x',
                    intersect: false
                },
                onClick: (event, elements) => {
                    if (elements.length > 0) {
                        const index = elements[0].index;
                        const timestamp = data[index].timestamp;
                        this.selectTimeRange(timestamp - 60000, timestamp + 60000);
                    }
                }
            }
        });

        // Add drag selection (simplified)
        canvas.addEventListener('click', (e) => {
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const chartArea = this.chart.chartArea;

            if (x >= chartArea.left && x <= chartArea.right) {
                const xScale = this.chart.scales.x;
                const timestamp = xScale.getValueForPixel(x);
                this.selectTimeRange(timestamp - 60000, timestamp + 60000);
            }
        });
    }

    async selectTimeRange(start, end) {
        this.selectedTimeRange = { start, end };

        // Update UI
        const selectionInfo = this.querySelector('#selection-info');
        const selectionRange = this.querySelector('#selection-range');

        selectionInfo.classList.add('visible');
        selectionRange.textContent = `${new Date(start).toLocaleTimeString()} - ${new Date(end).toLocaleTimeString()}`;

        // Load correlated data
        await this.loadCorrelatedData(start, end);
    }

    async loadCorrelatedData(start, end) {
        try {
            const params = new URLSearchParams({
                start: new Date(start).toISOString(),
                end: new Date(end).toISOString()
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/correlation/events?${params}`);

            if (!resp.ok) {
                this.correlatedData = this.generateDemoCorrelatedData();
            } else {
                this.correlatedData = await resp.json();
            }

            this.querySelector('#selection-count').textContent =
                (this.correlatedData.traces?.length || 0) + (this.correlatedData.logs?.length || 0);

            // Render active tab
            const activeTab = this.querySelector('.correlation-tab.active');
            this.renderCorrelatedData(activeTab?.dataset.tab || 'traces');
        } catch (e) {
            console.error('Failed to load correlated data:', e);
            this.correlatedData = this.generateDemoCorrelatedData();
            this.renderCorrelatedData('traces');
        }
    }

    generateDemoCorrelatedData() {
        return {
            traces: [
                { id: 'trace-1', service: 'api-gateway', operation: 'GET /api/users', duration: 245, status: 'error', timestamp: Date.now() - 30000 },
                { id: 'trace-2', service: 'user-service', operation: 'getUserById', duration: 180, status: 'ok', timestamp: Date.now() - 35000 },
                { id: 'trace-3', service: 'api-gateway', operation: 'POST /api/orders', duration: 520, status: 'error', timestamp: Date.now() - 40000 },
            ],
            logs: [
                { timestamp: Date.now() - 28000, level: 'error', service: 'api-gateway', message: 'Connection timeout to upstream service' },
                { timestamp: Date.now() - 32000, level: 'warn', service: 'user-service', message: 'Slow query detected: 180ms' },
                { timestamp: Date.now() - 38000, level: 'error', service: 'order-service', message: 'Database connection pool exhausted' },
                { timestamp: Date.now() - 42000, level: 'info', service: 'api-gateway', message: 'Retry attempt 1 for request xyz' },
            ],
            events: [
                { timestamp: Date.now() - 25000, type: 'deploy', service: 'api-gateway', message: 'Deployed v2.4.1' },
                { timestamp: Date.now() - 60000, type: 'alert', service: 'user-service', message: 'High latency alert triggered' },
            ]
        };
    }

    renderCorrelatedData(tab) {
        const content = this.querySelector('#content');
        if (!content || !this.correlatedData) return;

        if (tab === 'traces') {
            const traces = this.correlatedData.traces || [];
            if (traces.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No traces found in this time range</span></div>';
                return;
            }

            content.innerHTML = traces.map(t => `
                <div class="correlation-trace-item" data-trace-id="${t.id}">
                    <div class="correlation-item-header">
                        <span class="correlation-item-title">${t.operation}</span>
                        <span class="correlation-item-time">${new Date(t.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-item-meta">
                        <span>${t.service}</span>
                        <span>${t.duration}ms</span>
                        <span class="correlation-badge ${t.status}">${t.status}</span>
                    </div>
                </div>
            `).join('');

            // Add click handlers
            content.querySelectorAll('.correlation-trace-item').forEach(el => {
                el.addEventListener('click', () => {
                    this.dispatchEvent(new CustomEvent('trace-select', {
                        detail: { traceId: el.dataset.traceId }
                    }));
                });
            });
        } else if (tab === 'logs') {
            const logs = this.correlatedData.logs || [];
            if (logs.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No logs found in this time range</span></div>';
                return;
            }

            content.innerHTML = logs.map(l => `
                <div class="correlation-log-item">
                    <div class="correlation-item-header">
                        <span class="correlation-badge ${l.level}">${l.level}</span>
                        <span class="correlation-item-time">${new Date(l.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-log-message">${this.escapeHtml(l.message)}</div>
                    <div class="correlation-item-meta">
                        <span>${l.service}</span>
                    </div>
                </div>
            `).join('');
        } else if (tab === 'events') {
            const events = this.correlatedData.events || [];
            if (events.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No events found in this time range</span></div>';
                return;
            }

            content.innerHTML = events.map(e => `
                <div class="correlation-log-item">
                    <div class="correlation-item-header">
                        <span class="correlation-badge info">${e.type}</span>
                        <span class="correlation-item-time">${new Date(e.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-log-message">${this.escapeHtml(e.message)}</div>
                    <div class="correlation-item-meta">
                        <span>${e.service}</span>
                    </div>
                </div>
            `).join('');
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }
}

customElements.define('correlation-view', CorrelationView);
