/**
 * Service Map Web Component
 *
 * Usage:
 *   <dw-service-map></dw-service-map>
 *   <dw-service-map auto-refresh="10000"></dw-service-map>
 *
 * Attributes:
 *   - auto-refresh: Refresh interval in ms (0 to disable, default: uses WebSocket)
 *   - layout: force|hierarchical (default: force)
 */
class ServiceMap extends HTMLElement {
    static get observedAttributes() {
        return ['auto-refresh', 'layout'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.simulation = null;
        this.svg = null;
        this.data = null;
        this._unsubscribe = null;
        this._refreshInterval = null;
        this.zoom = null;
    }

    async connectedCallback() {
        this.render();
        await this.loadD3();
        this.initSVG();
        this.setupWebSocket();
        this.fetchData();
    }

    disconnectedCallback() {
        this.cleanup();
    }

    cleanup() {
        // Stop simulation
        if (this.simulation) {
            this.simulation.stop();
            this.simulation = null;
        }

        // Unsubscribe from WebSocket
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }

        // Clear refresh interval
        if (this._refreshInterval) {
            clearInterval(this._refreshInterval);
            this._refreshInterval = null;
        }
    }

    async loadD3() {
        try {
            if (window.Loader) {
                await window.Loader.load('d3');
            } else if (typeof d3 === 'undefined') {
                throw new Error('D3 not available and Loader not found');
            }
        } catch (e) {
            console.error('[ServiceMap] Failed to load D3:', e);
            this._showError('Failed to load D3 library', 'Service map visualization requires D3. Please refresh the page.');
            throw e;
        }
    }

    render() {
        this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: block;
                    width: 100%;
                    height: 100%;
                    position: relative;
                }
                .container {
                    width: 100%;
                    height: 100%;
                    background: radial-gradient(ellipse at center, #1a1f2e 0%, #0f1419 100%);
                    overflow: hidden;
                }
                svg {
                    width: 100%;
                    height: 100%;
                }
                .node {
                    cursor: pointer;
                }
                .node:hover {
                    filter: brightness(1.2);
                }
                .node-circle {
                    stroke-width: 2;
                    fill: #1a1f2e;
                }
                .node-healthy .node-circle { stroke: #00ba7c; }
                .node-degraded .node-circle { stroke: #ffd400; }
                .node-unhealthy .node-circle { stroke: #f4212e; }
                .node-external .node-circle { stroke: #7c3aed; }
                .node-label {
                    fill: #e7e9ea;
                    font-size: 11px;
                    font-weight: 600;
                    text-anchor: middle;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                }
                .node-metrics {
                    fill: #8b949e;
                    font-size: 9px;
                    text-anchor: middle;
                    font-family: 'SF Mono', Consolas, monospace;
                }
                .link {
                    stroke: #3f4346;
                    stroke-width: 2;
                    fill: none;
                }
                .link-active {
                    stroke: rgba(29, 155, 240, 0.6);
                    stroke-dasharray: 6 4;
                    animation: flow 0.8s linear infinite;
                }
                .link-error {
                    stroke: rgba(244, 33, 46, 0.7);
                    stroke-width: 2.5;
                }
                @keyframes flow {
                    from { stroke-dashoffset: 10; }
                    to { stroke-dashoffset: 0; }
                }
                .tooltip {
                    position: absolute;
                    background: rgba(26, 31, 46, 0.95);
                    border: 1px solid #3f4346;
                    border-radius: 8px;
                    padding: 12px 16px;
                    font-size: 12px;
                    pointer-events: none;
                    z-index: 100;
                    display: none;
                    color: #e7e9ea;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    min-width: 180px;
                    box-shadow: 0 8px 24px rgba(0,0,0,0.5);
                }
                .tooltip.visible { display: block; }
                .tooltip-title { font-weight: 600; margin-bottom: 8px; }
                .tooltip-row { display: flex; justify-content: space-between; padding: 4px 0; }
                .tooltip-label { color: #71767b; }
                .tooltip-value { font-family: 'SF Mono', Consolas, monospace; }
                .legend {
                    position: absolute;
                    bottom: 10px;
                    left: 10px;
                    display: flex;
                    gap: 16px;
                    font-size: 11px;
                    color: #8b949e;
                    background: rgba(15, 20, 25, 0.8);
                    padding: 6px 12px;
                    border-radius: 6px;
                }
                .legend-item { display: flex; align-items: center; gap: 6px; }
                .legend-dot {
                    width: 10px;
                    height: 10px;
                    border-radius: 50%;
                    box-shadow: 0 0 6px currentColor;
                }
                .legend-dot.healthy { background: #00ba7c; color: #00ba7c; }
                .legend-dot.degraded { background: #ffd400; color: #ffd400; }
                .legend-dot.unhealthy { background: #f4212e; color: #f4212e; }
                .legend-dot.external { background: #7c3aed; color: #7c3aed; }
                .empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: #71767b;
                    font-size: 14px;
                }
            </style>
            <div class="container">
                <svg></svg>
                <div class="tooltip"></div>
                <div class="legend">
                    <div class="legend-item"><span class="legend-dot healthy"></span>Healthy</div>
                    <div class="legend-item"><span class="legend-dot degraded"></span>Degraded</div>
                    <div class="legend-item"><span class="legend-dot unhealthy"></span>Unhealthy</div>
                    <div class="legend-item"><span class="legend-dot external"></span>External</div>
                </div>
            </div>
        `;
    }

    initSVG() {
        const container = this.shadowRoot.querySelector('.container');
        this.svg = d3.select(this.shadowRoot.querySelector('svg'));

        // Set up zoom
        this.zoom = d3.zoom()
            .scaleExtent([0.3, 3])
            .on('zoom', (event) => {
                this.svg.select('.main-group').attr('transform', event.transform);
            });

        this.svg.call(this.zoom);

        // Create main group for zoom/pan
        this.svg.append('g').attr('class', 'main-group');
    }

    setupWebSocket() {
        if (window.dwSocket) {
            this._unsubscribe = window.dwSocket.subscribe('servicemap', (msg) => {
                if (msg.payload) {
                    this.updateData(msg.payload);
                }
            });
        }

        // Fallback to polling if auto-refresh is set
        const refreshInterval = parseInt(this.getAttribute('auto-refresh'));
        if (refreshInterval > 0) {
            this._refreshInterval = setInterval(() => this.fetchData(), refreshInterval);
        }
    }

    async fetchData() {
        try {
            const response = await fetch('/api/servicemap');
            if (response.ok) {
                const data = await response.json();
                this.updateData(data);
            } else {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
        } catch (e) {
            console.error('[ServiceMap] Failed to fetch data:', e);
            this._showError('Failed to load service map', e.message);
        }
    }

    _showError(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'error', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.error(message, title);
        }
    }

    updateData(data) {
        this.data = data;
        this.renderGraph();
    }

    renderGraph() {
        if (!this.data || !this.svg) return;

        const { nodes, edges } = this.data;
        if (!nodes || nodes.length === 0) {
            this.shadowRoot.querySelector('.container').innerHTML = '<div class="empty">No services discovered yet</div>';
            return;
        }

        const container = this.shadowRoot.querySelector('.container');
        const width = container.clientWidth;
        const height = container.clientHeight;

        const mainGroup = this.svg.select('.main-group');
        mainGroup.selectAll('*').remove();

        // Create links
        const linkGroup = mainGroup.append('g').attr('class', 'links');
        const links = linkGroup.selectAll('.link')
            .data(edges || [])
            .enter()
            .append('line')
            .attr('class', d => {
                let cls = 'link';
                if (d.errorRate > 0.01) cls += ' link-error';
                else if (d.rps > 0) cls += ' link-active';
                return cls;
            });

        // Create nodes
        const nodeGroup = mainGroup.append('g').attr('class', 'nodes');
        const nodeElements = nodeGroup.selectAll('.node')
            .data(nodes)
            .enter()
            .append('g')
            .attr('class', d => `node node-${d.status || 'healthy'}`)
            .call(d3.drag()
                .on('start', this.dragStarted.bind(this))
                .on('drag', this.dragged.bind(this))
                .on('end', this.dragEnded.bind(this)));

        nodeElements.append('circle')
            .attr('class', 'node-circle')
            .attr('r', 20);

        nodeElements.append('text')
            .attr('class', 'node-label')
            .attr('dy', 30)
            .text(d => d.name.length > 12 ? d.name.substring(0, 10) + '...' : d.name);

        nodeElements.append('text')
            .attr('class', 'node-metrics')
            .attr('dy', 42)
            .text(d => d.latency ? `${d.latency}ms` : '');

        // Tooltip events
        const tooltip = this.shadowRoot.querySelector('.tooltip');
        nodeElements
            .on('mouseover', (event, d) => {
                tooltip.innerHTML = `
                    <div class="tooltip-title">${d.name}</div>
                    <div class="tooltip-row"><span class="tooltip-label">Status</span><span class="tooltip-value">${d.status || 'unknown'}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">RPS</span><span class="tooltip-value">${d.rps || 0}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Latency</span><span class="tooltip-value">${d.latency || 0}ms</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Error Rate</span><span class="tooltip-value">${((d.errorRate || 0) * 100).toFixed(2)}%</span></div>
                `;
                tooltip.classList.add('visible');
            })
            .on('mousemove', (event) => {
                const rect = container.getBoundingClientRect();
                tooltip.style.left = (event.clientX - rect.left + 10) + 'px';
                tooltip.style.top = (event.clientY - rect.top - 10) + 'px';
            })
            .on('mouseout', () => {
                tooltip.classList.remove('visible');
            });

        // Set up force simulation
        const nodeMap = new Map(nodes.map((n, i) => [n.id || n.name, i]));

        this.simulation = d3.forceSimulation(nodes)
            .force('link', d3.forceLink(edges || [])
                .id(d => d.id || d.name)
                .distance(100))
            .force('charge', d3.forceManyBody().strength(-300))
            .force('center', d3.forceCenter(width / 2, height / 2))
            .force('collision', d3.forceCollide().radius(40));

        this.simulation.on('tick', () => {
            links
                .attr('x1', d => d.source.x)
                .attr('y1', d => d.source.y)
                .attr('x2', d => d.target.x)
                .attr('y2', d => d.target.y);

            nodeElements.attr('transform', d => `translate(${d.x},${d.y})`);
        });
    }

    dragStarted(event, d) {
        if (!event.active) this.simulation.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
    }

    dragged(event, d) {
        d.fx = event.x;
        d.fy = event.y;
    }

    dragEnded(event, d) {
        if (!event.active) this.simulation.alphaTarget(0);
        d.fx = null;
        d.fy = null;
    }

    // Public API
    refresh() {
        this.fetchData();
    }

    zoomIn() {
        this.svg.transition().call(this.zoom.scaleBy, 1.3);
    }

    zoomOut() {
        this.svg.transition().call(this.zoom.scaleBy, 0.7);
    }

    resetZoom() {
        this.svg.transition().call(this.zoom.transform, d3.zoomIdentity);
    }
}

customElements.define('dw-service-map', ServiceMap);
