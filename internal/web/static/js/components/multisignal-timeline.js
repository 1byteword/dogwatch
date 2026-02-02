/**
 * Multi-Signal Correlation Timeline Widget
 * Unified timeline showing logs, traces, metrics, and deploys
 * with visual correlation lines connecting related signals
 */
class MultisignalTimeline extends HTMLElement {
    constructor() {
        super();
        this.timelineData = null;
        this.selectedSignal = null;
        this.filters = {
            service: '',
            signalTypes: ['log', 'trace', 'metric', 'deploy'],
            severity: 'all'
        };
        this.timeRange = { start: null, end: null };
        this.correlations = [];
        this.isDragging = false;
        this.dragStart = null;
        this._mounted = false;
        this._boundEventListeners = [];
        this._documentListeners = [];
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        this._mounted = false;
        // Clean up element event listeners
        this._boundEventListeners.forEach(({ element, event, handler }) => {
            if (element) element.removeEventListener(event, handler);
        });
        this._boundEventListeners = [];
        // Clean up document-level event listeners (critical!)
        this._documentListeners.forEach(({ event, handler }) => {
            document.removeEventListener(event, handler);
        });
        this._documentListeners = [];
    }

    static get observedAttributes() {
        return ['service', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            if (name === 'service') this.filters.service = newValue || '';
            this.loadData();
        }
    }

    get service() { return this.getAttribute('service') || ''; }
    get range() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="multisignal-container">
                <div class="multisignal-header">
                    <div class="multisignal-title">
                        <span class="title-icon">&#9776;</span>
                        <span>Multi-Signal Timeline</span>
                    </div>
                    <div class="multisignal-controls">
                        <select id="service-filter" class="control-select">
                            <option value="">All Services</option>
                        </select>
                        <select id="time-range" class="control-select">
                            <option value="15m">15 min</option>
                            <option value="1h" selected>1 hour</option>
                            <option value="6h">6 hours</option>
                            <option value="24h">24 hours</option>
                        </select>
                        <button class="btn-autocorrelate" id="btn-autocorrelate">
                            Auto-Correlate
                        </button>
                        <button class="btn-refresh" id="btn-refresh">&#8635;</button>
                    </div>
                </div>

                <div class="filter-bar">
                    <div class="signal-type-filters">
                        <label class="signal-filter">
                            <input type="checkbox" data-type="log" checked>
                            <span class="signal-badge log">Logs</span>
                        </label>
                        <label class="signal-filter">
                            <input type="checkbox" data-type="trace" checked>
                            <span class="signal-badge trace">Traces</span>
                        </label>
                        <label class="signal-filter">
                            <input type="checkbox" data-type="metric" checked>
                            <span class="signal-badge metric">Metrics</span>
                        </label>
                        <label class="signal-filter">
                            <input type="checkbox" data-type="deploy" checked>
                            <span class="signal-badge deploy">Deploys</span>
                        </label>
                    </div>
                    <div class="severity-filter">
                        <select id="severity-filter" class="control-select">
                            <option value="all">All Severities</option>
                            <option value="error">Errors Only</option>
                            <option value="warn">Warnings+</option>
                            <option value="info">Info+</option>
                        </select>
                    </div>
                </div>

                <div class="time-scrubber">
                    <div class="scrubber-track" id="scrubber-track">
                        <div class="scrubber-selection" id="scrubber-selection"></div>
                        <div class="scrubber-handle start" id="scrubber-start"></div>
                        <div class="scrubber-handle end" id="scrubber-end"></div>
                    </div>
                    <div class="scrubber-labels">
                        <span id="time-start"></span>
                        <span id="time-end"></span>
                    </div>
                </div>

                <div class="timeline-content">
                    <div class="timeline-view" id="timeline-view">
                        <svg class="correlation-lines" id="correlation-lines"></svg>
                        <div class="timeline-events" id="timeline-events">
                            <div class="loading">Loading timeline data...</div>
                        </div>
                    </div>
                    <div class="detail-panel" id="detail-panel">
                        <div class="detail-empty">
                            Select a signal to view details
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    getStyles() {
        return `
            .multisignal-container {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
            }
            .multisignal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .multisignal-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                font-size: 0.9rem;
            }
            .title-icon { font-size: 1.1rem; }
            .multisignal-controls {
                display: flex;
                gap: 0.5rem;
                align-items: center;
            }
            .control-select {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.4rem 0.6rem;
                color: var(--text, #e7e9ea);
                font-size: 0.8rem;
            }
            .btn-refresh, .btn-autocorrelate {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
                font-size: 0.8rem;
            }
            .btn-autocorrelate {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
            }
            .btn-autocorrelate:hover {
                background: #1a8cd8;
            }
            .filter-bar {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.5rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .signal-type-filters {
                display: flex;
                gap: 0.75rem;
            }
            .signal-filter {
                display: flex;
                align-items: center;
                gap: 0.25rem;
                cursor: pointer;
            }
            .signal-filter input {
                display: none;
            }
            .signal-filter input:not(:checked) + .signal-badge {
                opacity: 0.4;
            }
            .signal-badge {
                display: inline-flex;
                align-items: center;
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.75rem;
                font-weight: 500;
            }
            .signal-badge.log { background: rgba(96, 165, 250, 0.2); color: #60a5fa; }
            .signal-badge.trace { background: rgba(167, 139, 250, 0.2); color: #a78bfa; }
            .signal-badge.metric { background: rgba(52, 211, 153, 0.2); color: #34d399; }
            .signal-badge.deploy { background: rgba(251, 191, 36, 0.2); color: #fbbf24; }
            .time-scrubber {
                padding: 0.75rem 1rem;
                background: var(--bg-card, #16181c);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .scrubber-track {
                position: relative;
                height: 8px;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                cursor: pointer;
            }
            .scrubber-selection {
                position: absolute;
                height: 100%;
                background: var(--accent, #1d9bf0);
                opacity: 0.5;
                border-radius: 4px;
                left: 0%;
                right: 0%;
            }
            .scrubber-handle {
                position: absolute;
                width: 12px;
                height: 16px;
                top: -4px;
                background: var(--accent, #1d9bf0);
                border-radius: 3px;
                cursor: ew-resize;
            }
            .scrubber-handle.start { left: 0%; transform: translateX(-50%); }
            .scrubber-handle.end { right: 0%; transform: translateX(50%); }
            .scrubber-labels {
                display: flex;
                justify-content: space-between;
                margin-top: 0.5rem;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .timeline-content {
                display: flex;
                flex: 1;
                overflow: hidden;
            }
            .timeline-view {
                flex: 1;
                position: relative;
                overflow: hidden;
            }
            .correlation-lines {
                position: absolute;
                top: 0;
                left: 0;
                width: 100%;
                height: 100%;
                pointer-events: none;
                z-index: 1;
            }
            .timeline-events {
                position: relative;
                height: 100%;
                overflow-y: auto;
                padding: 0.5rem;
            }
            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
            }
            .timeline-event {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                padding: 0.75rem;
                margin-bottom: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid transparent;
                cursor: pointer;
                transition: all 0.15s ease;
            }
            .timeline-event:hover {
                background: rgba(29, 155, 240, 0.08);
            }
            .timeline-event.selected {
                border-left-color: var(--accent, #1d9bf0);
                background: rgba(29, 155, 240, 0.12);
            }
            .timeline-event.correlated {
                box-shadow: 0 0 0 1px var(--accent, #1d9bf0);
            }
            .event-icon {
                width: 28px;
                height: 28px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.9rem;
                flex-shrink: 0;
            }
            .event-icon.log { background: rgba(96, 165, 250, 0.2); }
            .event-icon.trace { background: rgba(167, 139, 250, 0.2); }
            .event-icon.metric { background: rgba(52, 211, 153, 0.2); }
            .event-icon.deploy { background: rgba(251, 191, 36, 0.2); }
            .event-icon.error { background: rgba(244, 63, 94, 0.2); }
            .event-content {
                flex: 1;
                min-width: 0;
            }
            .event-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.25rem;
            }
            .event-title {
                font-weight: 500;
                font-size: 0.85rem;
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
            }
            .event-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                flex-shrink: 0;
            }
            .event-meta {
                display: flex;
                gap: 0.75rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }
            .event-service {
                color: var(--accent, #1d9bf0);
            }
            .event-exemplar {
                color: var(--success, #00ba7c);
                cursor: pointer;
            }
            .event-exemplar:hover {
                text-decoration: underline;
            }
            .detail-panel {
                width: 320px;
                background: var(--bg-elevated, #1e2128);
                border-left: 1px solid var(--border, #2f3336);
                overflow-y: auto;
            }
            .detail-empty {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }
            .detail-header {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .detail-title {
                font-weight: 600;
                font-size: 1rem;
                margin-bottom: 0.5rem;
            }
            .detail-type {
                display: inline-flex;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.7rem;
                font-weight: 600;
            }
            .detail-body {
                padding: 1rem;
            }
            .detail-section {
                margin-bottom: 1rem;
            }
            .detail-section-title {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                text-transform: uppercase;
                margin-bottom: 0.5rem;
            }
            .detail-field {
                display: flex;
                justify-content: space-between;
                padding: 0.4rem 0;
                font-size: 0.85rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .detail-field-label {
                color: var(--text-muted, #71767b);
            }
            .detail-field-value {
                font-family: monospace;
                max-width: 60%;
                text-align: right;
                word-break: break-all;
            }
            .detail-message {
                font-family: monospace;
                font-size: 0.8rem;
                background: var(--bg-card, #16181c);
                padding: 0.75rem;
                border-radius: 4px;
                white-space: pre-wrap;
                word-break: break-all;
            }
            .detail-correlated {
                margin-top: 1rem;
            }
            .correlated-item {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                padding: 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                margin-bottom: 0.5rem;
                cursor: pointer;
            }
            .correlated-item:hover {
                background: rgba(29, 155, 240, 0.08);
            }
            .correlation-line {
                stroke: var(--accent, #1d9bf0);
                stroke-width: 2;
                stroke-dasharray: 4 2;
                opacity: 0.6;
            }
            @media (max-width: 800px) {
                .detail-panel { display: none; }
                .timeline-content { flex-direction: column; }
            }
        `;
    }

    setupEventListeners() {
        // Helper to track event listeners for cleanup
        const addListener = (selector, event, handler) => {
            const element = this.querySelector(selector);
            if (element) {
                element.addEventListener(event, handler);
                this._boundEventListeners.push({ element, event, handler });
            }
        };

        // Refresh button
        addListener('#btn-refresh', 'click', () => this.loadData());

        // Auto-correlate button
        addListener('#btn-autocorrelate', 'click', () => this.autoCorrelate());

        // Time range
        addListener('#time-range', 'change', (e) => {
            this.setAttribute('time-range', e.target.value);
        });

        // Service filter
        addListener('#service-filter', 'change', (e) => {
            this.filters.service = e.target.value;
            this.renderTimeline();
        });

        // Signal type filters
        this.querySelectorAll('.signal-filter input').forEach(cb => {
            const handler = (e) => {
                const type = e.target.dataset.type;
                if (e.target.checked) {
                    if (!this.filters.signalTypes.includes(type)) {
                        this.filters.signalTypes.push(type);
                    }
                } else {
                    this.filters.signalTypes = this.filters.signalTypes.filter(t => t !== type);
                }
                this.renderTimeline();
            };
            cb.addEventListener('change', handler);
            this._boundEventListeners.push({ element: cb, event: 'change', handler });
        });

        // Severity filter
        addListener('#severity-filter', 'change', (e) => {
            this.filters.severity = e.target.value;
            this.renderTimeline();
        });

        // Time scrubber
        this.setupScrubber();
    }

    setupScrubber() {
        const track = this.querySelector('#scrubber-track');
        const startHandle = this.querySelector('#scrubber-start');
        const endHandle = this.querySelector('#scrubber-end');

        if (!track) return;

        const onDrag = (e, handle) => {
            const rect = track.getBoundingClientRect();
            let percent = (e.clientX - rect.left) / rect.width * 100;
            percent = Math.max(0, Math.min(100, percent));

            if (handle === 'start') {
                startHandle.style.left = percent + '%';
                this.updateScrubberSelection();
            } else {
                endHandle.style.right = (100 - percent) + '%';
                this.updateScrubberSelection();
            }
        };

        if (startHandle) {
            const startHandler = () => {
                this.isDragging = true;
                this.dragStart = 'start';
            };
            startHandle.addEventListener('mousedown', startHandler);
            this._boundEventListeners.push({ element: startHandle, event: 'mousedown', handler: startHandler });
        }

        if (endHandle) {
            const endHandler = () => {
                this.isDragging = true;
                this.dragStart = 'end';
            };
            endHandle.addEventListener('mousedown', endHandler);
            this._boundEventListeners.push({ element: endHandle, event: 'mousedown', handler: endHandler });
        }

        // Document-level event listeners - must be tracked for cleanup!
        const mouseMoveHandler = (e) => {
            if (this.isDragging && this._mounted) {
                onDrag(e, this.dragStart);
            }
        };
        document.addEventListener('mousemove', mouseMoveHandler);
        this._documentListeners.push({ event: 'mousemove', handler: mouseMoveHandler });

        const mouseUpHandler = () => {
            if (this.isDragging && this._mounted) {
                this.isDragging = false;
                this.loadData();
            }
        };
        document.addEventListener('mouseup', mouseUpHandler);
        this._documentListeners.push({ event: 'mouseup', handler: mouseUpHandler });
    }

    updateScrubberSelection() {
        const selection = this.querySelector('#scrubber-selection');
        const startHandle = this.querySelector('#scrubber-start');
        const endHandle = this.querySelector('#scrubber-end');

        if (selection && startHandle && endHandle) {
            const startLeft = parseFloat(startHandle.style.left) || 0;
            const endRight = parseFloat(endHandle.style.right) || 0;
            selection.style.left = startLeft + '%';
            selection.style.right = endRight + '%';
        }
    }

    async loadData() {
        const eventsContainer = this.querySelector('#timeline-events');
        if (eventsContainer) {
            eventsContainer.innerHTML = '<div class="loading">Loading timeline data...</div>';
        }

        try {
            const params = new URLSearchParams({ range: this.range });
            if (this.filters.service) {
                params.append('service', this.filters.service);
            }

            const endpoint = this.filters.service
                ? `/api/multisignal/timeline/${encodeURIComponent(this.filters.service)}`
                : '/api/multisignal/timeline';

            const resp = await fetch(`${endpoint}?${params}`);

            if (!resp.ok) {
                this.timelineData = this.generateDemoData();
            } else {
                this.timelineData = await resp.json();
            }

            this.updateTimeLabels();
            this.renderTimeline();
            this.loadServices();
        } catch (e) {
            console.error('Failed to load timeline data:', e);
            this.timelineData = this.generateDemoData();
            this.renderTimeline();
        }
    }

    generateDemoData() {
        const now = Date.now();
        const events = [];

        // Generate logs
        for (let i = 0; i < 15; i++) {
            const severity = ['info', 'warn', 'error'][Math.floor(Math.random() * 3)];
            events.push({
                id: `log-${i}`,
                type: 'log',
                timestamp: now - Math.random() * 3600000,
                service: ['api-gateway', 'user-service', 'order-service'][Math.floor(Math.random() * 3)],
                severity: severity,
                title: severity === 'error' ? 'Connection timeout' : 'Request processed',
                message: `[${severity.toUpperCase()}] Service request completed with status ${severity === 'error' ? '500' : '200'}`,
                traceId: Math.random() > 0.5 ? `trace-${i % 5}` : null
            });
        }

        // Generate traces
        for (let i = 0; i < 8; i++) {
            const isError = Math.random() > 0.7;
            events.push({
                id: `trace-${i}`,
                type: 'trace',
                timestamp: now - Math.random() * 3600000,
                service: ['api-gateway', 'user-service'][Math.floor(Math.random() * 2)],
                severity: isError ? 'error' : 'info',
                title: `GET /api/users/${i}`,
                duration: Math.floor(50 + Math.random() * 450),
                spanCount: Math.floor(3 + Math.random() * 10),
                traceId: `trace-${i}`
            });
        }

        // Generate metrics
        for (let i = 0; i < 6; i++) {
            events.push({
                id: `metric-${i}`,
                type: 'metric',
                timestamp: now - Math.random() * 3600000,
                service: ['api-gateway', 'user-service', 'order-service'][Math.floor(Math.random() * 3)],
                severity: 'info',
                title: `http_request_latency_p99`,
                value: (50 + Math.random() * 200).toFixed(1),
                unit: 'ms',
                exemplarTraceId: Math.random() > 0.5 ? `trace-${i % 5}` : null
            });
        }

        // Generate deploys
        for (let i = 0; i < 2; i++) {
            events.push({
                id: `deploy-${i}`,
                type: 'deploy',
                timestamp: now - (i + 1) * 1800000,
                service: ['api-gateway', 'user-service'][i],
                severity: 'info',
                title: `Deployed v2.${4 + i}.${Math.floor(Math.random() * 10)}`,
                commit: 'abc123' + i,
                user: 'deploy-bot'
            });
        }

        // Sort by timestamp descending
        events.sort((a, b) => b.timestamp - a.timestamp);

        return {
            events,
            summary: {
                logs: events.filter(e => e.type === 'log').length,
                traces: events.filter(e => e.type === 'trace').length,
                metrics: events.filter(e => e.type === 'metric').length,
                deploys: events.filter(e => e.type === 'deploy').length
            }
        };
    }

    updateTimeLabels() {
        const startLabel = this.querySelector('#time-start');
        const endLabel = this.querySelector('#time-end');

        if (!this.timelineData?.events?.length) return;

        const timestamps = this.timelineData.events.map(e => e.timestamp);
        const minTime = Math.min(...timestamps);
        const maxTime = Math.max(...timestamps);

        const formatTime = (ts) => new Date(ts).toLocaleTimeString('en-US', {
            hour: '2-digit', minute: '2-digit', hour12: false
        });

        if (startLabel) startLabel.textContent = formatTime(minTime);
        if (endLabel) endLabel.textContent = formatTime(maxTime);
    }

    async loadServices() {
        try {
            const resp = await fetch('/api/services');
            if (resp.ok) {
                const services = await resp.json();
                const select = this.querySelector('#service-filter');
                if (select && services.length) {
                    select.innerHTML = '<option value="">All Services</option>' +
                        services.map(s => `<option value="${this.escapeHtml(s.name)}">${this.escapeHtml(s.name)}</option>`).join('');
                }
            }
        } catch (e) {
            // Ignore
        }
    }

    renderTimeline() {
        const container = this.querySelector('#timeline-events');
        if (!container || !this.timelineData?.events) return;

        const filteredEvents = this.timelineData.events.filter(event => {
            // Filter by signal type
            if (!this.filters.signalTypes.includes(event.type)) return false;

            // Filter by service
            if (this.filters.service && event.service !== this.filters.service) return false;

            // Filter by severity
            if (this.filters.severity !== 'all') {
                const severityOrder = { error: 3, warn: 2, info: 1 };
                const minSeverity = severityOrder[this.filters.severity] || 0;
                const eventSeverity = severityOrder[event.severity] || 1;
                if (eventSeverity < minSeverity) return false;
            }

            return true;
        });

        if (filteredEvents.length === 0) {
            container.innerHTML = '<div class="loading">No events match the current filters</div>';
            return;
        }

        container.innerHTML = filteredEvents.map(event => this.renderEvent(event)).join('');

        // Add click handlers
        container.querySelectorAll('.timeline-event').forEach(el => {
            el.addEventListener('click', () => {
                const eventId = el.dataset.eventId;
                const event = this.timelineData.events.find(e => e.id === eventId);
                this.selectSignal(event);
            });
        });

        // Add exemplar link handlers
        container.querySelectorAll('.event-exemplar').forEach(el => {
            el.addEventListener('click', (e) => {
                e.stopPropagation();
                const traceId = el.dataset.traceId;
                this.jumpToTrace(traceId);
            });
        });

        this.drawCorrelationLines();
    }

    renderEvent(event) {
        const icons = {
            log: '&#128221;',
            trace: '&#128268;',
            metric: '&#128200;',
            deploy: '&#128640;'
        };

        const isSelected = this.selectedSignal?.id === event.id;
        const isCorrelated = this.correlations.some(c => c.includes(event.id));

        let meta = `<span class="event-service">${this.escapeHtml(event.service)}</span>`;

        if (event.type === 'trace') {
            meta += `<span>${event.duration}ms</span><span>${event.spanCount} spans</span>`;
        } else if (event.type === 'metric') {
            meta += `<span>${event.value} ${event.unit || ''}</span>`;
            if (event.exemplarTraceId) {
                meta += `<span class="event-exemplar" data-trace-id="${this.escapeHtml(event.exemplarTraceId)}">View Trace</span>`;
            }
        } else if (event.type === 'deploy') {
            meta += `<span>${this.escapeHtml(event.commit?.slice(0, 7) || '')}</span>`;
        } else if (event.type === 'log' && event.traceId) {
            meta += `<span class="event-exemplar" data-trace-id="${this.escapeHtml(event.traceId)}">View Trace</span>`;
        }

        const iconClass = event.severity === 'error' ? 'error' : event.type;

        return `
            <div class="timeline-event ${isSelected ? 'selected' : ''} ${isCorrelated ? 'correlated' : ''}"
                 data-event-id="${this.escapeHtml(event.id)}"
                 data-event-type="${event.type}"
                 data-timestamp="${event.timestamp}">
                <div class="event-icon ${iconClass}">${icons[event.type] || '&#9679;'}</div>
                <div class="event-content">
                    <div class="event-header">
                        <span class="event-title">${this.escapeHtml(event.title)}</span>
                        <span class="event-time">${this.formatRelativeTime(event.timestamp)}</span>
                    </div>
                    <div class="event-meta">${meta}</div>
                </div>
            </div>
        `;
    }

    selectSignal(event) {
        this.selectedSignal = event;
        this.renderTimeline();
        this.renderDetailPanel(event);
        this.findCorrelatedSignals(event);
    }

    renderDetailPanel(event) {
        const panel = this.querySelector('#detail-panel');
        if (!panel || !event) {
            if (panel) panel.innerHTML = '<div class="detail-empty">Select a signal to view details</div>';
            return;
        }

        const typeColors = {
            log: 'background: rgba(96, 165, 250, 0.2); color: #60a5fa;',
            trace: 'background: rgba(167, 139, 250, 0.2); color: #a78bfa;',
            metric: 'background: rgba(52, 211, 153, 0.2); color: #34d399;',
            deploy: 'background: rgba(251, 191, 36, 0.2); color: #fbbf24;'
        };

        let fields = `
            <div class="detail-field">
                <span class="detail-field-label">Service</span>
                <span class="detail-field-value">${this.escapeHtml(event.service)}</span>
            </div>
            <div class="detail-field">
                <span class="detail-field-label">Time</span>
                <span class="detail-field-value">${new Date(event.timestamp).toLocaleString()}</span>
            </div>
        `;

        if (event.type === 'trace') {
            fields += `
                <div class="detail-field">
                    <span class="detail-field-label">Trace ID</span>
                    <span class="detail-field-value">${this.escapeHtml(event.traceId || event.id)}</span>
                </div>
                <div class="detail-field">
                    <span class="detail-field-label">Duration</span>
                    <span class="detail-field-value">${event.duration}ms</span>
                </div>
                <div class="detail-field">
                    <span class="detail-field-label">Span Count</span>
                    <span class="detail-field-value">${event.spanCount}</span>
                </div>
            `;
        } else if (event.type === 'metric') {
            fields += `
                <div class="detail-field">
                    <span class="detail-field-label">Value</span>
                    <span class="detail-field-value">${event.value} ${event.unit || ''}</span>
                </div>
            `;
            if (event.exemplarTraceId) {
                fields += `
                    <div class="detail-field">
                        <span class="detail-field-label">Exemplar Trace</span>
                        <span class="detail-field-value">${this.escapeHtml(event.exemplarTraceId)}</span>
                    </div>
                `;
            }
        } else if (event.type === 'deploy') {
            fields += `
                <div class="detail-field">
                    <span class="detail-field-label">Commit</span>
                    <span class="detail-field-value">${this.escapeHtml(event.commit || 'N/A')}</span>
                </div>
                <div class="detail-field">
                    <span class="detail-field-label">User</span>
                    <span class="detail-field-value">${this.escapeHtml(event.user || 'N/A')}</span>
                </div>
            `;
        }

        panel.innerHTML = `
            <div class="detail-header">
                <div class="detail-title">${this.escapeHtml(event.title)}</div>
                <span class="detail-type" style="${typeColors[event.type]}">${event.type.toUpperCase()}</span>
            </div>
            <div class="detail-body">
                <div class="detail-section">
                    <div class="detail-section-title">Properties</div>
                    ${fields}
                </div>
                ${event.message ? `
                    <div class="detail-section">
                        <div class="detail-section-title">Message</div>
                        <div class="detail-message">${this.escapeHtml(event.message)}</div>
                    </div>
                ` : ''}
                <div class="detail-section detail-correlated" id="correlated-signals">
                    <div class="detail-section-title">Correlated Signals</div>
                    <div class="loading">Finding correlations...</div>
                </div>
            </div>
        `;
    }

    async findCorrelatedSignals(event) {
        const container = this.querySelector('#correlated-signals');
        if (!container) return;

        try {
            let endpoint = '';
            const body = {};

            if (event.type === 'log') {
                endpoint = '/api/multisignal/log-to-trace';
                body.logId = event.id;
                body.timestamp = event.timestamp;
                body.service = event.service;
            } else if (event.type === 'metric') {
                endpoint = '/api/multisignal/metric-to-trace';
                body.metricName = event.title;
                body.timestamp = event.timestamp;
                body.service = event.service;
            }

            if (endpoint) {
                const resp = await fetch(endpoint, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body)
                });

                if (resp.ok) {
                    const correlated = await resp.json();
                    this.renderCorrelatedSignals(container, correlated);
                    return;
                }
            }

            // Fallback: find locally
            const correlated = this.findLocalCorrelations(event);
            this.renderCorrelatedSignals(container, correlated);
        } catch (e) {
            console.error('Failed to find correlations:', e);
            const correlated = this.findLocalCorrelations(event);
            this.renderCorrelatedSignals(container, correlated);
        }
    }

    findLocalCorrelations(event) {
        if (!this.timelineData?.events) return [];

        const timeWindow = 60000; // 1 minute
        return this.timelineData.events.filter(e => {
            if (e.id === event.id) return false;
            if (e.service !== event.service) return false;
            if (Math.abs(e.timestamp - event.timestamp) > timeWindow) return false;
            return true;
        }).slice(0, 5);
    }

    renderCorrelatedSignals(container, signals) {
        if (!signals || signals.length === 0) {
            container.innerHTML = `
                <div class="detail-section-title">Correlated Signals</div>
                <div style="font-size: 0.85rem; color: var(--text-muted);">No correlated signals found</div>
            `;
            return;
        }

        const icons = {
            log: '&#128221;',
            trace: '&#128268;',
            metric: '&#128200;',
            deploy: '&#128640;'
        };

        container.innerHTML = `
            <div class="detail-section-title">Correlated Signals (${signals.length})</div>
            ${signals.map(s => `
                <div class="correlated-item" data-event-id="${this.escapeHtml(s.id)}">
                    <span>${icons[s.type] || '&#9679;'}</span>
                    <span style="flex: 1; overflow: hidden; text-overflow: ellipsis;">${this.escapeHtml(s.title)}</span>
                    <span style="font-size: 0.7rem; color: var(--text-muted);">${this.formatRelativeTime(s.timestamp)}</span>
                </div>
            `).join('')}
        `;

        container.querySelectorAll('.correlated-item').forEach(el => {
            el.addEventListener('click', () => {
                const eventId = el.dataset.eventId;
                const event = this.timelineData.events.find(e => e.id === eventId) || signals.find(s => s.id === eventId);
                if (event) this.selectSignal(event);
            });
        });

        // Store correlations for line drawing
        if (this.selectedSignal) {
            this.correlations = [[this.selectedSignal.id, ...signals.map(s => s.id)]];
            this.drawCorrelationLines();
        }
    }

    drawCorrelationLines() {
        const svg = this.querySelector('#correlation-lines');
        if (!svg || this.correlations.length === 0) {
            if (svg) svg.innerHTML = '';
            return;
        }

        const lines = [];
        for (const group of this.correlations) {
            if (group.length < 2) continue;

            const sourceEl = this.querySelector(`[data-event-id="${group[0]}"]`);
            if (!sourceEl) continue;

            for (let i = 1; i < group.length; i++) {
                const targetEl = this.querySelector(`[data-event-id="${group[i]}"]`);
                if (!targetEl) continue;

                const sourceRect = sourceEl.getBoundingClientRect();
                const targetRect = targetEl.getBoundingClientRect();
                const svgRect = svg.getBoundingClientRect();

                lines.push({
                    x1: 20,
                    y1: sourceRect.top - svgRect.top + sourceRect.height / 2,
                    x2: 20,
                    y2: targetRect.top - svgRect.top + targetRect.height / 2
                });
            }
        }

        svg.innerHTML = lines.map(l =>
            `<line class="correlation-line" x1="${l.x1}" y1="${l.y1}" x2="${l.x2}" y2="${l.y2}"/>`
        ).join('');
    }

    async autoCorrelate() {
        if (!this.selectedSignal) {
            alert('Please select a signal first');
            return;
        }

        const btn = this.querySelector('#btn-autocorrelate');
        if (btn) btn.textContent = 'Finding...';

        await this.findCorrelatedSignals(this.selectedSignal);

        if (btn) btn.textContent = 'Auto-Correlate';
    }

    async jumpToTrace(traceId) {
        try {
            const resp = await fetch(`/api/multisignal/trace/${encodeURIComponent(traceId)}`);
            if (resp.ok) {
                const trace = await resp.json();
                // Dispatch event for parent to handle
                this.dispatchEvent(new CustomEvent('trace-select', {
                    detail: { traceId, trace },
                    bubbles: true
                }));
            }
        } catch (e) {
            console.error('Failed to load trace:', e);
        }
    }

    formatRelativeTime(timestamp) {
        const diff = Date.now() - timestamp;
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return new Date(timestamp).toLocaleDateString();
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // Public API
    refresh() {
        this.loadData();
    }

    setFilter(type, value) {
        if (type === 'service') {
            this.filters.service = value;
            const select = this.querySelector('#service-filter');
            if (select) select.value = value;
        }
        this.renderTimeline();
    }
}

customElements.define('multisignal-timeline', MultisignalTimeline);
