/**
 * Field Extraction Widget
 * Manages log field extraction patterns, Grok patterns,
 * pattern testing, and learning statistics
 */
class FieldExtraction extends HTMLElement {
    constructor() {
        super();
        this.patterns = [];
        this.sources = [];
        this.grokPatterns = {};
        this.stats = null;
        this.selectedPattern = null;
        this.testResult = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    disconnectedCallback() {
        // Cleanup
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="extraction-container">
                <div class="extraction-header">
                    <div class="extraction-title">
                        <span class="title-icon">&#128269;</span>
                        <span>Field Extraction</span>
                    </div>
                    <div class="extraction-controls">
                        <button class="btn-add" id="btn-add-pattern">+ Add Pattern</button>
                        <button class="btn-refresh" id="btn-refresh">&#8635;</button>
                    </div>
                </div>

                <div class="stats-bar">
                    <div class="stat-item">
                        <span class="stat-value" id="stat-patterns">-</span>
                        <span class="stat-label">Active Patterns</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-extractions">-</span>
                        <span class="stat-label">Total Extractions</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-fields">-</span>
                        <span class="stat-label">Fields Extracted</span>
                    </div>
                    <div class="stat-item">
                        <span class="stat-value" id="stat-success">-</span>
                        <span class="stat-label">Success Rate</span>
                    </div>
                </div>

                <div class="extraction-content">
                    <div class="content-tabs">
                        <div class="tab active" data-tab="patterns">Patterns</div>
                        <div class="tab" data-tab="test">Test Extraction</div>
                        <div class="tab" data-tab="grok">Grok Library</div>
                        <div class="tab" data-tab="sources">Source Mapping</div>
                    </div>

                    <div class="tab-content active" id="tab-patterns">
                        <div class="patterns-list" id="patterns-list">
                            <div class="loading">Loading patterns...</div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-test">
                        <div class="test-container">
                            <div class="test-input-section">
                                <label>Paste a log line to test extraction:</label>
                                <textarea id="test-input" placeholder="2024-01-15 10:30:00 INFO [api-gateway] Request completed status=200 duration=150ms"></textarea>
                                <div class="test-controls">
                                    <select id="test-source" class="test-source-select">
                                        <option value="">Auto-detect source</option>
                                    </select>
                                    <button class="btn-test" id="btn-test">Extract Fields</button>
                                </div>
                            </div>
                            <div class="test-result-section" id="test-result">
                                <div class="test-empty">
                                    Enter a log line and click "Extract Fields" to see results
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-grok">
                        <div class="grok-search">
                            <input type="text" id="grok-search" placeholder="Search Grok patterns...">
                        </div>
                        <div class="grok-list" id="grok-list">
                            <div class="loading">Loading Grok patterns...</div>
                        </div>
                    </div>

                    <div class="tab-content" id="tab-sources">
                        <div class="sources-list" id="sources-list">
                            <div class="loading">Loading source mappings...</div>
                        </div>
                    </div>
                </div>

                <div class="pattern-modal" id="pattern-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span id="modal-title">Add Pattern</span>
                            <button class="btn-close" id="btn-close-modal">&times;</button>
                        </div>
                        <div class="modal-body">
                            <div class="form-field">
                                <label>Pattern Name</label>
                                <input type="text" id="pattern-name" placeholder="my-custom-pattern">
                            </div>
                            <div class="form-field">
                                <label>Pattern Type</label>
                                <select id="pattern-type">
                                    <option value="regex">Regex</option>
                                    <option value="grok">Grok</option>
                                    <option value="json">JSON</option>
                                    <option value="kv">Key-Value</option>
                                </select>
                            </div>
                            <div class="form-field">
                                <label>Pattern Expression</label>
                                <textarea id="pattern-expr" placeholder="%{IP:client_ip} - %{WORD:user} %{GREEDYDATA:message}"></textarea>
                            </div>
                            <div class="form-field">
                                <label>Priority (0-100)</label>
                                <input type="number" id="pattern-priority" value="50" min="0" max="100">
                            </div>
                            <div class="form-field">
                                <label>
                                    <input type="checkbox" id="pattern-enabled" checked>
                                    Enabled
                                </label>
                            </div>
                        </div>
                        <div class="modal-actions">
                            <button class="btn-cancel" id="btn-cancel-pattern">Cancel</button>
                            <button class="btn-save" id="btn-save-pattern">Save Pattern</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.setupEventListeners();
    }

    getStyles() {
        return `
            .extraction-container {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                position: relative;
            }
            .extraction-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }
            .extraction-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
                font-size: 0.9rem;
            }
            .title-icon { font-size: 1.1rem; }
            .extraction-controls {
                display: flex;
                gap: 0.5rem;
            }
            .btn-refresh, .btn-add, .btn-test, .btn-save, .btn-cancel, .btn-close, .btn-remove {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.6rem;
                cursor: pointer;
                font-size: 0.8rem;
            }
            .btn-add {
                background: var(--accent, #1d9bf0);
                border-color: var(--accent, #1d9bf0);
            }
            .btn-test {
                background: var(--success, #00ba7c);
                border-color: var(--success, #00ba7c);
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
                gap: 1.5rem;
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
            .extraction-content {
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
                font-size: 0.85rem;
            }
            .pattern-item {
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                margin-bottom: 0.5rem;
            }
            .pattern-item.disabled {
                opacity: 0.5;
            }
            .pattern-info {
                flex: 1;
                min-width: 0;
            }
            .pattern-header {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                margin-bottom: 0.25rem;
            }
            .pattern-name {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .pattern-badge {
                font-size: 0.65rem;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                background: rgba(29, 155, 240, 0.2);
                color: #1d9bf0;
            }
            .pattern-badge.builtin {
                background: rgba(167, 139, 250, 0.2);
                color: #a78bfa;
            }
            .pattern-expr {
                font-family: monospace;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                white-space: nowrap;
                overflow: hidden;
                text-overflow: ellipsis;
                max-width: 400px;
            }
            .pattern-meta {
                display: flex;
                gap: 1rem;
                margin-top: 0.5rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }
            .pattern-actions {
                display: flex;
                gap: 0.5rem;
            }
            .test-container {
                display: flex;
                flex-direction: column;
                gap: 1rem;
                height: 100%;
            }
            .test-input-section {
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                padding: 1rem;
            }
            .test-input-section label {
                display: block;
                font-size: 0.85rem;
                margin-bottom: 0.5rem;
            }
            .test-input-section textarea {
                width: 100%;
                height: 100px;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.75rem;
                color: var(--text, #e7e9ea);
                font-family: monospace;
                font-size: 0.85rem;
                resize: vertical;
            }
            .test-controls {
                display: flex;
                gap: 0.5rem;
                margin-top: 0.75rem;
            }
            .test-source-select {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.4rem 0.6rem;
                color: var(--text, #e7e9ea);
                font-size: 0.8rem;
            }
            .test-result-section {
                flex: 1;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                padding: 1rem;
                overflow: auto;
            }
            .test-empty {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 100%;
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }
            .extracted-fields {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
                gap: 0.75rem;
            }
            .extracted-field {
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                padding: 0.75rem;
            }
            .field-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.25rem;
            }
            .field-name {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .field-type {
                font-size: 0.65rem;
                padding: 0.15rem 0.4rem;
                border-radius: 3px;
                background: rgba(52, 211, 153, 0.2);
                color: #34d399;
            }
            .field-value {
                font-family: monospace;
                font-size: 0.8rem;
                word-break: break-all;
            }
            .field-confidence {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }
            .grok-search {
                margin-bottom: 1rem;
            }
            .grok-search input {
                width: 100%;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.75rem;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }
            .grok-list {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
                gap: 0.75rem;
            }
            .grok-item {
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                padding: 0.75rem;
            }
            .grok-name {
                font-weight: 500;
                font-size: 0.85rem;
                color: #a78bfa;
                margin-bottom: 0.25rem;
            }
            .grok-pattern {
                font-family: monospace;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                word-break: break-all;
            }
            .source-item {
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                padding: 0.75rem;
                margin-bottom: 0.5rem;
            }
            .source-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 0.5rem;
            }
            .source-name {
                font-weight: 500;
                font-size: 0.85rem;
            }
            .source-patterns {
                display: flex;
                flex-wrap: wrap;
                gap: 0.5rem;
            }
            .source-pattern-tag {
                font-size: 0.75rem;
                padding: 0.25rem 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
            }
            .pattern-modal {
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
                width: 450px;
                max-height: 80%;
                display: flex;
                flex-direction: column;
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
                overflow-y: auto;
            }
            .form-field {
                margin-bottom: 1rem;
            }
            .form-field label {
                display: block;
                font-size: 0.85rem;
                margin-bottom: 0.5rem;
            }
            .form-field input[type="text"],
            .form-field input[type="number"],
            .form-field select,
            .form-field textarea {
                width: 100%;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                padding: 0.5rem;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }
            .form-field textarea {
                min-height: 80px;
                font-family: monospace;
            }
            .modal-actions {
                display: flex;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
                justify-content: flex-end;
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

        // Test extraction
        this.querySelector('#btn-test')?.addEventListener('click', () => this.testExtraction());

        // Grok search
        this.querySelector('#grok-search')?.addEventListener('input', (e) => {
            this.filterGrokPatterns(e.target.value);
        });

        // Add pattern modal
        this.querySelector('#btn-add-pattern')?.addEventListener('click', () => {
            this.openPatternModal();
        });

        this.querySelector('#btn-close-modal')?.addEventListener('click', () => {
            this.querySelector('#pattern-modal').style.display = 'none';
        });

        this.querySelector('#btn-cancel-pattern')?.addEventListener('click', () => {
            this.querySelector('#pattern-modal').style.display = 'none';
        });

        this.querySelector('#btn-save-pattern')?.addEventListener('click', () => this.savePattern());
    }

    async loadData() {
        try {
            const [patternsResp, sourcesResp, grokResp, statsResp] = await Promise.all([
                fetch('/api/logs/extraction/patterns'),
                fetch('/api/logs/extraction/sources'),
                fetch('/api/logs/extraction/grok'),
                fetch('/api/logs/extraction/stats')
            ]);

            if (patternsResp.ok) {
                this.patterns = await patternsResp.json();
            } else {
                this.patterns = this.generateDemoPatterns();
            }

            if (sourcesResp.ok) {
                this.sources = await sourcesResp.json();
            } else {
                this.sources = this.generateDemoSources();
            }

            if (grokResp.ok) {
                this.grokPatterns = await grokResp.json();
            } else {
                this.grokPatterns = this.generateDemoGrokPatterns();
            }

            if (statsResp.ok) {
                this.stats = await statsResp.json();
            } else {
                this.stats = this.generateDemoStats();
            }

            this.updateStats();
            this.renderPatterns();
            this.renderGrokPatterns();
            this.renderSources();
            this.updateSourceSelect();
        } catch (e) {
            console.error('Failed to load extraction data:', e);
            this.patterns = this.generateDemoPatterns();
            this.sources = this.generateDemoSources();
            this.grokPatterns = this.generateDemoGrokPatterns();
            this.stats = this.generateDemoStats();
            this.updateStats();
            this.renderPatterns();
            this.renderGrokPatterns();
            this.renderSources();
        }
    }

    generateDemoPatterns() {
        return [
            {
                id: 'builtin-json',
                name: 'JSON Parser',
                type: 'json',
                pattern: 'Auto-detect JSON objects',
                enabled: true,
                priority: 100,
                builtin: true,
                matchCount: 45678
            },
            {
                id: 'builtin-kv',
                name: 'Key-Value Parser',
                type: 'kv',
                pattern: 'key=value key="quoted value"',
                enabled: true,
                priority: 90,
                builtin: true,
                matchCount: 23456
            },
            {
                id: 'builtin-apache',
                name: 'Apache Combined Log',
                type: 'grok',
                pattern: '%{IP:client_ip} - %{USER:user} \\[%{HTTPDATE:timestamp}\\] "%{WORD:method} %{URIPATH:path}"...',
                enabled: true,
                priority: 80,
                builtin: true,
                matchCount: 12345
            },
            {
                id: 'custom-1',
                name: 'App Error Pattern',
                type: 'regex',
                pattern: '(?P<level>ERROR|WARN)\\s+\\[(?P<component>[^\\]]+)\\]\\s+(?P<message>.*)',
                enabled: true,
                priority: 50,
                builtin: false,
                matchCount: 5678
            },
            {
                id: 'custom-2',
                name: 'Request Trace',
                type: 'grok',
                pattern: '%{WORD:method} %{URIPATH:path} status=%{INT:status} duration=%{NUMBER:duration}ms',
                enabled: false,
                priority: 40,
                builtin: false,
                matchCount: 1234
            }
        ];
    }

    generateDemoSources() {
        return [
            {
                name: 'api-gateway',
                patterns: ['builtin-json', 'custom-1'],
                logCount: 45678
            },
            {
                name: 'nginx',
                patterns: ['builtin-apache'],
                logCount: 123456
            },
            {
                name: 'user-service',
                patterns: ['builtin-json', 'builtin-kv'],
                logCount: 34567
            }
        ];
    }

    generateDemoGrokPatterns() {
        return {
            'IP': '\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}',
            'INT': '[+-]?[0-9]+',
            'NUMBER': '(?:[+-]?(?:[0-9]+(?:\\.[0-9]+)?)|(?:\\.[0-9]+))',
            'WORD': '\\b\\w+\\b',
            'UUID': '[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}',
            'TIMESTAMP_ISO8601': '\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d+)?(?:Z|[+-]\\d{2}:\\d{2})?',
            'LOGLEVEL': '(?:DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)',
            'HTTPDATE': '\\d{2}/[A-Za-z]{3}/\\d{4}:\\d{2}:\\d{2}:\\d{2} [+-]\\d{4}',
            'USER': '[a-zA-Z0-9._-]+',
            'URIPATH': '/[^\\s?#]*',
            'EMAIL': '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}',
            'GREEDYDATA': '.*',
            'DATA': '.*?',
            'NOTSPACE': '\\S+',
            'SPACE': '\\s*'
        };
    }

    generateDemoStats() {
        return {
            totalExtractions: 567890,
            totalFields: 2345678,
            patternCount: 5,
            successRate: 0.94,
            avgFieldsPerLog: 4.2
        };
    }

    updateStats() {
        const stats = this.stats || {};
        this.querySelector('#stat-patterns').textContent = stats.patternCount || this.patterns.length;
        this.querySelector('#stat-extractions').textContent = this.formatNumber(stats.totalExtractions || 0);
        this.querySelector('#stat-fields').textContent = this.formatNumber(stats.totalFields || 0);
        this.querySelector('#stat-success').textContent = `${((stats.successRate || 0) * 100).toFixed(1)}%`;
    }

    renderPatterns() {
        const container = this.querySelector('#patterns-list');
        if (!container) return;

        if (this.patterns.length === 0) {
            container.innerHTML = '<div class="empty-state">No patterns configured</div>';
            return;
        }

        // Sort by priority descending
        const sorted = [...this.patterns].sort((a, b) => (b.priority || 0) - (a.priority || 0));

        container.innerHTML = sorted.map(pattern => `
            <div class="pattern-item ${pattern.enabled ? '' : 'disabled'}">
                <div class="pattern-info">
                    <div class="pattern-header">
                        <span class="pattern-name">${this.escapeHtml(pattern.name)}</span>
                        <span class="pattern-badge ${pattern.builtin ? 'builtin' : ''}">${pattern.type}</span>
                        ${pattern.builtin ? '<span class="pattern-badge builtin">builtin</span>' : ''}
                    </div>
                    <div class="pattern-expr" title="${this.escapeHtml(pattern.pattern)}">${this.escapeHtml(pattern.pattern)}</div>
                    <div class="pattern-meta">
                        <span>Priority: ${pattern.priority}</span>
                        <span>${this.formatNumber(pattern.matchCount || 0)} matches</span>
                        <span>${pattern.enabled ? 'Enabled' : 'Disabled'}</span>
                    </div>
                </div>
                ${!pattern.builtin ? `
                    <div class="pattern-actions">
                        <button class="btn-refresh" data-edit="${this.escapeHtml(pattern.id)}">Edit</button>
                        <button class="btn-remove" data-remove="${this.escapeHtml(pattern.id)}">Remove</button>
                    </div>
                ` : ''}
            </div>
        `).join('');

        // Add event handlers
        container.querySelectorAll('[data-edit]').forEach(btn => {
            btn.addEventListener('click', () => {
                const pattern = this.patterns.find(p => p.id === btn.dataset.edit);
                if (pattern) this.openPatternModal(pattern);
            });
        });

        container.querySelectorAll('[data-remove]').forEach(btn => {
            btn.addEventListener('click', () => {
                if (confirm('Remove this pattern?')) {
                    this.removePattern(btn.dataset.remove);
                }
            });
        });
    }

    renderGrokPatterns() {
        const container = this.querySelector('#grok-list');
        if (!container) return;

        const patterns = Object.entries(this.grokPatterns);
        if (patterns.length === 0) {
            container.innerHTML = '<div class="empty-state">No Grok patterns available</div>';
            return;
        }

        container.innerHTML = patterns.map(([name, pattern]) => `
            <div class="grok-item" data-name="${this.escapeHtml(name.toLowerCase())}">
                <div class="grok-name">%{${this.escapeHtml(name)}}</div>
                <div class="grok-pattern">${this.escapeHtml(pattern)}</div>
            </div>
        `).join('');
    }

    filterGrokPatterns(query) {
        const items = this.querySelectorAll('.grok-item');
        const q = query.toLowerCase();

        items.forEach(item => {
            const name = item.dataset.name || '';
            item.style.display = name.includes(q) ? '' : 'none';
        });
    }

    renderSources() {
        const container = this.querySelector('#sources-list');
        if (!container) return;

        if (this.sources.length === 0) {
            container.innerHTML = '<div class="empty-state">No source mappings configured</div>';
            return;
        }

        container.innerHTML = this.sources.map(source => `
            <div class="source-item">
                <div class="source-header">
                    <span class="source-name">${this.escapeHtml(source.name)}</span>
                    <span style="font-size: 0.75rem; color: var(--text-muted);">
                        ${this.formatNumber(source.logCount || 0)} logs
                    </span>
                </div>
                <div class="source-patterns">
                    ${(source.patterns || []).map(p => {
                        const pattern = this.patterns.find(pat => pat.id === p);
                        return `<span class="source-pattern-tag">${this.escapeHtml(pattern?.name || p)}</span>`;
                    }).join('')}
                </div>
            </div>
        `).join('');
    }

    updateSourceSelect() {
        const select = this.querySelector('#test-source');
        if (!select) return;

        select.innerHTML = '<option value="">Auto-detect source</option>' +
            this.sources.map(s => `<option value="${this.escapeHtml(s.name)}">${this.escapeHtml(s.name)}</option>`).join('');
    }

    async testExtraction() {
        const input = this.querySelector('#test-input')?.value;
        const source = this.querySelector('#test-source')?.value;
        const resultContainer = this.querySelector('#test-result');

        if (!input?.trim()) {
            alert('Please enter a log line to test');
            return;
        }

        resultContainer.innerHTML = '<div class="loading">Extracting fields...</div>';

        try {
            const resp = await fetch('/api/logs/extraction/extract', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: input, source: source || undefined })
            });

            if (resp.ok) {
                this.testResult = await resp.json();
            } else {
                // Demo extraction
                this.testResult = this.demoExtract(input);
            }

            this.renderTestResult();
        } catch (e) {
            console.error('Failed to test extraction:', e);
            this.testResult = this.demoExtract(input);
            this.renderTestResult();
        }
    }

    demoExtract(input) {
        const fields = [];

        // Try JSON
        try {
            const json = JSON.parse(input);
            for (const [key, value] of Object.entries(json)) {
                fields.push({
                    name: key,
                    value: String(value),
                    type: this.inferType(value),
                    confidence: 1.0
                });
            }
            return { fields, patternUsed: 'JSON Parser' };
        } catch (e) {
            // Not JSON
        }

        // Try key-value
        const kvMatches = input.matchAll(/(\w+)=("[^"]*"|[^\s]+)/g);
        for (const match of kvMatches) {
            let value = match[2];
            if (value.startsWith('"') && value.endsWith('"')) {
                value = value.slice(1, -1);
            }
            fields.push({
                name: match[1],
                value: value,
                type: this.inferType(value),
                confidence: 0.9
            });
        }

        if (fields.length > 0) {
            return { fields, patternUsed: 'Key-Value Parser' };
        }

        // Try some common patterns
        const ipMatch = input.match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
        if (ipMatch) {
            fields.push({ name: 'ip_address', value: ipMatch[1], type: 'ip', confidence: 0.95 });
        }

        const timestampMatch = input.match(/(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2})/);
        if (timestampMatch) {
            fields.push({ name: 'timestamp', value: timestampMatch[1], type: 'timestamp', confidence: 0.95 });
        }

        const levelMatch = input.match(/\b(DEBUG|INFO|WARN|WARNING|ERROR|FATAL|CRITICAL)\b/i);
        if (levelMatch) {
            fields.push({ name: 'level', value: levelMatch[1].toUpperCase(), type: 'loglevel', confidence: 0.95 });
        }

        return { fields, patternUsed: fields.length > 0 ? 'Pattern Matching' : 'No matches' };
    }

    inferType(value) {
        if (typeof value === 'number') return value % 1 === 0 ? 'int' : 'float';
        if (typeof value === 'boolean') return 'bool';
        if (typeof value !== 'string') return 'string';

        if (/^\d+$/.test(value)) return 'int';
        if (/^[+-]?\d+\.\d+$/.test(value)) return 'float';
        if (/^(true|false|yes|no)$/i.test(value)) return 'bool';
        if (/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(value)) return 'ip';
        if (/^[a-f0-9-]{36}$/i.test(value)) return 'uuid';
        if (/^.+@.+\..+$/.test(value)) return 'email';
        if (/^\d{4}-\d{2}-\d{2}/.test(value)) return 'timestamp';
        if (/^\d+ms$/.test(value)) return 'duration';

        return 'string';
    }

    renderTestResult() {
        const container = this.querySelector('#test-result');
        if (!container || !this.testResult) return;

        const { fields, patternUsed } = this.testResult;

        if (!fields || fields.length === 0) {
            container.innerHTML = `
                <div class="test-empty">
                    <div>No fields extracted</div>
                    <div style="font-size: 0.8rem; margin-top: 0.5rem;">Pattern: ${this.escapeHtml(patternUsed || 'None')}</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div style="margin-bottom: 1rem; font-size: 0.85rem;">
                <strong>Pattern Used:</strong> ${this.escapeHtml(patternUsed)}
                <span style="color: var(--text-muted); margin-left: 1rem;">${fields.length} fields extracted</span>
            </div>
            <div class="extracted-fields">
                ${fields.map(f => `
                    <div class="extracted-field">
                        <div class="field-header">
                            <span class="field-name">${this.escapeHtml(f.name)}</span>
                            <span class="field-type">${f.type}</span>
                        </div>
                        <div class="field-value">${this.escapeHtml(String(f.value))}</div>
                        <div class="field-confidence">Confidence: ${(f.confidence * 100).toFixed(0)}%</div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    openPatternModal(pattern = null) {
        this.selectedPattern = pattern;

        const modal = this.querySelector('#pattern-modal');
        const title = this.querySelector('#modal-title');
        const nameInput = this.querySelector('#pattern-name');
        const typeInput = this.querySelector('#pattern-type');
        const exprInput = this.querySelector('#pattern-expr');
        const priorityInput = this.querySelector('#pattern-priority');
        const enabledInput = this.querySelector('#pattern-enabled');

        if (pattern) {
            title.textContent = 'Edit Pattern';
            nameInput.value = pattern.name || '';
            typeInput.value = pattern.type || 'regex';
            exprInput.value = pattern.pattern || '';
            priorityInput.value = pattern.priority || 50;
            enabledInput.checked = pattern.enabled !== false;
        } else {
            title.textContent = 'Add Pattern';
            nameInput.value = '';
            typeInput.value = 'regex';
            exprInput.value = '';
            priorityInput.value = 50;
            enabledInput.checked = true;
        }

        modal.style.display = 'flex';
    }

    async savePattern() {
        const pattern = {
            id: this.selectedPattern?.id || 'custom-' + Date.now(),
            name: this.querySelector('#pattern-name')?.value,
            type: this.querySelector('#pattern-type')?.value,
            pattern: this.querySelector('#pattern-expr')?.value,
            priority: parseInt(this.querySelector('#pattern-priority')?.value) || 50,
            enabled: this.querySelector('#pattern-enabled')?.checked ?? true,
            builtin: false,
            matchCount: this.selectedPattern?.matchCount || 0
        };

        if (!pattern.name || !pattern.pattern) {
            alert('Please fill in all required fields');
            return;
        }

        try {
            const resp = await fetch('/api/logs/extraction/patterns', {
                method: this.selectedPattern ? 'PUT' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(pattern)
            });

            if (resp.ok) {
                this.querySelector('#pattern-modal').style.display = 'none';
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to save pattern:', e);
            // Demo: update locally
            if (this.selectedPattern) {
                const idx = this.patterns.findIndex(p => p.id === this.selectedPattern.id);
                if (idx >= 0) this.patterns[idx] = pattern;
            } else {
                this.patterns.push(pattern);
            }
            this.querySelector('#pattern-modal').style.display = 'none';
            this.renderPatterns();
            this.updateStats();
        }
    }

    async removePattern(id) {
        try {
            const resp = await fetch(`/api/logs/extraction/patterns/${encodeURIComponent(id)}`, {
                method: 'DELETE'
            });

            if (resp.ok) {
                await this.loadData();
            }
        } catch (e) {
            console.error('Failed to remove pattern:', e);
            // Demo: remove locally
            this.patterns = this.patterns.filter(p => p.id !== id);
            this.renderPatterns();
            this.updateStats();
        }
    }

    formatNumber(n) {
        if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
        if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
        return n.toString();
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

customElements.define('field-extraction', FieldExtraction);
