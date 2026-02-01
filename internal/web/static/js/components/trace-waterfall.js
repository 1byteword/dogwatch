/**
 * Trace Waterfall Widget
 * Displays a distributed trace as a Gantt-style waterfall chart
 */
class TraceWaterfall extends HTMLElement {
    constructor() {
        super();
        this.trace = null;
        this.selectedSpan = null;
        this.timeScale = 1;
    }

    connectedCallback() {
        this.render();
        this.setupEventListeners();
    }

    static get observedAttributes() {
        return ['trace-id'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'trace-id' && newValue && newValue !== oldValue) {
            this.loadTrace(newValue);
        }
    }

    async loadTrace(traceId) {
        try {
            this.showLoading();
            const resp = await fetch(`/api/traces/${traceId}`);
            if (!resp.ok) throw new Error('Trace not found');
            this.trace = await resp.json();
            this.render();
        } catch (e) {
            this.showError(e.message);
        }
    }

    setTrace(trace) {
        this.trace = trace;
        this.render();
    }

    showLoading() {
        this.innerHTML = `
            <div class="trace-waterfall-loading">
                <div class="spinner"></div>
                <span>Loading trace...</span>
            </div>
        `;
    }

    showError(message) {
        this.innerHTML = `
            <div class="trace-waterfall-error">
                <span class="icon">⚠️</span>
                <span>${message}</span>
            </div>
        `;
    }

    render() {
        if (!this.trace || !this.trace.spans || this.trace.spans.length === 0) {
            this.innerHTML = `
                <div class="trace-waterfall-empty">
                    <span class="icon">🔍</span>
                    <p>Select a trace to view</p>
                </div>
            `;
            return;
        }

        const spans = this.organizeSpans(this.trace.spans);
        const { minTime, maxTime, duration } = this.getTimeRange(spans);

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="trace-waterfall">
                <div class="trace-header">
                    <div class="trace-title">
                        <span class="trace-id">${this.trace.trace_id?.substring(0, 16) || 'Unknown'}...</span>
                        <span class="trace-duration">${this.formatDuration(duration)}</span>
                        <span class="trace-spans">${spans.length} spans</span>
                    </div>
                    <div class="trace-actions">
                        <button class="btn-icon" onclick="this.getRootNode().host.zoomIn()" title="Zoom In">+</button>
                        <button class="btn-icon" onclick="this.getRootNode().host.zoomOut()" title="Zoom Out">−</button>
                        <button class="btn-icon" onclick="this.getRootNode().host.resetZoom()" title="Reset">⟲</button>
                    </div>
                </div>
                <div class="trace-timeline">
                    ${this.renderTimeline(minTime, duration)}
                </div>
                <div class="trace-spans-container">
                    ${spans.map((span, i) => this.renderSpan(span, i, minTime, duration)).join('')}
                </div>
                <div class="span-details" id="span-details" style="display: none;">
                    <div class="span-details-header">
                        <span class="span-details-title">Span Details</span>
                        <button class="btn-close" onclick="this.getRootNode().host.closeDetails()">×</button>
                    </div>
                    <div class="span-details-content" id="span-details-content"></div>
                </div>
            </div>
        `;
    }

    organizeSpans(spans) {
        // Build parent-child relationships and calculate depth
        const spanMap = new Map();
        spans.forEach(s => spanMap.set(s.span_id, { ...s, children: [], depth: 0 }));

        const roots = [];
        spanMap.forEach(span => {
            if (span.parent_span_id && spanMap.has(span.parent_span_id)) {
                spanMap.get(span.parent_span_id).children.push(span);
            } else {
                roots.push(span);
            }
        });

        // Flatten with depth
        const result = [];
        const flatten = (span, depth) => {
            span.depth = depth;
            result.push(span);
            span.children
                .sort((a, b) => new Date(a.start_time) - new Date(b.start_time))
                .forEach(child => flatten(child, depth + 1));
        };

        roots
            .sort((a, b) => new Date(a.start_time) - new Date(b.start_time))
            .forEach(root => flatten(root, 0));

        return result;
    }

    getTimeRange(spans) {
        let minTime = Infinity;
        let maxTime = -Infinity;

        spans.forEach(span => {
            const start = new Date(span.start_time).getTime();
            const end = new Date(span.end_time).getTime();
            minTime = Math.min(minTime, start);
            maxTime = Math.max(maxTime, end);
        });

        return { minTime, maxTime, duration: maxTime - minTime };
    }

    renderTimeline(minTime, duration) {
        const ticks = 5;
        const tickMarks = [];
        for (let i = 0; i <= ticks; i++) {
            const pct = (i / ticks) * 100;
            const time = (duration * i) / ticks;
            tickMarks.push(`
                <div class="timeline-tick" style="left: ${pct}%">
                    <span class="tick-label">${this.formatDuration(time)}</span>
                </div>
            `);
        }
        return `
            <div class="timeline-ruler">
                ${tickMarks.join('')}
            </div>
        `;
    }

    renderSpan(span, index, minTime, duration) {
        const startTime = new Date(span.start_time).getTime();
        const endTime = new Date(span.end_time).getTime();
        const spanDuration = endTime - startTime;

        const leftPct = ((startTime - minTime) / duration) * 100;
        const widthPct = Math.max((spanDuration / duration) * 100, 0.5); // Min width for visibility

        const isError = span.status === 'ERROR' || span.status === 'error';
        const statusClass = isError ? 'span-error' : 'span-ok';
        const serviceColor = this.getServiceColor(span.service_name);

        return `
            <div class="span-row" data-span-index="${index}" onclick="this.getRootNode().host.selectSpan(${index})">
                <div class="span-info" style="padding-left: ${span.depth * 20 + 8}px">
                    <span class="span-service" style="background: ${serviceColor}">${span.service_name || 'unknown'}</span>
                    <span class="span-name">${span.name || 'unnamed'}</span>
                </div>
                <div class="span-bar-container">
                    <div class="span-bar ${statusClass}"
                         style="left: ${leftPct}%; width: ${widthPct}%; background: ${serviceColor};"
                         title="${span.name}: ${this.formatDuration(spanDuration)}">
                        ${widthPct > 8 ? `<span class="span-duration">${this.formatDuration(spanDuration)}</span>` : ''}
                    </div>
                </div>
            </div>
        `;
    }

    selectSpan(index) {
        const spans = this.organizeSpans(this.trace.spans);
        const span = spans[index];
        if (!span) return;

        this.selectedSpan = span;

        // Highlight selected row
        this.querySelectorAll('.span-row').forEach((row, i) => {
            row.classList.toggle('selected', i === index);
        });

        // Show details panel
        const detailsPanel = this.querySelector('#span-details');
        const detailsContent = this.querySelector('#span-details-content');

        detailsPanel.style.display = 'block';
        detailsContent.innerHTML = this.renderSpanDetails(span);
    }

    renderSpanDetails(span) {
        const attrs = span.attributes || {};
        const attrRows = Object.entries(attrs).map(([k, v]) => `
            <tr>
                <td class="attr-key">${this.escapeHtml(k)}</td>
                <td class="attr-value">${this.escapeHtml(String(v))}</td>
            </tr>
        `).join('');

        return `
            <div class="detail-section">
                <div class="detail-row">
                    <span class="detail-label">Service</span>
                    <span class="detail-value">${span.service_name || 'unknown'}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Operation</span>
                    <span class="detail-value">${span.name || 'unnamed'}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Span ID</span>
                    <span class="detail-value mono">${span.span_id}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Duration</span>
                    <span class="detail-value">${this.formatDuration(span.duration_ms)}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Status</span>
                    <span class="detail-value status-${span.status?.toLowerCase() || 'ok'}">${span.status || 'OK'}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Kind</span>
                    <span class="detail-value">${span.kind || 'INTERNAL'}</span>
                </div>
            </div>
            ${Object.keys(attrs).length > 0 ? `
                <div class="detail-section">
                    <h4>Attributes</h4>
                    <table class="attrs-table">
                        <tbody>${attrRows}</tbody>
                    </table>
                </div>
            ` : ''}
        `;
    }

    closeDetails() {
        const detailsPanel = this.querySelector('#span-details');
        if (detailsPanel) {
            detailsPanel.style.display = 'none';
        }
        this.querySelectorAll('.span-row.selected').forEach(row => {
            row.classList.remove('selected');
        });
    }

    zoomIn() {
        this.timeScale = Math.min(this.timeScale * 1.5, 10);
        this.applyZoom();
    }

    zoomOut() {
        this.timeScale = Math.max(this.timeScale / 1.5, 0.5);
        this.applyZoom();
    }

    resetZoom() {
        this.timeScale = 1;
        this.applyZoom();
    }

    applyZoom() {
        const container = this.querySelector('.trace-spans-container');
        if (container) {
            container.style.transform = `scaleX(${this.timeScale})`;
            container.style.transformOrigin = 'left';
        }
    }

    getServiceColor(serviceName) {
        const colors = [
            '#1d9bf0', '#00ba7c', '#f4212e', '#ffd400', '#7856ff',
            '#f91880', '#ff7a00', '#00d4aa', '#794bc4', '#17bf63'
        ];
        if (!serviceName) return colors[0];
        let hash = 0;
        for (let i = 0; i < serviceName.length; i++) {
            hash = serviceName.charCodeAt(i) + ((hash << 5) - hash);
        }
        return colors[Math.abs(hash) % colors.length];
    }

    formatDuration(ms) {
        if (ms === undefined || ms === null) return '—';
        if (ms < 1) return `${(ms * 1000).toFixed(0)}μs`;
        if (ms < 1000) return `${ms.toFixed(1)}ms`;
        return `${(ms / 1000).toFixed(2)}s`;
    }

    escapeHtml(str) {
        if (str === null || str === undefined) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    setupEventListeners() {
        // Keyboard navigation
        this.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                this.closeDetails();
            }
        });
    }

    getStyles() {
        return `
            .trace-waterfall {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                color: var(--text, #e7e9ea);
            }

            .trace-waterfall-loading,
            .trace-waterfall-error,
            .trace-waterfall-empty {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 0.75rem;
                padding: 3rem;
                color: var(--text-muted, #71767b);
                flex-direction: column;
            }

            .trace-waterfall-error { color: var(--error, #f4212e); }

            .spinner {
                width: 24px;
                height: 24px;
                border: 3px solid var(--border, #2f3336);
                border-top-color: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: spin 0.8s linear infinite;
            }

            @keyframes spin { to { transform: rotate(360deg); } }

            .trace-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .trace-title {
                display: flex;
                align-items: center;
                gap: 1rem;
            }

            .trace-id {
                font-family: monospace;
                font-size: 0.85rem;
                color: var(--accent, #1d9bf0);
            }

            .trace-duration {
                font-weight: 600;
                font-size: 0.9rem;
            }

            .trace-spans {
                color: var(--text-muted, #71767b);
                font-size: 0.8rem;
            }

            .trace-actions {
                display: flex;
                gap: 0.25rem;
            }

            .btn-icon {
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
                font-size: 1rem;
            }

            .btn-icon:hover {
                border-color: var(--accent, #1d9bf0);
                color: var(--accent, #1d9bf0);
            }

            .trace-timeline {
                position: relative;
                height: 24px;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .timeline-ruler {
                position: relative;
                height: 100%;
                margin-left: 200px;
            }

            .timeline-tick {
                position: absolute;
                top: 0;
                bottom: 0;
                border-left: 1px solid var(--border, #2f3336);
            }

            .tick-label {
                position: absolute;
                top: 4px;
                left: 4px;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                white-space: nowrap;
            }

            .trace-spans-container {
                max-height: 400px;
                overflow-y: auto;
                transition: transform 0.2s ease;
            }

            .span-row {
                display: flex;
                align-items: center;
                height: 32px;
                border-bottom: 1px solid var(--border, #2f3336);
                cursor: pointer;
                transition: background 0.15s;
            }

            .span-row:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .span-row.selected {
                background: rgba(29, 155, 240, 0.1);
            }

            .span-info {
                width: 200px;
                min-width: 200px;
                display: flex;
                align-items: center;
                gap: 0.5rem;
                padding-right: 0.5rem;
                overflow: hidden;
            }

            .span-service {
                font-size: 0.65rem;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                color: white;
                white-space: nowrap;
                flex-shrink: 0;
            }

            .span-name {
                font-size: 0.8rem;
                overflow: hidden;
                text-overflow: ellipsis;
                white-space: nowrap;
            }

            .span-bar-container {
                flex: 1;
                position: relative;
                height: 100%;
            }

            .span-bar {
                position: absolute;
                top: 6px;
                height: 20px;
                border-radius: 3px;
                display: flex;
                align-items: center;
                justify-content: flex-end;
                padding-right: 4px;
                min-width: 3px;
                opacity: 0.85;
            }

            .span-bar:hover {
                opacity: 1;
            }

            .span-bar.span-error {
                background: var(--error, #f4212e) !important;
            }

            .span-duration {
                font-size: 0.65rem;
                color: white;
                text-shadow: 0 1px 2px rgba(0,0,0,0.5);
            }

            .span-details {
                position: absolute;
                right: 0;
                top: 0;
                bottom: 0;
                width: 350px;
                background: var(--bg-card, #16181c);
                border-left: 1px solid var(--border, #2f3336);
                overflow-y: auto;
                z-index: 10;
            }

            .span-details-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
                position: sticky;
                top: 0;
            }

            .span-details-title {
                font-weight: 600;
                font-size: 0.9rem;
            }

            .btn-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.25rem;
                cursor: pointer;
                padding: 0;
                line-height: 1;
            }

            .btn-close:hover {
                color: var(--text, #e7e9ea);
            }

            .span-details-content {
                padding: 1rem;
            }

            .detail-section {
                margin-bottom: 1.5rem;
            }

            .detail-section h4 {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.75rem;
                text-transform: uppercase;
                letter-spacing: 0.5px;
            }

            .detail-row {
                display: flex;
                justify-content: space-between;
                padding: 0.4rem 0;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .detail-label {
                color: var(--text-muted, #71767b);
                font-size: 0.8rem;
            }

            .detail-value {
                font-size: 0.85rem;
                text-align: right;
            }

            .detail-value.mono {
                font-family: monospace;
                font-size: 0.75rem;
            }

            .detail-value.status-error {
                color: var(--error, #f4212e);
            }

            .detail-value.status-ok {
                color: var(--success, #00ba7c);
            }

            .attrs-table {
                width: 100%;
                font-size: 0.8rem;
                border-collapse: collapse;
            }

            .attrs-table tr {
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .attrs-table td {
                padding: 0.4rem 0;
            }

            .attr-key {
                color: var(--text-muted, #71767b);
                width: 40%;
            }

            .attr-value {
                font-family: monospace;
                word-break: break-all;
            }
        `;
    }
}

customElements.define('trace-waterfall', TraceWaterfall);

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = TraceWaterfall;
}
