/**
 * Resource Treemap Component
 * Hierarchical resource usage visualization
 */
class ResourceTreemap extends HTMLElement {
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
                this.renderTreemap();
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
        return ['resource', 'namespace'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            this.loadData();
        }
    }

    get resource() { return this.getAttribute('resource') || 'memory'; }
    get namespace() { return this.getAttribute('namespace') || ''; }

    render() {
        this.innerHTML = `
            <style>
                .treemap-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .treemap-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .treemap-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .treemap-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .treemap-body {
                    flex: 1;
                    position: relative;
                    min-height: 250px;
                }
                .treemap-cell {
                    position: absolute;
                    border: 1px solid var(--bg-primary, #0f1419);
                    overflow: hidden;
                    cursor: pointer;
                    transition: all 0.15s ease;
                }
                .treemap-cell:hover {
                    z-index: 10;
                    transform: scale(1.02);
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                }
                .treemap-cell-content {
                    padding: 0.5rem;
                    height: 100%;
                    display: flex;
                    flex-direction: column;
                }
                .treemap-cell-name {
                    font-size: 0.75rem;
                    font-weight: 600;
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .treemap-cell-value {
                    font-size: 0.9rem;
                    font-weight: 700;
                    margin-top: auto;
                }
                .treemap-cell-small .treemap-cell-name {
                    font-size: 0.65rem;
                }
                .treemap-cell-small .treemap-cell-value {
                    display: none;
                }
                .treemap-tooltip {
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
                .treemap-legend {
                    display: flex;
                    gap: 1rem;
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.75rem;
                    flex-wrap: wrap;
                }
                .treemap-legend-item {
                    display: flex;
                    align-items: center;
                    gap: 0.25rem;
                }
                .treemap-legend-dot {
                    width: 10px;
                    height: 10px;
                    border-radius: 2px;
                }
                .treemap-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="treemap-container">
                <div class="treemap-header">
                    <div class="treemap-title">Resource Usage by Container</div>
                    <div class="treemap-controls">
                        <select id="resource-select">
                            <option value="memory">Memory</option>
                            <option value="cpu">CPU</option>
                            <option value="disk">Disk</option>
                        </select>
                    </div>
                </div>
                <div class="treemap-body" id="body"></div>
                <div class="treemap-legend" id="legend"></div>
                <div class="treemap-tooltip" id="tooltip"></div>
            </div>
        `;

        this.querySelector('#resource-select')?.addEventListener('change', (e) => {
            this.setAttribute('resource', e.target.value);
            this.loadData();
        });
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/resources/treemap?resource=${this.resource}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.renderTreemap();
        } catch (e) {
            this.data = this.generateDemoData();
            this.renderTreemap();
        }
    }

    generateDemoData() {
        const services = [
            { name: 'api-gateway', namespace: 'production', value: 2048, limit: 4096 },
            { name: 'user-service', namespace: 'production', value: 1536, limit: 2048 },
            { name: 'order-service', namespace: 'production', value: 1800, limit: 2048 },
            { name: 'payment-service', namespace: 'production', value: 1024, limit: 2048 },
            { name: 'notification', namespace: 'production', value: 512, limit: 1024 },
            { name: 'postgres', namespace: 'database', value: 4096, limit: 8192 },
            { name: 'redis', namespace: 'database', value: 1024, limit: 2048 },
            { name: 'kafka', namespace: 'messaging', value: 3072, limit: 4096 },
            { name: 'monitoring', namespace: 'system', value: 768, limit: 1024 },
            { name: 'logging', namespace: 'system', value: 512, limit: 1024 },
        ];

        return { items: services };
    }

    renderTreemap() {
        const body = this.querySelector('#body');
        const legend = this.querySelector('#legend');
        const tooltip = this.querySelector('#tooltip');
        if (!body || !this.data) return;

        const items = this.data.items;

        if (!items || items.length === 0) {
            body.innerHTML = '<div class="treemap-empty">No resource data available</div>';
            return;
        }

        const width = body.clientWidth || 400;
        const height = body.clientHeight || 250;

        // Simple treemap layout
        const total = items.reduce((sum, i) => sum + i.value, 0);
        const rects = this.squarify(items, width, height);

        // Namespace colors
        const namespaces = [...new Set(items.map(i => i.namespace))];
        const colors = {
            production: '#3b82f6',
            database: '#22c55e',
            messaging: '#f59e0b',
            system: '#a855f7'
        };

        body.innerHTML = rects.map((r, i) => {
            const item = items[i];
            const color = colors[item.namespace] || '#6b7280';
            const usage = (item.value / item.limit * 100).toFixed(0);
            const isSmall = r.w < 80 || r.h < 50;

            return `
                <div class="treemap-cell ${isSmall ? 'treemap-cell-small' : ''}"
                     style="left:${r.x}px;top:${r.y}px;width:${r.w}px;height:${r.h}px;background:${color}"
                     data-index="${i}">
                    <div class="treemap-cell-content">
                        <div class="treemap-cell-name">${item.name}</div>
                        <div class="treemap-cell-value">${this.formatBytes(item.value)}</div>
                    </div>
                </div>
            `;
        }).join('');

        // Legend
        legend.innerHTML = namespaces.map(ns => `
            <div class="treemap-legend-item">
                <div class="treemap-legend-dot" style="background:${colors[ns] || '#6b7280'}"></div>
                <span>${ns}</span>
            </div>
        `).join('');

        // Tooltip events
        body.querySelectorAll('.treemap-cell').forEach(cell => {
            cell.addEventListener('mouseenter', (e) => {
                const idx = parseInt(cell.dataset.index);
                const item = items[idx];
                const usage = (item.value / item.limit * 100).toFixed(1);

                tooltip.innerHTML = `
                    <div style="font-weight:600;margin-bottom:0.5rem">${item.name}</div>
                    <div>Namespace: ${item.namespace}</div>
                    <div>Usage: ${this.formatBytes(item.value)} / ${this.formatBytes(item.limit)}</div>
                    <div>Utilization: ${usage}%</div>
                `;
                tooltip.style.display = 'block';
            });

            cell.addEventListener('mousemove', (e) => {
                tooltip.style.left = (e.clientX + 10) + 'px';
                tooltip.style.top = (e.clientY + 10) + 'px';
            });

            cell.addEventListener('mouseleave', () => {
                tooltip.style.display = 'none';
            });
        });
    }

    squarify(items, width, height) {
        // Simple slice-and-dice treemap layout
        const total = items.reduce((sum, i) => sum + i.value, 0);
        const rects = [];
        let x = 0, y = 0, w = width, h = height;

        const sorted = [...items].sort((a, b) => b.value - a.value);

        for (let i = 0; i < sorted.length; i++) {
            const ratio = sorted[i].value / (total - items.slice(0, i).reduce((s, it) => s + it.value, 0) || 1);

            if (w > h) {
                const cellW = w * ratio;
                rects.push({ x, y, w: cellW, h });
                x += cellW;
                w -= cellW;
            } else {
                const cellH = h * ratio;
                rects.push({ x, y, w, h: cellH });
                y += cellH;
                h -= cellH;
            }
        }

        return rects;
    }

    formatBytes(mb) {
        if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB';
        return mb + ' MB';
    }
}

customElements.define('resource-treemap', ResourceTreemap);
