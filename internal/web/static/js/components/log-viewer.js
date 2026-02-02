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
        this._mounted = false;
        this._boundEventListeners = [];
    }

    connectedCallback() {
        this._mounted = true;
        this.maxLogs = parseInt(this.getAttribute('limit')) || 100;
        this.filters.service = this.getAttribute('service') || '';
        this.filters.level = this.getAttribute('level') || '';
        this.render();
        this.setupWebSocket();
        this.fetchLogs();
    }

    disconnectedCallback() {
        this._mounted = false;
        if (this._unsubscribe) {
            this._unsubscribe();
            this._unsubscribe = null;
        }
        // Clean up event listeners
        this._boundEventListeners.forEach(({ element, event, handler }) => {
            if (element) element.removeEventListener(event, handler);
        });
        this._boundEventListeners = [];
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
        const container = this.shadowRoot.getElementById('log-container');

        // Helper to track event listeners for cleanup
        const addListener = (element, event, handler) => {
            if (element) {
                element.addEventListener(event, handler);
                this._boundEventListeners.push({ element, event, handler });
            }
        };

        const searchHandler = (e) => {
            this.filters.search = e.target.value;
            this.renderLogs();
        };
        addListener(search, 'input', searchHandler);

        const levelHandler = (e) => {
            this.filters.level = e.target.value;
            this.renderFilterPills();
            this.renderLogs();
        };
        addListener(levelFilter, 'change', levelHandler);

        const serviceHandler = (e) => {
            this.filters.service = e.target.value;
            this.renderFilterPills();
            this.renderLogs();
        };
        addListener(serviceFilter, 'change', serviceHandler);

        const autoScrollHandler = () => {
            this.autoScroll = !this.autoScroll;
            autoScrollToggle.textContent = `Auto-scroll: ${this.autoScroll ? 'ON' : 'OFF'}`;
        };
        addListener(autoScrollToggle, 'click', autoScrollHandler);

        const scrollHandler = () => {
            const isAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 50;
            if (!isAtBottom) {
                this.autoScroll = false;
                autoScrollToggle.textContent = 'Auto-scroll: OFF';
            }
        };
        addListener(container, 'scroll', scrollHandler);
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
                // Handle multiple response formats from API
                this.logs = data.entries || data.data || data.logs || (Array.isArray(data) ? data : []);
                this.updateServiceFilter();
                this.renderLogs();
            } else {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
        } catch (e) {
            console.error('[LogViewer] Failed to fetch logs:', e);
            this._showError('Failed to load logs', e.message);
        }
    }

    _showError(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'error', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.error(message, title);
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
