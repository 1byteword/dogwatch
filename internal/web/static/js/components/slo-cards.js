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
        const current = parseFloat(slo.current_value) || 0;
        const target = parseFloat(slo.target) || 99.9;
        const sloId = this.escapeAttr(slo.id);

        return `
            <div class="slo-card status-${status}" data-slo-id="${sloId}" onclick="this.getRootNode().host.showSLODetail(this.dataset.sloId)">
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
                    <span class="slo-window">${this.escapeHtml(slo.window || '30d')} window</span>
                    ${status === 'critical' ? '<span class="alert-badge">At Risk</span>' : ''}
                </div>
            </div>
        `;
    }

    renderSLORow(slo) {
        const budgetRemaining = this.getBudgetRemaining(slo);
        const burnRate = this.getBurnRate(slo);
        const status = this.getStatus(budgetRemaining);
        const current = parseFloat(slo.current_value) || 0;
        const target = parseFloat(slo.target) || 99.9;
        const sloId = this.escapeAttr(slo.id);

        return `
            <div class="slo-row status-${status}" data-slo-id="${sloId}" onclick="this.getRootNode().host.showSLODetail(this.dataset.sloId)">
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
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    escapeAttr(str) {
        if (!str) return '';
        return String(str).replace(/[&"'<>]/g, c => ({
            '&': '&amp;', '"': '&quot;', "'": '&#39;', '<': '&lt;', '>': '&gt;'
        }[c]));
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
