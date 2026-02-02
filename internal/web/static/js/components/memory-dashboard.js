/**
 * Memory & Runtime Dashboard Widget
 * Displays memory metrics, GC stats, object pools, and provides
 * runtime management controls
 */
class MemoryDashboard extends HTMLElement {
    constructor() {
        super();
        this.metrics = null;
        this.stats = null;
        this.history = [];
        this.config = null;
        this.refreshInterval = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
        this.refreshInterval = setInterval(() => this.loadData(), 5000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
        if (this.chart) {
            this.chart.destroy();
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="memory-container">
                <div class="memory-header">
                    <div class="memory-title">
                        <span class="title-icon">&#128190;</span>
                        <span>Memory & Runtime</span>
                    </div>
                    <div class="memory-controls">
                        <button class="btn-gc" id="btn-gc">Force GC</button>
                        <button class="btn-config" id="btn-config">&#9881;</button>
                        <button class="btn-refresh" id="btn-refresh">&#8635;</button>
                    </div>
                </div>

                <div class="pressure-indicator" id="pressure-indicator">
                    <div class="pressure-bar" id="pressure-bar"></div>
                    <span class="pressure-label" id="pressure-label">Normal</span>
                </div>

                <div class="gauges-row">
                    <div class="gauge-container">
                        <div class="gauge" id="gauge-heap-used">
                            <svg viewBox="0 0 36 36">
                                <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                                <path class="gauge-fill" stroke-dasharray="0, 100" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                            </svg>
                            <div class="gauge-value" id="heap-used-value">-</div>
                        </div>
                        <div class="gauge-label">Heap Used</div>
                    </div>
                    <div class="gauge-container">
                        <div class="gauge" id="gauge-heap-total">
                            <svg viewBox="0 0 36 36">
                                <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                                <path class="gauge-fill" stroke-dasharray="0, 100" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                            </svg>
                            <div class="gauge-value" id="heap-total-value">-</div>
                        </div>
                        <div class="gauge-label">Heap Total</div>
                    </div>
                    <div class="gauge-container">
                        <div class="gauge" id="gauge-stack">
                            <svg viewBox="0 0 36 36">
                                <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                                <path class="gauge-fill stack" stroke-dasharray="0, 100" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                            </svg>
                            <div class="gauge-value" id="stack-value">-</div>
                        </div>
                        <div class="gauge-label">Stack</div>
                    </div>
                    <div class="gauge-container">
                        <div class="gauge" id="gauge-gc">
                            <svg viewBox="0 0 36 36">
                                <path class="gauge-bg" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                                <path class="gauge-fill gc" stroke-dasharray="0, 100" d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"/>
                            </svg>
                            <div class="gauge-value" id="gc-value">-</div>
                        </div>
                        <div class="gauge-label">GC Overhead</div>
                    </div>
                </div>

                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-card-header">
                            <span>&#128202;</span> GC Stats
                        </div>
                        <div class="stat-card-body">
                            <div class="stat-row">
                                <span>Pause Time (avg)</span>
                                <span id="gc-pause-avg">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Pause Time (max)</span>
                                <span id="gc-pause-max">-</span>
                            </div>
                            <div class="stat-row">
                                <span>GC Frequency</span>
                                <span id="gc-frequency">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Last GC</span>
                                <span id="gc-last">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Total GCs</span>
                                <span id="gc-total">-</span>
                            </div>
                        </div>
                    </div>

                    <div class="stat-card">
                        <div class="stat-card-header">
                            <span>&#128230;</span> Object Pools
                        </div>
                        <div class="stat-card-body">
                            <div class="stat-row">
                                <span>Span Pool Hit Rate</span>
                                <span id="pool-span-hit">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Log Pool Hit Rate</span>
                                <span id="pool-log-hit">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Metric Pool Hit Rate</span>
                                <span id="pool-metric-hit">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Buffer Pool Utilization</span>
                                <span id="pool-buffer-util">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Pooled Allocations</span>
                                <span id="pool-allocations">-</span>
                            </div>
                        </div>
                    </div>

                    <div class="stat-card">
                        <div class="stat-card-header">
                            <span>&#9881;</span> Runtime
                        </div>
                        <div class="stat-card-body">
                            <div class="stat-row">
                                <span>Goroutines</span>
                                <span id="goroutines">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Heap Objects</span>
                                <span id="heap-objects">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Alloc Rate</span>
                                <span id="alloc-rate">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Memory Limit</span>
                                <span id="memory-limit">-</span>
                            </div>
                            <div class="stat-row">
                                <span>Uptime</span>
                                <span id="uptime">-</span>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="memory-chart-container">
                    <div class="chart-header">
                        <span>Memory Timeline</span>
                        <select id="chart-range" class="chart-range-select">
                            <option value="5m">5 min</option>
                            <option value="15m" selected>15 min</option>
                            <option value="1h">1 hour</option>
                        </select>
                    </div>
                    <canvas id="memory-chart" class="memory-chart"></canvas>
                </div>

                <div class="config-panel" id="config-panel" style="display: none;">
                    <div class="config-header">
                        <span>Memory Configuration</span>
                        <button class="btn-close" id="btn-close-config">&times;</button>
                    </div>
                    <div class="config-body">
                        <div class="config-field">
                            <label>Max Memory (MB)</label>
                            <input type="number" id="cfg-max-memory" value="500">
                        </div>
                        <div class="config-field">
                            <label>GC Target Percent</label>
                            <input type="number" id="cfg-gc-target" value="50">
                        </div>
                        <div class="config-field">
                            <label>Pressure Threshold (%)</label>
                            <input type="number" id="cfg-pressure" value="70">
                        </div>
                        <div class="config-field">
                            <label>Critical Threshold (%)</label>
                            <input type="number" id="cfg-critical" value="85">
                        </div>
                        <div class="config-field">
                            <label>Enable Object Pools</label>
                            <label class="toggle">
                                <input type="checkbox" id="cfg-pools" checked>
                                <span class="toggle-slider"></span>
                            </label>
                        </div>
                        <div class="config-field">
                            <label>Enable Buffer Recycling</label>
                            <label class="toggle">
                                <input type="checkbox" id="cfg-buffers" checked>
                                <span class="toggle-slider"></span>
                            </label>
                        </div>
                        <div class="config-actions">
                            <button class="btn-save" id="btn-save-config">Save Changes</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    getStyles() {
        return `
            .memory-container {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                position: relative;
            }
            .memory-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .memory-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                font-size: 0.9rem;
            }
            .title-icon { font-size: 1.1rem; }
            .memory-controls {
                display: flex;
                gap: 0.5rem;
            }
            .btn-refresh, .btn-config, .btn-gc, .btn-save, .btn-close {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
                font-size: 0.8rem;
            }
            .btn-gc {
                background: rgba(251, 191, 36, 0.2);
                color: #fbbf24;
                border-color: rgba(251, 191, 36, 0.3);
            }
            .btn-gc:hover {
                background: rgba(251, 191, 36, 0.3);
            }
            .btn-save {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
                width: 100%;
            }
            .pressure-indicator {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .pressure-bar {
                flex: 1;
                height: 8px;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                overflow: hidden;
                position: relative;
            }
            .pressure-bar::after {
                content: '';
                position: absolute;
                left: 0;
                top: 0;
                height: 100%;
                width: 30%;
                background: var(--success, #00ba7c);
                transition: all 0.3s ease;
            }
            .pressure-bar.elevated::after {
                width: 70%;
                background: #fbbf24;
            }
            .pressure-bar.critical::after {
                width: 90%;
                background: #f43f5e;
            }
            .pressure-label {
                font-size: 0.8rem;
                font-weight: 500;
                min-width: 80px;
                text-align: right;
            }
            .pressure-label.normal { color: var(--success, #00ba7c); }
            .pressure-label.elevated { color: #fbbf24; }
            .pressure-label.critical { color: #f43f5e; }
            .gauges-row {
                display: flex;
                justify-content: space-around;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .gauge-container {
                display: flex;
                flex-direction: column;
                align-items: center;
            }
            .gauge {
                width: 80px;
                height: 80px;
                position: relative;
            }
            .gauge svg {
                width: 100%;
                height: 100%;
                transform: rotate(-90deg);
            }
            .gauge-bg {
                fill: none;
                stroke: var(--bg-card, #16181c);
                stroke-width: 3;
            }
            .gauge-fill {
                fill: none;
                stroke: var(--accent, #1d9bf0);
                stroke-width: 3;
                stroke-linecap: round;
                transition: stroke-dasharray 0.3s ease;
            }
            .gauge-fill.stack { stroke: #a78bfa; }
            .gauge-fill.gc { stroke: #fbbf24; }
            .gauge-value {
                position: absolute;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                font-size: 0.85rem;
                font-weight: 600;
            }
            .gauge-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.5rem;
            }
            .stats-grid {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 0.75rem;
                padding: 0.75rem;
            }
            .stat-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                overflow: hidden;
            }
            .stat-card-header {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                padding: 0.5rem 0.75rem;
                background: rgba(255, 255, 255, 0.02);
                font-size: 0.8rem;
                font-weight: 500;
            }
            .stat-card-body {
                padding: 0.5rem 0.75rem;
            }
            .stat-row {
                display: flex;
                justify-content: space-between;
                padding: 0.3rem 0;
                font-size: 0.8rem;
            }
            .stat-row span:first-child {
                color: var(--text-muted, #71767b);
            }
            .stat-row span:last-child {
                font-family: monospace;
            }
            .memory-chart-container {
                flex: 1;
                min-height: 150px;
                padding: 0.75rem;
            }
            .chart-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
                font-size: 0.85rem;
                font-weight: 500;
            }
            .chart-range-select {
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.25rem 0.5rem;
                color: var(--text, #e7e9ea);
                font-size: 0.75rem;
            }
            .memory-chart {
                width: 100%;
                height: calc(100% - 30px);
            }
            .config-panel {
                position: absolute;
                top: 50px;
                right: 10px;
                width: 280px;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
                z-index: 100;
            }
            .config-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 500;
            }
            .config-body {
                padding: 1rem;
            }
            .config-field {
                margin-bottom: 1rem;
                display: flex;
                justify-content: space-between;
                align-items: center;
            }
            .config-field label {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }
            .config-field input[type="number"] {
                width: 80px;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.4rem;
                color: var(--text, #e7e9ea);
                text-align: right;
            }
            .toggle {
                position: relative;
                display: inline-block;
                width: 40px;
                height: 22px;
            }
            .toggle input { opacity: 0; width: 0; height: 0; }
            .toggle-slider {
                position: absolute;
                cursor: pointer;
                top: 0; left: 0; right: 0; bottom: 0;
                background: var(--bg-card, #16181c);
                border-radius: 11px;
                transition: 0.3s;
            }
            .toggle-slider:before {
                position: absolute;
                content: "";
                height: 16px;
                width: 16px;
                left: 3px;
                bottom: 3px;
                background: white;
                border-radius: 50%;
                transition: 0.3s;
            }
            .toggle input:checked + .toggle-slider {
                background: var(--accent, #1d9bf0);
            }
            .toggle input:checked + .toggle-slider:before {
                transform: translateX(18px);
            }
            .config-actions {
                margin-top: 1rem;
            }
            @media (max-width: 800px) {
                .stats-grid {
                    grid-template-columns: 1fr;
                }
                .gauges-row {
                    flex-wrap: wrap;
                }
            }
        `;
    }

    setupEventListeners() {
        // Refresh
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());

        // Force GC
        this.querySelector('#btn-gc')?.addEventListener('click', () => this.forceGC());

        // Config panel
        this.querySelector('#btn-config')?.addEventListener('click', () => {
            const panel = this.querySelector('#config-panel');
            if (panel) panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
        });

        this.querySelector('#btn-close-config')?.addEventListener('click', () => {
            const panel = this.querySelector('#config-panel');
            if (panel) panel.style.display = 'none';
        });

        this.querySelector('#btn-save-config')?.addEventListener('click', () => this.saveConfig());

        // Chart range
        this.querySelector('#chart-range')?.addEventListener('change', () => this.loadHistory());
    }

    async loadData() {
        try {
            const [metricsResp, statsResp, pressureResp, configResp] = await Promise.all([
                fetch('/api/memory/metrics'),
                fetch('/api/memory/stats'),
                fetch('/api/memory/pressure'),
                fetch('/api/memory/config')
            ]);

            if (metricsResp.ok) {
                this.metrics = await metricsResp.json();
            } else {
                this.metrics = this.generateDemoMetrics();
            }

            if (statsResp.ok) {
                this.stats = await statsResp.json();
            } else {
                this.stats = this.generateDemoStats();
            }

            if (pressureResp.ok) {
                const pressure = await pressureResp.json();
                this.updatePressure(pressure.level || 'normal');
            } else {
                this.updatePressure('normal');
            }

            if (configResp.ok) {
                this.config = await configResp.json();
                this.updateConfigPanel();
            }

            this.updateDisplay();
            this.loadHistory();
        } catch (e) {
            console.error('Failed to load memory data:', e);
            this.metrics = this.generateDemoMetrics();
            this.stats = this.generateDemoStats();
            this.updateDisplay();
        }
    }

    generateDemoMetrics() {
        return {
            allocMB: 156.4,
            heapAllocMB: 142.8,
            heapSysMB: 256.0,
            heapIdleMB: 80.2,
            heapInuseMB: 175.8,
            stackInuseMB: 2.4,
            heapObjects: 1284567,
            goroutines: 234,
            gcPauseNs: 1250000,
            gcPauseMaxNs: 4500000,
            numGC: 1456,
            lastGC: Date.now() - 15000,
            gcCPUPercent: 2.3,
            timestamp: Date.now()
        };
    }

    generateDemoStats() {
        return {
            pooledAllocations: 45678,
            poolHitRate: 0.87,
            bufferPoolSize: 1024,
            bufferPoolHits: 89234,
            bufferPoolMisses: 1234,
            spanPoolHitRate: 0.92,
            logPoolHitRate: 0.85,
            metricPoolHitRate: 0.89,
            forcedGCs: 3,
            uptime: 86400000 * 3 + 7200000 // 3 days, 2 hours
        };
    }

    updateDisplay() {
        if (!this.metrics) return;

        const m = this.metrics;
        const maxMemory = this.config?.maxMemoryMB || 500;

        // Update gauges
        this.updateGauge('heap-used', m.heapInuseMB || m.allocMB, maxMemory, this.formatBytes(m.heapInuseMB || m.allocMB, 'MB'));
        this.updateGauge('heap-total', m.heapSysMB, maxMemory, this.formatBytes(m.heapSysMB, 'MB'));
        this.updateGauge('stack', m.stackInuseMB, 50, this.formatBytes(m.stackInuseMB, 'MB'));
        this.updateGauge('gc', m.gcCPUPercent || 0, 10, `${(m.gcCPUPercent || 0).toFixed(1)}%`);

        // Update GC stats
        this.querySelector('#gc-pause-avg').textContent = this.formatNs(m.gcPauseNs);
        this.querySelector('#gc-pause-max').textContent = this.formatNs(m.gcPauseMaxNs);
        this.querySelector('#gc-frequency').textContent = m.numGC ? `${(m.numGC / (this.stats?.uptime || 86400000) * 60000).toFixed(1)}/min` : '-';
        this.querySelector('#gc-last').textContent = m.lastGC ? this.formatRelativeTime(m.lastGC) : '-';
        this.querySelector('#gc-total').textContent = m.numGC?.toLocaleString() || '-';

        // Update pool stats
        if (this.stats) {
            this.querySelector('#pool-span-hit').textContent = `${((this.stats.spanPoolHitRate || 0) * 100).toFixed(1)}%`;
            this.querySelector('#pool-log-hit').textContent = `${((this.stats.logPoolHitRate || 0) * 100).toFixed(1)}%`;
            this.querySelector('#pool-metric-hit').textContent = `${((this.stats.metricPoolHitRate || 0) * 100).toFixed(1)}%`;
            const bufferUtil = this.stats.bufferPoolHits / (this.stats.bufferPoolHits + this.stats.bufferPoolMisses + 1) * 100;
            this.querySelector('#pool-buffer-util').textContent = `${bufferUtil.toFixed(1)}%`;
            this.querySelector('#pool-allocations').textContent = this.stats.pooledAllocations?.toLocaleString() || '-';
        }

        // Update runtime stats
        this.querySelector('#goroutines').textContent = m.goroutines?.toLocaleString() || '-';
        this.querySelector('#heap-objects').textContent = m.heapObjects?.toLocaleString() || '-';
        this.querySelector('#alloc-rate').textContent = m.allocMB ? `${(m.allocMB / 60).toFixed(2)} MB/min` : '-';
        this.querySelector('#memory-limit').textContent = this.formatBytes(maxMemory, 'MB');
        this.querySelector('#uptime').textContent = this.stats?.uptime ? this.formatDuration(this.stats.uptime) : '-';
    }

    updateGauge(id, value, max, label) {
        const percent = Math.min((value / max) * 100, 100);
        const fill = this.querySelector(`#gauge-${id} .gauge-fill`);
        const valueEl = this.querySelector(`#${id}-value`);

        if (fill) {
            fill.setAttribute('stroke-dasharray', `${percent}, 100`);
        }
        if (valueEl) {
            valueEl.textContent = label;
        }
    }

    updatePressure(level) {
        const bar = this.querySelector('#pressure-bar');
        const label = this.querySelector('#pressure-label');

        if (bar) {
            bar.classList.remove('normal', 'elevated', 'critical');
            bar.classList.add(level);
        }

        if (label) {
            label.className = 'pressure-label ' + level;
            label.textContent = level.charAt(0).toUpperCase() + level.slice(1);
        }
    }

    async loadHistory() {
        const range = this.querySelector('#chart-range')?.value || '15m';

        try {
            const resp = await fetch(`/api/memory/history?range=${range}`);
            if (resp.ok) {
                this.history = await resp.json();
            } else {
                this.history = this.generateDemoHistory();
            }
            this.renderChart();
        } catch (e) {
            console.error('Failed to load history:', e);
            this.history = this.generateDemoHistory();
            this.renderChart();
        }
    }

    generateDemoHistory() {
        const points = 60;
        const now = Date.now();
        const history = [];

        for (let i = 0; i < points; i++) {
            const timestamp = now - (points - i) * 15000; // 15s intervals
            history.push({
                timestamp,
                allocMB: 140 + Math.random() * 40 + Math.sin(i / 10) * 20,
                heapSysMB: 250 + Math.random() * 10,
                goroutines: 220 + Math.random() * 30
            });
        }

        return history;
    }

    async renderChart() {
        const canvas = this.querySelector('#memory-chart');
        if (!canvas || !this.history.length) return;

        // Load Chart.js if needed
        if (!window.Chart) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['chart', 'chart-date']);
            }
            if (!window.Chart) return;
        }

        if (this.chart) {
            this.chart.destroy();
        }

        const ctx = canvas.getContext('2d');
        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: this.history.map(h => new Date(h.timestamp)),
                datasets: [
                    {
                        label: 'Heap Used',
                        data: this.history.map(h => h.allocMB),
                        borderColor: '#1d9bf0',
                        backgroundColor: 'rgba(29, 155, 240, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0
                    },
                    {
                        label: 'Heap Total',
                        data: this.history.map(h => h.heapSysMB),
                        borderColor: '#71767b',
                        backgroundColor: 'transparent',
                        borderDash: [5, 5],
                        tension: 0.3,
                        pointRadius: 0
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'bottom',
                        labels: { color: '#71767b', boxWidth: 12 }
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
                        ticks: {
                            color: '#71767b',
                            callback: (v) => v + ' MB'
                        }
                    }
                }
            }
        });
    }

    async forceGC() {
        const btn = this.querySelector('#btn-gc');
        if (btn) {
            btn.textContent = 'Running...';
            btn.disabled = true;
        }

        try {
            const resp = await fetch('/api/memory/gc', { method: 'POST' });
            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to trigger GC:', e);
        } finally {
            if (btn) {
                btn.textContent = 'Force GC';
                btn.disabled = false;
            }
        }
    }

    updateConfigPanel() {
        if (!this.config) return;

        const inputs = {
            'cfg-max-memory': this.config.maxMemoryMB,
            'cfg-gc-target': this.config.gcTargetPercent,
            'cfg-pressure': this.config.pressureThreshold,
            'cfg-critical': this.config.criticalThreshold
        };

        for (const [id, value] of Object.entries(inputs)) {
            const input = this.querySelector(`#${id}`);
            if (input && value !== undefined) input.value = value;
        }

        const poolsInput = this.querySelector('#cfg-pools');
        const buffersInput = this.querySelector('#cfg-buffers');
        if (poolsInput) poolsInput.checked = this.config.enableObjectPools !== false;
        if (buffersInput) buffersInput.checked = this.config.enableBufferRecycling !== false;
    }

    async saveConfig() {
        const config = {
            maxMemoryMB: parseInt(this.querySelector('#cfg-max-memory')?.value || 500),
            gcTargetPercent: parseInt(this.querySelector('#cfg-gc-target')?.value || 50),
            pressureThreshold: parseInt(this.querySelector('#cfg-pressure')?.value || 70),
            criticalThreshold: parseInt(this.querySelector('#cfg-critical')?.value || 85),
            enableObjectPools: this.querySelector('#cfg-pools')?.checked ?? true,
            enableBufferRecycling: this.querySelector('#cfg-buffers')?.checked ?? true
        };

        try {
            const resp = await fetch('/api/memory/config', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });

            if (resp.ok) {
                this.querySelector('#config-panel').style.display = 'none';
                this.config = config;
            }
        } catch (e) {
            console.error('Failed to save config:', e);
        }
    }

    formatBytes(mb, unit = 'MB') {
        if (mb === undefined || mb === null) return '-';
        if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
        return `${mb.toFixed(1)} ${unit}`;
    }

    formatNs(ns) {
        if (!ns) return '-';
        if (ns < 1000) return `${ns}ns`;
        if (ns < 1000000) return `${(ns / 1000).toFixed(1)}us`;
        if (ns < 1000000000) return `${(ns / 1000000).toFixed(2)}ms`;
        return `${(ns / 1000000000).toFixed(2)}s`;
    }

    formatDuration(ms) {
        const s = Math.floor(ms / 1000);
        const m = Math.floor(s / 60);
        const h = Math.floor(m / 60);
        const d = Math.floor(h / 24);

        if (d > 0) return `${d}d ${h % 24}h`;
        if (h > 0) return `${h}h ${m % 60}m`;
        if (m > 0) return `${m}m`;
        return `${s}s`;
    }

    formatRelativeTime(timestamp) {
        const diff = Date.now() - timestamp;
        if (diff < 1000) return 'just now';
        if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`;
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        return `${Math.floor(diff / 3600000)}h ago`;
    }

    // Public API
    refresh() {
        this.loadData();
    }
}

customElements.define('memory-dashboard', MemoryDashboard);
