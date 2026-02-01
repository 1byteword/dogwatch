/**
 * Latency Heatmap Component
 * Shows latency distribution over time with color intensity
 */
class LatencyHeatmap extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.canvas = null;
        this.ctx = null;
        this.resizeObserver = null;
        this.tooltip = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        this.resizeObserver = new ResizeObserver(() => this.drawHeatmap());
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['service', 'endpoint', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadData();
        }
    }

    get service() { return this.getAttribute('service') || ''; }
    get endpoint() { return this.getAttribute('endpoint') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .heatmap-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .heatmap-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .heatmap-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .heatmap-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .heatmap-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .heatmap-body {
                    flex: 1;
                    display: flex;
                    position: relative;
                    min-height: 200px;
                }
                .heatmap-y-axis {
                    width: 60px;
                    display: flex;
                    flex-direction: column;
                    justify-content: space-between;
                    padding: 1rem 0.5rem 2rem 0.5rem;
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                    text-align: right;
                }
                .heatmap-canvas-wrapper {
                    flex: 1;
                    position: relative;
                    padding: 1rem 1rem 2rem 0;
                }
                .heatmap-canvas {
                    width: 100%;
                    height: 100%;
                    cursor: crosshair;
                }
                .heatmap-x-axis {
                    position: absolute;
                    bottom: 0.5rem;
                    left: 60px;
                    right: 1rem;
                    display: flex;
                    justify-content: space-between;
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                }
                .heatmap-tooltip {
                    position: fixed;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.5rem 0.75rem;
                    font-size: 0.8rem;
                    pointer-events: none;
                    z-index: 1000;
                    display: none;
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                }
                .heatmap-tooltip-row {
                    display: flex;
                    justify-content: space-between;
                    gap: 1rem;
                }
                .heatmap-tooltip-label {
                    color: var(--text-muted, #71767b);
                }
                .heatmap-tooltip-value {
                    font-weight: 600;
                }
                .heatmap-legend {
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.75rem;
                }
                .heatmap-legend-label {
                    color: var(--text-muted, #71767b);
                }
                .heatmap-legend-gradient {
                    width: 120px;
                    height: 12px;
                    border-radius: 2px;
                    background: linear-gradient(to right, #1a1f2e, #1d4ed8, #7c3aed, #ec4899, #f43f5e);
                }
                .heatmap-loading {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
                .heatmap-percentiles {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.5rem 1rem;
                    font-size: 0.8rem;
                }
                .heatmap-percentile {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                }
                .heatmap-percentile-label {
                    color: var(--text-muted, #71767b);
                    font-size: 0.7rem;
                }
                .heatmap-percentile-value {
                    font-weight: 600;
                    font-size: 1rem;
                }
            </style>
            <div class="heatmap-container">
                <div class="heatmap-header">
                    <div class="heatmap-title">
                        <span>&#128200;</span>
                        <span>Latency Heatmap</span>
                    </div>
                    <div class="heatmap-controls">
                        <select id="time-select">
                            <option value="15m">15 min</option>
                            <option value="1h" selected>1 hour</option>
                            <option value="6h">6 hours</option>
                            <option value="24h">24 hours</option>
                        </select>
                    </div>
                </div>
                <div class="heatmap-percentiles" id="percentiles">
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P50</span>
                        <span class="heatmap-percentile-value" id="p50">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P90</span>
                        <span class="heatmap-percentile-value" id="p90">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P95</span>
                        <span class="heatmap-percentile-value" id="p95">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P99</span>
                        <span class="heatmap-percentile-value" id="p99">-</span>
                    </div>
                </div>
                <div class="heatmap-body">
                    <div class="heatmap-y-axis" id="y-axis"></div>
                    <div class="heatmap-canvas-wrapper">
                        <canvas class="heatmap-canvas" id="canvas"></canvas>
                    </div>
                    <div class="heatmap-x-axis" id="x-axis"></div>
                </div>
                <div class="heatmap-legend">
                    <span class="heatmap-legend-label">Requests:</span>
                    <span>Low</span>
                    <div class="heatmap-legend-gradient"></div>
                    <span>High</span>
                </div>
                <div class="heatmap-tooltip" id="tooltip"></div>
            </div>
        `;

        this.canvas = this.querySelector('#canvas');
        this.ctx = this.canvas?.getContext('2d');
        this.tooltip = this.querySelector('#tooltip');

        this.setupEventListeners();
    }

    setupEventListeners() {
        const timeSelect = this.querySelector('#time-select');
        timeSelect?.addEventListener('change', (e) => {
            this.setAttribute('time-range', e.target.value);
        });

        this.canvas?.addEventListener('mousemove', (e) => this.showTooltip(e));
        this.canvas?.addEventListener('mouseleave', () => this.hideTooltip());
    }

    async loadData() {
        try {
            const params = new URLSearchParams({ range: this.timeRange });
            if (this.service) params.append('service', this.service);
            if (this.endpoint) params.append('endpoint', this.endpoint);

            const resp = await fetch(`/api/metrics/latency-heatmap?${params}`);

            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }

            this.drawHeatmap();
            this.updatePercentiles();
        } catch (e) {
            console.error('Failed to load heatmap data:', e);
            this.data = this.generateDemoData();
            this.drawHeatmap();
            this.updatePercentiles();
        }
    }

    generateDemoData() {
        // Generate realistic latency heatmap data
        const buckets = ['0-10ms', '10-25ms', '25-50ms', '50-100ms', '100-250ms', '250-500ms', '500ms-1s', '1s+'];
        const numTimeSlots = 60; // 60 time buckets
        const data = {
            buckets: buckets,
            timeSlots: [],
            matrix: [],
            percentiles: { p50: 45, p90: 120, p95: 180, p99: 350 }
        };

        const now = Date.now();
        const slotDuration = 60000; // 1 minute per slot

        for (let t = 0; t < numTimeSlots; t++) {
            data.timeSlots.push(now - (numTimeSlots - t) * slotDuration);
            const row = [];

            // Create a realistic distribution - most requests are fast
            const baseDistribution = [0.3, 0.35, 0.2, 0.08, 0.04, 0.02, 0.008, 0.002];

            // Add some variation and occasional spikes
            const hasSpike = Math.random() > 0.9;
            const spikeMultiplier = hasSpike ? 3 : 1;

            for (let b = 0; b < buckets.length; b++) {
                let count = Math.floor(1000 * baseDistribution[b] * (0.5 + Math.random()));

                // Spikes affect slower buckets more
                if (hasSpike && b >= 3) {
                    count *= spikeMultiplier;
                }

                row.push(count);
            }
            data.matrix.push(row);
        }

        return data;
    }

    drawHeatmap() {
        if (!this.canvas || !this.ctx || !this.data) return;

        const wrapper = this.querySelector('.heatmap-canvas-wrapper');
        if (!wrapper) return;

        const rect = wrapper.getBoundingClientRect();
        const width = rect.width - 16; // padding
        const height = rect.height - 32; // padding

        // Set canvas size
        this.canvas.width = width * window.devicePixelRatio;
        this.canvas.height = height * window.devicePixelRatio;
        this.canvas.style.width = width + 'px';
        this.canvas.style.height = height + 'px';
        this.ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

        const { matrix, buckets, timeSlots } = this.data;
        if (!matrix || !matrix.length) return;

        const numRows = buckets.length;
        const numCols = matrix.length;
        const cellWidth = width / numCols;
        const cellHeight = height / numRows;

        // Find max value for color scaling
        let maxVal = 0;
        for (const row of matrix) {
            for (const val of row) {
                if (val > maxVal) maxVal = val;
            }
        }

        // Draw cells
        for (let col = 0; col < numCols; col++) {
            for (let row = 0; row < numRows; row++) {
                const value = matrix[col][row];
                const intensity = maxVal > 0 ? value / maxVal : 0;

                const x = col * cellWidth;
                const y = (numRows - 1 - row) * cellHeight; // Flip Y axis

                this.ctx.fillStyle = this.getHeatColor(intensity);
                this.ctx.fillRect(x, y, cellWidth + 0.5, cellHeight + 0.5);
            }
        }

        // Update axes
        this.updateAxes();
    }

    getHeatColor(intensity) {
        // Gradient from dark blue -> purple -> pink -> red
        if (intensity < 0.01) return '#1a1f2e';
        if (intensity < 0.1) return '#1e3a5f';
        if (intensity < 0.25) return '#1d4ed8';
        if (intensity < 0.5) return '#6366f1';
        if (intensity < 0.75) return '#a855f7';
        if (intensity < 0.9) return '#ec4899';
        return '#f43f5e';
    }

    updateAxes() {
        const yAxis = this.querySelector('#y-axis');
        const xAxis = this.querySelector('#x-axis');

        if (yAxis && this.data?.buckets) {
            yAxis.innerHTML = this.data.buckets
                .slice().reverse()
                .map(b => `<span>${b}</span>`)
                .join('');
        }

        if (xAxis && this.data?.timeSlots) {
            const slots = this.data.timeSlots;
            const formatTime = (ts) => {
                const d = new Date(ts);
                return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
            };

            xAxis.innerHTML = `
                <span>${formatTime(slots[0])}</span>
                <span>${formatTime(slots[Math.floor(slots.length / 2)])}</span>
                <span>${formatTime(slots[slots.length - 1])}</span>
            `;
        }
    }

    updatePercentiles() {
        if (!this.data?.percentiles) return;

        const formatMs = (ms) => {
            if (ms < 1000) return `${ms}ms`;
            return `${(ms / 1000).toFixed(2)}s`;
        };

        const p = this.data.percentiles;
        this.querySelector('#p50').textContent = formatMs(p.p50);
        this.querySelector('#p90').textContent = formatMs(p.p90);
        this.querySelector('#p95').textContent = formatMs(p.p95);
        this.querySelector('#p99').textContent = formatMs(p.p99);
    }

    showTooltip(e) {
        if (!this.canvas || !this.data || !this.tooltip) return;

        const rect = this.canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        const { matrix, buckets, timeSlots } = this.data;
        const cellWidth = rect.width / matrix.length;
        const cellHeight = rect.height / buckets.length;

        const col = Math.floor(x / cellWidth);
        const row = buckets.length - 1 - Math.floor(y / cellHeight);

        if (col < 0 || col >= matrix.length || row < 0 || row >= buckets.length) {
            this.hideTooltip();
            return;
        }

        const value = matrix[col][row];
        const bucket = buckets[row];
        const time = new Date(timeSlots[col]).toLocaleTimeString('en-US', {
            hour: '2-digit', minute: '2-digit', hour12: false
        });

        this.tooltip.innerHTML = `
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Time:</span>
                <span class="heatmap-tooltip-value">${time}</span>
            </div>
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Latency:</span>
                <span class="heatmap-tooltip-value">${bucket}</span>
            </div>
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Requests:</span>
                <span class="heatmap-tooltip-value">${value.toLocaleString()}</span>
            </div>
        `;

        this.tooltip.style.display = 'block';
        this.tooltip.style.left = (e.clientX + 10) + 'px';
        this.tooltip.style.top = (e.clientY + 10) + 'px';
    }

    hideTooltip() {
        if (this.tooltip) {
            this.tooltip.style.display = 'none';
        }
    }
}

customElements.define('latency-heatmap', LatencyHeatmap);
