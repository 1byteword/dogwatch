/**
 * Base Web Component with performance optimizations:
 * - Shadow DOM with adoptedStyleSheets (CSS parsed once, shared)
 * - Template caching (HTML parsed once)
 * - Batched updates via requestAnimationFrame
 */

// Shared stylesheet cache - parse CSS once per component class
const styleCache = new Map();

// Template cache - parse HTML structure once
const templateCache = new Map();

class BaseComponent extends HTMLElement {
    constructor() {
        super();
        this._updateScheduled = false;
        this._mounted = false;
    }

    // Override in subclass - return CSS string
    static get styles() { return ''; }

    // Override in subclass - return component name for caching
    static get componentName() { return 'base-component'; }

    connectedCallback() {
        // Create shadow root if using shadow DOM
        if (this.constructor.useShadowDom !== false) {
            if (!this.shadowRoot) {
                this.attachShadow({ mode: 'open' });
                this._applyStyles();
            }
        }
        this._mounted = true;
        this.onMount();
    }

    disconnectedCallback() {
        this._mounted = false;
        this.onUnmount();
    }

    // Override in subclass
    onMount() {}
    onUnmount() {}

    _applyStyles() {
        const name = this.constructor.componentName;

        // Check if we already have a parsed stylesheet
        if (!styleCache.has(name)) {
            const css = this.constructor.styles;
            if (css) {
                // Use adoptedStyleSheets if supported (much faster)
                if ('adoptedStyleSheets' in Document.prototype) {
                    const sheet = new CSSStyleSheet();
                    sheet.replaceSync(css);
                    styleCache.set(name, sheet);
                } else {
                    // Fallback: cache the CSS string
                    styleCache.set(name, css);
                }
            }
        }

        const cached = styleCache.get(name);
        if (cached instanceof CSSStyleSheet) {
            this.shadowRoot.adoptedStyleSheets = [cached];
        } else if (cached) {
            // Fallback for older browsers
            const style = document.createElement('style');
            style.textContent = cached;
            this.shadowRoot.appendChild(style);
        }
    }

    // Schedule batched render via rAF
    scheduleUpdate() {
        if (this._updateScheduled || !this._mounted) return;
        this._updateScheduled = true;
        requestAnimationFrame(() => {
            this._updateScheduled = false;
            if (this._mounted) this.render();
        });
    }

    // Get or create root element for rendering
    get root() {
        return this.shadowRoot || this;
    }

    // Efficient DOM update - only updates changed content
    updateContent(selector, html) {
        const el = this.root.querySelector(selector);
        if (el && el.innerHTML !== html) {
            el.innerHTML = html;
        }
    }

    // Update text content only (faster than innerHTML)
    updateText(selector, text) {
        const el = this.root.querySelector(selector);
        if (el && el.textContent !== text) {
            el.textContent = text;
        }
    }

    // Toggle class efficiently
    toggleClass(selector, className, force) {
        const el = this.root.querySelector(selector);
        if (el) el.classList.toggle(className, force);
    }

    // Override in subclass
    render() {}

    // Utility: escape HTML
    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // Utility: format duration
    formatDuration(ms) {
        if (ms < 0) ms = 0;
        const s = Math.floor(ms / 1000);
        if (s < 60) return `${s}s`;
        const m = Math.floor(s / 60);
        if (m < 60) return `${m}m`;
        const h = Math.floor(m / 60);
        if (h < 24) return `${h}h ${m % 60}m`;
        const d = Math.floor(h / 24);
        return `${d}d ${h % 24}h`;
    }

    // Utility: format relative time
    formatRelativeTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const diff = Date.now() - d.getTime();
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    // Utility: format currency
    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(0)}k`;
        return `$${amount.toFixed(0)}`;
    }

    // Utility: format bytes
    formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
    }
}

// Export for use
window.BaseComponent = BaseComponent;

/**
 * Alerts Dashboard Widget
 * Unified view of active alerts, history, and silences
 *
 * Optimizations:
 * - CSS via adoptedStyleSheets (parsed once)
 * - Selective DOM updates
 * - Request deduplication
 */

const alertsDashboardStyles = new CSSStyleSheet();
alertsDashboardStyles.replaceSync(`
    :host { display: block; height: 100%; }
    .alerts-dashboard {
        background: var(--bg-card, #16181c);
        border-radius: 8px;
        overflow: hidden;
        height: 100%;
        display: flex;
        flex-direction: column;
    }
    .alerts-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 0.75rem 1rem;
        background: var(--bg-elevated, #1e2128);
        border-bottom: 1px solid var(--border, #2f3336);
    }
    .alerts-tabs { display: flex; gap: 0.25rem; }
    .tab-btn {
        background: transparent;
        border: none;
        color: var(--text-muted, #71767b);
        padding: 0.5rem 0.75rem;
        border-radius: 6px;
        cursor: pointer;
        font-size: 0.85rem;
        display: flex;
        align-items: center;
        gap: 0.5rem;
    }
    .tab-btn:hover { background: var(--bg-card, #16181c); }
    .tab-btn.active { background: var(--bg-card, #16181c); color: var(--text, #e7e9ea); }
    .tab-count {
        background: var(--bg-elevated, #1e2128);
        padding: 0.1rem 0.4rem;
        border-radius: 10px;
        font-size: 0.7rem;
    }
    .tab-btn.active .tab-count { background: var(--accent, #1d9bf0); color: white; }
    .alerts-actions { display: flex; gap: 0.5rem; }
    .btn-icon {
        width: 32px; height: 32px;
        display: flex; align-items: center; justify-content: center;
        background: var(--bg-card, #16181c);
        border: 1px solid var(--border, #2f3336);
        border-radius: 6px;
        color: var(--text, #e7e9ea);
        cursor: pointer;
    }
    .btn-icon:hover { border-color: var(--accent, #1d9bf0); }
    .btn-sm {
        background: var(--accent, #1d9bf0);
        border: none;
        color: white;
        padding: 0.5rem 0.75rem;
        border-radius: 6px;
        cursor: pointer;
        font-size: 0.8rem;
    }
    .alerts-content {
        flex: 1;
        overflow-y: auto;
        padding: 1rem;
    }
    .loading, .empty-state {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        padding: 3rem;
        color: var(--text-muted, #71767b);
        text-align: center;
    }
    .empty-state .icon { font-size: 2.5rem; margin-bottom: 1rem; }
    .alerts-list, .events-list {
        display: flex;
        flex-direction: column;
        gap: 0.75rem;
    }
    .alert-card {
        display: flex;
        align-items: center;
        gap: 1rem;
        padding: 1rem;
        background: var(--bg-elevated, #1e2128);
        border-radius: 8px;
        border-left: 4px solid var(--border, #2f3336);
    }
    .alert-card.critical { border-left-color: var(--error, #f4212e); }
    .alert-card.warning { border-left-color: var(--warning, #ffd400); }
    .alert-card.silenced { border-left-color: var(--text-muted, #71767b); opacity: 0.7; }
    .alert-status { font-size: 1.5rem; }
    .alert-info { flex: 1; }
    .alert-name { font-weight: 600; font-size: 0.95rem; margin-bottom: 0.25rem; }
    .alert-condition {
        font-family: monospace;
        font-size: 0.8rem;
        color: var(--text-muted, #71767b);
        margin-bottom: 0.25rem;
    }
    .alert-meta {
        display: flex;
        gap: 1rem;
        font-size: 0.75rem;
        color: var(--text-muted, #71767b);
    }
    .alert-actions { display: flex; gap: 0.25rem; }
    .btn-action {
        width: 32px; height: 32px;
        display: flex; align-items: center; justify-content: center;
        background: transparent;
        border: 1px solid var(--border, #2f3336);
        border-radius: 6px;
        cursor: pointer;
        font-size: 0.9rem;
    }
    .btn-action:hover { background: var(--bg-card, #16181c); }
    .event-item {
        display: flex;
        align-items: center;
        gap: 0.75rem;
        padding: 0.75rem;
        background: var(--bg-elevated, #1e2128);
        border-radius: 6px;
        border-left: 3px solid var(--border, #2f3336);
    }
    .event-item.critical { border-left-color: var(--error, #f4212e); }
    .event-item.warning { border-left-color: var(--warning, #ffd400); }
    .event-item.recovered { border-left-color: var(--success, #00ba7c); }
    .event-icon { font-size: 1rem; }
    .event-info { flex: 1; }
    .event-name { font-weight: 500; font-size: 0.85rem; margin-bottom: 0.2rem; }
    .event-message { font-size: 0.75rem; color: var(--text-muted, #71767b); }
    .event-time { font-size: 0.75rem; color: var(--text-muted, #71767b); }
`);

class AlertsDashboard extends HTMLElement {
    constructor() {
        super();
        this.alerts = [];
        this.events = [];
        this.channels = [];
        this.view = 'active';
        this._loading = false;
        this._pendingRequest = null;

        this.attachShadow({ mode: 'open' });
        this.shadowRoot.adoptedStyleSheets = [alertsDashboardStyles];
    }

    connectedCallback() {
        this._initDOM();
        this.loadData();
        this._refreshInterval = setInterval(() => this.loadData(), 30000);
    }

    disconnectedCallback() {
        if (this._refreshInterval) clearInterval(this._refreshInterval);
        if (this._pendingRequest) this._pendingRequest.abort();
    }

    _initDOM() {
        this.shadowRoot.innerHTML = `
            <div class="alerts-dashboard">
                <div class="alerts-header">
                    <div class="alerts-tabs">
                        <button class="tab-btn active" data-view="active">
                            Active <span class="tab-count" id="active-count">0</span>
                        </button>
                        <button class="tab-btn" data-view="history">History</button>
                        <button class="tab-btn" data-view="silenced">
                            Silenced <span class="tab-count" id="silenced-count">0</span>
                        </button>
                    </div>
                    <div class="alerts-actions">
                        <button class="btn-icon" id="refresh-btn" title="Refresh">⟲</button>
                        <button class="btn-sm" id="new-alert-btn">+ New Alert</button>
                    </div>
                </div>
                <div class="alerts-content" id="alerts-content">
                    <div class="loading">Loading alerts...</div>
                </div>
            </div>
        `;

        // Event delegation for better performance
        this.shadowRoot.addEventListener('click', e => this._handleClick(e));
    }

    _handleClick(e) {
        const btn = e.target.closest('button');
        if (!btn) return;

        if (btn.dataset.view) {
            this.setView(btn.dataset.view);
        } else if (btn.id === 'refresh-btn') {
            this.loadData();
        } else if (btn.id === 'new-alert-btn') {
            window.location.href = '/#alerts';
        } else if (btn.dataset.action) {
            const id = btn.dataset.id;
            switch (btn.dataset.action) {
                case 'silence': this.silenceAlert(id); break;
                case 'ack': this.acknowledgeAlert(id); break;
                case 'details': this.showAlertDetails(id); break;
                case 'unsilence': this.unsilenceAlert(id); break;
            }
        }
    }

    setView(view) {
        this.view = view;
        this.shadowRoot.querySelectorAll('.tab-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.view === view);
        });
        this._renderContent();
    }

    async loadData() {
        if (this._loading) return;
        this._loading = true;

        try {
            const controller = new AbortController();
            this._pendingRequest = controller;

            const [alertsResp, eventsResp, channelsResp] = await Promise.all([
                fetch('/api/watches', { signal: controller.signal }),
                fetch('/api/watch/events?limit=100', { signal: controller.signal }),
                fetch('/api/watch/channels', { signal: controller.signal })
            ]);

            if (alertsResp.ok) this.alerts = await alertsResp.json() || [];
            if (eventsResp.ok) this.events = await eventsResp.json() || [];
            if (channelsResp.ok) this.channels = await channelsResp.json() || [];

            this._renderContent();
        } catch (e) {
            if (e.name !== 'AbortError') console.error('Failed to load alerts:', e);
        } finally {
            this._loading = false;
            this._pendingRequest = null;
        }
    }

    _renderContent() {
        const content = this.shadowRoot.getElementById('alerts-content');
        const activeCount = this.shadowRoot.getElementById('active-count');
        const silencedCount = this.shadowRoot.getElementById('silenced-count');

        const alertingAlerts = this.alerts.filter(a => a.state === 'alerting' || a.state === 'pending');
        const silencedAlerts = this.alerts.filter(a => a.muted_until && new Date(a.muted_until) > new Date());

        if (activeCount) activeCount.textContent = alertingAlerts.length;
        if (silencedCount) silencedCount.textContent = silencedAlerts.length;

        switch (this.view) {
            case 'active':
                content.innerHTML = this._renderActiveAlerts(alertingAlerts);
                break;
            case 'history':
                content.innerHTML = this._renderHistory();
                break;
            case 'silenced':
                content.innerHTML = this._renderSilenced(silencedAlerts);
                break;
        }
    }

    _renderActiveAlerts(alerts) {
        if (alerts.length === 0) {
            return `<div class="empty-state"><span class="icon">✓</span><p>All clear! No active alerts.</p></div>`;
        }
        return `<div class="alerts-list">${alerts.map(a => this._renderAlertCard(a)).join('')}</div>`;
    }

    _renderAlertCard(alert) {
        const stateClass = alert.state === 'alerting' ? 'critical' : 'warning';
        const duration = this._formatDuration(Date.now() - new Date(alert.state_at).getTime());
        return `
            <div class="alert-card ${stateClass}">
                <div class="alert-status">${alert.state === 'alerting' ? '🔴' : '🟡'}</div>
                <div class="alert-info">
                    <div class="alert-name">${this._esc(alert.name)}</div>
                    <div class="alert-condition">${this._esc(alert.metric)} ${alert.operator} ${alert.threshold}</div>
                    <div class="alert-meta">
                        <span>Current: ${alert.last_value?.toFixed(2) || '—'}</span>
                        <span>for ${duration}</span>
                    </div>
                </div>
                <div class="alert-actions">
                    <button class="btn-action" data-action="silence" data-id="${alert.id}" title="Silence">🔕</button>
                    <button class="btn-action" data-action="ack" data-id="${alert.id}" title="Acknowledge">✓</button>
                    <button class="btn-action" data-action="details" data-id="${alert.id}" title="Details">⋯</button>
                </div>
            </div>
        `;
    }

    _renderHistory() {
        if (this.events.length === 0) {
            return `<div class="empty-state"><span class="icon">📋</span><p>No alert history yet</p></div>`;
        }
        return `<div class="events-list">${this.events.slice(0, 50).map(e => {
            const isRecovery = e.to_state === 'ok';
            const icon = isRecovery ? '✓' : e.to_state === 'alerting' ? '🔴' : '🟡';
            const stateClass = isRecovery ? 'recovered' : e.to_state === 'alerting' ? 'critical' : 'warning';
            return `
                <div class="event-item ${stateClass}">
                    <span class="event-icon">${icon}</span>
                    <div class="event-info">
                        <div class="event-name">${this._esc(e.watch_name)}</div>
                        <div class="event-message">${this._esc(e.message)}</div>
                    </div>
                    <div class="event-time">${this._formatTime(e.timestamp)}</div>
                </div>
            `;
        }).join('')}</div>`;
    }

    _renderSilenced(alerts) {
        if (alerts.length === 0) {
            return `<div class="empty-state"><span class="icon">🔔</span><p>No silenced alerts</p></div>`;
        }
        return `<div class="alerts-list">${alerts.map(a => `
            <div class="alert-card silenced">
                <div class="alert-status">🔕</div>
                <div class="alert-info">
                    <div class="alert-name">${this._esc(a.name)}</div>
                    <div class="alert-meta">Silenced until ${this._formatTime(a.muted_until)}</div>
                </div>
                <div class="alert-actions">
                    <button class="btn-action" data-action="unsilence" data-id="${a.id}" title="Unsilence">🔔</button>
                </div>
            </div>
        `).join('')}</div>`;
    }

    async silenceAlert(id) {
        const duration = prompt('Silence for how long? (e.g., 1h, 30m, 2h)', '1h');
        if (!duration) return;
        try {
            const resp = await fetch(`/api/watches/${id}/mute`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ duration })
            });
            if (resp.ok) this.loadData();
            else alert('Failed to silence alert');
        } catch (e) {
            alert('Error: ' + e.message);
        }
    }

    async unsilenceAlert(id) {
        try {
            const resp = await fetch(`/api/watches/${id}/unmute`, { method: 'POST' });
            if (resp.ok) this.loadData();
        } catch (e) {
            console.error('Failed to unsilence:', e);
        }
    }

    acknowledgeAlert(id) {
        alert('Alert acknowledged');
    }

    showAlertDetails(id) {
        const alert = this.alerts.find(a => a.id === id);
        if (!alert) return;
        window.alert(`Alert: ${alert.name}\nMetric: ${alert.metric}\nCondition: ${alert.operator} ${alert.threshold}\nCurrent Value: ${alert.last_value?.toFixed(2) || '—'}\nState: ${alert.state}\nSince: ${new Date(alert.state_at).toLocaleString()}`);
    }

    _formatDuration(ms) {
        const s = Math.floor(ms / 1000);
        if (s < 60) return `${s}s`;
        const m = Math.floor(s / 60);
        if (m < 60) return `${m}m`;
        const h = Math.floor(m / 60);
        if (h < 24) return `${h}h ${m % 60}m`;
        return `${Math.floor(h / 24)}d ${h % 24}h`;
    }

    _formatTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const diff = Date.now() - d.getTime();
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    _esc(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }
}

customElements.define('alerts-dashboard', AlertsDashboard);

/**
 * Anomaly Overlay Component
 * Automatic highlighting of anomalous regions on charts
 */
class AnomalyOverlay extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        if (this.chart) this.chart.destroy();
    }

    static get observedAttributes() {
        return ['metric', 'service', 'time-range', 'sensitivity'];
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }
    get sensitivity() { return parseFloat(this.getAttribute('sensitivity')) || 2.0; }

    render() {
        this.innerHTML = `
            <style>
                .anomaly-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .anomaly-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .anomaly-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .anomaly-badge {
                    background: #f43f5e;
                    color: white;
                    padding: 0.2rem 0.5rem;
                    border-radius: 10px;
                    font-size: 0.7rem;
                    font-weight: 600;
                }
                .anomaly-controls {
                    display: flex;
                    gap: 0.5rem;
                    align-items: center;
                }
                .anomaly-controls label {
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                }
                .anomaly-controls input[type="range"] {
                    width: 80px;
                }
                .anomaly-body {
                    flex: 1;
                    padding: 1rem;
                    min-height: 200px;
                }
                .anomaly-chart {
                    width: 100%;
                    height: 100%;
                }
                .anomaly-list {
                    max-height: 150px;
                    overflow-y: auto;
                    border-top: 1px solid var(--border-color, #2f3336);
                }
                .anomaly-item {
                    display: flex;
                    align-items: center;
                    gap: 0.75rem;
                    padding: 0.75rem 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    cursor: pointer;
                    transition: background 0.15s ease;
                }
                .anomaly-item:hover {
                    background: var(--bg-elevated, #1a1f2e);
                }
                .anomaly-item:last-child {
                    border-bottom: none;
                }
                .anomaly-dot {
                    width: 10px;
                    height: 10px;
                    border-radius: 50%;
                    flex-shrink: 0;
                }
                .anomaly-dot.high { background: #f43f5e; }
                .anomaly-dot.medium { background: #f59e0b; }
                .anomaly-dot.low { background: #3b82f6; }
                .anomaly-info {
                    flex: 1;
                    min-width: 0;
                }
                .anomaly-time {
                    font-size: 0.8rem;
                    font-weight: 600;
                }
                .anomaly-desc {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .anomaly-score {
                    font-size: 0.8rem;
                    font-weight: 600;
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="anomaly-container">
                <div class="anomaly-header">
                    <div class="anomaly-title">
                        <span>&#128270;</span>
                        <span>Anomaly Detection</span>
                        <span class="anomaly-badge" id="count">0</span>
                    </div>
                    <div class="anomaly-controls">
                        <label>Sensitivity:</label>
                        <input type="range" id="sensitivity" min="1" max="4" step="0.5" value="${this.sensitivity}">
                    </div>
                </div>
                <div class="anomaly-body">
                    <canvas class="anomaly-chart" id="chart"></canvas>
                </div>
                <div class="anomaly-list" id="list"></div>
            </div>
        `;

        this.querySelector('#sensitivity')?.addEventListener('input', (e) => {
            this.setAttribute('sensitivity', e.target.value);
            this.detectAnomalies();
        });
    }

    async loadData() {
        try {
            const params = new URLSearchParams({
                metric: this.metric,
                range: this.timeRange
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/metrics/timeseries?${params}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.detectAnomalies();
        } catch (e) {
            this.data = this.generateDemoData();
            this.detectAnomalies();
        }
    }

    generateDemoData() {
        const points = 60;
        const data = [];
        const now = Date.now();

        for (let i = 0; i < points; i++) {
            let value = 50 + Math.random() * 20;

            // Add anomalies
            if (i === 15) value = 150;
            if (i === 16) value = 140;
            if (i === 35) value = 120;
            if (i === 50) value = 5;

            data.push({
                timestamp: now - (points - i) * 60000,
                value
            });
        }

        return { data };
    }

    detectAnomalies() {
        if (!this.data?.data) return;

        const values = this.data.data.map(d => d.value);

        // Calculate mean and std
        const mean = values.reduce((a, b) => a + b, 0) / values.length;
        const std = Math.sqrt(values.reduce((sum, v) => sum + Math.pow(v - mean, 2), 0) / values.length);

        // Detect anomalies based on z-score
        const anomalies = [];
        const threshold = this.sensitivity;

        this.data.data.forEach((d, i) => {
            const zScore = Math.abs((d.value - mean) / std);
            if (zScore > threshold) {
                anomalies.push({
                    index: i,
                    timestamp: d.timestamp,
                    value: d.value,
                    zScore,
                    severity: zScore > threshold * 1.5 ? 'high' : zScore > threshold * 1.2 ? 'medium' : 'low',
                    direction: d.value > mean ? 'spike' : 'drop'
                });
            }
        });

        this.anomalies = anomalies;
        this.renderChart();
        this.renderList();
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.data) return;

        if (!window.Chart && window.LibLoader) {
            await window.LibLoader.loadAll(['chart', 'chart-date']);
        }

        if (this.chart) this.chart.destroy();

        const ctx = canvas.getContext('2d');
        const data = this.data.data;

        // Create anomaly highlighting dataset
        const anomalyData = data.map((d, i) => {
            const anomaly = this.anomalies.find(a => a.index === i);
            return anomaly ? d.value : null;
        });

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(d => new Date(d.timestamp)),
                datasets: [
                    {
                        label: 'Metric',
                        data: data.map(d => d.value),
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                    },
                    {
                        label: 'Anomaly',
                        data: anomalyData,
                        borderColor: '#f43f5e',
                        backgroundColor: '#f43f5e',
                        pointRadius: 8,
                        pointHoverRadius: 10,
                        showLine: false,
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: (ctx) => {
                                if (ctx.datasetIndex === 1) {
                                    const anomaly = this.anomalies.find(a => a.index === ctx.dataIndex);
                                    return anomaly ? `Anomaly: ${ctx.raw.toFixed(1)} (z=${anomaly.zScore.toFixed(2)})` : '';
                                }
                                return `Value: ${ctx.raw.toFixed(1)}`;
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        type: 'time',
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b', maxTicksLimit: 6 }
                    },
                    y: {
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b' }
                    }
                }
            }
        });
    }

    renderList() {
        const list = this.querySelector('#list');
        const countBadge = this.querySelector('#count');

        if (!list) return;

        countBadge.textContent = this.anomalies.length;

        if (this.anomalies.length === 0) {
            list.innerHTML = '<div style="padding: 1rem; text-align: center; color: var(--text-muted);">No anomalies detected</div>';
            return;
        }

        list.innerHTML = this.anomalies.map(a => `
            <div class="anomaly-item" data-index="${a.index}">
                <div class="anomaly-dot ${a.severity}"></div>
                <div class="anomaly-info">
                    <div class="anomaly-time">${new Date(a.timestamp).toLocaleTimeString()}</div>
                    <div class="anomaly-desc">${a.direction === 'spike' ? 'Spike' : 'Drop'}: ${a.value.toFixed(1)} (${a.severity})</div>
                </div>
                <div class="anomaly-score">z=${a.zScore.toFixed(1)}</div>
            </div>
        `).join('');

        // Click to highlight on chart
        list.querySelectorAll('.anomaly-item').forEach(item => {
            item.addEventListener('click', () => {
                const index = parseInt(item.dataset.index);
                // Could zoom to this point on the chart
                this.dispatchEvent(new CustomEvent('anomaly-click', {
                    detail: this.anomalies.find(a => a.index === index)
                }));
            });
        });
    }
}

customElements.define('anomaly-overlay', AnomalyOverlay);

/**
 * Correlation View Component
 * Split-pane view with metrics on top and related logs/traces below
 */
class CorrelationView extends HTMLElement {
    constructor() {
        super();
        this.metricsData = null;
        this.correlatedData = null;
        this.selectedTimeRange = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadMetrics();
    }

    disconnectedCallback() {
        if (this.chart) {
            this.chart.destroy();
        }
    }

    static get observedAttributes() {
        return ['metric', 'service', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadMetrics();
        }
    }

    get metric() { return this.getAttribute('metric') || 'http_request_duration_seconds'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .correlation-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .correlation-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .correlation-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .correlation-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .correlation-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .correlation-metrics-panel {
                    height: 200px;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    position: relative;
                }
                .correlation-chart {
                    width: 100%;
                    height: 100%;
                }
                .correlation-hint {
                    position: absolute;
                    top: 0.5rem;
                    right: 1rem;
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-details-panel {
                    flex: 1;
                    display: flex;
                    flex-direction: column;
                    overflow: hidden;
                }
                .correlation-tabs {
                    display: flex;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .correlation-tab {
                    padding: 0.75rem 1rem;
                    cursor: pointer;
                    font-size: 0.85rem;
                    color: var(--text-muted, #71767b);
                    border-bottom: 2px solid transparent;
                    transition: all 0.15s ease;
                }
                .correlation-tab:hover {
                    color: var(--text-primary, #e7e9ea);
                }
                .correlation-tab.active {
                    color: var(--color-info, #1d9bf0);
                    border-bottom-color: var(--color-info, #1d9bf0);
                }
                .correlation-content {
                    flex: 1;
                    overflow: auto;
                    padding: 1rem;
                }
                .correlation-empty {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    text-align: center;
                    gap: 0.5rem;
                }
                .correlation-trace-item, .correlation-log-item {
                    padding: 0.75rem;
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    margin-bottom: 0.5rem;
                    cursor: pointer;
                    transition: all 0.15s ease;
                }
                .correlation-trace-item:hover, .correlation-log-item:hover {
                    border-color: var(--color-info, #1d9bf0);
                    background: var(--bg-elevated, #1a1f2e);
                }
                .correlation-item-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    margin-bottom: 0.5rem;
                }
                .correlation-item-title {
                    font-weight: 600;
                    font-size: 0.85rem;
                }
                .correlation-item-time {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-item-meta {
                    display: flex;
                    gap: 1rem;
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                }
                .correlation-log-message {
                    font-family: monospace;
                    font-size: 0.8rem;
                    white-space: nowrap;
                    overflow: hidden;
                    text-overflow: ellipsis;
                }
                .correlation-badge {
                    display: inline-flex;
                    padding: 0.2rem 0.5rem;
                    border-radius: 4px;
                    font-size: 0.7rem;
                    font-weight: 600;
                }
                .correlation-badge.error {
                    background: rgba(244, 63, 94, 0.2);
                    color: #f43f5e;
                }
                .correlation-badge.warning {
                    background: rgba(245, 158, 11, 0.2);
                    color: #f59e0b;
                }
                .correlation-badge.info {
                    background: rgba(59, 130, 246, 0.2);
                    color: #3b82f6;
                }
                .correlation-selection-info {
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                    display: none;
                }
                .correlation-selection-info.visible {
                    display: block;
                }
            </style>
            <div class="correlation-container">
                <div class="correlation-header">
                    <div class="correlation-title">
                        <span>&#128279;</span>
                        <span>Correlation View</span>
                    </div>
                    <div class="correlation-controls">
                        <select id="metric-select">
                            <option value="http_request_duration_seconds">Request Latency</option>
                            <option value="http_requests_total">Request Rate</option>
                            <option value="http_request_errors_total">Error Rate</option>
                        </select>
                        <select id="time-select">
                            <option value="15m">15 min</option>
                            <option value="1h" selected>1 hour</option>
                            <option value="6h">6 hours</option>
                        </select>
                    </div>
                </div>
                <div class="correlation-metrics-panel">
                    <canvas class="correlation-chart" id="chart"></canvas>
                    <div class="correlation-hint">Click and drag to select a time range</div>
                </div>
                <div class="correlation-selection-info" id="selection-info">
                    Selected: <span id="selection-range"></span> -
                    <span id="selection-count"></span> events found
                </div>
                <div class="correlation-details-panel">
                    <div class="correlation-tabs">
                        <div class="correlation-tab active" data-tab="traces">Traces</div>
                        <div class="correlation-tab" data-tab="logs">Logs</div>
                        <div class="correlation-tab" data-tab="events">Events</div>
                    </div>
                    <div class="correlation-content" id="content">
                        <div class="correlation-empty">
                            <span style="font-size: 2rem;">&#128270;</span>
                            <span>Select a time range on the chart above</span>
                            <span>to see correlated traces and logs</span>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        // Tab switching
        this.querySelectorAll('.correlation-tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.querySelectorAll('.correlation-tab').forEach(t => t.classList.remove('active'));
                e.target.classList.add('active');
                this.renderCorrelatedData(e.target.dataset.tab);
            });
        });

        // Metric selection
        this.querySelector('#metric-select')?.addEventListener('change', (e) => {
            this.setAttribute('metric', e.target.value);
        });

        // Time range
        this.querySelector('#time-select')?.addEventListener('change', (e) => {
            this.setAttribute('time-range', e.target.value);
        });
    }

    async loadMetrics() {
        try {
            const params = new URLSearchParams({
                metric: this.metric,
                range: this.timeRange
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/metrics/timeseries?${params}`);

            if (!resp.ok) {
                this.metricsData = this.generateDemoMetrics();
            } else {
                this.metricsData = await resp.json();
            }

            this.renderChart();
        } catch (e) {
            console.error('Failed to load metrics:', e);
            this.metricsData = this.generateDemoMetrics();
            this.renderChart();
        }
    }

    generateDemoMetrics() {
        const now = Date.now();
        const points = 60;
        const interval = 60000; // 1 minute
        const data = [];

        for (let i = 0; i < points; i++) {
            const timestamp = now - (points - i) * interval;
            let value = 50 + Math.random() * 30;

            // Add some spikes
            if (i === 25 || i === 26) value += 100;
            if (i === 45) value += 80;

            data.push({ timestamp, value });
        }

        return { data };
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.metricsData) return;

        // Ensure Chart.js is loaded
        if (!window.Chart) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['chart', 'chart-date']);
            }
        }

        if (this.chart) {
            this.chart.destroy();
        }

        const ctx = canvas.getContext('2d');
        const data = this.metricsData.data;

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(d => new Date(d.timestamp)),
                datasets: [{
                    data: data.map(d => d.value),
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 0,
                    pointHoverRadius: 4,
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        mode: 'index',
                        intersect: false,
                    }
                },
                scales: {
                    x: {
                        type: 'time',
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b', maxTicksLimit: 6 }
                    },
                    y: {
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b' }
                    }
                },
                interaction: {
                    mode: 'nearest',
                    axis: 'x',
                    intersect: false
                },
                onClick: (event, elements) => {
                    if (elements.length > 0) {
                        const index = elements[0].index;
                        const timestamp = data[index].timestamp;
                        this.selectTimeRange(timestamp - 60000, timestamp + 60000);
                    }
                }
            }
        });

        // Add drag selection (simplified)
        canvas.addEventListener('click', (e) => {
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const chartArea = this.chart.chartArea;

            if (x >= chartArea.left && x <= chartArea.right) {
                const xScale = this.chart.scales.x;
                const timestamp = xScale.getValueForPixel(x);
                this.selectTimeRange(timestamp - 60000, timestamp + 60000);
            }
        });
    }

    async selectTimeRange(start, end) {
        this.selectedTimeRange = { start, end };

        // Update UI
        const selectionInfo = this.querySelector('#selection-info');
        const selectionRange = this.querySelector('#selection-range');

        selectionInfo.classList.add('visible');
        selectionRange.textContent = `${new Date(start).toLocaleTimeString()} - ${new Date(end).toLocaleTimeString()}`;

        // Load correlated data
        await this.loadCorrelatedData(start, end);
    }

    async loadCorrelatedData(start, end) {
        try {
            const params = new URLSearchParams({
                start: new Date(start).toISOString(),
                end: new Date(end).toISOString()
            });
            if (this.service) params.append('service', this.service);

            const resp = await fetch(`/api/correlation/events?${params}`);

            if (!resp.ok) {
                this.correlatedData = this.generateDemoCorrelatedData();
            } else {
                this.correlatedData = await resp.json();
            }

            this.querySelector('#selection-count').textContent =
                (this.correlatedData.traces?.length || 0) + (this.correlatedData.logs?.length || 0);

            // Render active tab
            const activeTab = this.querySelector('.correlation-tab.active');
            this.renderCorrelatedData(activeTab?.dataset.tab || 'traces');
        } catch (e) {
            console.error('Failed to load correlated data:', e);
            this.correlatedData = this.generateDemoCorrelatedData();
            this.renderCorrelatedData('traces');
        }
    }

    generateDemoCorrelatedData() {
        return {
            traces: [
                { id: 'trace-1', service: 'api-gateway', operation: 'GET /api/users', duration: 245, status: 'error', timestamp: Date.now() - 30000 },
                { id: 'trace-2', service: 'user-service', operation: 'getUserById', duration: 180, status: 'ok', timestamp: Date.now() - 35000 },
                { id: 'trace-3', service: 'api-gateway', operation: 'POST /api/orders', duration: 520, status: 'error', timestamp: Date.now() - 40000 },
            ],
            logs: [
                { timestamp: Date.now() - 28000, level: 'error', service: 'api-gateway', message: 'Connection timeout to upstream service' },
                { timestamp: Date.now() - 32000, level: 'warn', service: 'user-service', message: 'Slow query detected: 180ms' },
                { timestamp: Date.now() - 38000, level: 'error', service: 'order-service', message: 'Database connection pool exhausted' },
                { timestamp: Date.now() - 42000, level: 'info', service: 'api-gateway', message: 'Retry attempt 1 for request xyz' },
            ],
            events: [
                { timestamp: Date.now() - 25000, type: 'deploy', service: 'api-gateway', message: 'Deployed v2.4.1' },
                { timestamp: Date.now() - 60000, type: 'alert', service: 'user-service', message: 'High latency alert triggered' },
            ]
        };
    }

    renderCorrelatedData(tab) {
        const content = this.querySelector('#content');
        if (!content || !this.correlatedData) return;

        if (tab === 'traces') {
            const traces = this.correlatedData.traces || [];
            if (traces.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No traces found in this time range</span></div>';
                return;
            }

            content.innerHTML = traces.map(t => `
                <div class="correlation-trace-item" data-trace-id="${t.id}">
                    <div class="correlation-item-header">
                        <span class="correlation-item-title">${t.operation}</span>
                        <span class="correlation-item-time">${new Date(t.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-item-meta">
                        <span>${t.service}</span>
                        <span>${t.duration}ms</span>
                        <span class="correlation-badge ${t.status}">${t.status}</span>
                    </div>
                </div>
            `).join('');

            // Add click handlers
            content.querySelectorAll('.correlation-trace-item').forEach(el => {
                el.addEventListener('click', () => {
                    this.dispatchEvent(new CustomEvent('trace-select', {
                        detail: { traceId: el.dataset.traceId }
                    }));
                });
            });
        } else if (tab === 'logs') {
            const logs = this.correlatedData.logs || [];
            if (logs.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No logs found in this time range</span></div>';
                return;
            }

            content.innerHTML = logs.map(l => `
                <div class="correlation-log-item">
                    <div class="correlation-item-header">
                        <span class="correlation-badge ${l.level}">${l.level}</span>
                        <span class="correlation-item-time">${new Date(l.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-log-message">${this.escapeHtml(l.message)}</div>
                    <div class="correlation-item-meta">
                        <span>${l.service}</span>
                    </div>
                </div>
            `).join('');
        } else if (tab === 'events') {
            const events = this.correlatedData.events || [];
            if (events.length === 0) {
                content.innerHTML = '<div class="correlation-empty"><span>No events found in this time range</span></div>';
                return;
            }

            content.innerHTML = events.map(e => `
                <div class="correlation-log-item">
                    <div class="correlation-item-header">
                        <span class="correlation-badge info">${e.type}</span>
                        <span class="correlation-item-time">${new Date(e.timestamp).toLocaleTimeString()}</span>
                    </div>
                    <div class="correlation-log-message">${this.escapeHtml(e.message)}</div>
                    <div class="correlation-item-meta">
                        <span>${e.service}</span>
                    </div>
                </div>
            `).join('');
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }
}

customElements.define('correlation-view', CorrelationView);

/**
 * Cost Dashboard Widget
 * "You'd pay $X on Datadog" visualization
 */
class CostDashboard extends HTMLElement {
    constructor() {
        super();
        this.estimate = null;
        this.recommendations = [];
        this.loading = true;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.render();

        try {
            const [estimateResp, recsResp] = await Promise.all([
                fetch('/api/cost/estimate'),
                fetch('/api/cost/recommendations')
            ]);

            if (estimateResp.ok) this.estimate = await estimateResp.json();
            if (recsResp.ok) this.recommendations = await recsResp.json() || [];
        } catch (e) {
            console.error('Failed to load cost data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-dashboard">
                    <div class="cost-header">
                        <span class="title-icon">💰</span>
                        <span>Cost Intelligence</span>
                    </div>
                    <div class="loading">Loading cost data...</div>
                </div>
            `;
            return;
        }

        const estimate = this.estimate || {};
        const datadog = estimate.datadog || {};
        const newrelic = estimate.newrelic || {};
        const splunk = estimate.splunk || {};
        const savings = this.calculateSavings();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cost-dashboard">
                <div class="cost-header">
                    <div class="header-title">
                        <span class="title-icon">💰</span>
                        <span>Cost Intelligence</span>
                    </div>
                    <button class="btn-refresh" onclick="this.getRootNode().host.loadData()">↻</button>
                </div>

                <div class="savings-banner">
                    <div class="savings-text">
                        <span class="savings-label">Your estimated savings</span>
                        <span class="savings-value">${this.formatCurrency(savings.total)}<span class="period">/month</span></span>
                    </div>
                    <div class="savings-subtext">
                        vs. commercial alternatives
                    </div>
                </div>

                <div class="comparison-grid">
                    <div class="comparison-card dogwatch">
                        <div class="card-header">
                            <span class="provider-icon">🐕</span>
                            <span class="provider-name">dogwatch</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">$0</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-subtitle">Self-hosted, unlimited</div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">🐕</span>
                            <span class="provider-name">Datadog</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(datadog.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${datadog.hosts ? `<div class="breakdown-item">Infrastructure: ${this.formatCurrency(datadog.hosts)}</div>` : ''}
                            ${datadog.apm ? `<div class="breakdown-item">APM: ${this.formatCurrency(datadog.apm)}</div>` : ''}
                            ${datadog.logs ? `<div class="breakdown-item">Logs: ${this.formatCurrency(datadog.logs)}</div>` : ''}
                        </div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">📊</span>
                            <span class="provider-name">New Relic</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(newrelic.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${newrelic.users ? `<div class="breakdown-item">Users: ${this.formatCurrency(newrelic.users)}</div>` : ''}
                            ${newrelic.data ? `<div class="breakdown-item">Data Ingest: ${this.formatCurrency(newrelic.data)}</div>` : ''}
                        </div>
                    </div>

                    <div class="comparison-card">
                        <div class="card-header">
                            <span class="provider-icon">🔍</span>
                            <span class="provider-name">Splunk</span>
                        </div>
                        <div class="card-price">
                            <span class="price-value">${this.formatCurrency(splunk.total || 0)}</span>
                            <span class="price-period">/month</span>
                        </div>
                        <div class="card-breakdown">
                            ${splunk.workload ? `<div class="breakdown-item">Workload: ${this.formatCurrency(splunk.workload)}</div>` : ''}
                            ${splunk.ingest ? `<div class="breakdown-item">Ingest: ${this.formatCurrency(splunk.ingest)}</div>` : ''}
                        </div>
                    </div>
                </div>

                <div class="usage-stats">
                    <h3>Current Usage</h3>
                    <div class="stats-grid">
                        <div class="stat-item">
                            <span class="stat-value">${estimate.metrics_count?.toLocaleString() || 0}</span>
                            <span class="stat-label">Custom Metrics</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${this.formatBytes(estimate.logs_gb || 0)}</span>
                            <span class="stat-label">Logs/day</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${(estimate.spans_million || 0).toFixed(1)}M</span>
                            <span class="stat-label">Spans/month</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-value">${estimate.hosts || 0}</span>
                            <span class="stat-label">Hosts</span>
                        </div>
                    </div>
                </div>

                ${this.recommendations.length > 0 ? `
                    <div class="recommendations">
                        <h3>Optimization Recommendations</h3>
                        <div class="recs-list">
                            ${this.recommendations.slice(0, 3).map(r => `
                                <div class="rec-item ${r.priority || 'medium'}">
                                    <div class="rec-icon">${this.getRecIcon(r.type)}</div>
                                    <div class="rec-content">
                                        <div class="rec-title">${this.escapeHtml(r.title)}</div>
                                        <div class="rec-desc">${this.escapeHtml(r.description)}</div>
                                        ${r.estimated_savings ? `<div class="rec-savings">Save ~${this.formatCurrency(r.estimated_savings)}/mo</div>` : ''}
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    calculateSavings() {
        if (!this.estimate) return { total: 0 };

        const datadog = this.estimate.datadog?.total || 0;
        const newrelic = this.estimate.newrelic?.total || 0;
        const splunk = this.estimate.splunk?.total || 0;

        // Average of the three
        const avg = (datadog + newrelic + splunk) / 3;

        return {
            total: avg,
            datadog,
            newrelic,
            splunk
        };
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(0)}k`;
        return `$${amount.toFixed(0)}`;
    }

    formatBytes(gb) {
        if (!gb || gb === 0) return '0 GB';
        if (gb >= 1000) return `${(gb / 1000).toFixed(1)} TB`;
        return `${gb.toFixed(1)} GB`;
    }

    getRecIcon(type) {
        switch (type) {
            case 'cardinality': return '📊';
            case 'retention': return '🗄️';
            case 'sampling': return '🎯';
            case 'aggregation': return '📈';
            default: return '💡';
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .cost-dashboard {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .cost-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.25rem 0.5rem;
                cursor: pointer;
            }

            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .savings-banner {
                background: linear-gradient(135deg, #00ba7c 0%, #00a36c 100%);
                padding: 1.25rem;
                text-align: center;
                color: white;
            }

            .savings-label {
                display: block;
                font-size: 0.85rem;
                opacity: 0.9;
                margin-bottom: 0.25rem;
            }

            .savings-value {
                font-size: 2.5rem;
                font-weight: 700;
            }

            .savings-value .period {
                font-size: 1rem;
                font-weight: 400;
                opacity: 0.8;
            }

            .savings-subtext {
                font-size: 0.8rem;
                opacity: 0.8;
                margin-top: 0.25rem;
            }

            .comparison-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.5rem;
                padding: 1rem;
            }

            .comparison-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 0.75rem;
                border: 1px solid var(--border, #2f3336);
            }

            .comparison-card.dogwatch {
                background: linear-gradient(135deg, rgba(0, 186, 124, 0.1) 0%, rgba(0, 163, 108, 0.1) 100%);
                border-color: var(--success, #00ba7c);
            }

            .card-header {
                display: flex;
                align-items: center;
                gap: 0.4rem;
                margin-bottom: 0.5rem;
            }

            .provider-icon { font-size: 1rem; }
            .provider-name { font-size: 0.8rem; font-weight: 500; }

            .card-price {
                margin-bottom: 0.25rem;
            }

            .price-value {
                font-size: 1.25rem;
                font-weight: 600;
            }

            .price-period {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .card-subtitle {
                font-size: 0.7rem;
                color: var(--success, #00ba7c);
            }

            .card-breakdown {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .breakdown-item {
                padding: 0.1rem 0;
            }

            .usage-stats {
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .usage-stats h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .stats-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.75rem;
            }

            .stat-item {
                text-align: center;
            }

            .stat-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .recommendations {
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
                flex: 1;
                overflow-y: auto;
            }

            .recommendations h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .recs-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .rec-item {
                display: flex;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .rec-item.high { border-left-color: var(--error, #f4212e); }
            .rec-item.medium { border-left-color: var(--warning, #ffd400); }
            .rec-item.low { border-left-color: var(--success, #00ba7c); }

            .rec-icon { font-size: 1.25rem; }

            .rec-content { flex: 1; }

            .rec-title {
                font-weight: 500;
                font-size: 0.85rem;
                margin-bottom: 0.25rem;
            }

            .rec-desc {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .rec-savings {
                font-size: 0.75rem;
                color: var(--success, #00ba7c);
                margin-top: 0.25rem;
            }

            @media (max-width: 800px) {
                .comparison-grid, .stats-grid {
                    grid-template-columns: repeat(2, 1fr);
                }
            }
        `;
    }
}

customElements.define('cost-dashboard', CostDashboard);

/**
 * CPU Flamegraph Component
 * Interactive flamegraph visualization for CPU profiling data
 */
class CpuFlamegraph extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Handle resize
        this.resizeObserver = new ResizeObserver(() => this.updateChart());
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['service', 'time-range', 'profile-type'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadData();
        }
    }

    get service() {
        return this.getAttribute('service') || '';
    }

    get timeRange() {
        return this.getAttribute('time-range') || '5m';
    }

    get profileType() {
        return this.getAttribute('profile-type') || 'cpu';
    }

    render() {
        this.innerHTML = `
            <style>
                .flamegraph-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .flamegraph-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .flamegraph-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .flamegraph-controls {
                    display: flex;
                    gap: 0.5rem;
                    align-items: center;
                }
                .flamegraph-controls input {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.75rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                    width: 200px;
                }
                .flamegraph-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .flamegraph-controls button {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.75rem;
                    color: var(--text-primary, #e7e9ea);
                    cursor: pointer;
                    font-size: 0.8rem;
                }
                .flamegraph-controls button:hover {
                    border-color: var(--color-info, #1d9bf0);
                }
                .flamegraph-body {
                    flex: 1;
                    overflow: auto;
                    padding: 1rem;
                    min-height: 300px;
                }
                .flamegraph-chart {
                    width: 100%;
                    min-height: 280px;
                }
                .flamegraph-loading, .flamegraph-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                    flex-direction: column;
                    gap: 1rem;
                }
                .flamegraph-loading .spinner {
                    width: 32px;
                    height: 32px;
                    border: 3px solid var(--border-color, #2f3336);
                    border-top-color: var(--color-info, #1d9bf0);
                    border-radius: 50%;
                    animation: spin 0.8s linear infinite;
                }
                @keyframes spin { to { transform: rotate(360deg); } }

                /* D3 Flamegraph overrides */
                .d3-flame-graph rect {
                    stroke: var(--bg-primary, #0f1419);
                    stroke-width: 1px;
                }
                .d3-flame-graph-tip {
                    background: var(--bg-elevated, #1a1f2e) !important;
                    border: 1px solid var(--border-color, #2f3336) !important;
                    color: var(--text-primary, #e7e9ea) !important;
                    padding: 0.5rem 0.75rem !important;
                    border-radius: 6px !important;
                    font-size: 0.8rem !important;
                }
                .flamegraph-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .flamegraph-stat {
                    display: flex;
                    gap: 0.5rem;
                }
                .flamegraph-stat-label {
                    color: var(--text-muted, #71767b);
                }
                .flamegraph-stat-value {
                    font-weight: 600;
                    color: var(--text-primary, #e7e9ea);
                }
            </style>
            <div class="flamegraph-container">
                <div class="flamegraph-header">
                    <div class="flamegraph-title">
                        <span>&#128293;</span>
                        <span>CPU Flamegraph</span>
                    </div>
                    <div class="flamegraph-controls">
                        <input type="text" id="search-input" placeholder="Search functions...">
                        <select id="profile-select">
                            <option value="cpu">CPU</option>
                            <option value="alloc">Allocations</option>
                            <option value="wall">Wall Time</option>
                        </select>
                        <button id="reset-btn">Reset Zoom</button>
                        <button id="refresh-btn">Refresh</button>
                    </div>
                </div>
                <div class="flamegraph-body">
                    <div class="flamegraph-chart" id="chart"></div>
                </div>
                <div class="flamegraph-stats" id="stats" style="display: none;">
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Samples:</span>
                        <span class="flamegraph-stat-value" id="stat-samples">0</span>
                    </div>
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Duration:</span>
                        <span class="flamegraph-stat-value" id="stat-duration">0s</span>
                    </div>
                    <div class="flamegraph-stat">
                        <span class="flamegraph-stat-label">Top Function:</span>
                        <span class="flamegraph-stat-value" id="stat-top">-</span>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        const searchInput = this.querySelector('#search-input');
        const profileSelect = this.querySelector('#profile-select');
        const resetBtn = this.querySelector('#reset-btn');
        const refreshBtn = this.querySelector('#refresh-btn');

        searchInput?.addEventListener('input', (e) => {
            if (this.chart) {
                this.chart.search(e.target.value);
            }
        });

        profileSelect?.addEventListener('change', (e) => {
            this.setAttribute('profile-type', e.target.value);
        });

        resetBtn?.addEventListener('click', () => {
            if (this.chart) {
                this.chart.resetZoom();
            }
        });

        refreshBtn?.addEventListener('click', () => {
            this.loadData();
        });
    }

    async loadData() {
        const chartEl = this.querySelector('#chart');
        if (!chartEl) return;

        chartEl.innerHTML = `
            <div class="flamegraph-loading">
                <div class="spinner"></div>
                <span>Loading profile data...</span>
            </div>
        `;

        try {
            const params = new URLSearchParams({
                type: this.profileType,
                range: this.timeRange
            });
            if (this.service) {
                params.append('service', this.service);
            }

            const resp = await fetch(`/api/profile/flamegraph?${params}`);

            if (!resp.ok) {
                // Generate demo data if API not available
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }

            await this.renderChart();
        } catch (e) {
            console.error('Failed to load flamegraph:', e);
            // Use demo data on error
            this.data = this.generateDemoData();
            await this.renderChart();
        }
    }

    generateDemoData() {
        // Generate realistic-looking flamegraph data
        const functions = [
            { name: 'main', children: ['runtime.main', 'http.ListenAndServe'] },
            { name: 'runtime.main', children: ['main.main'] },
            { name: 'main.main', children: ['http.handleRequest', 'db.Query', 'json.Marshal'] },
            { name: 'http.handleRequest', children: ['auth.Validate', 'router.Match', 'handler.Process'] },
            { name: 'handler.Process', children: ['db.Query', 'cache.Get', 'response.Write'] },
            { name: 'db.Query', children: ['sql.Query', 'driver.Exec'] },
            { name: 'sql.Query', children: ['driver.Prepare', 'driver.Exec', 'rows.Scan'] },
            { name: 'driver.Exec', children: ['net.Write', 'net.Read', 'protocol.Parse'] },
            { name: 'cache.Get', children: ['redis.Get', 'lru.Lookup'] },
            { name: 'json.Marshal', children: ['reflect.ValueOf', 'encoding.Write'] },
            { name: 'auth.Validate', children: ['jwt.Parse', 'crypto.Verify'] },
            { name: 'runtime.gcBgMarkWorker', children: ['runtime.gcDrain', 'runtime.scanobject'] },
            { name: 'runtime.gcDrain', children: ['runtime.markroot', 'runtime.scanobject'] },
        ];

        const buildNode = (name, depth = 0) => {
            const baseValue = Math.floor(Math.random() * 1000) + 100;
            const funcDef = functions.find(f => f.name === name);

            const node = {
                name: name,
                value: baseValue,
                children: []
            };

            if (depth < 6 && funcDef && funcDef.children) {
                node.children = funcDef.children
                    .filter(() => Math.random() > 0.3)
                    .map(childName => buildNode(childName, depth + 1));
            } else if (depth < 6 && Math.random() > 0.5) {
                const randomFuncs = ['syscall.Read', 'syscall.Write', 'runtime.mallocgc',
                    'sync.Lock', 'sync.Unlock', 'channel.send', 'channel.recv'];
                const numChildren = Math.floor(Math.random() * 3);
                for (let i = 0; i < numChildren; i++) {
                    const childName = randomFuncs[Math.floor(Math.random() * randomFuncs.length)];
                    node.children.push({
                        name: `${childName}.${Math.floor(Math.random() * 100)}`,
                        value: Math.floor(Math.random() * baseValue * 0.5),
                        children: []
                    });
                }
            }

            return node;
        };

        return {
            name: 'root',
            value: 10000,
            children: [
                buildNode('main'),
                buildNode('runtime.gcBgMarkWorker'),
                { name: 'runtime.sysmon', value: 500, children: [] }
            ]
        };
    }

    async renderChart() {
        const chartEl = this.querySelector('#chart');
        if (!chartEl || !this.data) return;

        // Ensure d3 and flamegraph are loaded
        if (!window.d3 || !window.flamegraph) {
            if (window.LibLoader) {
                await window.LibLoader.loadAll(['d3', 'flamegraph', 'flamegraph-css']);
            } else {
                chartEl.innerHTML = `
                    <div class="flamegraph-empty">
                        <span>D3/Flamegraph libraries not loaded</span>
                    </div>
                `;
                return;
            }
        }

        chartEl.innerHTML = '';

        const width = chartEl.clientWidth || 800;
        const cellHeight = 18;

        try {
            this.chart = flamegraph()
                .width(width)
                .cellHeight(cellHeight)
                .transitionDuration(300)
                .minFrameSize(2)
                .transitionEase(d3.easeCubic)
                .sort(true)
                .title('')
                .onClick((d) => {
                    this.dispatchEvent(new CustomEvent('frame-click', {
                        detail: { name: d.data.name, value: d.data.value }
                    }));
                })
                .setColorMapper((d, originalColor) => {
                    // Color based on function type
                    const name = d.data.name.toLowerCase();
                    if (name.includes('runtime.gc') || name.includes('runtime.malloc')) {
                        return '#e74c3c'; // Red for GC
                    } else if (name.includes('syscall') || name.includes('net.')) {
                        return '#3498db'; // Blue for syscalls/network
                    } else if (name.includes('sql') || name.includes('db.') || name.includes('redis')) {
                        return '#9b59b6'; // Purple for database
                    } else if (name.includes('http') || name.includes('handler')) {
                        return '#2ecc71'; // Green for HTTP
                    } else if (name.includes('json') || name.includes('encoding')) {
                        return '#f39c12'; // Orange for serialization
                    }
                    return originalColor;
                });

            d3.select(chartEl)
                .datum(this.data)
                .call(this.chart);

            // Update stats
            this.updateStats();
        } catch (e) {
            console.error('Failed to render flamegraph:', e);
            chartEl.innerHTML = `
                <div class="flamegraph-empty">
                    <span>Failed to render flamegraph</span>
                </div>
            `;
        }
    }

    updateStats() {
        const statsEl = this.querySelector('#stats');
        if (!statsEl || !this.data) return;

        statsEl.style.display = 'flex';

        const countSamples = (node) => {
            let count = node.value || 0;
            if (node.children) {
                for (const child of node.children) {
                    count += countSamples(child);
                }
            }
            return count;
        };

        const findTop = (node, top = null) => {
            if (!top || node.value > top.value) {
                top = node;
            }
            if (node.children) {
                for (const child of node.children) {
                    top = findTop(child, top);
                }
            }
            return top;
        };

        const samples = countSamples(this.data);
        const topFunc = findTop(this.data);

        this.querySelector('#stat-samples').textContent = samples.toLocaleString();
        this.querySelector('#stat-duration').textContent = this.timeRange;
        this.querySelector('#stat-top').textContent = topFunc.name || '-';
    }

    updateChart() {
        if (this.chart && this.data) {
            const chartEl = this.querySelector('#chart');
            if (chartEl) {
                const width = chartEl.clientWidth || 800;
                this.chart.width(width);
                d3.select(chartEl).datum(this.data).call(this.chart);
            }
        }
    }
}

customElements.define('cpu-flamegraph', CpuFlamegraph);

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
    }

    connectedCallback() {
        this.render();
        this.loadDependencies();
    }

    disconnectedCallback() {
        if (this.simulation) {
            this.simulation.stop();
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
        if (this.alpha < 0.001) return;

        this.alpha *= 0.99;

        // Apply forces
        this.applyForces();

        // Update positions in SVG
        this.updatePositions();

        requestAnimationFrame(() => this.animate());
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

/**
 * Deploy Timeline Widget
 * Shows recent deployments with correlation to incidents/metrics
 */
class DeployTimeline extends HTMLElement {
    constructor() {
        super();
        this.deploys = [];
        this.loading = true;
        this.selectedDeploy = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.render();

        try {
            const resp = await fetch('/api/deploys?limit=50');
            if (resp.ok) {
                this.deploys = await resp.json() || [];
            }
        } catch (e) {
            console.error('Failed to load deploy data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    selectDeploy(deployId) {
        this.selectedDeploy = this.deploys.find(d => d.id === deployId) || null;
        this.render();
    }

    closeDetail() {
        this.selectedDeploy = null;
        this.render();
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="deploy-timeline">
                    <div class="deploy-header">
                        <span class="title-icon">🚀</span>
                        <span>Deployments</span>
                    </div>
                    <div class="loading">Loading deployments...</div>
                </div>
            `;
            return;
        }

        const grouped = this.groupByDate();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="deploy-timeline">
                <div class="deploy-header">
                    <div class="header-title">
                        <span class="title-icon">🚀</span>
                        <span>Deployments</span>
                    </div>
                    <button class="btn-refresh" onclick="this.getRootNode().host.loadData()">↻</button>
                </div>

                ${this.selectedDeploy ? this.renderDetail() : ''}

                <div class="timeline-content ${this.selectedDeploy ? 'with-detail' : ''}">
                    ${this.deploys.length === 0 ? `
                        <div class="empty-state">No deployments recorded</div>
                    ` : Object.entries(grouped).map(([date, deploys]) => `
                        <div class="date-group">
                            <div class="date-header">${date}</div>
                            <div class="deploys-list">
                                ${deploys.map(d => this.renderDeploy(d)).join('')}
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderDeploy(deploy) {
        const statusClass = deploy.status === 'success' ? 'success' :
                           deploy.status === 'failed' ? 'failed' :
                           deploy.status === 'rolled_back' ? 'rolled-back' : 'pending';
        const statusIcon = deploy.status === 'success' ? '✓' :
                          deploy.status === 'failed' ? '✗' :
                          deploy.status === 'rolled_back' ? '↩' : '○';

        const hasIncident = deploy.incident_count > 0;
        const time = deploy.timestamp ? new Date(deploy.timestamp).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'}) : '';

        return `
            <div class="deploy-item ${statusClass} ${hasIncident ? 'has-incident' : ''}"
                 onclick="this.getRootNode().host.selectDeploy('${deploy.id}')">
                <div class="deploy-time">${time}</div>
                <div class="deploy-line">
                    <div class="deploy-dot">
                        <span class="dot-icon">${statusIcon}</span>
                    </div>
                    <div class="deploy-connector"></div>
                </div>
                <div class="deploy-info">
                    <div class="deploy-service">${this.escapeHtml(deploy.service)}</div>
                    <div class="deploy-version">
                        ${deploy.from_version ? `${this.escapeHtml(deploy.from_version)} → ` : ''}${this.escapeHtml(deploy.version || deploy.to_version || 'unknown')}
                    </div>
                    <div class="deploy-meta">
                        ${deploy.author ? `<span class="author">by ${this.escapeHtml(deploy.author)}</span>` : ''}
                        ${hasIncident ? `<span class="incident-badge">⚠ ${deploy.incident_count} incident${deploy.incident_count > 1 ? 's' : ''}</span>` : ''}
                    </div>
                </div>
            </div>
        `;
    }

    renderDetail() {
        const d = this.selectedDeploy;
        const time = d.timestamp ? new Date(d.timestamp).toLocaleString() : 'Unknown';

        return `
            <div class="detail-panel">
                <div class="detail-header">
                    <h3>Deploy Details</h3>
                    <button class="btn-close" onclick="this.getRootNode().host.closeDetail()">×</button>
                </div>
                <div class="detail-body">
                    <div class="detail-row">
                        <span class="detail-label">Service</span>
                        <span class="detail-value">${this.escapeHtml(d.service)}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Version</span>
                        <span class="detail-value">${this.escapeHtml(d.version || d.to_version || 'unknown')}</span>
                    </div>
                    ${d.from_version ? `
                        <div class="detail-row">
                            <span class="detail-label">Previous</span>
                            <span class="detail-value">${this.escapeHtml(d.from_version)}</span>
                        </div>
                    ` : ''}
                    <div class="detail-row">
                        <span class="detail-label">Status</span>
                        <span class="detail-value status-${d.status}">${d.status}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Time</span>
                        <span class="detail-value">${time}</span>
                    </div>
                    ${d.author ? `
                        <div class="detail-row">
                            <span class="detail-label">Author</span>
                            <span class="detail-value">${this.escapeHtml(d.author)}</span>
                        </div>
                    ` : ''}
                    ${d.commit ? `
                        <div class="detail-row">
                            <span class="detail-label">Commit</span>
                            <span class="detail-value code">${this.escapeHtml(d.commit.substring(0, 8))}</span>
                        </div>
                    ` : ''}
                    ${d.duration_seconds ? `
                        <div class="detail-row">
                            <span class="detail-label">Duration</span>
                            <span class="detail-value">${d.duration_seconds}s</span>
                        </div>
                    ` : ''}

                    ${d.changes && d.changes.length > 0 ? `
                        <div class="detail-section">
                            <h4>Changes</h4>
                            <ul class="changes-list">
                                ${d.changes.slice(0, 5).map(c => `<li>${this.escapeHtml(c)}</li>`).join('')}
                            </ul>
                        </div>
                    ` : ''}

                    ${d.incident_count > 0 ? `
                        <div class="detail-section incidents">
                            <h4>⚠ Related Incidents</h4>
                            <p>This deploy may have caused ${d.incident_count} incident${d.incident_count > 1 ? 's' : ''}</p>
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    groupByDate() {
        const groups = {};
        const today = new Date().toDateString();
        const yesterday = new Date(Date.now() - 86400000).toDateString();

        for (const deploy of this.deploys) {
            const date = deploy.timestamp ? new Date(deploy.timestamp).toDateString() : 'Unknown';
            let label = date;
            if (date === today) label = 'Today';
            else if (date === yesterday) label = 'Yesterday';

            if (!groups[label]) groups[label] = [];
            groups[label].push(deploy);
        }

        return groups;
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .deploy-timeline {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .deploy-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.25rem 0.5rem;
                cursor: pointer;
            }

            .loading, .empty-state {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            .timeline-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .timeline-content.with-detail {
                max-height: 40%;
            }

            .date-group {
                margin-bottom: 1.5rem;
            }

            .date-header {
                font-size: 0.75rem;
                font-weight: 600;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
                margin-bottom: 0.75rem;
                padding-left: 3.5rem;
            }

            .deploys-list {
                display: flex;
                flex-direction: column;
            }

            .deploy-item {
                display: grid;
                grid-template-columns: 3rem 1.5rem 1fr;
                gap: 0.5rem;
                padding: 0.5rem 0;
                cursor: pointer;
                transition: background 0.15s;
                border-radius: 4px;
            }

            .deploy-item:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .deploy-time {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                text-align: right;
                padding-top: 0.2rem;
            }

            .deploy-line {
                display: flex;
                flex-direction: column;
                align-items: center;
            }

            .deploy-dot {
                width: 20px;
                height: 20px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.7rem;
                font-weight: bold;
                flex-shrink: 0;
            }

            .deploy-item.success .deploy-dot {
                background: rgba(0, 186, 124, 0.2);
                color: var(--success, #00ba7c);
            }

            .deploy-item.failed .deploy-dot {
                background: rgba(244, 33, 46, 0.2);
                color: var(--error, #f4212e);
            }

            .deploy-item.rolled-back .deploy-dot {
                background: rgba(255, 212, 0, 0.2);
                color: var(--warning, #ffd400);
            }

            .deploy-item.pending .deploy-dot {
                background: var(--bg-elevated, #1e2128);
                color: var(--text-muted, #71767b);
                border: 1px solid var(--border, #2f3336);
            }

            .deploy-connector {
                width: 2px;
                flex: 1;
                min-height: 20px;
                background: var(--border, #2f3336);
            }

            .deploy-item:last-child .deploy-connector {
                display: none;
            }

            .deploy-info {
                padding-top: 0.1rem;
            }

            .deploy-service {
                font-weight: 500;
                font-size: 0.9rem;
            }

            .deploy-version {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                font-family: monospace;
            }

            .deploy-meta {
                display: flex;
                gap: 0.75rem;
                margin-top: 0.25rem;
                font-size: 0.7rem;
            }

            .author {
                color: var(--text-muted, #71767b);
            }

            .incident-badge {
                color: var(--warning, #ffd400);
            }

            .deploy-item.has-incident {
                background: rgba(255, 212, 0, 0.05);
            }

            /* Detail panel */
            .detail-panel {
                border-bottom: 1px solid var(--border, #2f3336);
                background: var(--bg-elevated, #1e2128);
            }

            .detail-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .detail-header h3 {
                margin: 0;
                font-size: 0.9rem;
            }

            .btn-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.25rem;
                cursor: pointer;
                padding: 0 0.25rem;
            }

            .detail-body {
                padding: 1rem;
                max-height: 250px;
                overflow-y: auto;
            }

            .detail-row {
                display: flex;
                justify-content: space-between;
                padding: 0.4rem 0;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .detail-label {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .detail-value {
                font-size: 0.8rem;
                font-weight: 500;
            }

            .detail-value.code {
                font-family: monospace;
            }

            .detail-value.status-success { color: var(--success, #00ba7c); }
            .detail-value.status-failed { color: var(--error, #f4212e); }
            .detail-value.status-rolled_back { color: var(--warning, #ffd400); }

            .detail-section {
                margin-top: 1rem;
                padding-top: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .detail-section h4 {
                margin: 0 0 0.5rem 0;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .detail-section.incidents {
                background: rgba(255, 212, 0, 0.1);
                margin: 1rem -1rem -1rem -1rem;
                padding: 1rem;
            }

            .detail-section.incidents h4 {
                color: var(--warning, #ffd400);
            }

            .detail-section.incidents p {
                margin: 0;
                font-size: 0.8rem;
            }

            .changes-list {
                margin: 0;
                padding-left: 1.25rem;
                font-size: 0.75rem;
            }

            .changes-list li {
                margin-bottom: 0.25rem;
            }
        `;
    }
}

customElements.define('deploy-timeline', DeployTimeline);

/**
 * Diff View Component
 * Side-by-side comparison of two time periods
 */
class DiffView extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.charts = [];
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        this.charts.forEach(c => c.destroy());
    }

    static get observedAttributes() {
        return ['metric', 'period1', 'period2'];
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get period1() { return this.getAttribute('period1') || 'today'; }
    get period2() { return this.getAttribute('period2') || 'yesterday'; }

    render() {
        this.innerHTML = `
            <style>
                .diff-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .diff-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .diff-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .diff-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .diff-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .diff-summary {
                    display: flex;
                    gap: 2rem;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .diff-stat {
                    flex: 1;
                    text-align: center;
                    padding: 0.75rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-radius: 6px;
                }
                .diff-stat-label {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .diff-stat-value {
                    font-size: 1.25rem;
                    font-weight: 700;
                    margin-top: 0.25rem;
                }
                .diff-stat-change {
                    font-size: 0.8rem;
                    margin-top: 0.25rem;
                }
                .diff-stat-change.positive { color: #22c55e; }
                .diff-stat-change.negative { color: #f43f5e; }
                .diff-stat-change.neutral { color: var(--text-muted, #71767b); }
                .diff-charts {
                    flex: 1;
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: 1px;
                    background: var(--border-color, #2f3336);
                    min-height: 200px;
                }
                .diff-chart-panel {
                    background: var(--bg-card, #16181c);
                    padding: 1rem;
                    display: flex;
                    flex-direction: column;
                }
                .diff-chart-title {
                    font-size: 0.8rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.5rem;
                    text-align: center;
                }
                .diff-chart {
                    flex: 1;
                }
            </style>
            <div class="diff-container">
                <div class="diff-header">
                    <div class="diff-title">Period Comparison</div>
                    <div class="diff-controls">
                        <select id="period1-select">
                            <option value="today">Today</option>
                            <option value="this-week">This Week</option>
                            <option value="this-month">This Month</option>
                        </select>
                        <span style="color: var(--text-muted)">vs</span>
                        <select id="period2-select">
                            <option value="yesterday">Yesterday</option>
                            <option value="last-week">Last Week</option>
                            <option value="last-month">Last Month</option>
                        </select>
                    </div>
                </div>
                <div class="diff-summary">
                    <div class="diff-stat">
                        <div class="diff-stat-label">Average Latency</div>
                        <div class="diff-stat-value" id="stat-latency">-</div>
                        <div class="diff-stat-change" id="stat-latency-change">-</div>
                    </div>
                    <div class="diff-stat">
                        <div class="diff-stat-label">Error Rate</div>
                        <div class="diff-stat-value" id="stat-errors">-</div>
                        <div class="diff-stat-change" id="stat-errors-change">-</div>
                    </div>
                    <div class="diff-stat">
                        <div class="diff-stat-label">Throughput</div>
                        <div class="diff-stat-value" id="stat-throughput">-</div>
                        <div class="diff-stat-change" id="stat-throughput-change">-</div>
                    </div>
                </div>
                <div class="diff-charts">
                    <div class="diff-chart-panel">
                        <div class="diff-chart-title" id="title1">Period 1</div>
                        <canvas class="diff-chart" id="chart1"></canvas>
                    </div>
                    <div class="diff-chart-panel">
                        <div class="diff-chart-title" id="title2">Period 2</div>
                        <canvas class="diff-chart" id="chart2"></canvas>
                    </div>
                </div>
            </div>
        `;

        this.querySelector('#period1-select')?.addEventListener('change', (e) => {
            this.setAttribute('period1', e.target.value);
            this.loadData();
        });

        this.querySelector('#period2-select')?.addEventListener('change', (e) => {
            this.setAttribute('period2', e.target.value);
            this.loadData();
        });
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/compare?period1=${this.period1}&period2=${this.period2}&metric=${this.metric}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateDisplay();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateDisplay();
        }
    }

    generateDemoData() {
        const generateSeries = (baseValue, variance) => {
            const points = 24;
            return Array.from({ length: points }, (_, i) => ({
                timestamp: Date.now() - (points - i) * 3600000,
                value: baseValue + (Math.random() - 0.5) * variance
            }));
        };

        return {
            period1: {
                label: 'Today',
                series: generateSeries(65, 30),
                stats: { latency: 65, errors: 0.8, throughput: 1250 }
            },
            period2: {
                label: 'Yesterday',
                series: generateSeries(55, 25),
                stats: { latency: 55, errors: 0.5, throughput: 1100 }
            }
        };
    }

    async updateDisplay() {
        if (!this.data) return;

        const { period1, period2 } = this.data;

        // Update titles
        this.querySelector('#title1').textContent = period1.label;
        this.querySelector('#title2').textContent = period2.label;

        // Update stats with diff
        this.updateStat('latency', period1.stats.latency, period2.stats.latency, 'ms', true);
        this.updateStat('errors', period1.stats.errors, period2.stats.errors, '%', true);
        this.updateStat('throughput', period1.stats.throughput, period2.stats.throughput, '/s', false);

        // Render charts
        await this.renderCharts();
    }

    updateStat(name, current, previous, unit, lowerIsBetter) {
        const valueEl = this.querySelector(`#stat-${name}`);
        const changeEl = this.querySelector(`#stat-${name}-change`);

        valueEl.textContent = current.toFixed(1) + unit;

        const diff = current - previous;
        const pct = ((diff / previous) * 100).toFixed(1);

        if (Math.abs(diff) < 0.1) {
            changeEl.textContent = 'No change';
            changeEl.className = 'diff-stat-change neutral';
        } else if (diff > 0) {
            changeEl.textContent = `+${pct}% vs previous`;
            changeEl.className = `diff-stat-change ${lowerIsBetter ? 'negative' : 'positive'}`;
        } else {
            changeEl.textContent = `${pct}% vs previous`;
            changeEl.className = `diff-stat-change ${lowerIsBetter ? 'positive' : 'negative'}`;
        }
    }

    async renderCharts() {
        if (!window.Chart && window.LibLoader) {
            await window.LibLoader.loadAll(['chart', 'chart-date']);
        }

        this.charts.forEach(c => c.destroy());
        this.charts = [];

        const { period1, period2 } = this.data;

        [
            { canvas: '#chart1', data: period1, color: '#3b82f6' },
            { canvas: '#chart2', data: period2, color: '#22c55e' }
        ].forEach(({ canvas, data, color }) => {
            const el = this.querySelector(canvas);
            if (!el) return;

            const chart = new Chart(el.getContext('2d'), {
                type: 'line',
                data: {
                    labels: data.series.map(d => new Date(d.timestamp)),
                    datasets: [{
                        data: data.series.map(d => d.value),
                        borderColor: color,
                        backgroundColor: color + '20',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: { legend: { display: false } },
                    scales: {
                        x: {
                            type: 'time',
                            display: false,
                        },
                        y: {
                            grid: { color: 'rgba(255,255,255,0.05)' },
                            ticks: { color: '#71767b' }
                        }
                    }
                }
            });

            this.charts.push(chart);
        });
    }
}

customElements.define('diff-view', DiffView);

/**
 * Error Budget Burndown Component
 * SLO visualization showing budget remaining over time
 */
class ErrorBudget extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        if (this.chart) this.chart.destroy();
    }

    static get observedAttributes() {
        return ['slo-id', 'window'];
    }

    get sloId() { return this.getAttribute('slo-id') || ''; }
    get window() { return this.getAttribute('window') || '30d'; }

    render() {
        this.innerHTML = `
            <style>
                .budget-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .budget-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .budget-summary {
                    display: flex;
                    gap: 2rem;
                    padding: 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-stat {
                    text-align: center;
                }
                .budget-stat-label {
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.25rem;
                }
                .budget-stat-value {
                    font-size: 1.5rem;
                    font-weight: 700;
                }
                .budget-stat-value.good { color: #22c55e; }
                .budget-stat-value.warning { color: #f59e0b; }
                .budget-stat-value.critical { color: #f43f5e; }
                .budget-progress {
                    padding: 0.75rem 1rem;
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .budget-progress-bar {
                    height: 8px;
                    background: var(--border-color, #2f3336);
                    border-radius: 4px;
                    overflow: hidden;
                }
                .budget-progress-fill {
                    height: 100%;
                    border-radius: 4px;
                    transition: width 0.5s ease;
                }
                .budget-progress-labels {
                    display: flex;
                    justify-content: space-between;
                    margin-top: 0.5rem;
                    font-size: 0.75rem;
                    color: var(--text-muted, #71767b);
                }
                .budget-chart {
                    flex: 1;
                    padding: 1rem;
                    min-height: 150px;
                }
                .budget-chart canvas {
                    width: 100%;
                    height: 100%;
                }
                .budget-footer {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .budget-footer-item {
                    display: flex;
                    gap: 0.5rem;
                }
                .budget-footer-label {
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="budget-container">
                <div class="budget-header">
                    <div class="budget-title">Error Budget - <span id="slo-name">SLO</span></div>
                </div>
                <div class="budget-summary">
                    <div class="budget-stat">
                        <div class="budget-stat-label">Budget Remaining</div>
                        <div class="budget-stat-value" id="remaining">--</div>
                    </div>
                    <div class="budget-stat">
                        <div class="budget-stat-label">Burn Rate</div>
                        <div class="budget-stat-value" id="burn-rate">--</div>
                    </div>
                    <div class="budget-stat">
                        <div class="budget-stat-label">Time Left</div>
                        <div class="budget-stat-value" id="time-left">--</div>
                    </div>
                </div>
                <div class="budget-progress">
                    <div class="budget-progress-bar">
                        <div class="budget-progress-fill" id="progress-fill"></div>
                    </div>
                    <div class="budget-progress-labels">
                        <span>0%</span>
                        <span>Budget consumed</span>
                        <span>100%</span>
                    </div>
                </div>
                <div class="budget-chart">
                    <canvas id="chart"></canvas>
                </div>
                <div class="budget-footer">
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Target:</span>
                        <span id="target">99.9%</span>
                    </div>
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Current:</span>
                        <span id="current">99.85%</span>
                    </div>
                    <div class="budget-footer-item">
                        <span class="budget-footer-label">Window:</span>
                        <span id="window">30 days</span>
                    </div>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/slos/${this.sloId}/budget?window=${this.window}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateDisplay();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateDisplay();
        }
    }

    generateDemoData() {
        const days = 30;
        const burndown = [];
        let budget = 100;

        for (let i = 0; i < days; i++) {
            const dailyBurn = Math.random() * 5;
            budget = Math.max(0, budget - dailyBurn);
            burndown.push({
                date: Date.now() - (days - i) * 86400000,
                remaining: budget
            });
        }

        return {
            name: 'API Availability',
            target: 99.9,
            current: 99.85,
            budgetRemaining: budget,
            budgetConsumed: 100 - budget,
            burnRate: 1.2,
            timeToExhaustion: budget / 1.2,
            burndown,
            window: '30d'
        };
    }

    async updateDisplay() {
        if (!this.data) return;

        const { name, target, current, budgetRemaining, budgetConsumed, burnRate, timeToExhaustion, burndown } = this.data;

        // Update text
        this.querySelector('#slo-name').textContent = name;
        this.querySelector('#target').textContent = target + '%';
        this.querySelector('#current').textContent = current + '%';
        this.querySelector('#window').textContent = this.window;

        // Budget remaining with color
        const remainingEl = this.querySelector('#remaining');
        remainingEl.textContent = budgetRemaining.toFixed(1) + '%';
        remainingEl.className = 'budget-stat-value ' +
            (budgetRemaining > 50 ? 'good' : budgetRemaining > 20 ? 'warning' : 'critical');

        // Burn rate
        const burnRateEl = this.querySelector('#burn-rate');
        burnRateEl.textContent = burnRate.toFixed(1) + 'x';
        burnRateEl.className = 'budget-stat-value ' +
            (burnRate < 1 ? 'good' : burnRate < 2 ? 'warning' : 'critical');

        // Time left
        const timeLeftEl = this.querySelector('#time-left');
        if (timeToExhaustion > 30) {
            timeLeftEl.textContent = '>30d';
            timeLeftEl.className = 'budget-stat-value good';
        } else {
            timeLeftEl.textContent = timeToExhaustion.toFixed(0) + 'd';
            timeLeftEl.className = 'budget-stat-value ' +
                (timeToExhaustion > 10 ? 'warning' : 'critical');
        }

        // Progress bar
        const progressFill = this.querySelector('#progress-fill');
        progressFill.style.width = budgetConsumed + '%';
        progressFill.style.background = budgetConsumed < 50 ? '#22c55e' :
            budgetConsumed < 80 ? '#f59e0b' : '#f43f5e';

        // Chart
        await this.renderChart(burndown);
    }

    async renderChart(burndown) {
        const canvas = this.querySelector('#chart');
        if (!canvas) return;

        if (!window.Chart && window.LibLoader) {
            await window.LibLoader.loadAll(['chart', 'chart-date']);
        }

        if (this.chart) this.chart.destroy();

        const ctx = canvas.getContext('2d');

        // Ideal burndown line
        const idealLine = burndown.map((_, i) =>
            100 - (100 / burndown.length) * i
        );

        this.chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: burndown.map(d => new Date(d.date)),
                datasets: [
                    {
                        label: 'Actual',
                        data: burndown.map(d => d.remaining),
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        fill: true,
                        tension: 0.3,
                    },
                    {
                        label: 'Ideal',
                        data: idealLine,
                        borderColor: '#71767b',
                        borderDash: [5, 5],
                        fill: false,
                        pointRadius: 0,
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        display: true,
                        position: 'top',
                        labels: { color: '#71767b', boxWidth: 12 }
                    }
                },
                scales: {
                    x: {
                        type: 'time',
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b', maxTicksLimit: 5 }
                    },
                    y: {
                        min: 0,
                        max: 100,
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: {
                            color: '#71767b',
                            callback: v => v + '%'
                        }
                    }
                }
            }
        });
    }
}

customElements.define('error-budget', ErrorBudget);

/**
 * Geo Map Component
 * Geographic visualization of traffic/errors by region
 */
class GeoMap extends HTMLElement {
    constructor() {
        super();
        this.data = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    static get observedAttributes() {
        return ['metric', 'time-range'];
    }

    get metric() { return this.getAttribute('metric') || 'requests'; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .geomap-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .geomap-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .geomap-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .geomap-body {
                    flex: 1;
                    position: relative;
                    min-height: 300px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                }
                .geomap-svg {
                    width: 100%;
                    height: 100%;
                }
                .geomap-region {
                    fill: var(--bg-elevated, #1a1f2e);
                    stroke: var(--border-color, #2f3336);
                    stroke-width: 0.5;
                    transition: fill 0.2s ease;
                }
                .geomap-region:hover {
                    stroke: var(--color-info, #1d9bf0);
                    stroke-width: 1;
                }
                .geomap-dot {
                    cursor: pointer;
                    transition: r 0.2s ease;
                }
                .geomap-dot:hover {
                    r: 12;
                }
                .geomap-tooltip {
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
                .geomap-legend {
                    position: absolute;
                    bottom: 1rem;
                    left: 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.75rem;
                }
                .geomap-legend-gradient {
                    width: 100px;
                    height: 8px;
                    background: linear-gradient(to right, #22c55e, #f59e0b, #f43f5e);
                    border-radius: 4px;
                    margin-bottom: 0.5rem;
                }
                .geomap-legend-labels {
                    display: flex;
                    justify-content: space-between;
                    color: var(--text-muted, #71767b);
                }
                .geomap-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .geomap-stat {
                    display: flex;
                    gap: 0.5rem;
                }
                .geomap-stat-label {
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="geomap-container">
                <div class="geomap-header">
                    <div class="geomap-title">
                        <span>&#127758;</span>
                        <span>Geographic Distribution</span>
                    </div>
                </div>
                <div class="geomap-body" id="body">
                    <svg class="geomap-svg" id="svg" viewBox="0 0 800 400"></svg>
                    <div class="geomap-legend">
                        <div class="geomap-legend-gradient"></div>
                        <div class="geomap-legend-labels">
                            <span>Low</span>
                            <span>High</span>
                        </div>
                    </div>
                </div>
                <div class="geomap-stats">
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Total Requests:</span>
                        <span id="stat-total">0</span>
                    </div>
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Regions:</span>
                        <span id="stat-regions">0</span>
                    </div>
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Top Region:</span>
                        <span id="stat-top">-</span>
                    </div>
                </div>
                <div class="geomap-tooltip" id="tooltip"></div>
            </div>
        `;

        this.renderMap();
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/geo/distribution?metric=${this.metric}&range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateMap();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateMap();
        }
    }

    generateDemoData() {
        return {
            regions: [
                { code: 'US-W', name: 'US West', lat: 37.7749, lng: -122.4194, requests: 45000, errors: 120, latency: 45 },
                { code: 'US-E', name: 'US East', lat: 40.7128, lng: -74.0060, requests: 38000, errors: 95, latency: 52 },
                { code: 'EU-W', name: 'EU West', lat: 51.5074, lng: -0.1278, requests: 28000, errors: 85, latency: 120 },
                { code: 'EU-C', name: 'EU Central', lat: 52.5200, lng: 13.4050, requests: 22000, errors: 65, latency: 115 },
                { code: 'APAC', name: 'Asia Pacific', lat: 35.6762, lng: 139.6503, requests: 18000, errors: 45, latency: 180 },
                { code: 'SA', name: 'South America', lat: -23.5505, lng: -46.6333, requests: 8000, errors: 25, latency: 200 },
                { code: 'AU', name: 'Australia', lat: -33.8688, lng: 151.2093, requests: 5000, errors: 12, latency: 220 },
            ],
            total: 164000
        };
    }

    renderMap() {
        const svg = this.querySelector('#svg');
        if (!svg) return;

        // Simple world map outline (simplified paths)
        svg.innerHTML = `
            <defs>
                <radialGradient id="dotGradient">
                    <stop offset="0%" stop-color="rgba(59, 130, 246, 0.8)"/>
                    <stop offset="100%" stop-color="rgba(59, 130, 246, 0.2)"/>
                </radialGradient>
            </defs>
            <g id="regions"></g>
            <g id="dots"></g>
        `;
    }

    updateMap() {
        if (!this.data) return;

        const svg = this.querySelector('#svg');
        const dotsGroup = svg.querySelector('#dots');
        const tooltip = this.querySelector('#tooltip');

        if (!dotsGroup) return;

        const { regions, total } = this.data;
        const maxRequests = Math.max(...regions.map(r => r.requests));

        // Convert lat/lng to SVG coordinates (simple equirectangular projection)
        const toX = lng => ((lng + 180) / 360) * 800;
        const toY = lat => ((90 - lat) / 180) * 400;

        dotsGroup.innerHTML = regions.map(r => {
            const x = toX(r.lng);
            const y = toY(r.lat);
            const radius = 5 + (r.requests / maxRequests) * 15;
            const color = this.getHeatColor(r.requests / maxRequests);

            return `
                <circle class="geomap-dot" cx="${x}" cy="${y}" r="${radius}"
                        fill="${color}" fill-opacity="0.7"
                        data-region="${r.code}"/>
            `;
        }).join('');

        // Tooltip events
        dotsGroup.querySelectorAll('.geomap-dot').forEach((dot, i) => {
            const r = regions[i];

            dot.addEventListener('mouseenter', (e) => {
                tooltip.innerHTML = `
                    <div style="font-weight:600;margin-bottom:0.5rem">${r.name}</div>
                    <div>Requests: ${r.requests.toLocaleString()}</div>
                    <div>Errors: ${r.errors} (${(r.errors/r.requests*100).toFixed(2)}%)</div>
                    <div>Avg Latency: ${r.latency}ms</div>
                `;
                tooltip.style.display = 'block';
            });

            dot.addEventListener('mousemove', (e) => {
                tooltip.style.left = (e.clientX + 10) + 'px';
                tooltip.style.top = (e.clientY + 10) + 'px';
            });

            dot.addEventListener('mouseleave', () => {
                tooltip.style.display = 'none';
            });
        });

        // Update stats
        const topRegion = regions.reduce((a, b) => a.requests > b.requests ? a : b);
        this.querySelector('#stat-total').textContent = total.toLocaleString();
        this.querySelector('#stat-regions').textContent = regions.length;
        this.querySelector('#stat-top').textContent = `${topRegion.name} (${(topRegion.requests/total*100).toFixed(0)}%)`;
    }

    getHeatColor(intensity) {
        if (intensity < 0.3) return '#22c55e';
        if (intensity < 0.6) return '#f59e0b';
        return '#f43f5e';
    }
}

customElements.define('geo-map', GeoMap);

/**
 * Histogram Chart Component
 * Shows latency distribution as a bar chart with percentile markers
 */
class HistogramChart extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.chart = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        if (this.chart) this.chart.destroy();
    }

    static get observedAttributes() {
        return ['metric', 'service', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) this.loadData();
    }

    get metric() { return this.getAttribute('metric') || 'latency'; }
    get service() { return this.getAttribute('service') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .histogram-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .histogram-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .histogram-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                }
                .histogram-body {
                    flex: 1;
                    padding: 1rem;
                    min-height: 200px;
                }
                .histogram-canvas {
                    width: 100%;
                    height: 100%;
                }
                .histogram-percentiles {
                    display: flex;
                    justify-content: space-around;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                }
                .histogram-percentile {
                    text-align: center;
                }
                .histogram-percentile-label {
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                    text-transform: uppercase;
                }
                .histogram-percentile-value {
                    font-size: 1.1rem;
                    font-weight: 600;
                    margin-top: 0.25rem;
                }
                .histogram-percentile-value.p50 { color: #22c55e; }
                .histogram-percentile-value.p90 { color: #3b82f6; }
                .histogram-percentile-value.p95 { color: #f59e0b; }
                .histogram-percentile-value.p99 { color: #f43f5e; }
            </style>
            <div class="histogram-container">
                <div class="histogram-header">
                    <div class="histogram-title">Latency Distribution</div>
                </div>
                <div class="histogram-body">
                    <canvas class="histogram-canvas" id="chart"></canvas>
                </div>
                <div class="histogram-percentiles">
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P50</div>
                        <div class="histogram-percentile-value p50" id="p50">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P90</div>
                        <div class="histogram-percentile-value p90" id="p90">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P95</div>
                        <div class="histogram-percentile-value p95" id="p95">-</div>
                    </div>
                    <div class="histogram-percentile">
                        <div class="histogram-percentile-label">P99</div>
                        <div class="histogram-percentile-value p99" id="p99">-</div>
                    </div>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/histogram?metric=${this.metric}&range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.renderChart();
        } catch (e) {
            this.data = this.generateDemoData();
            this.renderChart();
        }
    }

    generateDemoData() {
        // Generate log-normal distribution (realistic for latency)
        const buckets = ['0-5ms', '5-10ms', '10-25ms', '25-50ms', '50-100ms', '100-250ms', '250-500ms', '500ms-1s', '1s+'];
        const counts = [1200, 2800, 4500, 3200, 1800, 800, 300, 100, 50];

        return {
            buckets,
            counts,
            percentiles: { p50: 22, p90: 85, p95: 145, p99: 380 },
            total: counts.reduce((a, b) => a + b, 0)
        };
    }

    async renderChart() {
        const canvas = this.querySelector('#chart');
        if (!canvas || !this.data) return;

        if (!window.Chart && window.LibLoader) {
            await window.LibLoader.load('chart');
        }

        if (this.chart) this.chart.destroy();

        const ctx = canvas.getContext('2d');
        const { buckets, counts, percentiles } = this.data;

        // Create gradient colors
        const colors = counts.map((_, i) => {
            const ratio = i / (counts.length - 1);
            if (ratio < 0.5) return `rgba(34, 197, 94, ${0.6 + ratio * 0.4})`;
            if (ratio < 0.75) return `rgba(59, 130, 246, ${0.6 + (ratio - 0.5) * 0.8})`;
            if (ratio < 0.9) return `rgba(245, 158, 11, ${0.7 + (ratio - 0.75) * 0.6})`;
            return `rgba(244, 63, 94, ${0.8 + (ratio - 0.9) * 0.4})`;
        });

        this.chart = new Chart(ctx, {
            type: 'bar',
            data: {
                labels: buckets,
                datasets: [{
                    data: counts,
                    backgroundColor: colors,
                    borderRadius: 4,
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: (ctx) => {
                                const pct = ((ctx.raw / this.data.total) * 100).toFixed(1);
                                return `${ctx.raw.toLocaleString()} requests (${pct}%)`;
                            }
                        }
                    }
                },
                scales: {
                    x: {
                        grid: { display: false },
                        ticks: { color: '#71767b', font: { size: 10 } }
                    },
                    y: {
                        grid: { color: 'rgba(255,255,255,0.05)' },
                        ticks: { color: '#71767b' }
                    }
                }
            }
        });

        // Update percentiles
        const formatMs = (ms) => ms < 1000 ? `${ms}ms` : `${(ms/1000).toFixed(1)}s`;
        this.querySelector('#p50').textContent = formatMs(percentiles.p50);
        this.querySelector('#p90').textContent = formatMs(percentiles.p90);
        this.querySelector('#p95').textContent = formatMs(percentiles.p95);
        this.querySelector('#p99').textContent = formatMs(percentiles.p99);
    }
}

customElements.define('histogram-chart', HistogramChart);

/**
 * Incidents Timeline Widget
 * Active incidents, timeline, and war room view
 */
class IncidentsTimeline extends HTMLElement {
    constructor() {
        super();
        this.incidents = [];
        this.selectedIncident = null;
        this.filter = 'active'; // active, resolved, all
    }

    connectedCallback() {
        this.render();
        this.loadIncidents();
        this.refreshInterval = setInterval(() => this.loadIncidents(), 30000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) clearInterval(this.refreshInterval);
    }

    async loadIncidents() {
        try {
            const status = this.filter === 'all' ? '' : this.filter;
            const resp = await fetch(`/api/incidents?status=${status}&limit=50`);
            if (resp.ok) {
                this.incidents = await resp.json() || [];
                this.renderContent();
            }
        } catch (e) {
            console.error('Failed to load incidents:', e);
        }
    }

    setFilter(filter) {
        this.filter = filter;
        this.querySelectorAll('.filter-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.filter === filter);
        });
        this.loadIncidents();
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="incidents-timeline">
                <div class="incidents-header">
                    <div class="incidents-title">
                        <span class="title-icon">🚨</span>
                        <span>Incidents</span>
                    </div>
                    <div class="incidents-filters">
                        <button class="filter-btn active" data-filter="active" onclick="this.getRootNode().host.setFilter('active')">Active</button>
                        <button class="filter-btn" data-filter="resolved" onclick="this.getRootNode().host.setFilter('resolved')">Resolved</button>
                        <button class="filter-btn" data-filter="all" onclick="this.getRootNode().host.setFilter('all')">All</button>
                    </div>
                    <button class="btn-create" onclick="this.getRootNode().host.showCreateIncident()">+ Declare</button>
                </div>
                <div class="incidents-content" id="incidents-content">
                    <div class="loading">Loading...</div>
                </div>
                <div class="incident-detail" id="incident-detail" style="display: none;"></div>
            </div>
        `;
    }

    renderContent() {
        const container = this.querySelector('#incidents-content');

        const active = this.incidents.filter(i => i.status === 'active' || i.status === 'investigating' || i.status === 'identified');
        const headerEl = this.querySelector('.incidents-title');
        if (headerEl && active.length > 0) {
            headerEl.innerHTML = `<span class="title-icon">🚨</span><span>Incidents</span><span class="active-badge">${active.length}</span>`;
        }

        if (this.incidents.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <span class="icon">✓</span>
                    <p>${this.filter === 'active' ? 'No active incidents' : 'No incidents found'}</p>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="incidents-list">
                ${this.incidents.map(i => this.renderIncidentCard(i)).join('')}
            </div>
        `;
    }

    renderIncidentCard(incident) {
        const severity = incident.severity || 'medium';
        const status = incident.status || 'active';
        const duration = this.getDuration(incident.created_at, incident.resolved_at);

        return `
            <div class="incident-card severity-${severity}" onclick="this.getRootNode().host.showIncidentDetail('${incident.id}')">
                <div class="incident-severity">
                    <span class="severity-badge ${severity}">${this.getSeverityIcon(severity)}</span>
                </div>
                <div class="incident-main">
                    <div class="incident-header">
                        <span class="incident-title">${this.escapeHtml(incident.title)}</span>
                        <span class="incident-status status-${status}">${status}</span>
                    </div>
                    <div class="incident-meta">
                        ${incident.service ? `<span class="meta-service">${this.escapeHtml(incident.service)}</span>` : ''}
                        <span class="meta-time">${this.formatTime(incident.created_at)}</span>
                        <span class="meta-duration">${duration}</span>
                    </div>
                    ${incident.description ? `<div class="incident-desc">${this.escapeHtml(incident.description.substring(0, 100))}${incident.description.length > 100 ? '...' : ''}</div>` : ''}
                </div>
                <div class="incident-actions">
                    ${status !== 'resolved' ? `
                        <button class="btn-action" onclick="event.stopPropagation(); this.getRootNode().host.updateStatus('${incident.id}', 'investigating')" title="Investigate">🔍</button>
                        <button class="btn-action" onclick="event.stopPropagation(); this.getRootNode().host.resolveIncident('${incident.id}')" title="Resolve">✓</button>
                    ` : ''}
                </div>
            </div>
        `;
    }

    async showIncidentDetail(id) {
        const incident = this.incidents.find(i => i.id === id);
        if (!incident) return;

        this.selectedIncident = incident;

        // Try to load timeline
        let timeline = [];
        try {
            const resp = await fetch(`/api/incidents/${id}/timeline`);
            if (resp.ok) timeline = await resp.json() || [];
        } catch (e) {}

        const detail = this.querySelector('#incident-detail');
        const content = this.querySelector('#incidents-content');

        content.style.display = 'none';
        detail.style.display = 'block';
        detail.innerHTML = this.renderIncidentDetail(incident, timeline);
    }

    renderIncidentDetail(incident, timeline) {
        return `
            <div class="detail-header">
                <button class="btn-back" onclick="this.getRootNode().host.hideDetail()">← Back</button>
                <span class="severity-badge ${incident.severity}">${incident.severity?.toUpperCase()}</span>
                <span class="incident-status status-${incident.status}">${incident.status}</span>
            </div>
            <div class="detail-body">
                <h2 class="detail-title">${this.escapeHtml(incident.title)}</h2>
                <div class="detail-meta">
                    <div class="meta-item">
                        <span class="meta-label">Created</span>
                        <span class="meta-value">${new Date(incident.created_at).toLocaleString()}</span>
                    </div>
                    <div class="meta-item">
                        <span class="meta-label">Service</span>
                        <span class="meta-value">${incident.service || '—'}</span>
                    </div>
                    <div class="meta-item">
                        <span class="meta-label">Duration</span>
                        <span class="meta-value">${this.getDuration(incident.created_at, incident.resolved_at)}</span>
                    </div>
                    ${incident.commander ? `
                        <div class="meta-item">
                            <span class="meta-label">Commander</span>
                            <span class="meta-value">${this.escapeHtml(incident.commander)}</span>
                        </div>
                    ` : ''}
                </div>
                ${incident.description ? `
                    <div class="detail-section">
                        <h3>Description</h3>
                        <p>${this.escapeHtml(incident.description)}</p>
                    </div>
                ` : ''}
                <div class="detail-section">
                    <h3>Timeline</h3>
                    <div class="timeline">
                        ${timeline.length > 0 ? timeline.map(t => `
                            <div class="timeline-item">
                                <div class="timeline-marker"></div>
                                <div class="timeline-content">
                                    <div class="timeline-time">${this.formatTime(t.timestamp)}</div>
                                    <div class="timeline-text">${this.escapeHtml(t.message)}</div>
                                </div>
                            </div>
                        `).join('') : `
                            <div class="timeline-item">
                                <div class="timeline-marker"></div>
                                <div class="timeline-content">
                                    <div class="timeline-time">${this.formatTime(incident.created_at)}</div>
                                    <div class="timeline-text">Incident created</div>
                                </div>
                            </div>
                        `}
                    </div>
                </div>
                <div class="detail-actions">
                    ${incident.status !== 'resolved' ? `
                        <button class="btn-primary" onclick="this.getRootNode().host.resolveIncident('${incident.id}')">Resolve Incident</button>
                    ` : ''}
                    <button class="btn-secondary" onclick="this.getRootNode().host.addTimelineEntry('${incident.id}')">Add Update</button>
                </div>
            </div>
        `;
    }

    hideDetail() {
        const detail = this.querySelector('#incident-detail');
        const content = this.querySelector('#incidents-content');
        detail.style.display = 'none';
        content.style.display = 'block';
    }

    async updateStatus(id, status) {
        try {
            const resp = await fetch(`/api/incidents/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ status })
            });
            if (resp.ok) this.loadIncidents();
        } catch (e) {
            console.error('Failed to update status:', e);
        }
    }

    async resolveIncident(id) {
        if (!confirm('Resolve this incident?')) return;

        try {
            const resp = await fetch(`/api/incidents/${id}/resolve`, {
                method: 'POST'
            });
            if (resp.ok) {
                this.hideDetail();
                this.loadIncidents();
            }
        } catch (e) {
            console.error('Failed to resolve:', e);
        }
    }

    async addTimelineEntry(id) {
        const message = prompt('Add timeline update:');
        if (!message) return;

        try {
            await fetch(`/api/incidents/${id}/timeline`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message })
            });
            this.showIncidentDetail(id);
        } catch (e) {
            console.error('Failed to add entry:', e);
        }
    }

    showCreateIncident() {
        const title = prompt('Incident title:');
        if (!title) return;

        const severity = prompt('Severity (critical/high/medium/low):', 'high');
        const description = prompt('Description (optional):');

        this.createIncident({ title, severity, description });
    }

    async createIncident(data) {
        try {
            const resp = await fetch('/api/incidents', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
            if (resp.ok) this.loadIncidents();
        } catch (e) {
            console.error('Failed to create incident:', e);
        }
    }

    getSeverityIcon(severity) {
        switch (severity) {
            case 'critical': return '🔴';
            case 'high': return '🟠';
            case 'medium': return '🟡';
            case 'low': return '🟢';
            default: return '⚪';
        }
    }

    getDuration(start, end) {
        const startTime = new Date(start).getTime();
        const endTime = end ? new Date(end).getTime() : Date.now();
        const ms = endTime - startTime;

        const minutes = Math.floor(ms / 60000);
        if (minutes < 60) return `${minutes}m`;
        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ${minutes % 60}m`;
        const days = Math.floor(hours / 24);
        return `${days}d ${hours % 24}h`;
    }

    formatTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const now = new Date();
        const diffMs = now - d;

        if (diffMs < 60000) return 'just now';
        if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
        if (diffMs < 86400000) return `${Math.floor(diffMs / 3600000)}h ago`;

        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .incidents-timeline {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .incidents-header {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .incidents-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .active-badge {
                background: var(--error, #f4212e);
                color: white;
                padding: 0.1rem 0.5rem;
                border-radius: 10px;
                font-size: 0.75rem;
            }

            .incidents-filters {
                display: flex;
                gap: 0.25rem;
                margin-left: auto;
            }

            .filter-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.6rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.8rem;
            }

            .filter-btn:hover { background: var(--bg-card, #16181c); }
            .filter-btn.active { background: var(--bg-card, #16181c); color: var(--text, #e7e9ea); }

            .btn-create {
                background: var(--error, #f4212e);
                border: none;
                color: white;
                padding: 0.4rem 0.75rem;
                border-radius: 6px;
                cursor: pointer;
                font-size: 0.8rem;
            }

            .incidents-content, .incident-detail {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .empty-state {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .empty-state .icon { font-size: 2.5rem; margin-bottom: 1rem; }

            .incidents-list { display: flex; flex-direction: column; gap: 0.75rem; }

            .incident-card {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                cursor: pointer;
                border-left: 4px solid var(--border, #2f3336);
                transition: background 0.15s;
            }

            .incident-card:hover { background: var(--bg-card, #16181c); }

            .incident-card.severity-critical { border-left-color: var(--error, #f4212e); }
            .incident-card.severity-high { border-left-color: #ff7a00; }
            .incident-card.severity-medium { border-left-color: var(--warning, #ffd400); }
            .incident-card.severity-low { border-left-color: var(--success, #00ba7c); }

            .severity-badge {
                font-size: 1.25rem;
            }

            .incident-main { flex: 1; }

            .incident-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.25rem;
            }

            .incident-title { font-weight: 600; font-size: 0.9rem; }

            .incident-status {
                font-size: 0.7rem;
                padding: 0.15rem 0.5rem;
                border-radius: 10px;
                background: var(--bg-card, #16181c);
            }

            .status-active, .status-investigating { color: var(--error, #f4212e); }
            .status-identified { color: var(--warning, #ffd400); }
            .status-resolved { color: var(--success, #00ba7c); }

            .incident-meta {
                display: flex;
                gap: 1rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }

            .incident-desc {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .incident-actions { display: flex; gap: 0.25rem; }

            .btn-action {
                width: 28px;
                height: 28px;
                display: flex;
                align-items: center;
                justify-content: center;
                background: transparent;
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                cursor: pointer;
            }

            .btn-action:hover { background: var(--bg-card, #16181c); }

            .detail-header {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                margin-bottom: 1rem;
            }

            .btn-back {
                background: transparent;
                border: none;
                color: var(--accent, #1d9bf0);
                cursor: pointer;
                font-size: 0.85rem;
            }

            .detail-title {
                font-size: 1.25rem;
                margin-bottom: 1rem;
            }

            .detail-meta {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
                gap: 1rem;
                margin-bottom: 1.5rem;
            }

            .meta-item { display: flex; flex-direction: column; gap: 0.2rem; }
            .meta-label { font-size: 0.7rem; color: var(--text-muted, #71767b); text-transform: uppercase; }
            .meta-value { font-size: 0.85rem; }

            .detail-section {
                margin-bottom: 1.5rem;
            }

            .detail-section h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.75rem;
            }

            .timeline {
                position: relative;
                padding-left: 1.5rem;
            }

            .timeline::before {
                content: '';
                position: absolute;
                left: 5px;
                top: 0;
                bottom: 0;
                width: 2px;
                background: var(--border, #2f3336);
            }

            .timeline-item {
                position: relative;
                padding-bottom: 1rem;
            }

            .timeline-marker {
                position: absolute;
                left: -1.5rem;
                width: 12px;
                height: 12px;
                background: var(--accent, #1d9bf0);
                border-radius: 50%;
                margin-top: 3px;
            }

            .timeline-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }

            .timeline-text { font-size: 0.85rem; }

            .detail-actions {
                display: flex;
                gap: 0.75rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .btn-primary {
                background: var(--success, #00ba7c);
                border: none;
                color: white;
                padding: 0.5rem 1rem;
                border-radius: 6px;
                cursor: pointer;
            }

            .btn-secondary {
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                color: var(--text, #e7e9ea);
                padding: 0.5rem 1rem;
                border-radius: 6px;
                cursor: pointer;
            }
        `;
    }
}

customElements.define('incidents-timeline', IncidentsTimeline);

/**
 * Latency Heatmap Component
 * Shows latency distribution over time with color intensity
 */
class LatencyHeatmap extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.canvas = null;
        this.ctx = null;
        this.resizeObserver = null;
        this.tooltip = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        this.resizeObserver = new ResizeObserver(() => this.drawHeatmap());
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['service', 'endpoint', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.loadData();
        }
    }

    get service() { return this.getAttribute('service') || ''; }
    get endpoint() { return this.getAttribute('endpoint') || ''; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .heatmap-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .heatmap-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .heatmap-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .heatmap-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .heatmap-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.4rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.8rem;
                }
                .heatmap-body {
                    flex: 1;
                    display: flex;
                    position: relative;
                    min-height: 200px;
                }
                .heatmap-y-axis {
                    width: 60px;
                    display: flex;
                    flex-direction: column;
                    justify-content: space-between;
                    padding: 1rem 0.5rem 2rem 0.5rem;
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                    text-align: right;
                }
                .heatmap-canvas-wrapper {
                    flex: 1;
                    position: relative;
                    padding: 1rem 1rem 2rem 0;
                }
                .heatmap-canvas {
                    width: 100%;
                    height: 100%;
                    cursor: crosshair;
                }
                .heatmap-x-axis {
                    position: absolute;
                    bottom: 0.5rem;
                    left: 60px;
                    right: 1rem;
                    display: flex;
                    justify-content: space-between;
                    font-size: 0.7rem;
                    color: var(--text-muted, #71767b);
                }
                .heatmap-tooltip {
                    position: fixed;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.5rem 0.75rem;
                    font-size: 0.8rem;
                    pointer-events: none;
                    z-index: 1000;
                    display: none;
                    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
                }
                .heatmap-tooltip-row {
                    display: flex;
                    justify-content: space-between;
                    gap: 1rem;
                }
                .heatmap-tooltip-label {
                    color: var(--text-muted, #71767b);
                }
                .heatmap-tooltip-value {
                    font-weight: 600;
                }
                .heatmap-legend {
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                    padding: 0.5rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.75rem;
                }
                .heatmap-legend-label {
                    color: var(--text-muted, #71767b);
                }
                .heatmap-legend-gradient {
                    width: 120px;
                    height: 12px;
                    border-radius: 2px;
                    background: linear-gradient(to right, #1a1f2e, #1d4ed8, #7c3aed, #ec4899, #f43f5e);
                }
                .heatmap-loading {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
                .heatmap-percentiles {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.5rem 1rem;
                    font-size: 0.8rem;
                }
                .heatmap-percentile {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                }
                .heatmap-percentile-label {
                    color: var(--text-muted, #71767b);
                    font-size: 0.7rem;
                }
                .heatmap-percentile-value {
                    font-weight: 600;
                    font-size: 1rem;
                }
            </style>
            <div class="heatmap-container">
                <div class="heatmap-header">
                    <div class="heatmap-title">
                        <span>&#128200;</span>
                        <span>Latency Heatmap</span>
                    </div>
                    <div class="heatmap-controls">
                        <select id="time-select">
                            <option value="15m">15 min</option>
                            <option value="1h" selected>1 hour</option>
                            <option value="6h">6 hours</option>
                            <option value="24h">24 hours</option>
                        </select>
                    </div>
                </div>
                <div class="heatmap-percentiles" id="percentiles">
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P50</span>
                        <span class="heatmap-percentile-value" id="p50">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P90</span>
                        <span class="heatmap-percentile-value" id="p90">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P95</span>
                        <span class="heatmap-percentile-value" id="p95">-</span>
                    </div>
                    <div class="heatmap-percentile">
                        <span class="heatmap-percentile-label">P99</span>
                        <span class="heatmap-percentile-value" id="p99">-</span>
                    </div>
                </div>
                <div class="heatmap-body">
                    <div class="heatmap-y-axis" id="y-axis"></div>
                    <div class="heatmap-canvas-wrapper">
                        <canvas class="heatmap-canvas" id="canvas"></canvas>
                    </div>
                    <div class="heatmap-x-axis" id="x-axis"></div>
                </div>
                <div class="heatmap-legend">
                    <span class="heatmap-legend-label">Requests:</span>
                    <span>Low</span>
                    <div class="heatmap-legend-gradient"></div>
                    <span>High</span>
                </div>
                <div class="heatmap-tooltip" id="tooltip"></div>
            </div>
        `;

        this.canvas = this.querySelector('#canvas');
        this.ctx = this.canvas?.getContext('2d');
        this.tooltip = this.querySelector('#tooltip');

        this.setupEventListeners();
    }

    setupEventListeners() {
        const timeSelect = this.querySelector('#time-select');
        timeSelect?.addEventListener('change', (e) => {
            this.setAttribute('time-range', e.target.value);
        });

        this.canvas?.addEventListener('mousemove', (e) => this.showTooltip(e));
        this.canvas?.addEventListener('mouseleave', () => this.hideTooltip());
    }

    async loadData() {
        try {
            const params = new URLSearchParams({ range: this.timeRange });
            if (this.service) params.append('service', this.service);
            if (this.endpoint) params.append('endpoint', this.endpoint);

            const resp = await fetch(`/api/metrics/latency-heatmap?${params}`);

            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }

            this.drawHeatmap();
            this.updatePercentiles();
        } catch (e) {
            console.error('Failed to load heatmap data:', e);
            this.data = this.generateDemoData();
            this.drawHeatmap();
            this.updatePercentiles();
        }
    }

    generateDemoData() {
        // Generate realistic latency heatmap data
        const buckets = ['0-10ms', '10-25ms', '25-50ms', '50-100ms', '100-250ms', '250-500ms', '500ms-1s', '1s+'];
        const numTimeSlots = 60; // 60 time buckets
        const data = {
            buckets: buckets,
            timeSlots: [],
            matrix: [],
            percentiles: { p50: 45, p90: 120, p95: 180, p99: 350 }
        };

        const now = Date.now();
        const slotDuration = 60000; // 1 minute per slot

        for (let t = 0; t < numTimeSlots; t++) {
            data.timeSlots.push(now - (numTimeSlots - t) * slotDuration);
            const row = [];

            // Create a realistic distribution - most requests are fast
            const baseDistribution = [0.3, 0.35, 0.2, 0.08, 0.04, 0.02, 0.008, 0.002];

            // Add some variation and occasional spikes
            const hasSpike = Math.random() > 0.9;
            const spikeMultiplier = hasSpike ? 3 : 1;

            for (let b = 0; b < buckets.length; b++) {
                let count = Math.floor(1000 * baseDistribution[b] * (0.5 + Math.random()));

                // Spikes affect slower buckets more
                if (hasSpike && b >= 3) {
                    count *= spikeMultiplier;
                }

                row.push(count);
            }
            data.matrix.push(row);
        }

        return data;
    }

    drawHeatmap() {
        if (!this.canvas || !this.ctx || !this.data) return;

        const wrapper = this.querySelector('.heatmap-canvas-wrapper');
        if (!wrapper) return;

        const rect = wrapper.getBoundingClientRect();
        const width = rect.width - 16; // padding
        const height = rect.height - 32; // padding

        // Set canvas size
        this.canvas.width = width * window.devicePixelRatio;
        this.canvas.height = height * window.devicePixelRatio;
        this.canvas.style.width = width + 'px';
        this.canvas.style.height = height + 'px';
        this.ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

        const { matrix, buckets, timeSlots } = this.data;
        if (!matrix || !matrix.length) return;

        const numRows = buckets.length;
        const numCols = matrix.length;
        const cellWidth = width / numCols;
        const cellHeight = height / numRows;

        // Find max value for color scaling
        let maxVal = 0;
        for (const row of matrix) {
            for (const val of row) {
                if (val > maxVal) maxVal = val;
            }
        }

        // Draw cells
        for (let col = 0; col < numCols; col++) {
            for (let row = 0; row < numRows; row++) {
                const value = matrix[col][row];
                const intensity = maxVal > 0 ? value / maxVal : 0;

                const x = col * cellWidth;
                const y = (numRows - 1 - row) * cellHeight; // Flip Y axis

                this.ctx.fillStyle = this.getHeatColor(intensity);
                this.ctx.fillRect(x, y, cellWidth + 0.5, cellHeight + 0.5);
            }
        }

        // Update axes
        this.updateAxes();
    }

    getHeatColor(intensity) {
        // Gradient from dark blue -> purple -> pink -> red
        if (intensity < 0.01) return '#1a1f2e';
        if (intensity < 0.1) return '#1e3a5f';
        if (intensity < 0.25) return '#1d4ed8';
        if (intensity < 0.5) return '#6366f1';
        if (intensity < 0.75) return '#a855f7';
        if (intensity < 0.9) return '#ec4899';
        return '#f43f5e';
    }

    updateAxes() {
        const yAxis = this.querySelector('#y-axis');
        const xAxis = this.querySelector('#x-axis');

        if (yAxis && this.data?.buckets) {
            yAxis.innerHTML = this.data.buckets
                .slice().reverse()
                .map(b => `<span>${b}</span>`)
                .join('');
        }

        if (xAxis && this.data?.timeSlots) {
            const slots = this.data.timeSlots;
            const formatTime = (ts) => {
                const d = new Date(ts);
                return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
            };

            xAxis.innerHTML = `
                <span>${formatTime(slots[0])}</span>
                <span>${formatTime(slots[Math.floor(slots.length / 2)])}</span>
                <span>${formatTime(slots[slots.length - 1])}</span>
            `;
        }
    }

    updatePercentiles() {
        if (!this.data?.percentiles) return;

        const formatMs = (ms) => {
            if (ms < 1000) return `${ms}ms`;
            return `${(ms / 1000).toFixed(2)}s`;
        };

        const p = this.data.percentiles;
        this.querySelector('#p50').textContent = formatMs(p.p50);
        this.querySelector('#p90').textContent = formatMs(p.p90);
        this.querySelector('#p95').textContent = formatMs(p.p95);
        this.querySelector('#p99').textContent = formatMs(p.p99);
    }

    showTooltip(e) {
        if (!this.canvas || !this.data || !this.tooltip) return;

        const rect = this.canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        const { matrix, buckets, timeSlots } = this.data;
        const cellWidth = rect.width / matrix.length;
        const cellHeight = rect.height / buckets.length;

        const col = Math.floor(x / cellWidth);
        const row = buckets.length - 1 - Math.floor(y / cellHeight);

        if (col < 0 || col >= matrix.length || row < 0 || row >= buckets.length) {
            this.hideTooltip();
            return;
        }

        const value = matrix[col][row];
        const bucket = buckets[row];
        const time = new Date(timeSlots[col]).toLocaleTimeString('en-US', {
            hour: '2-digit', minute: '2-digit', hour12: false
        });

        this.tooltip.innerHTML = `
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Time:</span>
                <span class="heatmap-tooltip-value">${time}</span>
            </div>
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Latency:</span>
                <span class="heatmap-tooltip-value">${bucket}</span>
            </div>
            <div class="heatmap-tooltip-row">
                <span class="heatmap-tooltip-label">Requests:</span>
                <span class="heatmap-tooltip-value">${value.toLocaleString()}</span>
            </div>
        `;

        this.tooltip.style.display = 'block';
        this.tooltip.style.left = (e.clientX + 10) + 'px';
        this.tooltip.style.top = (e.clientY + 10) + 'px';
    }

    hideTooltip() {
        if (this.tooltip) {
            this.tooltip.style.display = 'none';
        }
    }
}

customElements.define('latency-heatmap', LatencyHeatmap);

/**
 * Log Explorer Widget
 * Full-featured log search with filters, facets, and live tail
 */
class LogExplorer extends HTMLElement {
    constructor() {
        super();
        this.logs = [];
        this.services = [];
        this.filters = {
            query: '',
            level: '',
            service: '',
            timeRange: '1h'
        };
        this.liveTail = false;
        this.liveTailInterval = null;
        this.expandedRows = new Set();
        this.loading = false;
    }

    connectedCallback() {
        this.render();
        this.loadServices();
        this.search();
    }

    disconnectedCallback() {
        this.stopLiveTail();
    }

    async loadServices() {
        try {
            const resp = await fetch('/api/logs/services');
            if (resp.ok) {
                this.services = await resp.json();
                this.updateServiceFilter();
            }
        } catch (e) {
            console.error('Failed to load services:', e);
        }
    }

    updateServiceFilter() {
        const select = this.querySelector('#service-filter');
        if (select && this.services.length > 0) {
            select.innerHTML = `
                <option value="">All Services</option>
                ${this.services.map(s => `<option value="${s}">${s}</option>`).join('')}
            `;
        }
    }

    async search() {
        if (this.loading) return;
        this.loading = true;
        this.showLoading();

        try {
            const params = new URLSearchParams({
                limit: '500'
            });

            if (this.filters.query) params.append('q', this.filters.query);
            if (this.filters.level) params.append('level', this.filters.level);
            if (this.filters.service) params.append('service', this.filters.service);

            // Time range
            const duration = this.parseTimeRange(this.filters.timeRange);
            const endTime = new Date();
            const startTime = new Date(endTime.getTime() - duration);
            params.append('start', startTime.toISOString());
            params.append('end', endTime.toISOString());

            const resp = await fetch(`/api/logs?${params}`);
            if (!resp.ok) throw new Error('Search failed');

            const data = await resp.json();
            this.logs = data.data || data.entries || [];
            this.renderResults();
        } catch (e) {
            this.showError(e.message);
        } finally {
            this.loading = false;
        }
    }

    parseTimeRange(range) {
        const match = range.match(/^(\d+)([mhd])$/);
        if (!match) return 15 * 60 * 1000; // default 15m

        const value = parseInt(match[1]);
        const unit = match[2];

        switch (unit) {
            case 'm': return value * 60 * 1000;
            case 'h': return value * 60 * 60 * 1000;
            case 'd': return value * 24 * 60 * 60 * 1000;
            default: return 15 * 60 * 1000;
        }
    }

    toggleLiveTail() {
        this.liveTail = !this.liveTail;

        const btn = this.querySelector('#live-tail-btn');
        if (btn) {
            btn.classList.toggle('active', this.liveTail);
            btn.innerHTML = this.liveTail ? '⏹ Stop' : '▶ Live Tail';
        }

        if (this.liveTail) {
            this.startLiveTail();
        } else {
            this.stopLiveTail();
        }
    }

    startLiveTail() {
        // Use WebSocket for real-time log streaming if available
        if (typeof dwSocket !== 'undefined' && dwSocket.subscribe) {
            this.liveTailUnsubscribe = dwSocket.subscribe('logs', (msg) => {
                if (msg.payload) {
                    // Add new log to the top
                    const newLog = {
                        id: Date.now().toString(),
                        timestamp: msg.payload.timestamp || new Date().toISOString(),
                        level: msg.payload.level || 'info',
                        message: msg.payload.message || '',
                        service: msg.payload.service || '',
                        host: msg.payload.host || ''
                    };
                    // Check if log matches current filters
                    if (this.matchesFilters(newLog)) {
                        this.logs.unshift(newLog);
                        if (this.logs.length > 500) this.logs.pop();
                        this.renderResults();
                    }
                }
            });
        } else {
            // Fallback to polling
            this.liveTailInterval = setInterval(() => {
                this.search();
            }, 2000);
        }
    }

    stopLiveTail() {
        if (this.liveTailUnsubscribe) {
            this.liveTailUnsubscribe();
            this.liveTailUnsubscribe = null;
        }
        if (this.liveTailInterval) {
            clearInterval(this.liveTailInterval);
            this.liveTailInterval = null;
        }
    }

    matchesFilters(log) {
        if (this.filters.level && log.level !== this.filters.level) return false;
        if (this.filters.service && log.service !== this.filters.service) return false;
        if (this.filters.query) {
            const q = this.filters.query.toLowerCase();
            if (!log.message.toLowerCase().includes(q)) return false;
        }
        return true;
    }

    setFilter(key, value) {
        this.filters[key] = value;
        this.search();
    }

    toggleRow(index) {
        if (this.expandedRows.has(index)) {
            this.expandedRows.delete(index);
        } else {
            this.expandedRows.add(index);
        }
        this.renderResults();
    }

    showLoading() {
        const container = this.querySelector('#log-results');
        if (container && this.logs.length === 0) {
            container.innerHTML = `
                <div class="log-loading">
                    <div class="spinner"></div>
                    <span>Searching logs...</span>
                </div>
            `;
        }
    }

    showError(message) {
        const container = this.querySelector('#log-results');
        if (container) {
            container.innerHTML = `
                <div class="log-error">
                    <span class="icon">⚠️</span>
                    <span>${message}</span>
                </div>
            `;
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="log-explorer">
                <div class="log-toolbar">
                    <div class="search-bar">
                        <input type="text"
                               id="search-input"
                               placeholder="Search logs... (supports regex)"
                               value="${this.escapeHtml(this.filters.query)}"
                               onkeydown="if(event.key==='Enter') this.getRootNode().host.setFilter('query', this.value)">
                        <button class="btn-search" onclick="this.getRootNode().host.setFilter('query', this.getRootNode().host.querySelector('#search-input').value)">
                            🔍
                        </button>
                    </div>
                    <div class="filters">
                        <select id="level-filter" onchange="this.getRootNode().host.setFilter('level', this.value)">
                            <option value="">All Levels</option>
                            <option value="debug" ${this.filters.level === 'debug' ? 'selected' : ''}>Debug</option>
                            <option value="info" ${this.filters.level === 'info' ? 'selected' : ''}>Info</option>
                            <option value="warn" ${this.filters.level === 'warn' ? 'selected' : ''}>Warn</option>
                            <option value="error" ${this.filters.level === 'error' ? 'selected' : ''}>Error</option>
                        </select>
                        <select id="service-filter" onchange="this.getRootNode().host.setFilter('service', this.value)">
                            <option value="">All Services</option>
                        </select>
                        <select id="time-filter" onchange="this.getRootNode().host.setFilter('timeRange', this.value)">
                            <option value="5m" ${this.filters.timeRange === '5m' ? 'selected' : ''}>Last 5m</option>
                            <option value="15m" ${this.filters.timeRange === '15m' ? 'selected' : ''}>Last 15m</option>
                            <option value="1h" ${this.filters.timeRange === '1h' ? 'selected' : ''}>Last 1h</option>
                            <option value="6h" ${this.filters.timeRange === '6h' ? 'selected' : ''}>Last 6h</option>
                            <option value="24h" ${this.filters.timeRange === '24h' ? 'selected' : ''}>Last 24h</option>
                            <option value="7d" ${this.filters.timeRange === '7d' ? 'selected' : ''}>Last 7d</option>
                        </select>
                    </div>
                    <div class="actions">
                        <button id="live-tail-btn" class="btn-live ${this.liveTail ? 'active' : ''}" onclick="this.getRootNode().host.toggleLiveTail()">
                            ${this.liveTail ? '⏹ Stop' : '▶ Live Tail'}
                        </button>
                        <button class="btn-refresh" onclick="this.getRootNode().host.search()">
                            ⟲ Refresh
                        </button>
                    </div>
                </div>
                <div class="log-stats" id="log-stats">
                    <span class="stat">
                        <span class="stat-value" id="log-count">0</span> logs
                    </span>
                    <span class="stat">
                        <span class="level-badge level-error" id="error-count">0</span> errors
                    </span>
                    <span class="stat">
                        <span class="level-badge level-warn" id="warn-count">0</span> warnings
                    </span>
                </div>
                <div class="log-results" id="log-results">
                    <div class="log-empty">
                        <span class="icon">📋</span>
                        <p>No logs found</p>
                    </div>
                </div>
            </div>
        `;
        this.updateServiceFilter();
    }

    renderResults() {
        const container = this.querySelector('#log-results');
        if (!container) return;

        // Update stats
        const errorCount = this.logs.filter(l => l.level === 'error').length;
        const warnCount = this.logs.filter(l => l.level === 'warn' || l.level === 'warning').length;

        const countEl = this.querySelector('#log-count');
        const errorEl = this.querySelector('#error-count');
        const warnEl = this.querySelector('#warn-count');

        if (countEl) countEl.textContent = this.logs.length;
        if (errorEl) errorEl.textContent = errorCount;
        if (warnEl) warnEl.textContent = warnCount;

        if (this.logs.length === 0) {
            container.innerHTML = `
                <div class="log-empty">
                    <span class="icon">📋</span>
                    <p>No logs found</p>
                    <p class="hint">Try adjusting your search or time range</p>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="log-table">
                <div class="log-header">
                    <div class="col-time">Time</div>
                    <div class="col-level">Level</div>
                    <div class="col-service">Service</div>
                    <div class="col-message">Message</div>
                </div>
                <div class="log-body">
                    ${this.logs.map((log, i) => this.renderLogRow(log, i)).join('')}
                </div>
            </div>
        `;
    }

    renderLogRow(log, index) {
        const isExpanded = this.expandedRows.has(index);
        const level = (log.level || 'info').toLowerCase();
        const timestamp = new Date(log.timestamp);
        const timeStr = timestamp.toLocaleTimeString('en-US', {
            hour12: false,
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        }) + '.' + String(timestamp.getMilliseconds()).padStart(3, '0');

        const message = this.highlightSearch(log.message || '');

        return `
            <div class="log-row ${isExpanded ? 'expanded' : ''}" onclick="this.getRootNode().host.toggleRow(${index})">
                <div class="log-main">
                    <div class="col-time">${timeStr}</div>
                    <div class="col-level">
                        <span class="level-badge level-${level}">${level.toUpperCase()}</span>
                    </div>
                    <div class="col-service">${this.escapeHtml(log.service || '—')}</div>
                    <div class="col-message">${message}</div>
                </div>
                ${isExpanded ? this.renderExpandedDetails(log) : ''}
            </div>
        `;
    }

    renderExpandedDetails(log) {
        const attrs = log.attributes || {};
        const hasAttrs = Object.keys(attrs).length > 0;

        return `
            <div class="log-details">
                <div class="log-details-section">
                    <div class="log-full-message">${this.escapeHtml(log.message || '')}</div>
                </div>
                <div class="log-details-grid">
                    ${log.trace_id ? `
                        <div class="detail-item">
                            <span class="detail-label">Trace ID</span>
                            <span class="detail-value mono">${log.trace_id}</span>
                        </div>
                    ` : ''}
                    ${log.span_id ? `
                        <div class="detail-item">
                            <span class="detail-label">Span ID</span>
                            <span class="detail-value mono">${log.span_id}</span>
                        </div>
                    ` : ''}
                    ${log.host ? `
                        <div class="detail-item">
                            <span class="detail-label">Host</span>
                            <span class="detail-value">${this.escapeHtml(log.host)}</span>
                        </div>
                    ` : ''}
                    ${log.logger ? `
                        <div class="detail-item">
                            <span class="detail-label">Logger</span>
                            <span class="detail-value">${this.escapeHtml(log.logger)}</span>
                        </div>
                    ` : ''}
                </div>
                ${hasAttrs ? `
                    <div class="log-details-section">
                        <h4>Attributes</h4>
                        <table class="attrs-table">
                            ${Object.entries(attrs).map(([k, v]) => `
                                <tr>
                                    <td class="attr-key">${this.escapeHtml(k)}</td>
                                    <td class="attr-value">${this.escapeHtml(JSON.stringify(v))}</td>
                                </tr>
                            `).join('')}
                        </table>
                    </div>
                ` : ''}
                <div class="log-details-actions">
                    ${log.trace_id ? `
                        <a href="#" onclick="event.stopPropagation(); window.open('/traces/${log.trace_id}', '_blank')" class="action-link">
                            View Trace →
                        </a>
                    ` : ''}
                    <button onclick="event.stopPropagation(); navigator.clipboard.writeText(JSON.stringify(${this.escapeHtml(JSON.stringify(log))}, null, 2))" class="action-btn">
                        Copy JSON
                    </button>
                </div>
            </div>
        `;
    }

    highlightSearch(text) {
        if (!this.filters.query || !text) return this.escapeHtml(text);

        try {
            const escaped = this.escapeHtml(text);
            const regex = new RegExp(`(${this.escapeRegex(this.filters.query)})`, 'gi');
            return escaped.replace(regex, '<mark>$1</mark>');
        } catch (e) {
            return this.escapeHtml(text);
        }
    }

    escapeHtml(str) {
        if (str === null || str === undefined) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    escapeRegex(str) {
        return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    getStyles() {
        return `
            .log-explorer {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                color: var(--text, #e7e9ea);
                display: flex;
                flex-direction: column;
                height: 100%;
            }

            .log-toolbar {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
                flex-wrap: wrap;
            }

            .search-bar {
                display: flex;
                flex: 1;
                min-width: 200px;
            }

            .search-bar input {
                flex: 1;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-right: none;
                border-radius: 6px 0 0 6px;
                color: var(--text, #e7e9ea);
                padding: 0.5rem 0.75rem;
                font-size: 0.85rem;
            }

            .search-bar input:focus {
                outline: none;
                border-color: var(--accent, #1d9bf0);
            }

            .btn-search {
                background: var(--accent, #1d9bf0);
                border: 1px solid var(--accent, #1d9bf0);
                border-radius: 0 6px 6px 0;
                color: white;
                padding: 0.5rem 0.75rem;
                cursor: pointer;
            }

            .filters {
                display: flex;
                gap: 0.5rem;
            }

            .filters select {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 6px;
                color: var(--text, #e7e9ea);
                padding: 0.5rem 0.75rem;
                font-size: 0.8rem;
                cursor: pointer;
            }

            .filters select:focus {
                outline: none;
                border-color: var(--accent, #1d9bf0);
            }

            .actions {
                display: flex;
                gap: 0.5rem;
            }

            .btn-live, .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 6px;
                color: var(--text, #e7e9ea);
                padding: 0.5rem 0.75rem;
                font-size: 0.8rem;
                cursor: pointer;
                transition: all 0.15s;
            }

            .btn-live:hover, .btn-refresh:hover {
                border-color: var(--accent, #1d9bf0);
            }

            .btn-live.active {
                background: var(--error, #f4212e);
                border-color: var(--error, #f4212e);
                animation: pulse 1.5s ease-in-out infinite;
            }

            @keyframes pulse {
                0%, 100% { opacity: 1; }
                50% { opacity: 0.7; }
            }

            .log-stats {
                display: flex;
                gap: 1.5rem;
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
                font-size: 0.8rem;
            }

            .stat {
                display: flex;
                align-items: center;
                gap: 0.4rem;
                color: var(--text-muted, #71767b);
            }

            .stat-value {
                font-weight: 600;
                color: var(--text, #e7e9ea);
            }

            .log-results {
                flex: 1;
                overflow-y: auto;
            }

            .log-loading, .log-error, .log-empty {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
                gap: 0.75rem;
            }

            .log-empty .icon { font-size: 2rem; }
            .log-empty .hint { font-size: 0.8rem; }

            .spinner {
                width: 24px;
                height: 24px;
                border: 3px solid var(--border, #2f3336);
                border-top-color: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: spin 0.8s linear infinite;
            }

            @keyframes spin { to { transform: rotate(360deg); } }

            .log-table {
                font-size: 0.8rem;
            }

            .log-header {
                display: flex;
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 600;
                color: var(--text-muted, #71767b);
                position: sticky;
                top: 0;
                z-index: 1;
            }

            .col-time { width: 100px; flex-shrink: 0; }
            .col-level { width: 70px; flex-shrink: 0; }
            .col-service { width: 120px; flex-shrink: 0; }
            .col-message { flex: 1; overflow: hidden; }

            .log-row {
                border-bottom: 1px solid var(--border, #2f3336);
                cursor: pointer;
                transition: background 0.15s;
            }

            .log-row:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .log-row.expanded {
                background: rgba(29, 155, 240, 0.05);
            }

            .log-main {
                display: flex;
                padding: 0.6rem 1rem;
                align-items: center;
            }

            .col-message {
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
                font-family: 'Monaco', 'Menlo', monospace;
                font-size: 0.75rem;
            }

            .log-row.expanded .col-message {
                white-space: normal;
                word-break: break-word;
            }

            .level-badge {
                display: inline-block;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                font-size: 0.65rem;
                font-weight: 600;
                text-transform: uppercase;
            }

            .level-debug { background: #3d5a80; color: white; }
            .level-info { background: #1d9bf0; color: white; }
            .level-warn, .level-warning { background: #ffd400; color: #1a1a1a; }
            .level-error { background: #f4212e; color: white; }

            .log-details {
                padding: 1rem;
                background: var(--bg-card, #16181c);
                border-top: 1px solid var(--border, #2f3336);
            }

            .log-details-section {
                margin-bottom: 1rem;
            }

            .log-details-section h4 {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
                text-transform: uppercase;
            }

            .log-full-message {
                font-family: 'Monaco', 'Menlo', monospace;
                font-size: 0.8rem;
                white-space: pre-wrap;
                word-break: break-word;
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 6px;
                max-height: 200px;
                overflow-y: auto;
            }

            .log-details-grid {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
                gap: 0.75rem;
                margin-bottom: 1rem;
            }

            .detail-item {
                display: flex;
                flex-direction: column;
                gap: 0.2rem;
            }

            .detail-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
            }

            .detail-value {
                font-size: 0.8rem;
            }

            .detail-value.mono {
                font-family: 'Monaco', 'Menlo', monospace;
                font-size: 0.75rem;
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
                width: 30%;
            }

            .attr-value {
                font-family: monospace;
                font-size: 0.75rem;
                word-break: break-all;
            }

            .log-details-actions {
                display: flex;
                gap: 0.75rem;
                padding-top: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .action-link, .action-btn {
                font-size: 0.8rem;
                color: var(--accent, #1d9bf0);
                background: none;
                border: none;
                cursor: pointer;
                text-decoration: none;
            }

            .action-link:hover, .action-btn:hover {
                text-decoration: underline;
            }

            mark {
                background: rgba(255, 212, 0, 0.3);
                color: inherit;
                padding: 0 2px;
                border-radius: 2px;
            }
        `;
    }
}

customElements.define('log-explorer', LogExplorer);

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = LogExplorer;
}

/**
 * Log Viewer Web Component
 *
 * Usage:
 *   <dw-log-viewer></dw-log-viewer>
 *   <dw-log-viewer service="api-server" level="error"></dw-log-viewer>
 *
 * Attributes:
 *   - service: Filter by service name
 *   - level: Filter by log level (debug, info, warn, error, fatal)
 *   - limit: Max number of logs to display (default: 100)
 */
class LogViewer extends HTMLElement {
    static get observedAttributes() {
        return ['service', 'level', 'limit'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
        this.logs = [];
        this.filters = {
            service: '',
            level: '',
            search: ''
        };
        this._unsubscribe = null;
        this.maxLogs = 100;
        this.autoScroll = true;
    }

    connectedCallback() {
        this.maxLogs = parseInt(this.getAttribute('limit')) || 100;
        this.filters.service = this.getAttribute('service') || '';
        this.filters.level = this.getAttribute('level') || '';
        this.render();
        this.setupWebSocket();
        this.fetchLogs();
    }

    disconnectedCallback() {
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'service') this.filters.service = newValue || '';
        if (name === 'level') this.filters.level = newValue || '';
        if (name === 'limit') this.maxLogs = parseInt(newValue) || 100;
        this.renderLogs();
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
                .controls {
                    display: flex;
                    gap: 0.5rem;
                    padding: 0.5rem 0.8rem;
                    border-bottom: 1px solid #2f3336;
                    flex-wrap: wrap;
                    align-items: center;
                }
                .controls input,
                .controls select {
                    padding: 0.3rem 0.5rem;
                    background: #2f3336;
                    border: 1px solid #3f4346;
                    border-radius: 4px;
                    color: #e7e9ea;
                    font-size: 0.7rem;
                }
                .controls input { flex: 1; min-width: 150px; }
                .controls select { min-width: 80px; }
                .controls input:focus,
                .controls select:focus {
                    outline: none;
                    border-color: #1d9bf0;
                }
                .filter-pills {
                    display: flex;
                    gap: 0.3rem;
                    flex-wrap: wrap;
                }
                .filter-pill {
                    display: inline-flex;
                    align-items: center;
                    gap: 0.3rem;
                    padding: 0.2rem 0.5rem;
                    background: #2f3336;
                    border-radius: 12px;
                    font-size: 0.7rem;
                }
                .filter-pill .remove {
                    cursor: pointer;
                    color: #71767b;
                    font-weight: bold;
                }
                .filter-pill .remove:hover { color: #f4212e; }
                .filter-pill.level-error { background: rgba(244, 33, 46, 0.2); color: #f4212e; }
                .filter-pill.level-warn { background: rgba(255, 212, 0, 0.2); color: #ffd400; }
                .log-container {
                    flex: 1;
                    overflow-y: auto;
                    font-family: 'SF Mono', Consolas, monospace;
                    font-size: 0.7rem;
                }
                .log-entry {
                    padding: 0.4rem 0.8rem;
                    border-bottom: 1px solid #1d1f23;
                    display: flex;
                    gap: 0.5rem;
                }
                .log-entry:hover { background: #1d1f23; }
                .log-time { color: #71767b; white-space: nowrap; flex-shrink: 0; }
                .log-level {
                    padding: 0.1rem 0.3rem;
                    border-radius: 3px;
                    font-size: 0.6rem;
                    font-weight: 600;
                    text-transform: uppercase;
                    flex-shrink: 0;
                }
                .log-level.debug { background: #2f3336; color: #71767b; }
                .log-level.info { background: rgba(29, 155, 240, 0.2); color: #1d9bf0; }
                .log-level.warn { background: rgba(255, 212, 0, 0.2); color: #ffd400; }
                .log-level.error { background: rgba(244, 33, 46, 0.2); color: #f4212e; }
                .log-level.fatal { background: rgba(122, 25, 25, 1); color: #ff6b6b; }
                .log-service { color: #00ba7c; flex-shrink: 0; }
                .log-message { color: #e7e9ea; word-break: break-all; flex: 1; }
                .log-message .highlight {
                    background: rgba(255, 212, 0, 0.3);
                    padding: 0 2px;
                    border-radius: 2px;
                }
                .empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: #71767b;
                    font-size: 0.85rem;
                }
                .status-bar {
                    display: flex;
                    justify-content: space-between;
                    padding: 0.3rem 0.8rem;
                    border-top: 1px solid #2f3336;
                    font-size: 0.65rem;
                    color: #71767b;
                }
                .auto-scroll-toggle {
                    cursor: pointer;
                    color: #1d9bf0;
                }
            </style>
            <div class="controls">
                <input type="text" id="search" placeholder="Search logs...">
                <select id="level-filter">
                    <option value="">All Levels</option>
                    <option value="debug">Debug</option>
                    <option value="info">Info</option>
                    <option value="warn">Warn</option>
                    <option value="error">Error</option>
                    <option value="fatal">Fatal</option>
                </select>
                <select id="service-filter">
                    <option value="">All Services</option>
                </select>
                <div class="filter-pills" id="filter-pills"></div>
            </div>
            <div class="log-container" id="log-container"></div>
            <div class="status-bar">
                <span id="log-count">0 logs</span>
                <span class="auto-scroll-toggle" id="auto-scroll-toggle">Auto-scroll: ON</span>
            </div>
        `;

        this.setupEventListeners();
    }

    setupEventListeners() {
        const search = this.shadowRoot.getElementById('search');
        const levelFilter = this.shadowRoot.getElementById('level-filter');
        const serviceFilter = this.shadowRoot.getElementById('service-filter');
        const autoScrollToggle = this.shadowRoot.getElementById('auto-scroll-toggle');

        search.addEventListener('input', (e) => {
            this.filters.search = e.target.value;
            this.renderLogs();
        });

        levelFilter.addEventListener('change', (e) => {
            this.filters.level = e.target.value;
            this.renderFilterPills();
            this.renderLogs();
        });

        serviceFilter.addEventListener('change', (e) => {
            this.filters.service = e.target.value;
            this.renderFilterPills();
            this.renderLogs();
        });

        autoScrollToggle.addEventListener('click', () => {
            this.autoScroll = !this.autoScroll;
            autoScrollToggle.textContent = `Auto-scroll: ${this.autoScroll ? 'ON' : 'OFF'}`;
        });

        const container = this.shadowRoot.getElementById('log-container');
        container.addEventListener('scroll', () => {
            const isAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 50;
            if (!isAtBottom) {
                this.autoScroll = false;
                autoScrollToggle.textContent = 'Auto-scroll: OFF';
            }
        });
    }

    setupWebSocket() {
        if (window.dwSocket) {
            this._unsubscribe = window.dwSocket.subscribe('logs', (msg) => {
                if (msg.type === 'entry' && msg.payload) {
                    this.addLog(msg.payload);
                }
            });
        }
    }

    async fetchLogs() {
        try {
            const params = new URLSearchParams({ limit: this.maxLogs.toString() });
            if (this.filters.service) params.set('service', this.filters.service);
            if (this.filters.level) params.set('level', this.filters.level);

            const response = await fetch(`/api/logs?${params}`);
            if (response.ok) {
                const data = await response.json();
                this.logs = data.entries || [];
                this.updateServiceFilter();
                this.renderLogs();
            }
        } catch (e) {
            console.error('[LogViewer] Failed to fetch logs:', e);
        }
    }

    addLog(log) {
        this.logs.unshift(log);
        if (this.logs.length > this.maxLogs) {
            this.logs.pop();
        }
        this.renderLogs();
    }

    updateServiceFilter() {
        const services = [...new Set(this.logs.map(l => l.service).filter(Boolean))];
        const select = this.shadowRoot.getElementById('service-filter');
        const currentValue = select.value;

        select.innerHTML = '<option value="">All Services</option>' +
            services.map(s => `<option value="${s}">${s}</option>`).join('');

        select.value = currentValue;
    }

    renderFilterPills() {
        const pills = this.shadowRoot.getElementById('filter-pills');
        let html = '';

        if (this.filters.level) {
            html += `<span class="filter-pill level-${this.filters.level}">
                ${this.filters.level}
                <span class="remove" data-clear="level">&times;</span>
            </span>`;
        }

        if (this.filters.service) {
            html += `<span class="filter-pill">
                ${this.filters.service}
                <span class="remove" data-clear="service">&times;</span>
            </span>`;
        }

        pills.innerHTML = html;

        pills.querySelectorAll('.remove').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const key = e.target.dataset.clear;
                this.filters[key] = '';
                this.shadowRoot.getElementById(key + '-filter').value = '';
                this.renderFilterPills();
                this.renderLogs();
            });
        });
    }

    renderLogs() {
        const container = this.shadowRoot.getElementById('log-container');
        const countEl = this.shadowRoot.getElementById('log-count');

        // Filter logs
        let filtered = this.logs;
        if (this.filters.level) {
            filtered = filtered.filter(l => l.level === this.filters.level);
        }
        if (this.filters.service) {
            filtered = filtered.filter(l => l.service === this.filters.service);
        }
        if (this.filters.search) {
            const search = this.filters.search.toLowerCase();
            filtered = filtered.filter(l =>
                (l.message || '').toLowerCase().includes(search) ||
                (l.service || '').toLowerCase().includes(search)
            );
        }

        if (filtered.length === 0) {
            container.innerHTML = '<div class="empty">No logs matching filters</div>';
            countEl.textContent = '0 logs';
            return;
        }

        container.innerHTML = filtered.map(log => `
            <div class="log-entry">
                <span class="log-time">${this.formatTime(log.timestamp)}</span>
                <span class="log-level ${log.level || 'info'}">${log.level || 'info'}</span>
                <span class="log-service">${log.service || ''}</span>
                <span class="log-message">${this.highlightSearch(this.escapeHtml(log.message || ''))}</span>
            </div>
        `).join('');

        countEl.textContent = `${filtered.length} logs`;

        // Auto-scroll to bottom
        if (this.autoScroll) {
            container.scrollTop = container.scrollHeight;
        }
    }

    formatTime(timestamp) {
        if (!timestamp) return '';
        const date = new Date(timestamp);
        return date.toLocaleTimeString('en-US', {
            hour12: false,
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        }) + '.' + String(date.getMilliseconds()).padStart(3, '0');
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    highlightSearch(text) {
        if (!this.filters.search) return text;
        const regex = new RegExp(`(${this.escapeRegex(this.filters.search)})`, 'gi');
        return text.replace(regex, '<span class="highlight">$1</span>');
    }

    escapeRegex(string) {
        return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    // Public API
    refresh() {
        this.fetchLogs();
    }

    setFilter(key, value) {
        this.filters[key] = value;
        this.renderFilterPills();
        this.renderLogs();
    }

    clearFilters() {
        this.filters = { service: '', level: '', search: '' };
        this.shadowRoot.getElementById('search').value = '';
        this.shadowRoot.getElementById('level-filter').value = '';
        this.shadowRoot.getElementById('service-filter').value = '';
        this.renderFilterPills();
        this.renderLogs();
    }
}

customElements.define('dw-log-viewer', LogViewer);

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

/**
 * On-Call Calendar Widget
 * Who's on-call now, schedule view, shift swaps
 */
class OncallCalendar extends HTMLElement {
    constructor() {
        super();
        this.schedules = [];
        this.currentOnCall = [];
        this.view = 'current'; // current, week, month
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        try {
            const [schedulesResp, currentResp] = await Promise.all([
                fetch('/api/oncall/schedules'),
                fetch('/api/oncall/current')
            ]);

            if (schedulesResp.ok) this.schedules = await schedulesResp.json() || [];
            if (currentResp.ok) this.currentOnCall = await currentResp.json() || [];

            this.renderContent();
        } catch (e) {
            console.error('Failed to load on-call data:', e);
        }
    }

    setView(view) {
        this.view = view;
        this.querySelectorAll('.view-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.view === view);
        });
        this.renderContent();
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="oncall-calendar">
                <div class="oncall-header">
                    <div class="header-title">
                        <span class="title-icon">📞</span>
                        <span>On-Call</span>
                    </div>
                    <div class="header-views">
                        <button class="view-btn active" data-view="current" onclick="this.getRootNode().host.setView('current')">Now</button>
                        <button class="view-btn" data-view="week" onclick="this.getRootNode().host.setView('week')">Week</button>
                        <button class="view-btn" data-view="month" onclick="this.getRootNode().host.setView('month')">Month</button>
                    </div>
                </div>
                <div class="oncall-content" id="oncall-content">
                    <div class="loading">Loading...</div>
                </div>
            </div>
        `;
    }

    renderContent() {
        const container = this.querySelector('#oncall-content');
        if (!container) return;

        switch (this.view) {
            case 'current':
                container.innerHTML = this.renderCurrentOnCall();
                break;
            case 'week':
                container.innerHTML = this.renderWeekView();
                break;
            case 'month':
                container.innerHTML = this.renderMonthView();
                break;
        }
    }

    renderCurrentOnCall() {
        if (this.currentOnCall.length === 0 && this.schedules.length === 0) {
            return `
                <div class="empty-state">
                    <span class="icon">📞</span>
                    <p>No on-call schedules configured</p>
                </div>
            `;
        }

        // Group by schedule/team
        const bySchedule = new Map();
        this.currentOnCall.forEach(oc => {
            const key = oc.schedule_name || oc.team || 'Default';
            if (!bySchedule.has(key)) bySchedule.set(key, []);
            bySchedule.get(key).push(oc);
        });

        return `
            <div class="current-oncall">
                ${Array.from(bySchedule.entries()).map(([name, people]) => `
                    <div class="schedule-card">
                        <div class="schedule-name">${this.escapeHtml(name)}</div>
                        <div class="oncall-people">
                            ${people.map((p, i) => `
                                <div class="person-card ${i === 0 ? 'primary' : 'backup'}">
                                    <div class="person-avatar">${this.getInitials(p.user_name || p.name)}</div>
                                    <div class="person-info">
                                        <div class="person-name">${this.escapeHtml(p.user_name || p.name || 'Unknown')}</div>
                                        <div class="person-role">${i === 0 ? 'Primary' : 'Backup'}</div>
                                        <div class="shift-time">Until ${this.formatTime(p.end_time)}</div>
                                    </div>
                                    <div class="person-actions">
                                        <a href="tel:${p.phone || ''}" class="btn-contact" title="Call">📞</a>
                                        <a href="mailto:${p.email || ''}" class="btn-contact" title="Email">✉️</a>
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                `).join('')}
                ${this.schedules.filter(s => !Array.from(bySchedule.keys()).includes(s.name)).map(s => `
                    <div class="schedule-card empty">
                        <div class="schedule-name">${this.escapeHtml(s.name)}</div>
                        <div class="no-oncall">No one currently on-call</div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderWeekView() {
        const days = this.getWeekDays();

        return `
            <div class="week-view">
                <div class="week-header">
                    ${days.map(d => `
                        <div class="day-header ${d.isToday ? 'today' : ''}">
                            <span class="day-name">${d.name}</span>
                            <span class="day-date">${d.date}</span>
                        </div>
                    `).join('')}
                </div>
                <div class="week-body">
                    ${this.schedules.slice(0, 3).map(schedule => `
                        <div class="schedule-row">
                            <div class="schedule-label">${this.escapeHtml(schedule.name)}</div>
                            <div class="schedule-shifts">
                                ${days.map(d => `
                                    <div class="shift-cell ${d.isToday ? 'today' : ''}">
                                        ${this.getShiftForDay(schedule, d.fullDate) || '<span class="no-shift">—</span>'}
                                    </div>
                                `).join('')}
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderMonthView() {
        const today = new Date();
        const year = today.getFullYear();
        const month = today.getMonth();
        const firstDay = new Date(year, month, 1);
        const lastDay = new Date(year, month + 1, 0);
        const startPadding = firstDay.getDay();

        const days = [];
        for (let i = 0; i < startPadding; i++) {
            days.push({ empty: true });
        }
        for (let d = 1; d <= lastDay.getDate(); d++) {
            days.push({
                date: d,
                isToday: d === today.getDate(),
                fullDate: new Date(year, month, d)
            });
        }

        return `
            <div class="month-view">
                <div class="month-header">
                    <span class="month-name">${today.toLocaleDateString('en-US', { month: 'long', year: 'numeric' })}</span>
                </div>
                <div class="month-grid">
                    <div class="weekday-row">
                        ${['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map(d => `<div class="weekday">${d}</div>`).join('')}
                    </div>
                    <div class="days-grid">
                        ${days.map(d => d.empty ? '<div class="day-cell empty"></div>' : `
                            <div class="day-cell ${d.isToday ? 'today' : ''}">
                                <span class="day-num">${d.date}</span>
                                ${this.getShiftIndicator(d.fullDate)}
                            </div>
                        `).join('')}
                    </div>
                </div>
            </div>
        `;
    }

    getWeekDays() {
        const days = [];
        const today = new Date();
        const dayOfWeek = today.getDay();
        const startOfWeek = new Date(today);
        startOfWeek.setDate(today.getDate() - dayOfWeek);

        for (let i = 0; i < 7; i++) {
            const d = new Date(startOfWeek);
            d.setDate(startOfWeek.getDate() + i);
            days.push({
                name: d.toLocaleDateString('en-US', { weekday: 'short' }),
                date: d.getDate(),
                isToday: d.toDateString() === today.toDateString(),
                fullDate: d
            });
        }
        return days;
    }

    getShiftForDay(schedule, date) {
        // In a real implementation, this would look up actual shifts
        const person = this.currentOnCall.find(oc =>
            (oc.schedule_name === schedule.name || oc.schedule_id === schedule.id)
        );

        if (person) {
            return `<span class="shift-person">${this.getInitials(person.user_name || person.name)}</span>`;
        }
        return '';
    }

    getShiftIndicator(date) {
        const hasShift = this.currentOnCall.length > 0;
        return hasShift ? '<div class="shift-indicator"></div>' : '';
    }

    getInitials(name) {
        if (!name) return '?';
        return name.split(' ').map(n => n[0]).join('').toUpperCase().substring(0, 2);
    }

    formatTime(timestamp) {
        if (!timestamp) return 'N/A';
        const d = new Date(timestamp);
        return d.toLocaleString('en-US', {
            weekday: 'short',
            hour: 'numeric',
            minute: '2-digit'
        });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .oncall-calendar {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .oncall-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .header-views { display: flex; gap: 0.25rem; }

            .view-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.6rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.8rem;
            }

            .view-btn:hover { background: var(--bg-card, #16181c); }
            .view-btn.active { background: var(--bg-card, #16181c); color: var(--text, #e7e9ea); }

            .oncall-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .empty-state {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .empty-state .icon { font-size: 2rem; margin-bottom: 0.5rem; }

            /* Current On-Call */
            .current-oncall { display: flex; flex-direction: column; gap: 1rem; }

            .schedule-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .schedule-card.empty { opacity: 0.6; }

            .schedule-name {
                font-weight: 600;
                font-size: 0.85rem;
                margin-bottom: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .oncall-people { display: flex; flex-direction: column; gap: 0.5rem; }

            .person-card {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                border-left: 3px solid var(--accent, #1d9bf0);
            }

            .person-card.backup {
                border-left-color: var(--text-muted, #71767b);
                opacity: 0.8;
            }

            .person-avatar {
                width: 40px;
                height: 40px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
                font-size: 0.9rem;
            }

            .person-info { flex: 1; }

            .person-name { font-weight: 500; }

            .person-role {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .shift-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .person-actions { display: flex; gap: 0.5rem; }

            .btn-contact {
                width: 32px;
                height: 32px;
                display: flex;
                align-items: center;
                justify-content: center;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                text-decoration: none;
                font-size: 0.9rem;
            }

            .no-oncall {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            /* Week View */
            .week-view { display: flex; flex-direction: column; }

            .week-header {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
                margin-bottom: 0.5rem;
            }

            .day-header {
                text-align: center;
                padding: 0.5rem;
            }

            .day-header.today {
                background: var(--accent, #1d9bf0);
                border-radius: 6px;
                color: white;
            }

            .day-name { display: block; font-size: 0.7rem; color: var(--text-muted, #71767b); }
            .day-header.today .day-name { color: rgba(255,255,255,0.8); }

            .day-date { font-weight: 600; }

            .schedule-row {
                display: flex;
                align-items: center;
                margin-bottom: 0.5rem;
            }

            .schedule-label {
                width: 80px;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .schedule-shifts {
                flex: 1;
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
            }

            .shift-cell {
                background: var(--bg-elevated, #1e2128);
                padding: 0.5rem;
                border-radius: 4px;
                text-align: center;
                min-height: 40px;
                display: flex;
                align-items: center;
                justify-content: center;
            }

            .shift-cell.today { border: 1px solid var(--accent, #1d9bf0); }

            .shift-person {
                width: 28px;
                height: 28px;
                background: var(--accent, #1d9bf0);
                color: white;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.7rem;
                font-weight: 600;
            }

            .no-shift { color: var(--text-muted, #71767b); }

            /* Month View */
            .month-view { }

            .month-header {
                text-align: center;
                margin-bottom: 1rem;
            }

            .month-name { font-weight: 600; font-size: 1.1rem; }

            .weekday-row {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
                margin-bottom: 0.25rem;
            }

            .weekday {
                text-align: center;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                padding: 0.25rem;
            }

            .days-grid {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
            }

            .day-cell {
                aspect-ratio: 1;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                padding: 0.25rem;
                position: relative;
            }

            .day-cell.empty { background: transparent; }

            .day-cell.today {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .day-num { font-size: 0.75rem; }

            .shift-indicator {
                position: absolute;
                bottom: 4px;
                left: 50%;
                transform: translateX(-50%);
                width: 6px;
                height: 6px;
                background: var(--success, #00ba7c);
                border-radius: 50%;
            }
        `;
    }
}

customElements.define('oncall-calendar', OncallCalendar);

/**
 * Resource Treemap Component
 * Hierarchical resource usage visualization
 */
class ResourceTreemap extends HTMLElement {
    constructor() {
        super();
        this.data = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    static get observedAttributes() {
        return ['resource', 'namespace'];
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

        const width = body.clientWidth;
        const height = body.clientHeight || 250;
        const items = this.data.items;

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

/**
 * Sankey Diagram Component
 * Request flow visualization through services
 */
class SankeyDiagram extends HTMLElement {
    constructor() {
        super();
        this.data = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    static get observedAttributes() {
        return ['time-range'];
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

        const rect = svg.getBoundingClientRect();
        const width = rect.width || 600;
        const height = rect.height || 300;
        const padding = { top: 20, right: 100, bottom: 20, left: 100 };

        const { nodes, links } = this.data;

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

/**
 * Service Detail Widget
 * Single service deep-dive: RED metrics, traces, logs, deploys
 */
class ServiceDetail extends HTMLElement {
    constructor() {
        super();
        this.service = null;
        this.serviceName = '';
        this.metrics = {};
        this.traces = [];
        this.logs = [];
        this.deploys = [];
        this.incidents = [];
        this.activeTab = 'overview';
    }

    static get observedAttributes() {
        return ['service-name'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'service-name' && newValue && newValue !== oldValue) {
            this.serviceName = newValue;
            this.loadServiceData();
        }
    }

    connectedCallback() {
        this.render();
        if (this.serviceName) {
            this.loadServiceData();
        }
    }

    async loadServiceData() {
        if (!this.serviceName) return;

        this.showLoading();

        try {
            const [catalogResp, tracesResp, deploysResp, incidentsResp] = await Promise.all([
                fetch(`/api/catalog/services?name=${encodeURIComponent(this.serviceName)}`),
                fetch(`/api/traces?service=${encodeURIComponent(this.serviceName)}&limit=20`),
                fetch(`/api/deploys?service=${encodeURIComponent(this.serviceName)}&limit=10`),
                fetch(`/api/incidents?service=${encodeURIComponent(this.serviceName)}&limit=5`)
            ]);

            if (catalogResp.ok) {
                const services = await catalogResp.json();
                this.service = services?.find(s => s.name === this.serviceName) || { name: this.serviceName };
            }

            if (tracesResp.ok) this.traces = await tracesResp.json() || [];
            if (deploysResp.ok) this.deploys = await deploysResp.json() || [];
            if (incidentsResp.ok) this.incidents = await incidentsResp.json() || [];

            // Calculate RED metrics from traces
            this.calculateMetrics();

            this.renderContent();
        } catch (e) {
            console.error('Failed to load service data:', e);
            this.showError(e.message);
        }
    }

    calculateMetrics() {
        if (this.traces.length === 0) {
            this.metrics = { rate: 0, errorRate: 0, p50: 0, p95: 0, p99: 0 };
            return;
        }

        const durations = this.traces.map(t => t.duration_ms || 0).sort((a, b) => a - b);
        const errors = this.traces.filter(t => t.has_error).length;

        this.metrics = {
            rate: this.traces.length, // per time window
            errorRate: (errors / this.traces.length) * 100,
            p50: durations[Math.floor(durations.length * 0.5)] || 0,
            p95: durations[Math.floor(durations.length * 0.95)] || 0,
            p99: durations[Math.floor(durations.length * 0.99)] || 0
        };
    }

    setTab(tab) {
        this.activeTab = tab;
        this.querySelectorAll('.tab-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.tab === tab);
        });
        this.renderTabContent();
    }

    showLoading() {
        const content = this.querySelector('#service-content');
        if (content) {
            content.innerHTML = `<div class="loading"><div class="spinner"></div>Loading...</div>`;
        }
    }

    showError(message) {
        const content = this.querySelector('#service-content');
        if (content) {
            content.innerHTML = `<div class="error">Error: ${message}</div>`;
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="service-detail">
                <div class="service-header" id="service-header">
                    <div class="header-loading">Select a service to view details</div>
                </div>
                <div class="service-tabs">
                    <button class="tab-btn active" data-tab="overview" onclick="this.getRootNode().host.setTab('overview')">Overview</button>
                    <button class="tab-btn" data-tab="traces" onclick="this.getRootNode().host.setTab('traces')">Traces</button>
                    <button class="tab-btn" data-tab="logs" onclick="this.getRootNode().host.setTab('logs')">Logs</button>
                    <button class="tab-btn" data-tab="deploys" onclick="this.getRootNode().host.setTab('deploys')">Deploys</button>
                    <button class="tab-btn" data-tab="incidents" onclick="this.getRootNode().host.setTab('incidents')">Incidents</button>
                </div>
                <div class="service-content" id="service-content">
                    <div class="empty-state">
                        <span class="icon">🔧</span>
                        <p>Select a service to view details</p>
                    </div>
                </div>
            </div>
        `;
    }

    renderContent() {
        this.renderHeader();
        this.renderTabContent();
    }

    renderHeader() {
        const header = this.querySelector('#service-header');
        if (!header) return;

        const health = this.getHealthStatus();
        const tier = this.service?.tier || 'unknown';

        header.innerHTML = `
            <div class="header-main">
                <div class="service-icon">${this.getServiceIcon()}</div>
                <div class="service-info">
                    <h1 class="service-name">${this.escapeHtml(this.serviceName)}</h1>
                    <div class="service-meta">
                        ${this.service?.team ? `<span class="meta-item">Team: ${this.escapeHtml(this.service.team)}</span>` : ''}
                        ${this.service?.owner ? `<span class="meta-item">Owner: ${this.escapeHtml(this.service.owner)}</span>` : ''}
                        <span class="meta-item">Tier: ${tier}</span>
                    </div>
                </div>
                <div class="service-status">
                    <span class="health-badge ${health.status}">${health.label}</span>
                </div>
            </div>
            <div class="header-metrics">
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.rate}</span>
                    <span class="metric-label">Requests</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value ${this.metrics.errorRate > 1 ? 'error' : ''}">${this.metrics.errorRate.toFixed(2)}%</span>
                    <span class="metric-label">Error Rate</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p50.toFixed(0)}ms</span>
                    <span class="metric-label">p50 Latency</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p95.toFixed(0)}ms</span>
                    <span class="metric-label">p95 Latency</span>
                </div>
                <div class="metric-card">
                    <span class="metric-value">${this.metrics.p99.toFixed(0)}ms</span>
                    <span class="metric-label">p99 Latency</span>
                </div>
            </div>
        `;
    }

    renderTabContent() {
        const content = this.querySelector('#service-content');
        if (!content) return;

        switch (this.activeTab) {
            case 'overview':
                content.innerHTML = this.renderOverview();
                break;
            case 'traces':
                content.innerHTML = this.renderTraces();
                break;
            case 'logs':
                content.innerHTML = this.renderLogs();
                break;
            case 'deploys':
                content.innerHTML = this.renderDeploys();
                break;
            case 'incidents':
                content.innerHTML = this.renderIncidents();
                break;
        }
    }

    renderOverview() {
        return `
            <div class="overview-grid">
                <div class="overview-card">
                    <h3>Service Info</h3>
                    <div class="info-list">
                        <div class="info-item">
                            <span class="info-label">Repository</span>
                            <span class="info-value">${this.service?.repo_url ? `<a href="${this.escapeHtml(this.service.repo_url)}" target="_blank">${this.escapeHtml(this.service.repo_url)}</a>` : '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Language</span>
                            <span class="info-value">${this.service?.language || '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Framework</span>
                            <span class="info-value">${this.service?.framework || '—'}</span>
                        </div>
                        <div class="info-item">
                            <span class="info-label">Tags</span>
                            <span class="info-value">${this.service?.tags?.join(', ') || '—'}</span>
                        </div>
                    </div>
                </div>
                <div class="overview-card">
                    <h3>Recent Activity</h3>
                    <div class="activity-list">
                        ${this.deploys.slice(0, 3).map(d => `
                            <div class="activity-item">
                                <span class="activity-icon">🚀</span>
                                <span class="activity-text">Deploy: ${this.escapeHtml(d.version || d.commit?.substring(0, 7) || 'unknown')}</span>
                                <span class="activity-time">${this.formatTime(d.deployed_at)}</span>
                            </div>
                        `).join('') || '<div class="empty">No recent deploys</div>'}
                        ${this.incidents.filter(i => i.status !== 'resolved').slice(0, 2).map(i => `
                            <div class="activity-item incident">
                                <span class="activity-icon">🚨</span>
                                <span class="activity-text">${this.escapeHtml(i.title)}</span>
                                <span class="activity-time">${this.formatTime(i.created_at)}</span>
                            </div>
                        `).join('')}
                    </div>
                </div>
                <div class="overview-card full-width">
                    <h3>Dependencies</h3>
                    <div class="deps-list">
                        ${this.service?.dependencies?.map(d => `
                            <span class="dep-badge">${this.escapeHtml(d)}</span>
                        `).join('') || '<span class="empty">No dependencies tracked</span>'}
                    </div>
                </div>
            </div>
        `;
    }

    renderTraces() {
        if (this.traces.length === 0) {
            return `<div class="empty-state"><span class="icon">🔍</span><p>No traces found</p></div>`;
        }

        return `
            <div class="traces-list">
                <div class="list-header">
                    <span class="col-status">Status</span>
                    <span class="col-operation">Operation</span>
                    <span class="col-duration">Duration</span>
                    <span class="col-time">Time</span>
                </div>
                ${this.traces.map(t => `
                    <div class="trace-row ${t.has_error ? 'error' : ''}" onclick="window.open('/traces.html?id=${t.trace_id}', '_blank')">
                        <span class="col-status">
                            <span class="status-dot ${t.has_error ? 'error' : 'ok'}"></span>
                        </span>
                        <span class="col-operation">${this.escapeHtml(t.root_span || 'unknown')}</span>
                        <span class="col-duration">${(t.duration_ms || 0).toFixed(1)}ms</span>
                        <span class="col-time">${this.formatTime(t.start_time)}</span>
                    </div>
                `).join('')}
            </div>
            <div class="list-footer">
                <a href="/traces.html?service=${encodeURIComponent(this.serviceName)}" class="link-more">View all traces →</a>
            </div>
        `;
    }

    renderLogs() {
        return `
            <div class="logs-embed">
                <log-explorer></log-explorer>
            </div>
            <div class="list-footer">
                <a href="/logs.html?service=${encodeURIComponent(this.serviceName)}" class="link-more">Open in Log Explorer →</a>
            </div>
        `;
    }

    renderDeploys() {
        if (this.deploys.length === 0) {
            return `<div class="empty-state"><span class="icon">🚀</span><p>No deploys found</p></div>`;
        }

        return `
            <div class="deploys-list">
                ${this.deploys.map(d => `
                    <div class="deploy-card ${d.status || 'success'}">
                        <div class="deploy-main">
                            <div class="deploy-version">${this.escapeHtml(d.version || d.commit?.substring(0, 7) || 'unknown')}</div>
                            <div class="deploy-meta">
                                <span>by ${this.escapeHtml(d.deployed_by || 'unknown')}</span>
                                <span>${this.formatTime(d.deployed_at)}</span>
                            </div>
                            ${d.message ? `<div class="deploy-message">${this.escapeHtml(d.message)}</div>` : ''}
                        </div>
                        <div class="deploy-status">
                            <span class="status-badge ${d.status || 'success'}">${d.status || 'success'}</span>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderIncidents() {
        if (this.incidents.length === 0) {
            return `<div class="empty-state"><span class="icon">✓</span><p>No incidents</p></div>`;
        }

        return `
            <div class="incidents-list">
                ${this.incidents.map(i => `
                    <div class="incident-card ${i.severity}">
                        <div class="incident-severity">
                            ${this.getSeverityIcon(i.severity)}
                        </div>
                        <div class="incident-main">
                            <div class="incident-title">${this.escapeHtml(i.title)}</div>
                            <div class="incident-meta">
                                <span class="status-badge ${i.status}">${i.status}</span>
                                <span>${this.formatTime(i.created_at)}</span>
                            </div>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    getHealthStatus() {
        if (this.metrics.errorRate > 5) return { status: 'critical', label: 'Critical' };
        if (this.metrics.errorRate > 1) return { status: 'degraded', label: 'Degraded' };
        return { status: 'healthy', label: 'Healthy' };
    }

    getServiceIcon() {
        const type = this.service?.type || 'service';
        switch (type) {
            case 'database': return '🗄️';
            case 'cache': return '⚡';
            case 'queue': return '📬';
            case 'gateway': return '🚪';
            case 'frontend': return '🖥️';
            default: return '🔧';
        }
    }

    getSeverityIcon(severity) {
        switch (severity) {
            case 'critical': return '🔴';
            case 'high': return '🟠';
            case 'medium': return '🟡';
            case 'low': return '🟢';
            default: return '⚪';
        }
    }

    formatTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const now = new Date();
        const diffMs = now - d;

        if (diffMs < 60000) return 'just now';
        if (diffMs < 3600000) return `${Math.floor(diffMs / 60000)}m ago`;
        if (diffMs < 86400000) return `${Math.floor(diffMs / 3600000)}h ago`;

        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .service-detail {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .service-header {
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-loading {
                text-align: center;
                color: var(--text-muted, #71767b);
                padding: 1rem;
            }

            .header-main {
                display: flex;
                align-items: center;
                gap: 1rem;
                margin-bottom: 1rem;
            }

            .service-icon {
                font-size: 2rem;
            }

            .service-info { flex: 1; }

            .service-name {
                font-size: 1.25rem;
                font-weight: 600;
                margin: 0 0 0.25rem 0;
            }

            .service-meta {
                display: flex;
                gap: 1rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .health-badge {
                padding: 0.25rem 0.75rem;
                border-radius: 12px;
                font-size: 0.8rem;
                font-weight: 500;
            }

            .health-badge.healthy { background: var(--success, #00ba7c); color: white; }
            .health-badge.degraded { background: var(--warning, #ffd400); color: #1a1a1a; }
            .health-badge.critical { background: var(--error, #f4212e); color: white; }

            .header-metrics {
                display: flex;
                gap: 1rem;
            }

            .metric-card {
                background: var(--bg-card, #16181c);
                padding: 0.75rem 1rem;
                border-radius: 8px;
                text-align: center;
                min-width: 80px;
            }

            .metric-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
            }

            .metric-value.error { color: var(--error, #f4212e); }

            .metric-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .service-tabs {
                display: flex;
                gap: 0.25rem;
                padding: 0 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tab-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.75rem 1rem;
                cursor: pointer;
                border-bottom: 2px solid transparent;
                margin-bottom: -1px;
            }

            .tab-btn:hover { color: var(--text, #e7e9ea); }
            .tab-btn.active {
                color: var(--accent, #1d9bf0);
                border-bottom-color: var(--accent, #1d9bf0);
            }

            .service-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .empty-state, .error {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .spinner {
                width: 24px;
                height: 24px;
                border: 3px solid var(--border, #2f3336);
                border-top-color: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: spin 0.8s linear infinite;
                margin-bottom: 0.5rem;
            }

            @keyframes spin { to { transform: rotate(360deg); } }

            .empty-state .icon { font-size: 2rem; margin-bottom: 0.5rem; }

            /* Overview */
            .overview-grid {
                display: grid;
                grid-template-columns: repeat(2, 1fr);
                gap: 1rem;
            }

            .overview-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .overview-card.full-width { grid-column: span 2; }

            .overview-card h3 {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
            }

            .info-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .info-item {
                display: flex;
                justify-content: space-between;
                font-size: 0.85rem;
            }

            .info-label { color: var(--text-muted, #71767b); }

            .info-value a { color: var(--accent, #1d9bf0); text-decoration: none; }

            .activity-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .activity-item {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-size: 0.85rem;
            }

            .activity-item.incident { color: var(--error, #f4212e); }

            .activity-time {
                margin-left: auto;
                color: var(--text-muted, #71767b);
                font-size: 0.75rem;
            }

            .deps-list { display: flex; flex-wrap: wrap; gap: 0.5rem; }

            .dep-badge {
                background: var(--bg-card, #16181c);
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.8rem;
            }

            /* Traces */
            .traces-list { display: flex; flex-direction: column; }

            .list-header {
                display: flex;
                padding: 0.5rem 0;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .trace-row {
                display: flex;
                padding: 0.75rem 0;
                border-bottom: 1px solid var(--border, #2f3336);
                cursor: pointer;
            }

            .trace-row:hover { background: var(--bg-elevated, #1e2128); }

            .col-status { width: 40px; }
            .col-operation { flex: 1; }
            .col-duration { width: 80px; text-align: right; }
            .col-time { width: 100px; text-align: right; color: var(--text-muted, #71767b); }

            .status-dot {
                display: inline-block;
                width: 8px;
                height: 8px;
                border-radius: 50%;
            }

            .status-dot.ok { background: var(--success, #00ba7c); }
            .status-dot.error { background: var(--error, #f4212e); }

            .list-footer {
                padding: 1rem 0;
                text-align: center;
            }

            .link-more {
                color: var(--accent, #1d9bf0);
                text-decoration: none;
                font-size: 0.85rem;
            }

            /* Deploys */
            .deploys-list { display: flex; flex-direction: column; gap: 0.75rem; }

            .deploy-card {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 3px solid var(--success, #00ba7c);
            }

            .deploy-card.failed { border-left-color: var(--error, #f4212e); }

            .deploy-version { font-weight: 600; margin-bottom: 0.25rem; }

            .deploy-meta {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                display: flex;
                gap: 1rem;
            }

            .deploy-message {
                font-size: 0.85rem;
                margin-top: 0.5rem;
            }

            .status-badge {
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.7rem;
                text-transform: uppercase;
            }

            .status-badge.success { background: var(--success, #00ba7c); color: white; }
            .status-badge.failed { background: var(--error, #f4212e); color: white; }

            /* Incidents */
            .incidents-list { display: flex; flex-direction: column; gap: 0.75rem; }

            .incident-card {
                display: flex;
                gap: 0.75rem;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .incident-card.critical { border-left-color: var(--error, #f4212e); }
            .incident-card.high { border-left-color: #ff7a00; }

            .incident-severity { font-size: 1.25rem; }
            .incident-main { flex: 1; }

            .incident-title { font-weight: 500; margin-bottom: 0.25rem; }

            .incident-meta {
                display: flex;
                gap: 0.75rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .status-badge.active { background: var(--error, #f4212e); color: white; }
            .status-badge.resolved { background: var(--success, #00ba7c); color: white; }

            .logs-embed {
                height: 300px;
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                overflow: hidden;
            }

            .empty { color: var(--text-muted, #71767b); font-size: 0.85rem; }
        `;
    }
}

customElements.define('service-detail', ServiceDetail);

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
        if (window.Loader) {
            await window.Loader.load('d3');
        } else if (typeof d3 === 'undefined') {
            throw new Error('D3 not available and Loader not found');
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
            }
        } catch (e) {
            console.error('[ServiceMap] Failed to fetch data:', e);
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

/**
 * SLO Status Cards Widget
 * Error budgets, burn rate visualization
 */
class SloCards extends HTMLElement {
    constructor() {
        super();
        this.slos = [];
        this.view = 'grid'; // grid, list
    }

    connectedCallback() {
        this.render();
        this.loadSLOs();
        this.refreshInterval = setInterval(() => this.loadSLOs(), 60000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) clearInterval(this.refreshInterval);
    }

    async loadSLOs() {
        try {
            const resp = await fetch('/api/slos');
            if (resp.ok) {
                this.slos = await resp.json() || [];
                this.renderContent();
            }
        } catch (e) {
            console.error('Failed to load SLOs:', e);
        }
    }

    setView(view) {
        this.view = view;
        this.querySelectorAll('.view-btn').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.view === view);
        });
        this.renderContent();
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="slo-cards">
                <div class="slo-header">
                    <div class="slo-title">
                        <span class="title-icon">🎯</span>
                        <span>SLO Status</span>
                    </div>
                    <div class="slo-view-toggle">
                        <button class="view-btn active" data-view="grid" onclick="this.getRootNode().host.setView('grid')">▦</button>
                        <button class="view-btn" data-view="list" onclick="this.getRootNode().host.setView('list')">☰</button>
                    </div>
                    <button class="btn-sm" onclick="this.getRootNode().host.showCreateSLO()">+ New SLO</button>
                </div>
                <div class="slo-summary" id="slo-summary"></div>
                <div class="slo-content" id="slo-content">
                    <div class="loading">Loading SLOs...</div>
                </div>
            </div>
        `;
    }

    renderContent() {
        const container = this.querySelector('#slo-content');
        const summary = this.querySelector('#slo-summary');

        // Calculate summary stats
        const atRisk = this.slos.filter(s => this.getBudgetRemaining(s) < 20).length;
        const healthy = this.slos.filter(s => this.getBudgetRemaining(s) >= 20).length;

        summary.innerHTML = `
            <div class="summary-stat">
                <span class="stat-value healthy">${healthy}</span>
                <span class="stat-label">Healthy</span>
            </div>
            <div class="summary-stat">
                <span class="stat-value at-risk">${atRisk}</span>
                <span class="stat-label">At Risk</span>
            </div>
            <div class="summary-stat">
                <span class="stat-value">${this.slos.length}</span>
                <span class="stat-label">Total</span>
            </div>
        `;

        if (this.slos.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <span class="icon">🎯</span>
                    <p>No SLOs configured</p>
                    <button class="btn-create" onclick="this.getRootNode().host.showCreateSLO()">Create SLO</button>
                </div>
            `;
            return;
        }

        const sorted = [...this.slos].sort((a, b) => this.getBudgetRemaining(a) - this.getBudgetRemaining(b));

        if (this.view === 'grid') {
            container.innerHTML = `
                <div class="slo-grid">
                    ${sorted.map(slo => this.renderSLOCard(slo)).join('')}
                </div>
            `;
        } else {
            container.innerHTML = `
                <div class="slo-list">
                    ${sorted.map(slo => this.renderSLORow(slo)).join('')}
                </div>
            `;
        }
    }

    renderSLOCard(slo) {
        const budgetRemaining = this.getBudgetRemaining(slo);
        const burnRate = this.getBurnRate(slo);
        const status = this.getStatus(budgetRemaining);
        const current = slo.current_value || 0;
        const target = slo.target || 99.9;

        return `
            <div class="slo-card status-${status}" onclick="this.getRootNode().host.showSLODetail('${slo.id}')">
                <div class="card-header">
                    <span class="slo-name">${this.escapeHtml(slo.name)}</span>
                    <span class="slo-service">${this.escapeHtml(slo.service || '')}</span>
                </div>
                <div class="card-body">
                    <div class="budget-ring">
                        <svg viewBox="0 0 36 36" class="budget-chart">
                            <path class="budget-bg" d="M18 2.0845
                                a 15.9155 15.9155 0 0 1 0 31.831
                                a 15.9155 15.9155 0 0 1 0 -31.831"/>
                            <path class="budget-fill ${status}" stroke-dasharray="${budgetRemaining}, 100" d="M18 2.0845
                                a 15.9155 15.9155 0 0 1 0 31.831
                                a 15.9155 15.9155 0 0 1 0 -31.831"/>
                        </svg>
                        <div class="budget-value">
                            <span class="budget-pct">${budgetRemaining.toFixed(0)}%</span>
                            <span class="budget-label">budget</span>
                        </div>
                    </div>
                    <div class="slo-metrics">
                        <div class="metric">
                            <span class="metric-label">Current</span>
                            <span class="metric-value">${current.toFixed(2)}%</span>
                        </div>
                        <div class="metric">
                            <span class="metric-label">Target</span>
                            <span class="metric-value">${target}%</span>
                        </div>
                        <div class="metric">
                            <span class="metric-label">Burn Rate</span>
                            <span class="metric-value ${burnRate > 1 ? 'warning' : ''}">${burnRate.toFixed(1)}x</span>
                        </div>
                    </div>
                </div>
                <div class="card-footer">
                    <span class="slo-window">${slo.window || '30d'} window</span>
                    ${status === 'critical' ? '<span class="alert-badge">At Risk</span>' : ''}
                </div>
            </div>
        `;
    }

    renderSLORow(slo) {
        const budgetRemaining = this.getBudgetRemaining(slo);
        const burnRate = this.getBurnRate(slo);
        const status = this.getStatus(budgetRemaining);
        const current = slo.current_value || 0;
        const target = slo.target || 99.9;

        return `
            <div class="slo-row status-${status}" onclick="this.getRootNode().host.showSLODetail('${slo.id}')">
                <div class="row-status">
                    <span class="status-dot ${status}"></span>
                </div>
                <div class="row-name">
                    <span class="slo-name">${this.escapeHtml(slo.name)}</span>
                    <span class="slo-service">${this.escapeHtml(slo.service || '')}</span>
                </div>
                <div class="row-metric">
                    <span class="metric-label">Current</span>
                    <span class="metric-value">${current.toFixed(2)}%</span>
                </div>
                <div class="row-metric">
                    <span class="metric-label">Target</span>
                    <span class="metric-value">${target}%</span>
                </div>
                <div class="row-budget">
                    <div class="budget-bar">
                        <div class="budget-bar-fill ${status}" style="width: ${budgetRemaining}%"></div>
                    </div>
                    <span class="budget-text">${budgetRemaining.toFixed(0)}% remaining</span>
                </div>
                <div class="row-burn">
                    <span class="${burnRate > 1 ? 'warning' : ''}">${burnRate.toFixed(1)}x</span>
                </div>
            </div>
        `;
    }

    getBudgetRemaining(slo) {
        if (slo.budget_remaining !== undefined) return Math.max(0, Math.min(100, slo.budget_remaining));

        const target = slo.target || 99.9;
        const current = slo.current_value || target;
        const errorBudget = 100 - target;
        const errorUsed = Math.max(0, target - current);
        const remaining = ((errorBudget - errorUsed) / errorBudget) * 100;

        return Math.max(0, Math.min(100, remaining));
    }

    getBurnRate(slo) {
        if (slo.burn_rate !== undefined) return slo.burn_rate;
        // Estimate burn rate based on current vs target
        const budgetRemaining = this.getBudgetRemaining(slo);
        // Assuming 30 days, if we're at 50% with 15 days left, burn rate is 1x
        return budgetRemaining < 50 ? 1.5 : 1.0;
    }

    getStatus(budgetRemaining) {
        if (budgetRemaining < 10) return 'critical';
        if (budgetRemaining < 30) return 'warning';
        return 'healthy';
    }

    showSLODetail(id) {
        const slo = this.slos.find(s => s.id === id);
        if (!slo) return;

        const budgetRemaining = this.getBudgetRemaining(slo);
        const burnRate = this.getBurnRate(slo);

        alert(`
SLO: ${slo.name}
Service: ${slo.service || '—'}
Target: ${slo.target}%
Current: ${(slo.current_value || 0).toFixed(2)}%
Budget Remaining: ${budgetRemaining.toFixed(1)}%
Burn Rate: ${burnRate.toFixed(2)}x
Window: ${slo.window || '30d'}
        `);
    }

    showCreateSLO() {
        window.location.href = '/#slos';
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .slo-cards {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .slo-header {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .slo-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .slo-view-toggle {
                display: flex;
                margin-left: auto;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                overflow: hidden;
            }

            .view-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
            }

            .view-btn:hover { color: var(--text, #e7e9ea); }
            .view-btn.active { background: var(--bg-elevated, #1e2128); color: var(--text, #e7e9ea); }

            .btn-sm {
                background: var(--accent, #1d9bf0);
                border: none;
                color: white;
                padding: 0.4rem 0.75rem;
                border-radius: 6px;
                cursor: pointer;
                font-size: 0.8rem;
            }

            .slo-summary {
                display: flex;
                gap: 2rem;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .summary-stat {
                display: flex;
                flex-direction: column;
                align-items: center;
            }

            .stat-value {
                font-size: 1.5rem;
                font-weight: 600;
            }

            .stat-value.healthy { color: var(--success, #00ba7c); }
            .stat-value.at-risk { color: var(--error, #f4212e); }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
            }

            .slo-content {
                flex: 1;
                overflow-y: auto;
                padding: 1rem;
            }

            .loading, .empty-state {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .empty-state .icon { font-size: 2.5rem; margin-bottom: 1rem; }

            .btn-create {
                background: var(--accent, #1d9bf0);
                border: none;
                color: white;
                padding: 0.5rem 1rem;
                border-radius: 6px;
                cursor: pointer;
                margin-top: 1rem;
            }

            .slo-grid {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
                gap: 1rem;
            }

            .slo-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                cursor: pointer;
                border: 1px solid var(--border, #2f3336);
                transition: all 0.15s;
            }

            .slo-card:hover { border-color: var(--accent, #1d9bf0); }

            .slo-card.status-critical { border-left: 4px solid var(--error, #f4212e); }
            .slo-card.status-warning { border-left: 4px solid var(--warning, #ffd400); }
            .slo-card.status-healthy { border-left: 4px solid var(--success, #00ba7c); }

            .card-header {
                margin-bottom: 1rem;
            }

            .slo-name {
                display: block;
                font-weight: 600;
                font-size: 0.9rem;
                margin-bottom: 0.25rem;
            }

            .slo-service {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .card-body {
                display: flex;
                gap: 1rem;
                align-items: center;
            }

            .budget-ring {
                position: relative;
                width: 80px;
                height: 80px;
            }

            .budget-chart {
                transform: rotate(-90deg);
            }

            .budget-bg {
                fill: none;
                stroke: var(--bg-card, #16181c);
                stroke-width: 3.5;
            }

            .budget-fill {
                fill: none;
                stroke-width: 3.5;
                stroke-linecap: round;
            }

            .budget-fill.healthy { stroke: var(--success, #00ba7c); }
            .budget-fill.warning { stroke: var(--warning, #ffd400); }
            .budget-fill.critical { stroke: var(--error, #f4212e); }

            .budget-value {
                position: absolute;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                text-align: center;
            }

            .budget-pct {
                display: block;
                font-size: 1.1rem;
                font-weight: 600;
            }

            .budget-label {
                font-size: 0.6rem;
                color: var(--text-muted, #71767b);
            }

            .slo-metrics {
                flex: 1;
            }

            .metric {
                display: flex;
                justify-content: space-between;
                padding: 0.25rem 0;
                font-size: 0.8rem;
            }

            .metric-label { color: var(--text-muted, #71767b); }
            .metric-value.warning { color: var(--warning, #ffd400); }

            .card-footer {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-top: 1rem;
                padding-top: 0.75rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .slo-window {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .alert-badge {
                background: var(--error, #f4212e);
                color: white;
                padding: 0.15rem 0.5rem;
                border-radius: 10px;
                font-size: 0.65rem;
            }

            /* List view */
            .slo-list { display: flex; flex-direction: column; gap: 0.5rem; }

            .slo-row {
                display: flex;
                align-items: center;
                gap: 1rem;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                cursor: pointer;
            }

            .slo-row:hover { background: var(--bg-card, #16181c); }

            .row-status {
                width: 20px;
            }

            .status-dot {
                display: block;
                width: 10px;
                height: 10px;
                border-radius: 50%;
            }

            .status-dot.healthy { background: var(--success, #00ba7c); }
            .status-dot.warning { background: var(--warning, #ffd400); }
            .status-dot.critical { background: var(--error, #f4212e); }

            .row-name {
                flex: 1;
            }

            .row-name .slo-name {
                margin-bottom: 0;
            }

            .row-metric {
                width: 80px;
                text-align: right;
            }

            .row-metric .metric-label {
                display: block;
                font-size: 0.65rem;
            }

            .row-metric .metric-value {
                font-size: 0.85rem;
            }

            .row-budget {
                width: 150px;
            }

            .budget-bar {
                height: 6px;
                background: var(--bg-card, #16181c);
                border-radius: 3px;
                overflow: hidden;
                margin-bottom: 0.25rem;
            }

            .budget-bar-fill {
                height: 100%;
                border-radius: 3px;
            }

            .budget-bar-fill.healthy { background: var(--success, #00ba7c); }
            .budget-bar-fill.warning { background: var(--warning, #ffd400); }
            .budget-bar-fill.critical { background: var(--error, #f4212e); }

            .budget-text {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .row-burn {
                width: 50px;
                text-align: right;
                font-size: 0.85rem;
            }

            .row-burn .warning { color: var(--warning, #ffd400); }
        `;
    }
}

customElements.define('slo-cards', SloCards);

/**
 * Stat Gauge Component
 * Big number display with optional gauge arc
 */
class StatGauge extends HTMLElement {
    constructor() {
        super();
        this.value = 0;
        this.animationFrame = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Auto-refresh
        this.refreshInterval = setInterval(() => this.loadData(), 10000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        if (this.animationFrame) cancelAnimationFrame(this.animationFrame);
    }

    static get observedAttributes() {
        return ['metric', 'title', 'unit', 'min', 'max', 'thresholds', 'show-gauge'];
    }

    get metric() { return this.getAttribute('metric') || ''; }
    get title() { return this.getAttribute('title') || 'Metric'; }
    get unit() { return this.getAttribute('unit') || ''; }
    get min() { return parseFloat(this.getAttribute('min')) || 0; }
    get max() { return parseFloat(this.getAttribute('max')) || 100; }
    get showGauge() { return this.getAttribute('show-gauge') !== 'false'; }
    get thresholds() {
        try {
            return JSON.parse(this.getAttribute('thresholds') || '{}');
        } catch { return {}; }
    }

    render() {
        this.innerHTML = `
            <style>
                .stat-gauge-container {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    padding: 1rem;
                }
                .stat-gauge-title {
                    font-size: 0.85rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.5rem;
                }
                .stat-gauge-value-container {
                    position: relative;
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                }
                .stat-gauge-svg {
                    width: 120px;
                    height: 70px;
                }
                .stat-gauge-bg {
                    fill: none;
                    stroke: var(--border-color, #2f3336);
                    stroke-width: 8;
                    stroke-linecap: round;
                }
                .stat-gauge-fill {
                    fill: none;
                    stroke-width: 8;
                    stroke-linecap: round;
                    transition: stroke-dashoffset 0.5s ease, stroke 0.3s ease;
                }
                .stat-gauge-value {
                    font-size: 2rem;
                    font-weight: 700;
                    line-height: 1;
                    margin-top: 0.5rem;
                }
                .stat-gauge-unit {
                    font-size: 0.9rem;
                    color: var(--text-muted, #71767b);
                    margin-top: 0.25rem;
                }
                .stat-gauge-trend {
                    display: flex;
                    align-items: center;
                    gap: 0.25rem;
                    font-size: 0.8rem;
                    margin-top: 0.5rem;
                }
                .stat-gauge-trend.up { color: #22c55e; }
                .stat-gauge-trend.down { color: #f43f5e; }
                .stat-gauge-trend.neutral { color: var(--text-muted, #71767b); }
            </style>
            <div class="stat-gauge-container">
                <div class="stat-gauge-title">${this.title}</div>
                <div class="stat-gauge-value-container">
                    ${this.showGauge ? `
                    <svg class="stat-gauge-svg" viewBox="0 0 120 70">
                        <path class="stat-gauge-bg" d="M 10 60 A 50 50 0 0 1 110 60"></path>
                        <path class="stat-gauge-fill" id="gauge-fill" d="M 10 60 A 50 50 0 0 1 110 60"></path>
                    </svg>
                    ` : ''}
                    <div class="stat-gauge-value" id="value">--</div>
                    <div class="stat-gauge-unit">${this.unit}</div>
                </div>
                <div class="stat-gauge-trend neutral" id="trend">
                    <span id="trend-icon">→</span>
                    <span id="trend-value">--</span>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/current?metric=${this.metric}`);
            let data;
            if (!resp.ok) {
                data = this.generateDemoData();
            } else {
                data = await resp.json();
            }
            this.updateDisplay(data);
        } catch (e) {
            this.updateDisplay(this.generateDemoData());
        }
    }

    generateDemoData() {
        const metrics = {
            'cpu_usage': { value: 45 + Math.random() * 30, trend: Math.random() * 10 - 5 },
            'memory_usage': { value: 60 + Math.random() * 20, trend: Math.random() * 5 - 2 },
            'requests_per_second': { value: 800 + Math.random() * 400, trend: Math.random() * 100 - 50 },
            'error_rate': { value: Math.random() * 2, trend: Math.random() * 0.5 - 0.25 },
            'active_connections': { value: Math.floor(100 + Math.random() * 200), trend: Math.floor(Math.random() * 20 - 10) },
        };
        return metrics[this.metric] || { value: 50 + Math.random() * 50, trend: Math.random() * 10 - 5 };
    }

    updateDisplay(data) {
        const { value, trend } = data;
        const prevValue = this.value;
        this.value = value;

        // Animate value
        this.animateValue(prevValue, value);

        // Update gauge
        if (this.showGauge) {
            this.updateGauge(value);
        }

        // Update trend
        const trendEl = this.querySelector('#trend');
        const trendIcon = this.querySelector('#trend-icon');
        const trendValue = this.querySelector('#trend-value');

        if (trend > 0.5) {
            trendEl.className = 'stat-gauge-trend up';
            trendIcon.textContent = '↑';
            trendValue.textContent = `+${this.formatValue(trend)}`;
        } else if (trend < -0.5) {
            trendEl.className = 'stat-gauge-trend down';
            trendIcon.textContent = '↓';
            trendValue.textContent = this.formatValue(trend);
        } else {
            trendEl.className = 'stat-gauge-trend neutral';
            trendIcon.textContent = '→';
            trendValue.textContent = 'stable';
        }
    }

    animateValue(from, to) {
        const valueEl = this.querySelector('#value');
        if (!valueEl) return;

        const duration = 500;
        const start = performance.now();

        const animate = (now) => {
            const elapsed = now - start;
            const progress = Math.min(elapsed / duration, 1);
            const eased = 1 - Math.pow(1 - progress, 3); // ease-out cubic

            const current = from + (to - from) * eased;
            valueEl.textContent = this.formatValue(current);
            valueEl.style.color = this.getValueColor(current);

            if (progress < 1) {
                this.animationFrame = requestAnimationFrame(animate);
            }
        };

        this.animationFrame = requestAnimationFrame(animate);
    }

    updateGauge(value) {
        const fill = this.querySelector('#gauge-fill');
        if (!fill) return;

        const percentage = Math.min(1, Math.max(0, (value - this.min) / (this.max - this.min)));

        // Arc length calculation
        const arcLength = Math.PI * 50; // radius 50, semicircle
        const dashOffset = arcLength * (1 - percentage);

        fill.style.strokeDasharray = arcLength;
        fill.style.strokeDashoffset = dashOffset;
        fill.style.stroke = this.getValueColor(value);
    }

    getValueColor(value) {
        const { warning, critical } = this.thresholds;

        if (critical !== undefined && value >= critical) return '#f43f5e';
        if (warning !== undefined && value >= warning) return '#f59e0b';
        return '#22c55e';
    }

    formatValue(value) {
        if (value >= 1000000) return (value / 1000000).toFixed(1) + 'M';
        if (value >= 1000) return (value / 1000).toFixed(1) + 'K';
        if (Number.isInteger(value)) return value.toString();
        return value.toFixed(1);
    }
}

customElements.define('stat-gauge', StatGauge);

/**
 * Status Badge Web Component
 *
 * Usage:
 *   <dw-badge status="ok"></dw-badge>
 *   <dw-badge status="warning" pulse></dw-badge>
 *   <dw-badge status="error" size="lg">Custom Text</dw-badge>
 *
 * Attributes:
 *   - status: ok|warning|error|info|muted|pending|alerting|healthy|degraded|unhealthy
 *   - pulse: Add pulsing animation
 *   - size: sm|md|lg (default: md)
 */
class StatusBadge extends HTMLElement {
    static get observedAttributes() {
        return ['status', 'pulse', 'size'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
    }

    connectedCallback() {
        this.render();
    }

    attributeChangedCallback() {
        this.render();
    }

    render() {
        const status = this.getAttribute('status') || 'muted';
        const pulse = this.hasAttribute('pulse');
        const size = this.getAttribute('size') || 'md';

        // Map status to colors
        const statusColors = {
            ok: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            success: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            healthy: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            up: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            resolved: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            met: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },

            warning: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            pending: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            degraded: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            acknowledged: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            at_risk: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },

            error: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            alerting: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            unhealthy: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            down: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            triggered: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            critical: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            breached: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },

            info: { bg: 'rgba(29, 155, 240, 0.2)', color: '#1d9bf0' },

            muted: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' },
            unknown: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' },
            no_data: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' }
        };

        const colors = statusColors[status.toLowerCase()] || statusColors.muted;

        // Size mappings
        const sizes = {
            sm: { padding: '0.1rem 0.3rem', fontSize: '0.6rem' },
            md: { padding: '0.15rem 0.4rem', fontSize: '0.65rem' },
            lg: { padding: '0.2rem 0.5rem', fontSize: '0.7rem' }
        };
        const sizeStyle = sizes[size] || sizes.md;

        // Get display text
        const text = this.textContent.trim() || this.formatStatus(status);

        this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: inline-block;
                }
                .badge {
                    padding: ${sizeStyle.padding};
                    border-radius: 3px;
                    font-size: ${sizeStyle.fontSize};
                    font-weight: 600;
                    text-transform: uppercase;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    background: ${colors.bg};
                    color: ${colors.color};
                    white-space: nowrap;
                    ${pulse ? 'animation: pulse 1s infinite;' : ''}
                }
                @keyframes pulse {
                    0%, 100% { opacity: 1; }
                    50% { opacity: 0.6; }
                }
            </style>
            <span class="badge">${text}</span>
        `;
    }

    formatStatus(status) {
        return status.replace(/_/g, ' ');
    }

    // Public API
    setStatus(status) {
        this.setAttribute('status', status);
    }

    getStatus() {
        return this.getAttribute('status');
    }
}

customElements.define('dw-badge', StatusBadge);

/**
 * Synthetics Uptime Widget
 * Shows synthetic monitor status and uptime percentages
 */
class SyntheticsUptime extends HTMLElement {
    constructor() {
        super();
        this.monitors = [];
        this.loading = true;
        this.filter = 'all'; // all, failing, passing
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.render();

        try {
            const resp = await fetch('/api/synthetics/monitors');
            if (resp.ok) {
                this.monitors = await resp.json() || [];
            }
        } catch (e) {
            console.error('Failed to load synthetics data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    setFilter(filter) {
        this.filter = filter;
        this.render();
    }

    getFilteredMonitors() {
        if (this.filter === 'all') return this.monitors;
        if (this.filter === 'failing') return this.monitors.filter(m => m.status === 'failing' || m.status === 'degraded');
        if (this.filter === 'passing') return this.monitors.filter(m => m.status === 'passing');
        return this.monitors;
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="synthetics-uptime">
                    <div class="synth-header">
                        <span class="title-icon">🔍</span>
                        <span>Synthetic Monitors</span>
                    </div>
                    <div class="loading">Loading monitors...</div>
                </div>
            `;
            return;
        }

        const monitors = this.getFilteredMonitors();
        const stats = this.calculateStats();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="synthetics-uptime">
                <div class="synth-header">
                    <div class="header-title">
                        <span class="title-icon">🔍</span>
                        <span>Synthetic Monitors</span>
                    </div>
                    <button class="btn-refresh" onclick="this.getRootNode().host.loadData()">↻</button>
                </div>

                <div class="stats-bar">
                    <div class="stat-item">
                        <span class="stat-value">${stats.total}</span>
                        <span class="stat-label">Total</span>
                    </div>
                    <div class="stat-item passing">
                        <span class="stat-value">${stats.passing}</span>
                        <span class="stat-label">Passing</span>
                    </div>
                    <div class="stat-item failing">
                        <span class="stat-value">${stats.failing}</span>
                        <span class="stat-label">Failing</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value">${stats.avgUptime.toFixed(1)}%</span>
                        <span class="stat-label">Avg Uptime</span>
                    </div>
                </div>

                <div class="filter-tabs">
                    <button class="tab ${this.filter === 'all' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('all')">All</button>
                    <button class="tab ${this.filter === 'failing' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('failing')">Failing</button>
                    <button class="tab ${this.filter === 'passing' ? 'active' : ''}"
                            onclick="this.getRootNode().host.setFilter('passing')">Passing</button>
                </div>

                <div class="monitors-list">
                    ${monitors.length === 0 ? `
                        <div class="empty-state">No monitors match filter</div>
                    ` : monitors.map(m => this.renderMonitor(m)).join('')}
                </div>
            </div>
        `;
    }

    renderMonitor(monitor) {
        const statusClass = monitor.status === 'passing' ? 'passing' :
                           monitor.status === 'degraded' ? 'degraded' : 'failing';
        const statusIcon = monitor.status === 'passing' ? '✓' :
                          monitor.status === 'degraded' ? '!' : '✗';

        return `
            <div class="monitor-item ${statusClass}">
                <div class="monitor-status">
                    <span class="status-icon">${statusIcon}</span>
                </div>
                <div class="monitor-info">
                    <div class="monitor-name">${this.escapeHtml(monitor.name)}</div>
                    <div class="monitor-url">${this.escapeHtml(monitor.url || monitor.endpoint || '')}</div>
                </div>
                <div class="monitor-metrics">
                    <div class="metric">
                        <span class="metric-value">${monitor.uptime?.toFixed(1) || 0}%</span>
                        <span class="metric-label">Uptime</span>
                    </div>
                    <div class="metric">
                        <span class="metric-value">${monitor.latency_ms || 0}ms</span>
                        <span class="metric-label">Latency</span>
                    </div>
                </div>
                <div class="uptime-bar">
                    ${this.renderUptimeBar(monitor.history || [])}
                </div>
            </div>
        `;
    }

    renderUptimeBar(history) {
        // Show last 30 checks as small bars
        const checks = history.slice(-30);
        if (checks.length === 0) {
            // Generate placeholder
            return '<div class="uptime-placeholder">No history</div>';
        }

        return `
            <div class="uptime-bars">
                ${checks.map(c => `
                    <div class="uptime-tick ${c.success ? 'up' : 'down'}"
                         title="${c.timestamp ? new Date(c.timestamp).toLocaleString() : ''}: ${c.success ? 'Up' : 'Down'}">
                    </div>
                `).join('')}
            </div>
        `;
    }

    calculateStats() {
        const total = this.monitors.length;
        const passing = this.monitors.filter(m => m.status === 'passing').length;
        const failing = this.monitors.filter(m => m.status === 'failing' || m.status === 'degraded').length;
        const avgUptime = total > 0
            ? this.monitors.reduce((sum, m) => sum + (m.uptime || 0), 0) / total
            : 100;

        return { total, passing, failing, avgUptime };
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .synthetics-uptime {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .synth-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.25rem 0.5rem;
                cursor: pointer;
            }

            .loading, .empty-state {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }

            .stats-bar {
                display: flex;
                justify-content: space-around;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .stat-item {
                text-align: center;
            }

            .stat-value {
                display: block;
                font-size: 1.25rem;
                font-weight: 600;
            }

            .stat-item.passing .stat-value { color: var(--success, #00ba7c); }
            .stat-item.failing .stat-value { color: var(--error, #f4212e); }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .filter-tabs {
                display: flex;
                gap: 0.5rem;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tab {
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.75rem;
                font-size: 0.8rem;
                cursor: pointer;
            }

            .tab.active {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
                color: white;
            }

            .monitors-list {
                flex: 1;
                overflow-y: auto;
                padding: 0.5rem;
            }

            .monitor-item {
                display: grid;
                grid-template-columns: auto 1fr auto auto;
                gap: 0.75rem;
                align-items: center;
                padding: 0.75rem;
                margin-bottom: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .monitor-item.passing { border-left-color: var(--success, #00ba7c); }
            .monitor-item.degraded { border-left-color: var(--warning, #ffd400); }
            .monitor-item.failing { border-left-color: var(--error, #f4212e); }

            .monitor-status {
                width: 28px;
                height: 28px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.9rem;
                font-weight: bold;
            }

            .monitor-item.passing .monitor-status {
                background: rgba(0, 186, 124, 0.2);
                color: var(--success, #00ba7c);
            }

            .monitor-item.degraded .monitor-status {
                background: rgba(255, 212, 0, 0.2);
                color: var(--warning, #ffd400);
            }

            .monitor-item.failing .monitor-status {
                background: rgba(244, 33, 46, 0.2);
                color: var(--error, #f4212e);
            }

            .monitor-info {
                min-width: 0;
            }

            .monitor-name {
                font-weight: 500;
                font-size: 0.9rem;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .monitor-url {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }

            .monitor-metrics {
                display: flex;
                gap: 1rem;
            }

            .metric {
                text-align: center;
            }

            .metric-value {
                display: block;
                font-size: 0.9rem;
                font-weight: 600;
            }

            .metric-label {
                font-size: 0.65rem;
                color: var(--text-muted, #71767b);
            }

            .uptime-bar {
                width: 120px;
            }

            .uptime-bars {
                display: flex;
                gap: 2px;
                height: 20px;
                align-items: flex-end;
            }

            .uptime-tick {
                flex: 1;
                min-width: 3px;
                height: 100%;
                border-radius: 1px;
            }

            .uptime-tick.up { background: var(--success, #00ba7c); }
            .uptime-tick.down { background: var(--error, #f4212e); }

            .uptime-placeholder {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                text-align: center;
            }

            @media (max-width: 700px) {
                .monitor-item {
                    grid-template-columns: auto 1fr;
                    grid-template-rows: auto auto;
                }

                .monitor-metrics, .uptime-bar {
                    grid-column: 1 / -1;
                }
            }
        `;
    }
}

customElements.define('synthetics-uptime', SyntheticsUptime);

/**
 * Toast Notifications System
 * Global notification manager for alerts and messages
 */
class ToastContainer extends HTMLElement {
    constructor() {
        super();
        this.toasts = [];
        this.maxToasts = 5;
    }

    connectedCallback() {
        this.render();
        // Expose global function
        window.showToast = this.addToast.bind(this);

        // Listen for custom events
        window.addEventListener('toast', (e) => {
            this.addToast(e.detail);
        });

        // Listen for WebSocket notifications
        if (window.ws) {
            window.ws.subscribe('notification', (data) => {
                this.addToast({
                    type: data.severity || 'info',
                    title: data.title,
                    message: data.message,
                    duration: data.duration
                });
            });
        }
    }

    addToast(options) {
        const toast = {
            id: Date.now() + Math.random(),
            type: options.type || 'info', // info, success, warning, error
            title: options.title || '',
            message: options.message || '',
            duration: options.duration !== undefined ? options.duration : 5000,
            action: options.action || null, // { label, callback }
            timestamp: new Date()
        };

        this.toasts.unshift(toast);

        // Limit visible toasts
        if (this.toasts.length > this.maxToasts) {
            this.toasts = this.toasts.slice(0, this.maxToasts);
        }

        this.render();

        // Auto dismiss
        if (toast.duration > 0) {
            setTimeout(() => this.removeToast(toast.id), toast.duration);
        }

        return toast.id;
    }

    removeToast(id) {
        const index = this.toasts.findIndex(t => t.id === id);
        if (index !== -1) {
            // Add exit animation class
            const toastEl = this.querySelector(`[data-toast-id="${id}"]`);
            if (toastEl) {
                toastEl.classList.add('exiting');
                setTimeout(() => {
                    this.toasts = this.toasts.filter(t => t.id !== id);
                    this.render();
                }, 300);
            } else {
                this.toasts = this.toasts.filter(t => t.id !== id);
                this.render();
            }
        }
    }

    handleAction(id) {
        const toast = this.toasts.find(t => t.id === id);
        if (toast && toast.action && toast.action.callback) {
            toast.action.callback();
        }
        this.removeToast(id);
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="toast-container">
                ${this.toasts.map(toast => this.renderToast(toast)).join('')}
            </div>
        `;
    }

    renderToast(toast) {
        const icon = this.getIcon(toast.type);

        return `
            <div class="toast toast-${toast.type}" data-toast-id="${toast.id}">
                <div class="toast-icon">${icon}</div>
                <div class="toast-content">
                    ${toast.title ? `<div class="toast-title">${this.escapeHtml(toast.title)}</div>` : ''}
                    ${toast.message ? `<div class="toast-message">${this.escapeHtml(toast.message)}</div>` : ''}
                </div>
                <div class="toast-actions">
                    ${toast.action ? `
                        <button class="toast-action-btn" onclick="this.getRootNode().host.handleAction(${toast.id})">
                            ${this.escapeHtml(toast.action.label)}
                        </button>
                    ` : ''}
                    <button class="toast-close" onclick="this.getRootNode().host.removeToast(${toast.id})">×</button>
                </div>
                ${toast.duration > 0 ? `
                    <div class="toast-progress">
                        <div class="toast-progress-bar" style="animation-duration: ${toast.duration}ms"></div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    getIcon(type) {
        switch (type) {
            case 'success': return '✓';
            case 'warning': return '⚠';
            case 'error': return '✗';
            default: return 'ℹ';
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            :host {
                position: fixed;
                top: 1rem;
                right: 1rem;
                z-index: 10000;
                pointer-events: none;
            }

            .toast-container {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
                max-width: 400px;
            }

            .toast {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                padding: 0.875rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 4px solid var(--border, #2f3336);
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
                pointer-events: auto;
                animation: slideIn 0.3s ease-out;
                position: relative;
                overflow: hidden;
            }

            .toast.exiting {
                animation: slideOut 0.3s ease-in forwards;
            }

            @keyframes slideIn {
                from {
                    transform: translateX(100%);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }

            @keyframes slideOut {
                from {
                    transform: translateX(0);
                    opacity: 1;
                }
                to {
                    transform: translateX(100%);
                    opacity: 0;
                }
            }

            .toast-info { border-left-color: var(--accent, #1d9bf0); }
            .toast-success { border-left-color: var(--success, #00ba7c); }
            .toast-warning { border-left-color: var(--warning, #ffd400); }
            .toast-error { border-left-color: var(--error, #f4212e); }

            .toast-icon {
                width: 24px;
                height: 24px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.8rem;
                font-weight: bold;
                flex-shrink: 0;
            }

            .toast-info .toast-icon {
                background: rgba(29, 155, 240, 0.2);
                color: var(--accent, #1d9bf0);
            }

            .toast-success .toast-icon {
                background: rgba(0, 186, 124, 0.2);
                color: var(--success, #00ba7c);
            }

            .toast-warning .toast-icon {
                background: rgba(255, 212, 0, 0.2);
                color: var(--warning, #ffd400);
            }

            .toast-error .toast-icon {
                background: rgba(244, 33, 46, 0.2);
                color: var(--error, #f4212e);
            }

            .toast-content {
                flex: 1;
                min-width: 0;
            }

            .toast-title {
                font-weight: 600;
                font-size: 0.9rem;
                margin-bottom: 0.25rem;
            }

            .toast-message {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                line-height: 1.4;
            }

            .toast-actions {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                flex-shrink: 0;
            }

            .toast-action-btn {
                background: var(--accent, #1d9bf0);
                border: none;
                border-radius: 4px;
                color: white;
                padding: 0.3rem 0.6rem;
                font-size: 0.75rem;
                cursor: pointer;
                font-weight: 500;
            }

            .toast-action-btn:hover {
                filter: brightness(1.1);
            }

            .toast-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.25rem;
                cursor: pointer;
                padding: 0;
                line-height: 1;
            }

            .toast-close:hover {
                color: var(--text, #e7e9ea);
            }

            .toast-progress {
                position: absolute;
                bottom: 0;
                left: 0;
                right: 0;
                height: 3px;
                background: rgba(255, 255, 255, 0.1);
            }

            .toast-progress-bar {
                height: 100%;
                width: 100%;
                transform-origin: left;
                animation: progress linear forwards;
            }

            @keyframes progress {
                from { transform: scaleX(1); }
                to { transform: scaleX(0); }
            }

            .toast-info .toast-progress-bar { background: var(--accent, #1d9bf0); }
            .toast-success .toast-progress-bar { background: var(--success, #00ba7c); }
            .toast-warning .toast-progress-bar { background: var(--warning, #ffd400); }
            .toast-error .toast-progress-bar { background: var(--error, #f4212e); }

            @media (max-width: 500px) {
                :host {
                    left: 1rem;
                    right: 1rem;
                }

                .toast-container {
                    max-width: none;
                }
            }
        `;
    }
}

customElements.define('toast-container', ToastContainer);

// Convenience functions for programmatic use
window.toast = {
    info: (message, title = '', options = {}) => window.showToast({ type: 'info', message, title, ...options }),
    success: (message, title = '', options = {}) => window.showToast({ type: 'success', message, title, ...options }),
    warning: (message, title = '', options = {}) => window.showToast({ type: 'warning', message, title, ...options }),
    error: (message, title = '', options = {}) => window.showToast({ type: 'error', message, title, ...options })
};

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
            }
        } catch (e) {
            console.error('[TraceViewer] Failed to fetch traces:', e);
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
            }
        } catch (e) {
            console.error('[TraceViewer] Failed to load trace:', e);
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
