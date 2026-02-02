/**
 * Clock Skew Diagnostics Widget
 * Displays clock skew between service pairs, violations, NTP drift tracking,
 * and manual correction controls
 */
class ClockSkewDiagnostics extends HTMLElement {
    constructor() {
        super();
        this.stats = null;
        this.pairs = [];
        this.violations = [];
        this.corrections = {};
        this.ntpStats = {};
        this.config = null;
        this.refreshInterval = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
        this.refreshInterval = setInterval(() => this.loadData(), 30000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="clockskew-container">
                <div class="clockskew-header">
                    <div class="clockskew-title">
                        <span class="title-icon">&#128338;</span>
                        <span>Clock Skew Diagnostics</span>
                    </div>
                    <div class="clockskew-controls">
                        <button class="btn-config" id="btn-config">&#9881; Config</button>
                        <button class="btn-refresh" id="btn-refresh">&#8635;</button>
                    </div>
                </div>

                <div class="stats-bar">
                    <div class="stat-item">
                        <span class="stat-value" id="stat-pairs">-</span>
                        <span class="stat-label">Service Pairs</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-violations">-</span>
                        <span class="stat-label">Violations</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-corrections">-</span>
                        <span class="stat-label">Active Corrections</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-max-skew">-</span>
                        <span class="stat-label">Max Skew</span>
                    </div>
                </div>

                <div class="clockskew-content">
                    <div class="content-tabs">
                        <div class="tab active" data-tab="matrix">Pair Matrix</div>
                        <div class="tab" data-tab="violations">Violations</div>
                        <div class="tab" data-tab="ntp">NTP Drift</div>
                        <div class="tab" data-tab="corrections">Corrections</div>
                    </div>

                    <div class="tab-content active" id="tab-matrix">
                        <div class="matrix-container" id="matrix-container">
                            <div class="loading">Loading skew data...</div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-violations">
                        <div class="violations-list" id="violations-list">
                            <div class="loading">Loading violations...</div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-ntp">
                        <div class="ntp-container" id="ntp-container">
                            <div class="loading">Loading NTP data...</div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-corrections">
                        <div class="corrections-container" id="corrections-container">
                            <div class="corrections-header">
                                <h3>Manual Corrections</h3>
                                <button class="btn-add" id="btn-add-correction">+ Add Correction</button>
                            </div>
                            <div class="corrections-list" id="corrections-list">
                                <div class="loading">Loading corrections...</div>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="config-panel" id="config-panel" style="display: none;">
                    <div class="config-header">
                        <span>Configuration</span>
                        <button class="btn-close" id="btn-close-config">&times;</button>
                    </div>
                    <div class="config-body">
                        <div class="config-field">
                            <label>Max Skew Tolerance</label>
                            <div class="config-input-group">
                                <input type="number" id="cfg-tolerance" value="5000">
                                <span>ms</span>
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Detection Window</label>
                            <div class="config-input-group">
                                <input type="number" id="cfg-window" value="5">
                                <span>min</span>
                            </div>
                        </div>
                        <div class="config-field">
                            <label>Auto-Correction</label>
                            <label class="toggle">
                                <input type="checkbox" id="cfg-autocorrect" checked>
                                <span class="toggle-slider"></span>
                            </label>
                        </div>
                        <div class="config-actions">
                            <button class="btn-save" id="btn-save-config">Save Changes</button>
                        </div>
                    </div>
                </div>

                <div class="correction-modal" id="correction-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span>Add Manual Correction</span>
                            <button class="btn-close" id="btn-close-modal">&times;</button>
                        </div>
                        <div class="modal-body">
                            <div class="config-field">
                                <label>Service</label>
                                <input type="text" id="correction-service" placeholder="service-name">
                            </div>
                            <div class="config-field">
                                <label>Offset (milliseconds)</label>
                                <input type="number" id="correction-offset" placeholder="100">
                            </div>
                        </div>
                        <div class="modal-actions">
                            <button class="btn-cancel" id="btn-cancel-correction">Cancel</button>
                            <button class="btn-save" id="btn-save-correction">Add Correction</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    getStyles() {
        return `
            .clockskew-container {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                position: relative;
            }
            .clockskew-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .clockskew-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                font-size: 0.9rem;
            }
            .title-icon { font-size: 1.1rem; }
            .clockskew-controls {
                display: flex;
                gap: 0.5rem;
            }
            .btn-refresh, .btn-config, .btn-add, .btn-save, .btn-cancel, .btn-close, .btn-remove {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
                font-size: 0.8rem;
            }
            .btn-save {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
            }
            .btn-remove {
                background: rgba(244, 63, 94, 0.2);
                color: #f43f5e;
                border-color: rgba(244, 63, 94, 0.3);
            }
            .stats-bar {
                display: flex;
                gap: 1rem;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .stat-item {
                display: flex;
                flex-direction: column;
                align-items: center;
            }
            .stat-value {
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }
            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .clockskew-content {
                flex: 1;
                display: flex;
                flex-direction: column;
                overflow: hidden;
            }
            .content-tabs {
                display: flex;
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .tab {
                padding: 0.75rem 1rem;
                cursor: pointer;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                border-bottom: 2px solid transparent;
                transition: all 0.15s ease;
            }
            .tab:hover { color: var(--text, #e7e9ea); }
            .tab.active {
                color: var(--accent, #1d9bf0);
                border-bottom-color: var(--accent, #1d9bf0);
            }
            .tab-content {
                display: none;
                flex: 1;
                overflow: auto;
                padding: 1rem;
            }
            .tab-content.active { display: block; }
            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
            }
            .matrix-container {
                overflow: auto;
            }
            .skew-matrix {
                border-collapse: collapse;
                width: 100%;
                font-size: 0.8rem;
            }
            .skew-matrix th, .skew-matrix td {
                padding: 0.5rem;
                text-align: center;
                border: 1px solid var(--border, #2f3336);
            }
            .skew-matrix th {
                background: var(--bg-elevated, #1e2128);
                font-weight: 500;
            }
            .skew-matrix td {
                font-family: monospace;
            }
            .skew-cell {
                min-width: 60px;
            }
            .skew-cell.good { background: rgba(0, 186, 124, 0.2); }
            .skew-cell.warn { background: rgba(251, 191, 36, 0.2); }
            .skew-cell.bad { background: rgba(244, 63, 94, 0.2); }
            .skew-cell.self { background: var(--bg-card, #16181c); color: var(--text-muted); }
            .violation-item {
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
                border-left: 3px solid #f43f5e;
            }
            .violation-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
            }
            .violation-title {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .violation-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .violation-meta {
                display: flex;
                gap: 1rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }
            .violation-meta span {
                display: flex;
                align-items: center;
                gap: 0.25rem;
            }
            .ntp-item {
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
            }
            .ntp-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
            }
            .ntp-service {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .ntp-stats {
                display: flex;
                gap: 1.5rem;
                font-size: 0.8rem;
            }
            .ntp-stat {
                display: flex;
                flex-direction: column;
            }
            .ntp-stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }
            .ntp-stat-value {
                font-family: monospace;
            }
            .ntp-drift-bar {
                height: 8px;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                overflow: hidden;
                margin-top: 0.5rem;
            }
            .ntp-drift-fill {
                height: 100%;
                background: var(--accent, #1d9bf0);
                transition: width 0.3s ease;
            }
            .corrections-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 1rem;
            }
            .corrections-header h3 {
                margin: 0;
                font-size: 0.9rem;
            }
            .correction-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
            }
            .correction-info {
                display: flex;
                flex-direction: column;
                gap: 0.25rem;
            }
            .correction-service {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .correction-offset {
                font-size: 0.8rem;
                font-family: monospace;
                color: var(--accent, #1d9bf0);
            }
            .config-panel {
                position: absolute;
                top: 50px;
                right: 10px;
                width: 280px;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
                z-index: 100;
            }
            .config-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 500;
            }
            .config-body {
                padding: 1rem;
            }
            .config-field {
                margin-bottom: 1rem;
            }
            .config-field label {
                display: block;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }
            .config-input-group {
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }
            .config-input-group input {
                flex: 1;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.5rem;
                color: var(--text, #e7e9ea);
            }
            .config-input-group span {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }
            .toggle {
                position: relative;
                display: inline-block;
                width: 40px;
                height: 22px;
            }
            .toggle input { opacity: 0; width: 0; height: 0; }
            .toggle-slider {
                position: absolute;
                cursor: pointer;
                top: 0; left: 0; right: 0; bottom: 0;
                background: var(--bg-card, #16181c);
                border-radius: 11px;
                transition: 0.3s;
            }
            .toggle-slider:before {
                position: absolute;
                content: "";
                height: 16px;
                width: 16px;
                left: 3px;
                bottom: 3px;
                background: white;
                border-radius: 50%;
                transition: 0.3s;
            }
            .toggle input:checked + .toggle-slider {
                background: var(--accent, #1d9bf0);
            }
            .toggle input:checked + .toggle-slider:before {
                transform: translateX(18px);
            }
            .config-actions {
                margin-top: 1rem;
            }
            .config-actions .btn-save {
                width: 100%;
            }
            .correction-modal {
                position: absolute;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 200;
            }
            .modal-content {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                width: 320px;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            }
            .modal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 500;
            }
            .modal-body {
                padding: 1rem;
            }
            .modal-body input {
                width: 100%;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.5rem;
                color: var(--text, #e7e9ea);
            }
            .modal-actions {
                display: flex;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }
            .modal-actions button {
                flex: 1;
            }
            .confidence-badge {
                display: inline-flex;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                font-size: 0.7rem;
                font-weight: 500;
            }
            .confidence-badge.high {
                background: rgba(0, 186, 124, 0.2);
                color: #00ba7c;
            }
            .confidence-badge.medium {
                background: rgba(251, 191, 36, 0.2);
                color: #fbbf24;
            }
            .confidence-badge.low {
                background: rgba(244, 63, 94, 0.2);
                color: #f43f5e;
            }
            .empty-state {
                text-align: center;
                padding: 2rem;
                color: var(--text-muted, #71767b);
            }
        `;
    }

    setupEventListeners() {
        // Refresh
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());

        // Tab switching
        this.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', (e) => {
                this.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                this.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                e.target.classList.add('active');
                const tabId = e.target.dataset.tab;
                this.querySelector(`#tab-${tabId}`)?.classList.add('active');
            });
        });

        // Config panel
        this.querySelector('#btn-config')?.addEventListener('click', () => {
            const panel = this.querySelector('#config-panel');
            if (panel) panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
        });

        this.querySelector('#btn-close-config')?.addEventListener('click', () => {
            const panel = this.querySelector('#config-panel');
            if (panel) panel.style.display = 'none';
        });

        this.querySelector('#btn-save-config')?.addEventListener('click', () => this.saveConfig());

        // Correction modal
        this.querySelector('#btn-add-correction')?.addEventListener('click', () => {
            const modal = this.querySelector('#correction-modal');
            if (modal) modal.style.display = 'flex';
        });

        this.querySelector('#btn-close-modal')?.addEventListener('click', () => {
            const modal = this.querySelector('#correction-modal');
            if (modal) modal.style.display = 'none';
        });

        this.querySelector('#btn-cancel-correction')?.addEventListener('click', () => {
            const modal = this.querySelector('#correction-modal');
            if (modal) modal.style.display = 'none';
        });

        this.querySelector('#btn-save-correction')?.addEventListener('click', () => this.addCorrection());
    }

    async loadData() {
        try {
            const [statsResp, pairsResp, correctionsResp, ntpResp, reportResp] = await Promise.all([
                fetch('/api/clockskew/stats'),
                fetch('/api/clockskew/pairs'),
                fetch('/api/clockskew/corrections'),
                fetch('/api/clockskew/ntp'),
                fetch('/api/clockskew/report')
            ]);

            if (statsResp.ok) {
                this.stats = await statsResp.json();
            } else {
                this.stats = this.generateDemoStats();
            }

            if (pairsResp.ok) {
                this.pairs = await pairsResp.json();
            } else {
                this.pairs = this.generateDemoPairs();
            }

            if (correctionsResp.ok) {
                this.corrections = await correctionsResp.json();
            } else {
                this.corrections = this.generateDemoCorrections();
            }

            if (ntpResp.ok) {
                this.ntpStats = await ntpResp.json();
            } else {
                this.ntpStats = this.generateDemoNTP();
            }

            if (reportResp.ok) {
                const report = await reportResp.json();
                this.violations = report.violations || [];
                this.config = report.config;
            } else {
                this.violations = this.generateDemoViolations();
            }

            this.updateStats();
            this.renderMatrix();
            this.renderViolations();
            this.renderNTP();
            this.renderCorrections();
            this.updateConfigPanel();
        } catch (e) {
            console.error('Failed to load clock skew data:', e);
            // Use demo data
            this.stats = this.generateDemoStats();
            this.pairs = this.generateDemoPairs();
            this.corrections = this.generateDemoCorrections();
            this.ntpStats = this.generateDemoNTP();
            this.violations = this.generateDemoViolations();
            this.updateStats();
            this.renderMatrix();
            this.renderViolations();
            this.renderNTP();
            this.renderCorrections();
        }
    }

    generateDemoStats() {
        return {
            totalSpansProcessed: 45678,
            totalViolationsFound: 12,
            activePairs: 6,
            maxSkew: 234
        };
    }

    generateDemoPairs() {
        const services = ['api-gateway', 'user-service', 'order-service', 'payment-service'];
        const pairs = [];

        for (let i = 0; i < services.length; i++) {
            for (let j = 0; j < services.length; j++) {
                if (i !== j) {
                    pairs.push({
                        source: services[i],
                        target: services[j],
                        skewMs: Math.floor(Math.random() * 500 - 250),
                        sampleCount: Math.floor(100 + Math.random() * 900),
                        lastUpdated: Date.now() - Math.random() * 300000
                    });
                }
            }
        }

        return pairs;
    }

    generateDemoCorrections() {
        return {
            'user-service': 50,
            'order-service': -30
        };
    }

    generateDemoNTP() {
        return {
            'api-gateway': {
                meanDrift: 5,
                maxDrift: 15,
                minDrift: -3,
                sampleCount: 1440
            },
            'user-service': {
                meanDrift: 12,
                maxDrift: 45,
                minDrift: -8,
                sampleCount: 1420
            },
            'order-service': {
                meanDrift: -8,
                maxDrift: 10,
                minDrift: -25,
                sampleCount: 1435
            }
        };
    }

    generateDemoViolations() {
        return [
            {
                traceId: 'trace-abc123',
                parentService: 'api-gateway',
                childService: 'user-service',
                skewMs: 234,
                timestamp: Date.now() - 300000
            },
            {
                traceId: 'trace-def456',
                parentService: 'user-service',
                childService: 'order-service',
                skewMs: -156,
                timestamp: Date.now() - 1200000
            },
            {
                traceId: 'trace-ghi789',
                parentService: 'api-gateway',
                childService: 'payment-service',
                skewMs: 89,
                timestamp: Date.now() - 3600000
            }
        ];
    }

    updateStats() {
        const stats = this.stats || {};
        this.querySelector('#stat-pairs').textContent = stats.activePairs || this.pairs.length / 2 || 0;
        this.querySelector('#stat-violations').textContent = stats.totalViolationsFound || this.violations.length;
        this.querySelector('#stat-corrections').textContent = Object.keys(this.corrections).length;
        this.querySelector('#stat-max-skew').textContent = stats.maxSkew ? `${stats.maxSkew}ms` : '-';
    }

    renderMatrix() {
        const container = this.querySelector('#matrix-container');
        if (!container) return;

        if (!this.pairs || this.pairs.length === 0) {
            container.innerHTML = '<div class="empty-state">No service pairs detected yet</div>';
            return;
        }

        // Extract unique services
        const services = [...new Set([
            ...this.pairs.map(p => p.source),
            ...this.pairs.map(p => p.target)
        ])].sort();

        // Build skew lookup
        const skewLookup = {};
        for (const pair of this.pairs) {
            skewLookup[`${pair.source}:${pair.target}`] = pair.skewMs;
        }

        // Build table
        let html = '<table class="skew-matrix">';
        html += '<tr><th></th>' + services.map(s => `<th>${this.escapeHtml(s)}</th>`).join('') + '</tr>';

        for (const source of services) {
            html += `<tr><th>${this.escapeHtml(source)}</th>`;
            for (const target of services) {
                if (source === target) {
                    html += '<td class="skew-cell self">-</td>';
                } else {
                    const skew = skewLookup[`${source}:${target}`];
                    const skewClass = this.getSkewClass(skew);
                    const skewText = skew !== undefined ? `${skew}ms` : '-';
                    html += `<td class="skew-cell ${skewClass}">${skewText}</td>`;
                }
            }
            html += '</tr>';
        }

        html += '</table>';
        container.innerHTML = html;
    }

    getSkewClass(skewMs) {
        if (skewMs === undefined) return '';
        const absSkew = Math.abs(skewMs);
        if (absSkew < 50) return 'good';
        if (absSkew < 200) return 'warn';
        return 'bad';
    }

    renderViolations() {
        const container = this.querySelector('#violations-list');
        if (!container) return;

        if (!this.violations || this.violations.length === 0) {
            container.innerHTML = '<div class="empty-state">No violations detected</div>';
            return;
        }

        container.innerHTML = this.violations.map(v => `
            <div class="violation-item">
                <div class="violation-header">
                    <span class="violation-title">${this.escapeHtml(v.parentService)} -> ${this.escapeHtml(v.childService)}</span>
                    <span class="violation-time">${this.formatRelativeTime(v.timestamp)}</span>
                </div>
                <div class="violation-meta">
                    <span>Skew: <strong>${v.skewMs}ms</strong></span>
                    <span>Trace: ${this.escapeHtml(v.traceId?.slice(0, 12) || 'N/A')}...</span>
                </div>
            </div>
        `).join('');
    }

    renderNTP() {
        const container = this.querySelector('#ntp-container');
        if (!container) return;

        const services = Object.keys(this.ntpStats);
        if (services.length === 0) {
            container.innerHTML = '<div class="empty-state">No NTP data available</div>';
            return;
        }

        const maxDrift = Math.max(...services.map(s => Math.abs(this.ntpStats[s].maxDrift || 0)), 100);

        container.innerHTML = services.map(service => {
            const stats = this.ntpStats[service];
            const driftPercent = Math.min(Math.abs(stats.meanDrift) / maxDrift * 100, 100);
            const confidenceClass = stats.sampleCount > 1000 ? 'high' : stats.sampleCount > 500 ? 'medium' : 'low';

            return `
                <div class="ntp-item">
                    <div class="ntp-header">
                        <span class="ntp-service">${this.escapeHtml(service)}</span>
                        <span class="confidence-badge ${confidenceClass}">${stats.sampleCount} samples</span>
                    </div>
                    <div class="ntp-stats">
                        <div class="ntp-stat">
                            <span class="ntp-stat-label">Mean Drift</span>
                            <span class="ntp-stat-value">${stats.meanDrift}ms</span>
                        </div>
                        <div class="ntp-stat">
                            <span class="ntp-stat-label">Max</span>
                            <span class="ntp-stat-value">${stats.maxDrift}ms</span>
                        </div>
                        <div class="ntp-stat">
                            <span class="ntp-stat-label">Min</span>
                            <span class="ntp-stat-value">${stats.minDrift}ms</span>
                        </div>
                    </div>
                    <div class="ntp-drift-bar">
                        <div class="ntp-drift-fill" style="width: ${driftPercent}%"></div>
                    </div>
                </div>
            `;
        }).join('');
    }

    renderCorrections() {
        const container = this.querySelector('#corrections-list');
        if (!container) return;

        const services = Object.keys(this.corrections);
        if (services.length === 0) {
            container.innerHTML = '<div class="empty-state">No manual corrections configured</div>';
            return;
        }

        container.innerHTML = services.map(service => `
            <div class="correction-item">
                <div class="correction-info">
                    <span class="correction-service">${this.escapeHtml(service)}</span>
                    <span class="correction-offset">${this.corrections[service] > 0 ? '+' : ''}${this.corrections[service]}ms</span>
                </div>
                <button class="btn-remove" data-service="${this.escapeHtml(service)}">Remove</button>
            </div>
        `).join('');

        // Add remove handlers
        container.querySelectorAll('.btn-remove').forEach(btn => {
            btn.addEventListener('click', () => this.removeCorrection(btn.dataset.service));
        });
    }

    updateConfigPanel() {
        if (!this.config) return;

        const toleranceInput = this.querySelector('#cfg-tolerance');
        const windowInput = this.querySelector('#cfg-window');
        const autocorrectInput = this.querySelector('#cfg-autocorrect');

        if (toleranceInput && this.config.maxSkewTolerance) {
            toleranceInput.value = this.config.maxSkewTolerance / 1000000; // Convert from nanoseconds
        }
        if (windowInput && this.config.detectionWindow) {
            windowInput.value = this.config.detectionWindow / 60000000000; // Convert from nanoseconds
        }
        if (autocorrectInput && this.config.enableAutoCorrection !== undefined) {
            autocorrectInput.checked = this.config.enableAutoCorrection;
        }
    }

    async saveConfig() {
        const toleranceMs = parseInt(this.querySelector('#cfg-tolerance')?.value || 5000);
        const windowMin = parseInt(this.querySelector('#cfg-window')?.value || 5);
        const autoCorrect = this.querySelector('#cfg-autocorrect')?.checked ?? true;

        const config = {
            maxSkewTolerance: toleranceMs * 1000000, // Convert to nanoseconds
            detectionWindow: windowMin * 60000000000,
            enableAutoCorrection: autoCorrect
        };

        try {
            const resp = await fetch('/api/clockskew/config', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config)
            });

            if (resp.ok) {
                this.querySelector('#config-panel').style.display = 'none';
                this.loadData();
            }
        } catch (e) {
            console.error('Failed to save config:', e);
        }
    }

    async addCorrection() {
        const service = this.querySelector('#correction-service')?.value;
        const offset = parseInt(this.querySelector('#correction-offset')?.value || 0);

        if (!service) {
            alert('Please enter a service name');
            return;
        }

        try {
            const resp = await fetch('/api/clockskew/corrections', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ service, offsetMs: offset })
            });

            if (resp.ok) {
                this.querySelector('#correction-modal').style.display = 'none';
                this.querySelector('#correction-service').value = '';
                this.querySelector('#correction-offset').value = '';
                this.loadData();
            }
        } catch (e) {
            console.error('Failed to add correction:', e);
            // Add locally for demo
            this.corrections[service] = offset;
            this.querySelector('#correction-modal').style.display = 'none';
            this.renderCorrections();
        }
    }

    async removeCorrection(service) {
        try {
            const resp = await fetch(`/api/clockskew/corrections/${encodeURIComponent(service)}`, {
                method: 'DELETE'
            });

            if (resp.ok) {
                this.loadData();
            }
        } catch (e) {
            console.error('Failed to remove correction:', e);
            // Remove locally for demo
            delete this.corrections[service];
            this.renderCorrections();
            this.updateStats();
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
}

customElements.define('clock-skew-diagnostics', ClockSkewDiagnostics);
