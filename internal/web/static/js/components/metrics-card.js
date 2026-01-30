/**
 * Metrics Card Web Component
 *
 * Usage:
 *   <dw-metrics title="CPU" value="45" unit="%" type="cpu"></dw-metrics>
 *   <dw-metrics title="Memory" value="8.2" unit="GB" max="16" type="mem"></dw-metrics>
 *   <dw-metrics title="Requests" value="1.2K" unit="/s"></dw-metrics>
 *
 * Attributes:
 *   - title: Label for the metric
 *   - value: Current value
 *   - unit: Unit string (%, GB, /s, etc.)
 *   - max: Maximum value (for progress bar)
 *   - type: cpu|mem|disk|net (determines bar color)
 *   - detail: Additional detail text
 */
class MetricsCard extends HTMLElement {
    static get observedAttributes() {
        return ['title', 'value', 'unit', 'max', 'type', 'detail'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this._unsubscribe = null;
    }

    connectedCallback() {
        this.render();
        this.setupWebSocket();
    }

    disconnectedCallback() {
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
    }

    attributeChangedCallback() {
        this.render();
    }

    setupWebSocket() {
        // Subscribe to system stats if we have a metric-id
        const metricId = this.getAttribute('metric-id');
        if (metricId && window.dwSocket) {
            this._unsubscribe = window.dwSocket.subscribe('system', (msg) => {
                if (msg.payload && msg.payload[metricId] !== undefined) {
                    this.setValue(msg.payload[metricId]);
                }
            });
        }
    }

    render() {
        const title = this.getAttribute('title') || 'Metric';
        const value = this.getAttribute('value') || '0';
        const unit = this.getAttribute('unit') || '';
        const max = parseFloat(this.getAttribute('max')) || 100;
        const type = this.getAttribute('type') || '';
        const detail = this.getAttribute('detail') || '';

        const numValue = parseFloat(value) || 0;
        const percentage = max > 0 ? Math.min((numValue / max) * 100, 100) : 0;

        // Color gradients based on type
        const gradients = {
            cpu: 'linear-gradient(90deg, #1d9bf0, #1d4ed8)',
            mem: 'linear-gradient(90deg, #00ba7c, #059669)',
            disk: 'linear-gradient(90deg, #f4212e, #dc2626)',
            net: 'linear-gradient(90deg, #7c3aed, #6d28d9)',
            default: 'linear-gradient(90deg, #1d9bf0, #1d4ed8)'
        };
        const gradient = gradients[type] || gradients.default;

        this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: block;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                }
                .card {
                    padding: 0.5rem;
                }
                .title {
                    font-size: 0.65rem;
                    color: #71767b;
                    text-transform: uppercase;
                    letter-spacing: 0.5px;
                    margin-bottom: 0.25rem;
                }
                .value-row {
                    display: flex;
                    align-items: baseline;
                    gap: 0.25rem;
                }
                .value {
                    font-size: 2rem;
                    font-weight: 700;
                    color: #e7e9ea;
                    line-height: 1;
                }
                .unit {
                    font-size: 0.8rem;
                    color: #71767b;
                }
                .bar {
                    height: 6px;
                    background: #2f3336;
                    border-radius: 3px;
                    margin-top: 0.5rem;
                    overflow: hidden;
                }
                .bar-fill {
                    height: 100%;
                    border-radius: 3px;
                    background: ${gradient};
                    transition: width 0.3s ease;
                    width: ${percentage}%;
                }
                .detail {
                    font-size: 0.7rem;
                    color: #71767b;
                    margin-top: 0.4rem;
                    display: flex;
                    justify-content: space-between;
                }
            </style>
            <div class="card">
                <div class="title">${title}</div>
                <div class="value-row">
                    <span class="value">${value}</span>
                    <span class="unit">${unit}</span>
                </div>
                ${max ? `<div class="bar"><div class="bar-fill"></div></div>` : ''}
                ${detail ? `<div class="detail">${detail}</div>` : ''}
            </div>
        `;
    }

    // Public API
    setValue(value) {
        this.setAttribute('value', value);
    }

    getValue() {
        return parseFloat(this.getAttribute('value')) || 0;
    }

    setMax(max) {
        this.setAttribute('max', max);
    }

    setDetail(detail) {
        this.setAttribute('detail', detail);
    }
}

customElements.define('dw-metrics', MetricsCard);
