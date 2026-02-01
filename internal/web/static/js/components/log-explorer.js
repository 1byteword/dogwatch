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
            this.logs = data.entries || [];
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
        // Poll every 2 seconds for new logs
        this.liveTailInterval = setInterval(() => {
            this.search();
        }, 2000);
    }

    stopLiveTail() {
        if (this.liveTailInterval) {
            clearInterval(this.liveTailInterval);
            this.liveTailInterval = null;
        }
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
