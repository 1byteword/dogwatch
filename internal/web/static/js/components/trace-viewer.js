/**
 * Trace Viewer Web Component
 *
 * Usage:
 *   <dw-trace-viewer></dw-trace-viewer>
 *   <dw-trace-viewer trace-id="abc123"></dw-trace-viewer>
 */
class TraceViewer extends HTMLElement {
    static get observedAttributes() {
        return ['trace-id'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.traces = [];
        this.selectedTrace = null;
        this.selectedSpan = null;
        this._unsubscribe = null;
    }

    connectedCallback() {
        this.render();
        this.setupWebSocket();
        this.fetchTraces();
    }

    disconnectedCallback() {
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'trace-id' && newValue) {
            this.loadTrace(newValue);
        }
    }

    render() {
        this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    background: #0f1419;
                    color: #e7e9ea;
                }
                .trace-list {
                    max-height: 200px;
                    overflow-y: auto;
                    border-bottom: 1px solid #2f3336;
                }
                .trace-item {
                    padding: 0.6rem 0.8rem;
                    border-bottom: 1px solid #2f3336;
                    cursor: pointer;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    transition: background 0.15s;
                }
                .trace-item:hover { background: rgba(29, 155, 240, 0.1); }
                .trace-item.selected {
                    background: linear-gradient(90deg, rgba(29, 155, 240, 0.2), rgba(29, 155, 240, 0.1));
                    border-left: 3px solid #1d9bf0;
                }
                .trace-name { font-weight: 500; font-size: 0.75rem; }
                .trace-service { color: #71767b; font-size: 0.65rem; }
                .trace-duration { font-family: 'SF Mono', Consolas, monospace; font-size: 0.7rem; }
                .trace-status {
                    display: inline-block;
                    width: 8px;
                    height: 8px;
                    border-radius: 50%;
                    margin-right: 0.5rem;
                }
                .trace-status.ok { background: #00ba7c; box-shadow: 0 0 6px #00ba7c; }
                .trace-status.error { background: #f4212e; box-shadow: 0 0 6px #f4212e; }
                .waterfall { flex: 1; overflow: auto; }
                .waterfall-header {
                    display: flex;
                    font-size: 0.65rem;
                    color: #536471;
                    padding: 8px 12px;
                    background: rgba(47, 51, 54, 0.5);
                    border-bottom: 1px solid #2f3336;
                    position: sticky;
                    top: 0;
                    z-index: 5;
                }
                .waterfall-header-op { width: 240px; flex-shrink: 0; }
                .waterfall-header-timeline { flex: 1; display: flex; justify-content: space-between; padding: 0 8px; }
                .span-row {
                    display: flex;
                    align-items: center;
                    padding: 4px 12px;
                    cursor: pointer;
                    border-left: 3px solid transparent;
                }
                .span-row:hover { background: rgba(29, 155, 240, 0.08); }
                .span-row.selected { background: rgba(29, 155, 240, 0.15); border-left-color: #1d9bf0; }
                .span-info { width: 240px; flex-shrink: 0; overflow: hidden; }
                .span-name {
                    font-size: 0.75rem;
                    font-weight: 500;
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .span-service {
                    font-size: 0.65rem;
                    padding: 1px 5px;
                    border-radius: 3px;
                    background: rgba(255, 255, 255, 0.05);
                }
                .span-timeline { flex: 1; position: relative; height: 24px; margin: 0 8px; }
                .span-bar {
                    position: absolute;
                    height: 16px;
                    top: 4px;
                    border-radius: 4px;
                    min-width: 4px;
                    transition: all 0.15s;
                }
                .span-bar:hover { transform: scaleY(1.2); }
                .span-bar.error { box-shadow: 0 0 8px rgba(244, 33, 46, 0.5); }
                .span-duration {
                    position: absolute;
                    right: 8px;
                    font-size: 0.65rem;
                    font-family: 'SF Mono', Consolas, monospace;
                    color: #8b949e;
                }
                .empty {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: #71767b;
                    font-size: 0.85rem;
                }
                .service-colors {
                    --svc-0: #1d9bf0;
                    --svc-1: #00ba7c;
                    --svc-2: #ffd400;
                    --svc-3: #7c3aed;
                    --svc-4: #f97316;
                    --svc-5: #06b6d4;
                }
            </style>
            <div class="service-colors"></div>
            <div class="trace-list" id="trace-list"></div>
            <div class="waterfall" id="waterfall">
                <div class="empty">Select a trace to view details</div>
            </div>
        `;
    }

    setupWebSocket() {
        if (window.dwSocket) {
            this._unsubscribe = window.dwSocket.subscribe('traces', (msg) => {
                if (msg.type === 'new' && msg.payload) {
                    this.addTrace(msg.payload);
                }
            });
        }
    }

    async fetchTraces() {
        try {
            const response = await fetch('/api/traces?limit=20');
            if (response.ok) {
                const data = await response.json();
                this.traces = data.traces || [];
                this.renderTraceList();
            } else {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
        } catch (e) {
            console.error('[TraceViewer] Failed to fetch traces:', e);
            this._showError('Failed to load traces', e.message);
        }
    }

    _showError(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'error', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.error(message, title);
        }
    }

    addTrace(trace) {
        this.traces.unshift(trace);
        if (this.traces.length > 50) {
            this.traces.pop();
        }
        this.renderTraceList();
    }

    renderTraceList() {
        const list = this.shadowRoot.getElementById('trace-list');
        if (!list) return;

        list.innerHTML = this.traces.map((trace, i) => `
            <div class="trace-item ${this.selectedTrace?.id === trace.id ? 'selected' : ''}"
                 data-index="${i}">
                <div>
                    <span class="trace-status ${trace.status === 'error' ? 'error' : 'ok'}"></span>
                    <span class="trace-name">${trace.name || trace.operationName || 'Unknown'}</span>
                    <div class="trace-service">${trace.service || trace.serviceName || ''}</div>
                </div>
                <div class="trace-meta">
                    <span class="trace-duration">${this.formatDuration(trace.duration)}</span>
                </div>
            </div>
        `).join('');

        // Add click handlers
        list.querySelectorAll('.trace-item').forEach(item => {
            item.addEventListener('click', () => {
                const index = parseInt(item.dataset.index);
                this.selectTrace(this.traces[index]);
            });
        });
    }

    selectTrace(trace) {
        this.selectedTrace = trace;
        this.renderTraceList();
        this.renderWaterfall(trace);
    }

    async loadTrace(traceId) {
        try {
            const response = await fetch(`/api/traces/${traceId}`);
            if (response.ok) {
                const trace = await response.json();
                this.selectTrace(trace);
            } else {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
        } catch (e) {
            console.error('[TraceViewer] Failed to load trace:', e);
            this._showError('Failed to load trace', e.message);
        }
    }

    renderWaterfall(trace) {
        const waterfall = this.shadowRoot.getElementById('waterfall');
        if (!trace || !trace.spans || trace.spans.length === 0) {
            waterfall.innerHTML = '<div class="empty">No spans in this trace</div>';
            return;
        }

        const spans = trace.spans;
        const minTime = Math.min(...spans.map(s => s.startTime || 0));
        const maxTime = Math.max(...spans.map(s => (s.startTime || 0) + (s.duration || 0)));
        const totalDuration = maxTime - minTime;

        // Service colors
        const services = [...new Set(spans.map(s => s.service || s.serviceName))];
        const colors = ['#1d9bf0', '#00ba7c', '#ffd400', '#7c3aed', '#f97316', '#06b6d4'];

        waterfall.innerHTML = `
            <div class="waterfall-header">
                <div class="waterfall-header-op">Operation</div>
                <div class="waterfall-header-timeline">
                    <span>0ms</span>
                    <span>${this.formatDuration(totalDuration / 1000000)}</span>
                </div>
            </div>
            <div class="waterfall-body">
                ${spans.map((span, i) => {
                    const start = ((span.startTime || 0) - minTime) / totalDuration * 100;
                    const width = Math.max((span.duration || 0) / totalDuration * 100, 0.5);
                    const serviceIndex = services.indexOf(span.service || span.serviceName);
                    const color = colors[serviceIndex % colors.length];

                    return `
                        <div class="span-row" data-index="${i}">
                            <div class="span-info">
                                <div class="span-name">${span.operationName || span.name || 'Unknown'}</div>
                                <span class="span-service" style="color: ${color}">${span.service || span.serviceName || ''}</span>
                            </div>
                            <div class="span-timeline">
                                <div class="span-bar ${span.status === 'error' ? 'error' : ''}"
                                     style="left: ${start}%; width: ${width}%; background: ${color};">
                                </div>
                                <span class="span-duration">${this.formatDuration(span.duration / 1000000)}</span>
                            </div>
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    }

    formatDuration(ms) {
        if (ms < 1) return `${(ms * 1000).toFixed(0)}µs`;
        if (ms < 1000) return `${ms.toFixed(1)}ms`;
        return `${(ms / 1000).toFixed(2)}s`;
    }

    // Public API
    refresh() {
        this.fetchTraces();
    }
}

customElements.define('dw-trace-viewer', TraceViewer);
