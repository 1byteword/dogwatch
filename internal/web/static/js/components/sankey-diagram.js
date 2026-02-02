/**
 * Sankey Diagram Component
 * Request flow visualization through services
 */
class SankeyDiagram extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Handle resize
        this.resizeObserver = new ResizeObserver(() => {
            if (this.data) {
                this.renderSankey();
            }
        });
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            this.loadData();
        }
    }

    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .sankey-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .sankey-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .sankey-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .sankey-body {
                    flex: 1;
                    position: relative;
                    min-height: 300px;
                    overflow: hidden;
                }
                .sankey-svg {
                    width: 100%;
                    height: 100%;
                }
                .sankey-node rect {
                    cursor: pointer;
                    transition: opacity 0.2s ease;
                }
                .sankey-node rect:hover {
                    opacity: 0.8;
                }
                .sankey-node text {
                    fill: var(--text-primary, #e7e9ea);
                    font-size: 11px;
                    pointer-events: none;
                }
                .sankey-link {
                    fill: none;
                    stroke-opacity: 0.3;
                    transition: stroke-opacity 0.2s ease;
                }
                .sankey-link:hover {
                    stroke-opacity: 0.6;
                }
                .sankey-tooltip {
                    position: fixed;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.8rem;
                    pointer-events: none;
                    z-index: 1000;
                    display: none;
                }
                .sankey-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .sankey-stat-label {
                    color: var(--text-muted, #71767b);
                }
                .sankey-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    flex-direction: column;
                    gap: 0.5rem;
                }
            </style>
            <div class="sankey-container">
                <div class="sankey-header">
                    <div class="sankey-title">Request Flow</div>
                </div>
                <div class="sankey-body">
                    <svg class="sankey-svg" id="svg"></svg>
                </div>
                <div class="sankey-stats">
                    <div>
                        <span class="sankey-stat-label">Total Flow:</span>
                        <span id="stat-total">0</span>
                    </div>
                    <div>
                        <span class="sankey-stat-label">Services:</span>
                        <span id="stat-nodes">0</span>
                    </div>
                    <div>
                        <span class="sankey-stat-label">Connections:</span>
                        <span id="stat-links">0</span>
                    </div>
                </div>
                <div class="sankey-tooltip" id="tooltip"></div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/flow/sankey?range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.renderSankey();
        } catch (e) {
            this.data = this.generateDemoData();
            this.renderSankey();
        }
    }

    generateDemoData() {
        return {
            nodes: [
                { id: 'ingress', name: 'Ingress', type: 'entry' },
                { id: 'api-gateway', name: 'API Gateway', type: 'service' },
                { id: 'auth', name: 'Auth Service', type: 'service' },
                { id: 'users', name: 'User Service', type: 'service' },
                { id: 'orders', name: 'Order Service', type: 'service' },
                { id: 'payments', name: 'Payment Service', type: 'service' },
                { id: 'postgres', name: 'PostgreSQL', type: 'database' },
                { id: 'redis', name: 'Redis', type: 'cache' },
                { id: 'stripe', name: 'Stripe API', type: 'external' },
                { id: 'success', name: 'Success', type: 'exit' },
                { id: 'error', name: 'Errors', type: 'exit' },
            ],
            links: [
                { source: 'ingress', target: 'api-gateway', value: 10000 },
                { source: 'api-gateway', target: 'auth', value: 8000 },
                { source: 'api-gateway', target: 'users', value: 4000 },
                { source: 'api-gateway', target: 'orders', value: 3000 },
                { source: 'auth', target: 'redis', value: 7500 },
                { source: 'auth', target: 'postgres', value: 500 },
                { source: 'users', target: 'postgres', value: 3800 },
                { source: 'orders', target: 'postgres', value: 2500 },
                { source: 'orders', target: 'payments', value: 2000 },
                { source: 'payments', target: 'stripe', value: 1800 },
                { source: 'payments', target: 'postgres', value: 200 },
                { source: 'auth', target: 'success', value: 7800 },
                { source: 'auth', target: 'error', value: 200 },
                { source: 'users', target: 'success', value: 3700 },
                { source: 'users', target: 'error', value: 100 },
                { source: 'payments', target: 'success', value: 1700 },
                { source: 'payments', target: 'error', value: 100 },
            ]
        };
    }

    renderSankey() {
        const svg = this.querySelector('#svg');
        const tooltip = this.querySelector('#tooltip');
        if (!svg || !this.data) return;

        const { nodes, links } = this.data;
        if (!nodes || nodes.length === 0) {
            const body = this.querySelector('.sankey-body');
            if (body) {
                body.innerHTML = '<div class="sankey-empty"><span>No flow data available</span></div>';
            }
            return;
        }

        const rect = svg.getBoundingClientRect();
        const width = rect.width || 600;
        const height = rect.height || 300;
        const padding = { top: 20, right: 100, bottom: 20, left: 100 };

        // Create node map
        const nodeMap = new Map(nodes.map((n, i) => [n.id, { ...n, index: i }]));

        // Calculate node positions (simple layered layout)
        const layers = this.computeLayers(nodes, links);
        const layerCount = Math.max(...nodes.map(n => layers.get(n.id))) + 1;
        const layerWidth = (width - padding.left - padding.right) / layerCount;

        // Position nodes
        const nodesByLayer = new Map();
        nodes.forEach(n => {
            const layer = layers.get(n.id);
            if (!nodesByLayer.has(layer)) nodesByLayer.set(layer, []);
            nodesByLayer.get(layer).push(n);
        });

        const nodeHeight = 30;
        const nodePositions = new Map();

        nodesByLayer.forEach((layerNodes, layer) => {
            const totalHeight = layerNodes.length * (nodeHeight + 10);
            const startY = (height - totalHeight) / 2;

            layerNodes.forEach((n, i) => {
                nodePositions.set(n.id, {
                    x: padding.left + layer * layerWidth,
                    y: startY + i * (nodeHeight + 10),
                    width: 15,
                    height: nodeHeight
                });
            });
        });

        // Colors
        const colors = {
            entry: '#3b82f6',
            service: '#22c55e',
            database: '#a855f7',
            cache: '#f59e0b',
            external: '#71717a',
            exit: '#6b7280'
        };

        // Render
        svg.innerHTML = `
            <g class="sankey-links">
                ${links.map(l => {
                    const source = nodePositions.get(l.source);
                    const target = nodePositions.get(l.target);
                    if (!source || !target) return '';

                    const sourceNode = nodeMap.get(l.source);
                    const thickness = Math.max(2, Math.log(l.value) * 2);

                    const path = this.createLinkPath(
                        source.x + source.width, source.y + source.height / 2,
                        target.x, target.y + target.height / 2
                    );

                    return `<path class="sankey-link" d="${path}"
                                  stroke="${colors[sourceNode?.type] || '#3b82f6'}"
                                  stroke-width="${thickness}"
                                  data-source="${l.source}" data-target="${l.target}"
                                  data-value="${l.value}"/>`;
                }).join('')}
            </g>
            <g class="sankey-nodes">
                ${nodes.map(n => {
                    const pos = nodePositions.get(n.id);
                    if (!pos) return '';

                    return `
                        <g class="sankey-node" data-id="${n.id}">
                            <rect x="${pos.x}" y="${pos.y}"
                                  width="${pos.width}" height="${pos.height}"
                                  fill="${colors[n.type] || '#3b82f6'}"
                                  rx="3"/>
                            <text x="${pos.x + pos.width + 5}" y="${pos.y + pos.height / 2 + 4}">
                                ${n.name}
                            </text>
                        </g>
                    `;
                }).join('')}
            </g>
        `;

        // Tooltip events for links
        svg.querySelectorAll('.sankey-link').forEach(path => {
            path.addEventListener('mouseenter', (e) => {
                const source = path.dataset.source;
                const target = path.dataset.target;
                const value = parseInt(path.dataset.value);

                tooltip.innerHTML = `
                    <div style="font-weight:600">${source} → ${target}</div>
                    <div>Requests: ${value.toLocaleString()}</div>
                `;
                tooltip.style.display = 'block';
            });

            path.addEventListener('mousemove', (e) => {
                tooltip.style.left = (e.clientX + 10) + 'px';
                tooltip.style.top = (e.clientY + 10) + 'px';
            });

            path.addEventListener('mouseleave', () => {
                tooltip.style.display = 'none';
            });
        });

        // Stats
        const totalFlow = links.filter(l => l.source === 'ingress').reduce((s, l) => s + l.value, 0);
        this.querySelector('#stat-total').textContent = totalFlow.toLocaleString();
        this.querySelector('#stat-nodes').textContent = nodes.length;
        this.querySelector('#stat-links').textContent = links.length;
    }

    computeLayers(nodes, links) {
        const layers = new Map();
        const visited = new Set();

        // Find entry nodes (no incoming links)
        const hasIncoming = new Set(links.map(l => l.target));
        const entryNodes = nodes.filter(n => !hasIncoming.has(n.id));

        // BFS to assign layers
        const queue = entryNodes.map(n => ({ id: n.id, layer: 0 }));

        while (queue.length > 0) {
            const { id, layer } = queue.shift();

            if (visited.has(id)) continue;
            visited.add(id);
            layers.set(id, layer);

            // Find outgoing links
            links.filter(l => l.source === id).forEach(l => {
                if (!visited.has(l.target)) {
                    queue.push({ id: l.target, layer: layer + 1 });
                }
            });
        }

        // Handle any unvisited nodes
        nodes.forEach(n => {
            if (!layers.has(n.id)) {
                layers.set(n.id, 0);
            }
        });

        return layers;
    }

    createLinkPath(x1, y1, x2, y2) {
        const midX = (x1 + x2) / 2;
        return `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`;
    }
}

customElements.define('sankey-diagram', SankeyDiagram);
