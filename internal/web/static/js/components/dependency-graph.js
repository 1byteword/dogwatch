/**
 * Dependency Graph Widget
 * Interactive force-directed graph of service relationships
 */
class DependencyGraph extends HTMLElement {
    constructor() {
        super();
        this.nodes = [];
        this.links = [];
        this.simulation = null;
        this.svg = null;
        this.selectedNode = null;
        this._mounted = false;
        this._animationFrame = null;
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadDependencies();
    }

    disconnectedCallback() {
        this._mounted = false;
        if (this.simulation) {
            this.simulation.stop();
            this.simulation = null;
        }
        if (this._animationFrame) {
            cancelAnimationFrame(this._animationFrame);
            this._animationFrame = null;
        }
    }

    async loadDependencies() {
        try {
            const resp = await fetch('/api/trace/dependencies');
            if (resp.ok) {
                const data = await resp.json();
                this.processDependencies(data);
                this.renderGraph();
            }
        } catch (e) {
            console.error('Failed to load dependencies:', e);
            this.showError('Failed to load dependency data');
        }
    }

    processDependencies(data) {
        // Build nodes and links from dependency data
        const nodeMap = new Map();
        const links = [];

        if (Array.isArray(data)) {
            data.forEach(dep => {
                const parent = dep.parent || dep.source || dep.from;
                const child = dep.child || dep.target || dep.to;
                const count = dep.call_count || dep.count || 1;

                if (parent && !nodeMap.has(parent)) {
                    nodeMap.set(parent, { id: parent, name: parent, calls: 0, errors: 0 });
                }
                if (child && !nodeMap.has(child)) {
                    nodeMap.set(child, { id: child, name: child, calls: 0, errors: 0 });
                }

                if (parent && child) {
                    nodeMap.get(parent).calls += count;
                    links.push({
                        source: parent,
                        target: child,
                        value: count,
                        errorRate: dep.error_rate || 0
                    });
                }
            });
        }

        this.nodes = Array.from(nodeMap.values());
        this.links = links;
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="dependency-graph">
                <div class="graph-header">
                    <div class="graph-title">
                        <span class="title-icon">🕸️</span>
                        <span>Service Dependencies</span>
                    </div>
                    <div class="graph-controls">
                        <button class="btn-control" onclick="this.getRootNode().host.resetZoom()" title="Reset view">⟲</button>
                        <button class="btn-control" onclick="this.getRootNode().host.loadDependencies()" title="Refresh">↻</button>
                    </div>
                </div>
                <div class="graph-container" id="graph-container">
                    <div class="loading">Loading dependencies...</div>
                </div>
                <div class="graph-legend">
                    <div class="legend-item">
                        <span class="legend-dot healthy"></span>
                        <span>Healthy</span>
                    </div>
                    <div class="legend-item">
                        <span class="legend-dot warning"></span>
                        <span>Degraded</span>
                    </div>
                    <div class="legend-item">
                        <span class="legend-dot error"></span>
                        <span>Errors</span>
                    </div>
                </div>
                <div class="node-details" id="node-details" style="display: none;"></div>
            </div>
        `;
    }

    renderGraph() {
        const container = this.querySelector('#graph-container');
        if (!container) return;

        if (this.nodes.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <span class="icon">🕸️</span>
                    <p>No dependency data available</p>
                    <p class="hint">Send traces with parent-child relationships to see the graph</p>
                </div>
            `;
            return;
        }

        container.innerHTML = '';

        const width = container.clientWidth || 600;
        const height = container.clientHeight || 400;

        // Create SVG
        const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('width', '100%');
        svg.setAttribute('height', '100%');
        svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
        container.appendChild(svg);

        this.svg = svg;

        // Create defs for arrow markers
        const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
        defs.innerHTML = `
            <marker id="arrowhead" viewBox="0 -5 10 10" refX="20" refY="0" markerWidth="6" markerHeight="6" orient="auto">
                <path d="M0,-5L10,0L0,5" fill="#71767b"/>
            </marker>
        `;
        svg.appendChild(defs);

        // Create groups for links and nodes
        const linkGroup = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        linkGroup.setAttribute('class', 'links');
        svg.appendChild(linkGroup);

        const nodeGroup = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        nodeGroup.setAttribute('class', 'nodes');
        svg.appendChild(nodeGroup);

        // Simple force simulation (no D3 required)
        this.simulateForces(width, height);

        // Render links
        this.links.forEach(link => {
            const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
            line.setAttribute('class', 'link');
            line.setAttribute('marker-end', 'url(#arrowhead)');
            line.setAttribute('stroke-width', Math.min(Math.max(link.value / 100, 1), 5));
            if (link.errorRate > 0.05) {
                line.setAttribute('class', 'link error');
            }
            line.dataset.source = link.source;
            line.dataset.target = link.target;
            linkGroup.appendChild(line);
        });

        // Render nodes
        this.nodes.forEach(node => {
            const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
            g.setAttribute('class', 'node');
            g.setAttribute('data-id', node.id);

            const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
            circle.setAttribute('r', Math.min(20 + node.calls / 50, 40));
            circle.setAttribute('class', this.getNodeClass(node));

            const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
            text.setAttribute('dy', '0.35em');
            text.setAttribute('text-anchor', 'middle');
            text.textContent = node.name.length > 12 ? node.name.substring(0, 12) + '...' : node.name;

            g.appendChild(circle);
            g.appendChild(text);

            g.addEventListener('click', () => this.selectNode(node));
            g.addEventListener('mouseenter', () => this.highlightConnections(node));
            g.addEventListener('mouseleave', () => this.clearHighlight());

            nodeGroup.appendChild(g);
        });

        // Start animation
        this.animate();
    }

    simulateForces(width, height) {
        // Initialize positions
        this.nodes.forEach((node, i) => {
            node.x = width / 2 + (Math.random() - 0.5) * width * 0.5;
            node.y = height / 2 + (Math.random() - 0.5) * height * 0.5;
            node.vx = 0;
            node.vy = 0;
        });

        // Create node lookup
        const nodeById = new Map(this.nodes.map(n => [n.id, n]));

        // Link sources and targets
        this.links.forEach(link => {
            link.sourceNode = nodeById.get(link.source);
            link.targetNode = nodeById.get(link.target);
        });

        this.width = width;
        this.height = height;
        this.alpha = 1;
    }

    animate() {
        // Stop animation if component is unmounted or alpha is too low
        if (!this._mounted || this.alpha < 0.001) {
            this._animationFrame = null;
            return;
        }

        this.alpha *= 0.99;

        // Apply forces
        this.applyForces();

        // Update positions in SVG
        this.updatePositions();

        this._animationFrame = requestAnimationFrame(() => this.animate());
    }

    applyForces() {
        const centerX = this.width / 2;
        const centerY = this.height / 2;

        // Center force
        this.nodes.forEach(node => {
            node.vx += (centerX - node.x) * 0.01 * this.alpha;
            node.vy += (centerY - node.y) * 0.01 * this.alpha;
        });

        // Repulsion between nodes
        for (let i = 0; i < this.nodes.length; i++) {
            for (let j = i + 1; j < this.nodes.length; j++) {
                const a = this.nodes[i];
                const b = this.nodes[j];
                const dx = b.x - a.x;
                const dy = b.y - a.y;
                const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                const force = 1000 / (dist * dist) * this.alpha;

                a.vx -= dx / dist * force;
                a.vy -= dy / dist * force;
                b.vx += dx / dist * force;
                b.vy += dy / dist * force;
            }
        }

        // Link attraction
        this.links.forEach(link => {
            if (!link.sourceNode || !link.targetNode) return;
            const dx = link.targetNode.x - link.sourceNode.x;
            const dy = link.targetNode.y - link.sourceNode.y;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const force = (dist - 150) * 0.01 * this.alpha;

            link.sourceNode.vx += dx / dist * force;
            link.sourceNode.vy += dy / dist * force;
            link.targetNode.vx -= dx / dist * force;
            link.targetNode.vy -= dy / dist * force;
        });

        // Apply velocity with damping
        this.nodes.forEach(node => {
            node.vx *= 0.9;
            node.vy *= 0.9;
            node.x += node.vx;
            node.y += node.vy;

            // Bounds
            node.x = Math.max(50, Math.min(this.width - 50, node.x));
            node.y = Math.max(50, Math.min(this.height - 50, node.y));
        });
    }

    updatePositions() {
        // Update node positions
        this.querySelectorAll('.node').forEach(g => {
            const node = this.nodes.find(n => n.id === g.dataset.id);
            if (node) {
                g.setAttribute('transform', `translate(${node.x}, ${node.y})`);
            }
        });

        // Update link positions
        this.querySelectorAll('.link').forEach(line => {
            const source = this.nodes.find(n => n.id === line.dataset.source);
            const target = this.nodes.find(n => n.id === line.dataset.target);
            if (source && target) {
                line.setAttribute('x1', source.x);
                line.setAttribute('y1', source.y);
                line.setAttribute('x2', target.x);
                line.setAttribute('y2', target.y);
            }
        });
    }

    getNodeClass(node) {
        if (node.errors > 0) return 'node-circle error';
        if (node.calls > 1000) return 'node-circle warning';
        return 'node-circle healthy';
    }

    selectNode(node) {
        this.selectedNode = node;
        const details = this.querySelector('#node-details');

        const incoming = this.links.filter(l => l.target === node.id);
        const outgoing = this.links.filter(l => l.source === node.id);

        details.style.display = 'block';
        details.innerHTML = `
            <div class="details-header">
                <span class="details-title">${this.escapeHtml(node.name)}</span>
                <button class="btn-close" onclick="this.parentElement.parentElement.style.display='none'">×</button>
            </div>
            <div class="details-body">
                <div class="detail-row">
                    <span class="detail-label">Total Calls</span>
                    <span class="detail-value">${node.calls}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Incoming</span>
                    <span class="detail-value">${incoming.length} services</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Outgoing</span>
                    <span class="detail-value">${outgoing.length} services</span>
                </div>
                ${outgoing.length > 0 ? `
                    <div class="detail-section">
                        <span class="section-title">Depends On</span>
                        ${outgoing.map(l => `<span class="dep-tag">${this.escapeHtml(l.target)}</span>`).join('')}
                    </div>
                ` : ''}
                ${incoming.length > 0 ? `
                    <div class="detail-section">
                        <span class="section-title">Called By</span>
                        ${incoming.map(l => `<span class="dep-tag">${this.escapeHtml(l.source)}</span>`).join('')}
                    </div>
                ` : ''}
            </div>
            <div class="details-actions">
                <a href="/traces.html?service=${encodeURIComponent(node.name)}" class="btn-link">View Traces</a>
            </div>
        `;
    }

    highlightConnections(node) {
        const connectedIds = new Set([node.id]);
        this.links.forEach(l => {
            if (l.source === node.id) connectedIds.add(l.target);
            if (l.target === node.id) connectedIds.add(l.source);
        });

        this.querySelectorAll('.node').forEach(g => {
            g.classList.toggle('dimmed', !connectedIds.has(g.dataset.id));
        });

        this.querySelectorAll('.link').forEach(line => {
            const connected = line.dataset.source === node.id || line.dataset.target === node.id;
            line.classList.toggle('dimmed', !connected);
            line.classList.toggle('highlighted', connected);
        });
    }

    clearHighlight() {
        this.querySelectorAll('.node, .link').forEach(el => {
            el.classList.remove('dimmed', 'highlighted');
        });
    }

    resetZoom() {
        this.alpha = 1;
        this.animate();
    }

    showError(message) {
        const container = this.querySelector('#graph-container');
        if (container) {
            container.innerHTML = `<div class="error">${message}</div>`;
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .dependency-graph {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
                position: relative;
            }

            .graph-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .graph-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .graph-controls { display: flex; gap: 0.25rem; }

            .btn-control {
                width: 28px;
                height: 28px;
                display: flex;
                align-items: center;
                justify-content: center;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                cursor: pointer;
            }

            .btn-control:hover { border-color: var(--accent, #1d9bf0); }

            .graph-container {
                flex: 1;
                overflow: hidden;
            }

            .loading, .empty-state, .error {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
            }

            .empty-state .icon { font-size: 2rem; margin-bottom: 0.5rem; }
            .empty-state .hint { font-size: 0.8rem; }

            .graph-legend {
                display: flex;
                gap: 1rem;
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-top: 1px solid var(--border, #2f3336);
                font-size: 0.75rem;
            }

            .legend-item { display: flex; align-items: center; gap: 0.4rem; }

            .legend-dot {
                width: 10px;
                height: 10px;
                border-radius: 50%;
            }

            .legend-dot.healthy { background: var(--success, #00ba7c); }
            .legend-dot.warning { background: var(--warning, #ffd400); }
            .legend-dot.error { background: var(--error, #f4212e); }

            /* SVG Styles */
            .link {
                stroke: var(--border, #2f3336);
                stroke-opacity: 0.6;
                fill: none;
            }

            .link.error { stroke: var(--error, #f4212e); }
            .link.dimmed { stroke-opacity: 0.1; }
            .link.highlighted { stroke-opacity: 1; stroke-width: 3px !important; }

            .node { cursor: pointer; }
            .node.dimmed { opacity: 0.2; }

            .node-circle {
                fill: var(--success, #00ba7c);
                stroke: var(--bg-card, #16181c);
                stroke-width: 2px;
            }

            .node-circle.warning { fill: var(--warning, #ffd400); }
            .node-circle.error { fill: var(--error, #f4212e); }

            .node text {
                fill: var(--text, #e7e9ea);
                font-size: 10px;
                pointer-events: none;
            }

            .node-details {
                position: absolute;
                top: 60px;
                right: 10px;
                width: 250px;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                z-index: 10;
                box-shadow: 0 4px 12px rgba(0,0,0,0.3);
            }

            .details-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .details-title { font-weight: 600; }

            .btn-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                cursor: pointer;
                font-size: 1.25rem;
            }

            .details-body { padding: 0.75rem; }

            .detail-row {
                display: flex;
                justify-content: space-between;
                padding: 0.3rem 0;
                font-size: 0.85rem;
            }

            .detail-label { color: var(--text-muted, #71767b); }

            .detail-section {
                margin-top: 0.75rem;
                padding-top: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .section-title {
                display: block;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .dep-tag {
                display: inline-block;
                background: var(--bg-elevated, #1e2128);
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.75rem;
                margin: 0.2rem;
            }

            .details-actions {
                padding: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .btn-link {
                color: var(--accent, #1d9bf0);
                text-decoration: none;
                font-size: 0.85rem;
            }
        `;
    }
}

customElements.define('dependency-graph', DependencyGraph);
