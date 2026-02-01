/**
 * Metric Sparkline Widget
 * Compact inline chart for displaying metric trends
 *
 * Optimizations:
 * - CSS parsed once via adoptedStyleSheets (or cached <style> fallback)
 * - Selective DOM updates instead of full innerHTML replacement
 */

// Static stylesheet - parsed once, shared across all instances
const sparklineStyles = new CSSStyleSheet();
sparklineStyles.replaceSync(`
    :host {
        display: inline-block;
        min-width: 100px;
    }
    .sparkline-container {
        display: flex;
        align-items: center;
        gap: 0.5rem;
    }
    .sparkline-label {
        font-size: 0.7rem;
        color: var(--text-muted, #71767b);
        white-space: nowrap;
        max-width: 80px;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .sparkline-chart { flex-shrink: 0; }
    .sparkline-chart svg { display: block; }
    .sparkline-value {
        display: flex;
        align-items: center;
        gap: 0.25rem;
    }
    .value {
        font-size: 0.85rem;
        font-weight: 600;
    }
    .trend { font-size: 0.75rem; }
    .trend-up { color: var(--success, #00ba7c); }
    .trend-down { color: var(--error, #f4212e); }
    .trend-flat { color: var(--text-muted, #71767b); }
    .loading-shimmer {
        width: 100%;
        height: 100%;
        background: linear-gradient(90deg,
            var(--bg-elevated, #1e2128) 25%,
            var(--bg-card, #16181c) 50%,
            var(--bg-elevated, #1e2128) 75%);
        background-size: 200% 100%;
        animation: shimmer 1.5s infinite;
        border-radius: 4px;
    }
    @keyframes shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }
    .no-data {
        font-size: 0.7rem;
        color: var(--text-muted, #71767b);
        width: 100%;
        text-align: center;
    }
`);

class MetricSparkline extends HTMLElement {
    static get observedAttributes() {
        return ['metric', 'period', 'color', 'height', 'show-value', 'show-label'];
    }

    constructor() {
        super();
        this.data = [];
        this.loading = true;
        this._initialized = false;

        // Create shadow root with shared stylesheet
        this.attachShadow({ mode: 'open' });
        this.shadowRoot.adoptedStyleSheets = [sparklineStyles];
    }

    connectedCallback() {
        this._initDOM();
        this.loadData();
    }

    attributeChangedCallback() {
        if (this.isConnected) {
            this.loadData();
        }
    }

    get metric() { return this.getAttribute('metric') || ''; }
    get period() { return this.getAttribute('period') || '1h'; }
    get color() { return this.getAttribute('color') || 'var(--accent, #1d9bf0)'; }
    get height() { return parseInt(this.getAttribute('height') || '40'); }
    get showValue() { return this.hasAttribute('show-value'); }
    get showLabel() { return this.hasAttribute('show-label'); }

    // Create DOM structure once
    _initDOM() {
        if (this._initialized) return;
        this._initialized = true;

        const container = document.createElement('div');
        container.className = 'sparkline-container';
        container.style.height = `${this.height}px`;

        // Label (conditionally shown)
        this._label = document.createElement('div');
        this._label.className = 'sparkline-label';
        this._label.style.display = this.showLabel ? '' : 'none';
        container.appendChild(this._label);

        // Chart container
        this._chart = document.createElement('div');
        this._chart.className = 'sparkline-chart';
        container.appendChild(this._chart);

        // Value display (conditionally shown)
        this._valueContainer = document.createElement('div');
        this._valueContainer.className = 'sparkline-value';
        this._valueContainer.style.display = this.showValue ? '' : 'none';
        this._valueContainer.innerHTML = '<span class="value"></span><span class="trend"></span>';
        container.appendChild(this._valueContainer);

        this.shadowRoot.appendChild(container);
    }

    async loadData() {
        if (!this.metric) {
            this.loading = false;
            this._renderState();
            return;
        }

        this.loading = true;
        this._renderState();

        try {
            const resp = await fetch(`/api/metrics/query?metric=${encodeURIComponent(this.metric)}&period=${this.period}&points=50`);
            if (resp.ok) {
                const result = await resp.json();
                this.data = result.values || result.data || [];
            }
        } catch (e) {
            console.error('Failed to load sparkline data:', e);
        } finally {
            this.loading = false;
            this._renderState();
        }
    }

    // Update only what changed
    _renderState() {
        if (!this._initialized) return;

        if (this.showLabel) {
            this._label.textContent = this.metric;
            this._label.style.display = '';
        } else {
            this._label.style.display = 'none';
        }

        if (this.loading) {
            this._chart.innerHTML = `<div class="loading-shimmer" style="width:100px;height:${this.height}px"></div>`;
            this._valueContainer.style.display = 'none';
            return;
        }

        if (this.data.length === 0) {
            this._chart.innerHTML = '<div class="no-data">No data</div>';
            this._valueContainer.style.display = 'none';
            return;
        }

        // Render chart
        const width = this.clientWidth || 150;
        const height = this.height;
        const values = this.data.map(d => typeof d === 'number' ? d : (d.value || d.v || 0));
        const currentValue = values[values.length - 1];
        const minValue = Math.min(...values);
        const maxValue = Math.max(...values);
        const range = maxValue - minValue || 1;

        // Calculate trend
        const mid = Math.floor(values.length / 2);
        const firstAvg = values.slice(0, mid).reduce((a, b) => a + b, 0) / mid;
        const secondAvg = values.slice(mid).reduce((a, b) => a + b, 0) / (values.length - mid);
        const trend = secondAvg > firstAvg * 1.01 ? 'up' : secondAvg < firstAvg * 0.99 ? 'down' : 'flat';

        const gradientId = `sg-${this.metric.replace(/[^a-z0-9]/gi, '')}`;
        const path = this._generatePath(values, width - 4, height - 8, minValue, range);
        const areaPath = this._generateAreaPath(values, width - 4, height - 8, minValue, range);
        const dotY = height - 4 - ((currentValue - minValue) / range) * (height - 8);

        this._chart.innerHTML = `
            <svg width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
                <defs>
                    <linearGradient id="${gradientId}" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stop-color="${this.color}" stop-opacity="0.3"/>
                        <stop offset="100%" stop-color="${this.color}" stop-opacity="0"/>
                    </linearGradient>
                </defs>
                <path d="${areaPath}" fill="url(#${gradientId})"/>
                <path d="${path}" fill="none" stroke="${this.color}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                <circle cx="${width - 4}" cy="${dotY}" r="2.5" fill="${this.color}"/>
            </svg>
        `;

        // Update value display
        if (this.showValue) {
            this._valueContainer.style.display = '';
            this._valueContainer.querySelector('.value').textContent = this._formatValue(currentValue);
            const trendEl = this._valueContainer.querySelector('.trend');
            trendEl.className = `trend trend-${trend}`;
            trendEl.textContent = trend === 'up' ? '↑' : trend === 'down' ? '↓' : '→';
        } else {
            this._valueContainer.style.display = 'none';
        }
    }

    _generatePath(values, width, height, min, range) {
        if (values.length < 2) return '';
        return 'M ' + values.map((v, i) => {
            const x = 2 + (i / (values.length - 1)) * width;
            const y = 4 + height - ((v - min) / range) * height;
            return `${x},${y}`;
        }).join(' L ');
    }

    _generateAreaPath(values, width, height, min, range) {
        if (values.length < 2) return '';
        const points = values.map((v, i) => {
            const x = 2 + (i / (values.length - 1)) * width;
            const y = 4 + height - ((v - min) / range) * height;
            return `${x},${y}`;
        });
        return `M 2,${height + 4} L ${points.join(' L ')} L ${2 + width},${height + 4} Z`;
    }

    _formatValue(value) {
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
        if (value % 1 !== 0) return value.toFixed(2);
        return value.toString();
    }
}

customElements.define('metric-sparkline', MetricSparkline);
