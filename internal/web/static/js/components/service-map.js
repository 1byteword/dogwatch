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
        this._resizeObserver = null;
        this.zoom = null;
    }

    async connectedCallback() {
        this.render();
        await this.loadD3();
        this.initSVG();
        this.setupWebSocket();
        this.fetchData();

        // Handle resize
        this._resizeObserver = new ResizeObserver(() => {
            if (this.data && this.simulation) {
                const container = this.shadowRoot.querySelector('.container');
                const width = container.clientWidth;
                const height = container.clientHeight;
                this.simulation.force('center', d3.forceCenter(width / 2, height / 2));
                this.simulation.alpha(0.3).restart();
            }
        });
        this._resizeObserver.observe(this);
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

        // Disconnect resize observer
        if (this._resizeObserver) {
            this._resizeObserver.disconnect();
            this._resizeObserver = null;
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
                // Use demo data on API error
                this.updateData(this._generateDemoData());
            }
        } catch (e) {
            console.error('[ServiceMap] Failed to fetch data:', e);
            // Use demo data on network error
            this.updateData(this._generateDemoData());
        }
    }

    _generateDemoData() {
        return {
            nodes: [
                // Tier 0: Entry point (nucleus)
                { id: 'api-gateway', name: 'api-gateway', status: 'healthy', rps: 2500, latency: 12, errorRate: 0.001 },

                // Tier 1: Core services (first electron shell)
                { id: 'auth-service', name: 'auth-service', status: 'healthy', rps: 1800, latency: 25, errorRate: 0 },
                { id: 'user-service', name: 'user-service', status: 'degraded', rps: 1200, latency: 85, errorRate: 0.02 },
                { id: 'order-service', name: 'order-service', status: 'healthy', rps: 900, latency: 65, errorRate: 0.005 },
                { id: 'product-service', name: 'product-service', status: 'healthy', rps: 1100, latency: 45, errorRate: 0.001 },
                { id: 'search-service', name: 'search-service', status: 'healthy', rps: 800, latency: 35, errorRate: 0 },

                // Tier 2: Supporting services (second shell)
                { id: 'notification-svc', name: 'notification-svc', status: 'healthy', rps: 400, latency: 120, errorRate: 0 },
                { id: 'payment-service', name: 'payment-service', status: 'healthy', rps: 350, latency: 180, errorRate: 0.003 },
                { id: 'inventory-svc', name: 'inventory-svc', status: 'healthy', rps: 600, latency: 55, errorRate: 0 },
                { id: 'recommendation', name: 'recommendation', status: 'healthy', rps: 500, latency: 95, errorRate: 0 },

                // Tier 3: Data stores (outer shell)
                { id: 'postgres', name: 'postgres', status: 'healthy', rps: 3500, latency: 5, errorRate: 0 },
                { id: 'redis', name: 'redis', status: 'healthy', rps: 8000, latency: 1, errorRate: 0 },
                { id: 'elasticsearch', name: 'elasticsearch', status: 'healthy', rps: 1200, latency: 15, errorRate: 0 },
                { id: 'kafka', name: 'kafka', status: 'healthy', rps: 2000, latency: 8, errorRate: 0 },
            ],
            edges: [
                // Gateway to core services
                { source: 'api-gateway', target: 'auth-service', rps: 1800, errorRate: 0 },
                { source: 'api-gateway', target: 'user-service', rps: 1200, errorRate: 0.02 },
                { source: 'api-gateway', target: 'order-service', rps: 900, errorRate: 0 },
                { source: 'api-gateway', target: 'product-service', rps: 1100, errorRate: 0 },
                { source: 'api-gateway', target: 'search-service', rps: 800, errorRate: 0 },

                // Core to supporting services
                { source: 'order-service', target: 'payment-service', rps: 350, errorRate: 0.003 },
                { source: 'order-service', target: 'inventory-svc', rps: 600, errorRate: 0 },
                { source: 'order-service', target: 'notification-svc', rps: 400, errorRate: 0 },
                { source: 'user-service', target: 'notification-svc', rps: 200, errorRate: 0 },
                { source: 'product-service', target: 'recommendation', rps: 500, errorRate: 0 },
                { source: 'product-service', target: 'inventory-svc', rps: 300, errorRate: 0 },

                // Services to data stores
                { source: 'auth-service', target: 'redis', rps: 3000, errorRate: 0 },
                { source: 'user-service', target: 'postgres', rps: 1000, errorRate: 0 },
                { source: 'order-service', target: 'postgres', rps: 800, errorRate: 0 },
                { source: 'product-service', target: 'postgres', rps: 700, errorRate: 0 },
                { source: 'search-service', target: 'elasticsearch', rps: 1200, errorRate: 0 },
                { source: 'inventory-svc', target: 'postgres', rps: 500, errorRate: 0 },
                { source: 'recommendation', target: 'redis', rps: 400, errorRate: 0 },
                { source: 'notification-svc', target: 'kafka', rps: 600, errorRate: 0 },
                { source: 'payment-service', target: 'kafka', rps: 350, errorRate: 0 },
            ]
        };
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
        const centerX = width / 2;
        const centerY = height / 2;

        // Calculate orbital positions (electron cloud layout)
        this.calculateOrbitalPositions(nodes, edges, centerX, centerY, Math.min(width, height) * 0.4);

        const mainGroup = this.svg.select('.main-group');
        mainGroup.selectAll('*').remove();

        // Draw orbital rings (electron shells)
        const orbitGroup = mainGroup.append('g').attr('class', 'orbits');
        const maxOrbit = Math.max(...nodes.map(n => n._orbit || 0));
        const baseRadius = Math.min(width, height) * 0.12;

        for (let i = 1; i <= maxOrbit; i++) {
            orbitGroup.append('circle')
                .attr('cx', centerX)
                .attr('cy', centerY)
                .attr('r', baseRadius * i * 1.8)
                .attr('fill', 'none')
                .attr('stroke', 'rgba(63, 67, 70, 0.3)')
                .attr('stroke-width', 1)
                .attr('stroke-dasharray', '4 4');
        }

        // Create curved links
        const linkGroup = mainGroup.append('g').attr('class', 'links');
        const links = linkGroup.selectAll('.link')
            .data(edges || [])
            .enter()
            .append('path')
            .attr('class', d => {
                let cls = 'link';
                if (d.errorRate > 0.01) cls += ' link-error';
                else if (d.rps > 0) cls += ' link-active';
                return cls;
            })
            .attr('d', d => {
                const source = nodes.find(n => (n.id || n.name) === (d.source.id || d.source.name || d.source));
                const target = nodes.find(n => (n.id || n.name) === (d.target.id || d.target.name || d.target));
                if (!source || !target) return '';

                // Curved path
                const dx = target.x - source.x;
                const dy = target.y - source.y;
                const dr = Math.sqrt(dx * dx + dy * dy) * 0.8;
                return `M${source.x},${source.y}A${dr},${dr} 0 0,1 ${target.x},${target.y}`;
            });

        // Create nodes
        const nodeGroup = mainGroup.append('g').attr('class', 'nodes');
        const nodeElements = nodeGroup.selectAll('.node')
            .data(nodes)
            .enter()
            .append('g')
            .attr('class', d => `node node-${d.status || 'healthy'}`)
            .attr('transform', d => `translate(${d.x},${d.y})`)
            .call(d3.drag()
                .on('start', this.dragStarted.bind(this))
                .on('drag', this.draggedOrbital.bind(this))
                .on('end', this.dragEnded.bind(this)));

        // Nucleus (center node) gets special treatment
        nodeElements.append('circle')
            .attr('class', 'node-circle')
            .attr('r', d => d._orbit === 0 ? 28 : 20);

        // Add glow effect for center node
        nodeElements.filter(d => d._orbit === 0)
            .insert('circle', ':first-child')
            .attr('r', 35)
            .attr('fill', 'none')
            .attr('stroke', 'rgba(29, 155, 240, 0.3)')
            .attr('stroke-width', 8);

        nodeElements.append('text')
            .attr('class', 'node-label')
            .attr('dy', d => d._orbit === 0 ? 45 : 30)
            .text(d => d.name.length > 14 ? d.name.substring(0, 12) + '...' : d.name);

        nodeElements.append('text')
            .attr('class', 'node-metrics')
            .attr('dy', d => d._orbit === 0 ? 57 : 42)
            .text(d => d.latency ? `${d.latency}ms` : '');

        // Tooltip events
        const tooltip = this.shadowRoot.querySelector('.tooltip');
        nodeElements
            .on('mouseover', (event, d) => {
                tooltip.innerHTML = `
                    <div class="tooltip-title">${d.name}</div>
                    <div class="tooltip-row"><span class="tooltip-label">Status</span><span class="tooltip-value">${d.status || 'unknown'}</span></div>
                    <div class="tooltip-row"><span class="tooltip-label">Tier</span><span class="tooltip-value">${d._orbit === 0 ? 'Entry Point' : 'Tier ' + d._orbit}</span></div>
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

        // Store references for dragging
        this._nodes = nodes;
        this._nodeElements = nodeElements;
        this._links = links;
        this._centerX = centerX;
        this._centerY = centerY;
    }

    calculateOrbitalPositions(nodes, edges, centerX, centerY, maxRadius) {
        // Build adjacency map (who calls who)
        const outgoing = new Map();
        const incoming = new Map();
        nodes.forEach(n => {
            const id = n.id || n.name;
            outgoing.set(id, []);
            incoming.set(id, []);
        });

        (edges || []).forEach(e => {
            const src = e.source.id || e.source.name || e.source;
            const tgt = e.target.id || e.target.name || e.target;
            if (outgoing.has(src)) outgoing.get(src).push(tgt);
            if (incoming.has(tgt)) incoming.get(tgt).push(src);
        });

        // Find entry point (node with most outgoing and least incoming, typically api-gateway)
        let entryNode = nodes[0];
        let bestScore = -Infinity;
        nodes.forEach(n => {
            const id = n.id || n.name;
            const score = (outgoing.get(id)?.length || 0) - (incoming.get(id)?.length || 0) * 2;
            // Boost score for common entry point names
            const name = (n.name || '').toLowerCase();
            const bonus = (name.includes('gateway') || name.includes('ingress') || name.includes('frontend') || name.includes('api')) ? 10 : 0;
            if (score + bonus > bestScore) {
                bestScore = score + bonus;
                entryNode = n;
            }
        });

        // BFS to calculate orbit levels (distance from entry point)
        const orbits = new Map();
        const entryId = entryNode.id || entryNode.name;
        orbits.set(entryId, 0);

        const queue = [entryId];
        while (queue.length > 0) {
            const current = queue.shift();
            const currentOrbit = orbits.get(current);

            // Process outgoing connections
            (outgoing.get(current) || []).forEach(target => {
                if (!orbits.has(target)) {
                    orbits.set(target, currentOrbit + 1);
                    queue.push(target);
                }
            });
        }

        // Assign remaining unconnected nodes to outer orbit
        const maxOrbit = Math.max(...orbits.values(), 0);
        nodes.forEach(n => {
            const id = n.id || n.name;
            if (!orbits.has(id)) {
                orbits.set(id, maxOrbit + 1);
            }
        });

        // Group nodes by orbit
        const orbitGroups = new Map();
        nodes.forEach(n => {
            const id = n.id || n.name;
            const orbit = orbits.get(id);
            n._orbit = orbit;
            if (!orbitGroups.has(orbit)) orbitGroups.set(orbit, []);
            orbitGroups.get(orbit).push(n);
        });

        // Position nodes in orbital layout
        const baseRadius = maxRadius * 0.3;
        orbitGroups.forEach((groupNodes, orbit) => {
            if (orbit === 0) {
                // Center node
                groupNodes.forEach(n => {
                    n.x = centerX;
                    n.y = centerY;
                });
            } else {
                // Distribute around the orbit
                const radius = baseRadius * orbit * 1.8;
                const angleStep = (2 * Math.PI) / groupNodes.length;
                const startAngle = -Math.PI / 2; // Start from top

                groupNodes.forEach((n, i) => {
                    const angle = startAngle + angleStep * i;
                    n.x = centerX + radius * Math.cos(angle);
                    n.y = centerY + radius * Math.sin(angle);
                    n._angle = angle;
                    n._radius = radius;
                });
            }
        });
    }

    draggedOrbital(event, d) {
        // Allow free dragging but snap back to orbit on release
        d.x = event.x;
        d.y = event.y;

        // Update node position
        d3.select(event.sourceEvent.target.closest('.node'))
            .attr('transform', `translate(${d.x},${d.y})`);

        // Update connected links
        this._links.attr('d', link => {
            const source = this._nodes.find(n => (n.id || n.name) === (link.source.id || link.source.name || link.source));
            const target = this._nodes.find(n => (n.id || n.name) === (link.target.id || link.target.name || link.target));
            if (!source || !target) return '';
            const dx = target.x - source.x;
            const dy = target.y - source.y;
            const dr = Math.sqrt(dx * dx + dy * dy) * 0.8;
            return `M${source.x},${source.y}A${dr},${dr} 0 0,1 ${target.x},${target.y}`;
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
