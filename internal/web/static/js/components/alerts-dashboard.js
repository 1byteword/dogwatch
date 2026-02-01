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
