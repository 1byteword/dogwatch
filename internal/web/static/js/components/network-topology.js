/**
 * Network Topology Component
 * Real-time network connection map using D3 force-directed graph
 */
class NetworkTopology extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.simulation = null;
        this.svg = null;
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        this.resizeObserver = new ResizeObserver(() => this.updateLayout());
        this.resizeObserver.observe(this);

        // Auto-refresh
        this.refreshInterval = setInterval(() => this.loadData(), 30000);
    }

    disconnectedCallback() {
        if (this.resizeObserver) this.resizeObserver.disconnect();
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        if (this.simulation) this.simulation.stop();
    }

    static get observedAttributes() {
        return ['namespace', 'show-external'];
    }

    get namespace() { return this.getAttribute('namespace') || ''; }
    get showExternal() { return this.getAttribute('show-external') !== 'false'; }

    render() {
        this.innerHTML = `
            <style>
                .topology-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .topology-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .topology-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .topology-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .topology-controls button {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.75rem;
                    color: var(--text-primary, #e7e9ea);
                    cursor: pointer;
                    font-size: 0.8rem;
                }
                .topology-controls button:hover {
                    border-color: var(--color-info, #1d9bf0);
                }
                .topology-body {
                    flex: 1;
                    position: relative;
                    overflow: hidden;
                }
                .topology-svg {
                    width: 100%;
                    height: 100%;
                }
                .topology-node {
                    cursor: pointer;
                }
                .topology-node circle {
                    stroke: var(--border-color, #2f3336);
                    stroke-width: 2px;
                    transition: all 0.15s ease;
                }
                .topology-node:hover circle {
                    stroke: var(--color-info, #1d9bf0);
                    stroke-width: 3px;
                }
                .topology-node text {
                    fill: var(--text-primary, #e7e9ea);
                    font-size: 11px;
                    text-anchor: middle;
                    pointer-events: none;
                }
                .topology-link {
                    stroke: var(--border-color, #2f3336);
                    stroke-opacity: 0.6;
                    fill: none;
                }
                .topology-link.active {
                    stroke: var(--color-success, #00ba7c);
                    stroke-opacity: 1;
                }
                .topology-link.error {
                    stroke: var(--color-error, #f4212e);
                    stroke-opacity: 1;
                }
                .topology-legend {
                    position: absolute;
                    bottom: 1rem;
                    left: 1rem;
                    display: flex;
                    flex-direction: column;
                    gap: 0.5rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.75rem;
                }
                .topology-legend-item {
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .topology-legend-dot {
                    width: 12px;
                    height: 12px;
                    border-radius: 50%;
                }
                .topology-tooltip {
                    position: fixed;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.8rem;
                    pointer-events: none;
                    z-index: 1000;
                    display: none;
                    max-width: 300px;
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                }
                .topology-tooltip-title {
                    font-weight: 600;
                    margin-bottom: 0.5rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .topology-tooltip-row {
                    display: flex;
                    justify-content: space-between;
                    gap: 1rem;
                    margin-top: 0.25rem;
                }
                .topology-tooltip-label {
                    color: var(--text-muted, #71767b);
                }
                .topology-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .topology-stat {
                    display: flex;
                    gap: 0.5rem;
                }
                .topology-stat-label {
                    color: var(--text-muted, #71767b);
                }
                .topology-stat-value {
                    font-weight: 600;
                }
            </style>
            <div class="topology-container">
                <div class="topology-header">
                    <div class="topology-title">
                        <span>&#127760;</span>
                        <span>Network Topology</span>
                    </div>
                    <div class="topology-controls">
                        <button id="zoom-in">+</button>
                        <button id="zoom-out">-</button>
                        <button id="reset-view">Reset</button>
                        <button id="refresh">Refresh</button>
                    </div>
                </div>
                <div class="topology-body" id="body">
                    <svg class="topology-svg" id="svg"></svg>
                    <div class="topology-legend">
                        <div class="topology-legend-item">
                            <div class="topology-legend-dot" style="background: #3b82f6;"></div>
                            <span>Service</span>
                        </div>
                        <div class="topology-legend-item">
                            <div class="topology-legend-dot" style="background: #22c55e;"></div>
                            <span>Database</span>
                        </div>
                        <div class="topology-legend-item">
                            <div class="topology-legend-dot" style="background: #a855f7;"></div>
                            <span>Cache</span>
                        </div>
                        <div class="topology-legend-item">
                            <div class="topology-legend-dot" style="background: #71717a;"></div>
                            <span>External</span>
                        </div>
                    </div>
                </div>
                <div class="topology-stats">
                    <div class="topology-stat">
                        <span class="topology-stat-label">Nodes:</span>
                        <span class="topology-stat-value" id="stat-nodes">0</span>
                    </div>
                    <div class="topology-stat">
                        <span class="topology-stat-label">Connections:</span>
                        <span class="topology-stat-value" id="stat-connections">0</span>
                    </div>
                    <div class="topology-stat">
                        <span class="topology-stat-label">Requests/s:</span>
                        <span class="topology-stat-value" id="stat-rps">0</span>
                    </div>
                </div>
                <div class="topology-tooltip" id="tooltip"></div>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        this.querySelector('#zoom-in')?.addEventListener('click', () => this.zoom(1.2));
        this.querySelector('#zoom-out')?.addEventListener('click', () => this.zoom(0.8));
        this.querySelector('#reset-view')?.addEventListener('click', () => this.resetView());
        this.querySelector('#refresh')?.addEventListener('click', () => this.loadData());
    }

    async loadData() {
        try {
            const params = new URLSearchParams();
            if (this.namespace) params.append('namespace', this.namespace);

            const resp = await fetch(`/api/network/topology?${params}`);

            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }

            this.renderTopology();
            this.updateStats();
        } catch (e) {
            console.error('Failed to load topology:', e);
            this.data = this.generateDemoData();
            this.renderTopology();
            this.updateStats();
        }
    }

    generateDemoData() {
        const nodes = [
            { id: 'api-gateway', name: 'api-gateway', type: 'service', rps: 1200, latency: 45, errors: 0.1 },
            { id: 'auth-service', name: 'auth-service', type: 'service', rps: 800, latency: 25, errors: 0 },
            { id: 'user-service', name: 'user-service', type: 'service', rps: 600, latency: 35, errors: 0.2 },
            { id: 'order-service', name: 'order-service', type: 'service', rps: 400, latency: 80, errors: 0.5 },
            { id: 'payment-service', name: 'payment-service', type: 'service', rps: 200, latency: 120, errors: 0.1 },
            { id: 'notification-svc', name: 'notification-svc', type: 'service', rps: 150, latency: 15, errors: 0 },
            { id: 'postgres-main', name: 'postgres-main', type: 'database', rps: 2000, latency: 5, errors: 0 },
            { id: 'postgres-replica', name: 'postgres-replica', type: 'database', rps: 1500, latency: 3, errors: 0 },
            { id: 'redis-cache', name: 'redis-cache', type: 'cache', rps: 5000, latency: 1, errors: 0 },
            { id: 'kafka', name: 'kafka', type: 'queue', rps: 3000, latency: 2, errors: 0 },
            { id: 'stripe-api', name: 'stripe-api', type: 'external', rps: 100, latency: 200, errors: 0.5 },
            { id: 'sendgrid', name: 'sendgrid', type: 'external', rps: 50, latency: 150, errors: 0.2 },
        ];

        const links = [
            { source: 'api-gateway', target: 'auth-service', rps: 800, latency: 10 },
            { source: 'api-gateway', target: 'user-service', rps: 400, latency: 15 },
            { source: 'api-gateway', target: 'order-service', rps: 300, latency: 20 },
            { source: 'auth-service', target: 'redis-cache', rps: 2000, latency: 1 },
            { source: 'auth-service', target: 'postgres-main', rps: 200, latency: 5 },
            { source: 'user-service', target: 'postgres-main', rps: 500, latency: 5 },
            { source: 'user-service', target: 'redis-cache', rps: 1000, latency: 1 },
            { source: 'order-service', target: 'postgres-main', rps: 400, latency: 8 },
            { source: 'order-service', target: 'payment-service', rps: 150, latency: 50 },
            { source: 'order-service', target: 'kafka', rps: 300, latency: 2 },
            { source: 'payment-service', target: 'stripe-api', rps: 100, latency: 200 },
            { source: 'payment-service', target: 'postgres-main', rps: 100, latency: 5 },
            { source: 'notification-svc', target: 'kafka', rps: 200, latency: 2 },
            { source: 'notification-svc', target: 'sendgrid', rps: 50, latency: 150 },
            { source: 'postgres-main', target: 'postgres-replica', rps: 500, latency: 1 },
        ];

        return { nodes, links };
    }

    async renderTopology() {
        if (!this.data) return;

        // Ensure D3 is loaded
        if (!window.d3) {
            if (window.LibLoader) {
                await window.LibLoader.load('d3');
            } else {
                console.error('D3 not available');
                return;
            }
        }

        const body = this.querySelector('#body');
        const svgEl = this.querySelector('#svg');
        if (!body || !svgEl) return;

        const width = body.clientWidth;
        const height = body.clientHeight;

        // Clear previous
        d3.select(svgEl).selectAll('*').remove();

        const svg = d3.select(svgEl)
            .attr('viewBox', [0, 0, width, height]);

        // Create zoom behavior
        const zoom = d3.zoom()
            .scaleExtent([0.3, 3])
            .on('zoom', (event) => {
                g.attr('transform', event.transform);
            });

        svg.call(zoom);

        const g = svg.append('g');

        // Arrow markers
        svg.append('defs').selectAll('marker')
            .data(['arrow'])
            .join('marker')
            .attr('id', 'arrow')
            .attr('viewBox', '0 -5 10 10')
            .attr('refX', 25)
            .attr('refY', 0)
            .attr('markerWidth', 6)
            .attr('markerHeight', 6)
            .attr('orient', 'auto')
            .append('path')
            .attr('fill', '#71767b')
            .attr('d', 'M0,-5L10,0L0,5');

        // Create simulation
        const simulation = d3.forceSimulation(this.data.nodes)
            .force('link', d3.forceLink(this.data.links).id(d => d.id).distance(120))
            .force('charge', d3.forceManyBody().strength(-400))
            .force('center', d3.forceCenter(width / 2, height / 2))
            .force('collision', d3.forceCollide().radius(50));

        this.simulation = simulation;

        // Draw links
        const link = g.append('g')
            .selectAll('line')
            .data(this.data.links)
            .join('line')
            .attr('class', 'topology-link')
            .attr('stroke-width', d => Math.max(1, Math.log(d.rps / 100)))
            .attr('marker-end', 'url(#arrow)');

        // Draw nodes
        const node = g.append('g')
            .selectAll('g')
            .data(this.data.nodes)
            .join('g')
            .attr('class', 'topology-node')
            .call(d3.drag()
                .on('start', (event, d) => {
                    if (!event.active) simulation.alphaTarget(0.3).restart();
                    d.fx = d.x;
                    d.fy = d.y;
                })
                .on('drag', (event, d) => {
                    d.fx = event.x;
                    d.fy = event.y;
                })
                .on('end', (event, d) => {
                    if (!event.active) simulation.alphaTarget(0);
                    d.fx = null;
                    d.fy = null;
                }));

        node.append('circle')
            .attr('r', d => 15 + Math.log(d.rps / 100) * 3)
            .attr('fill', d => this.getNodeColor(d.type));

        node.append('text')
            .attr('dy', 30)
            .text(d => d.name);

        // Tooltip events
        const tooltip = this.querySelector('#tooltip');
        node.on('mouseenter', (event, d) => {
            tooltip.innerHTML = `
                <div class="topology-tooltip-title">
                    <span style="color: ${this.getNodeColor(d.type)}">&#9679;</span>
                    ${d.name}
                </div>
                <div class="topology-tooltip-row">
                    <span class="topology-tooltip-label">Type:</span>
                    <span>${d.type}</span>
                </div>
                <div class="topology-tooltip-row">
                    <span class="topology-tooltip-label">Requests/s:</span>
                    <span>${d.rps.toLocaleString()}</span>
                </div>
                <div class="topology-tooltip-row">
                    <span class="topology-tooltip-label">Latency:</span>
                    <span>${d.latency}ms</span>
                </div>
                <div class="topology-tooltip-row">
                    <span class="topology-tooltip-label">Error Rate:</span>
                    <span style="color: ${d.errors > 0 ? '#f43f5e' : '#22c55e'}">${d.errors}%</span>
                </div>
            `;
            tooltip.style.display = 'block';
            tooltip.style.left = (event.pageX + 10) + 'px';
            tooltip.style.top = (event.pageY + 10) + 'px';
        });

        node.on('mouseleave', () => {
            tooltip.style.display = 'none';
        });

        node.on('click', (event, d) => {
            this.dispatchEvent(new CustomEvent('node-click', { detail: d }));
        });

        // Update positions on tick
        simulation.on('tick', () => {
            link
                .attr('x1', d => d.source.x)
                .attr('y1', d => d.source.y)
                .attr('x2', d => d.target.x)
                .attr('y2', d => d.target.y);

            node.attr('transform', d => `translate(${d.x},${d.y})`);
        });

        this.svg = svg;
        this.zoomBehavior = zoom;
    }

    getNodeColor(type) {
        const colors = {
            service: '#3b82f6',
            database: '#22c55e',
            cache: '#a855f7',
            queue: '#f59e0b',
            external: '#71717a'
        };
        return colors[type] || colors.service;
    }

    zoom(factor) {
        if (this.svg && this.zoomBehavior) {
            this.svg.transition().call(this.zoomBehavior.scaleBy, factor);
        }
    }

    resetView() {
        if (this.svg && this.zoomBehavior) {
            this.svg.transition().call(this.zoomBehavior.transform, d3.zoomIdentity);
        }
    }

    updateLayout() {
        if (this.simulation && this.data) {
            const body = this.querySelector('#body');
            if (body) {
                const width = body.clientWidth;
                const height = body.clientHeight;
                this.simulation.force('center', d3.forceCenter(width / 2, height / 2));
                this.simulation.alpha(0.3).restart();
            }
        }
    }

    updateStats() {
        if (!this.data) return;

        this.querySelector('#stat-nodes').textContent = this.data.nodes.length;
        this.querySelector('#stat-connections').textContent = this.data.links.length;

        const totalRps = this.data.nodes.reduce((sum, n) => sum + n.rps, 0);
        this.querySelector('#stat-rps').textContent = totalRps.toLocaleString();
    }
}

customElements.define('network-topology', NetworkTopology);
