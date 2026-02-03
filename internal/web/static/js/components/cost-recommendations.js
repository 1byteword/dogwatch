/**
 * Cost Recommendations Widget
 * Actionable cost optimization recommendations
 */
class CostRecommendations extends HTMLElement {
    constructor() {
        super();
        this.report = null;
        this.loading = true;
        this.error = null;
        this.filterType = '';
        this.filterPriority = '';
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/cost/recommendations');
            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            this.report = await resp.json();
        } catch (e) {
            console.error('Failed to load recommendations:', e);
            this.error = e.message;
        } finally {
            this.loading = false;
            this.render();
        }
    }

    async refreshRecommendations() {
        this.loading = true;
        this.render();

        try {
            const resp = await fetch('/api/cost/recommendations', { method: 'POST' });
            if (!resp.ok) {
                throw new Error(`HTTP ${resp.status}`);
            }
            await this.loadData();
        } catch (e) {
            console.error('Failed to refresh recommendations:', e);
            this.error = e.message;
            this.loading = false;
            this.render();
        }
    }

    getFilteredRecommendations() {
        let recs = this.report?.recommendations || [];

        if (this.filterType) {
            recs = recs.filter(r => r.type === this.filterType);
        }

        if (this.filterPriority) {
            recs = recs.filter(r => r.priority === this.filterPriority);
        }

        return recs;
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-recommendations">
                    <div class="rec-header">
                        <div class="header-title">
                            <span class="title-icon">&#128161;</span>
                            <span>Cost Optimization Recommendations</span>
                        </div>
                    </div>
                    <div class="loading">Analyzing your data for cost optimizations...</div>
                </div>
            `;
            return;
        }

        if (this.error) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="cost-recommendations">
                    <div class="rec-header">
                        <div class="header-title">
                            <span class="title-icon">&#128161;</span>
                            <span>Cost Optimization Recommendations</span>
                        </div>
                        <button class="btn-refresh" id="btn-refresh">Retry</button>
                    </div>
                    <div class="error">Failed to load recommendations: ${this.escapeHtml(this.error)}</div>
                </div>
            `;
            this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());
            return;
        }

        const totalSavings = this.report?.total_monthly_savings || 0;
        const totalAnnual = this.report?.total_annual_savings || 0;
        const totalCount = this.report?.total_recommendations || 0;
        const byPriority = this.report?.by_priority || {};
        const byType = this.report?.by_type || {};
        const filteredRecs = this.getFilteredRecommendations();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="cost-recommendations">
                <div class="rec-header">
                    <div class="header-title">
                        <span class="title-icon">&#128161;</span>
                        <span>Cost Optimization Recommendations</span>
                    </div>
                    <button class="btn-refresh" id="btn-refresh">&#8635; Refresh Analysis</button>
                </div>

                <div class="savings-summary">
                    <div class="summary-card main">
                        <div class="summary-label">Potential Monthly Savings</div>
                        <div class="summary-value">${this.formatCurrency(totalSavings)}</div>
                        <div class="summary-detail">${this.formatCurrency(totalAnnual)}/year</div>
                    </div>
                    <div class="summary-card">
                        <div class="summary-label">Total Recommendations</div>
                        <div class="summary-value">${totalCount}</div>
                    </div>
                    <div class="summary-card critical">
                        <div class="summary-label">Critical</div>
                        <div class="summary-value">${byPriority.critical || 0}</div>
                    </div>
                    <div class="summary-card high">
                        <div class="summary-label">High Priority</div>
                        <div class="summary-value">${byPriority.high || 0}</div>
                    </div>
                    <div class="summary-card medium">
                        <div class="summary-label">Medium</div>
                        <div class="summary-value">${byPriority.medium || 0}</div>
                    </div>
                </div>

                <div class="filters">
                    <div class="filter-group">
                        <label>Priority:</label>
                        <select id="filter-priority">
                            <option value="">All</option>
                            <option value="critical" ${this.filterPriority === 'critical' ? 'selected' : ''}>Critical</option>
                            <option value="high" ${this.filterPriority === 'high' ? 'selected' : ''}>High</option>
                            <option value="medium" ${this.filterPriority === 'medium' ? 'selected' : ''}>Medium</option>
                            <option value="low" ${this.filterPriority === 'low' ? 'selected' : ''}>Low</option>
                        </select>
                    </div>
                    <div class="filter-group">
                        <label>Type:</label>
                        <select id="filter-type">
                            <option value="">All</option>
                            ${Object.keys(byType).map(t => `
                                <option value="${t}" ${this.filterType === t ? 'selected' : ''}>${this.formatType(t)}</option>
                            `).join('')}
                        </select>
                    </div>
                    <div class="filter-count">
                        Showing ${filteredRecs.length} of ${totalCount} recommendations
                    </div>
                </div>

                <div class="rec-list">
                    ${filteredRecs.length === 0 ? `
                        <div class="empty-state">
                            <p>No recommendations match your filters</p>
                            <p class="empty-hint">Try adjusting the filters or refreshing the analysis</p>
                        </div>
                    ` : filteredRecs.map(rec => this.renderRecommendation(rec)).join('')}
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderRecommendation(rec) {
        const impact = rec.impact || {};
        const savings = impact.monthly_savings || 0;
        const reduction = impact.reduction_percent || 0;

        return `
            <div class="rec-item priority-${rec.priority || 'medium'}">
                <div class="rec-main">
                    <div class="rec-icon">${this.getTypeIcon(rec.type)}</div>
                    <div class="rec-content">
                        <div class="rec-title">${this.escapeHtml(rec.title || rec.description)}</div>
                        <div class="rec-desc">${this.escapeHtml(rec.description)}</div>
                        <div class="rec-meta">
                            <span class="rec-type">${this.formatType(rec.type)}</span>
                            <span class="rec-data-type">${this.escapeHtml(rec.data_type || 'General')}</span>
                            ${rec.target ? `<span class="rec-target">${this.escapeHtml(rec.target)}</span>` : ''}
                        </div>
                    </div>
                </div>
                <div class="rec-impact">
                    <div class="impact-savings">
                        <span class="savings-amount">${this.formatCurrency(savings)}</span>
                        <span class="savings-period">/month</span>
                    </div>
                    ${reduction > 0 ? `
                        <div class="impact-reduction">
                            ${reduction.toFixed(0)}% reduction
                        </div>
                    ` : ''}
                    <div class="rec-actions">
                        <button class="btn-apply" data-id="${rec.id}" title="Apply this recommendation">Apply</button>
                        <button class="btn-dismiss" data-id="${rec.id}" title="Dismiss this recommendation">Dismiss</button>
                    </div>
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.refreshRecommendations());

        this.querySelector('#filter-priority')?.addEventListener('change', (e) => {
            this.filterPriority = e.target.value;
            this.render();
        });

        this.querySelector('#filter-type')?.addEventListener('change', (e) => {
            this.filterType = e.target.value;
            this.render();
        });

        this.querySelectorAll('.btn-apply').forEach(btn => {
            btn.addEventListener('click', () => this.applyRecommendation(btn.dataset.id));
        });

        this.querySelectorAll('.btn-dismiss').forEach(btn => {
            btn.addEventListener('click', () => this.dismissRecommendation(btn.dataset.id));
        });
    }

    async applyRecommendation(id) {
        try {
            const resp = await fetch(`/api/cost/recommendations/${id}/apply`, { method: 'POST' });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

            const result = await resp.json();
            this.showToast(`Recommendation applied. ${result.message || ''}`, 'success');
        } catch (e) {
            this.showToast('Failed to apply recommendation: ' + e.message, 'error');
        }
    }

    async dismissRecommendation(id) {
        try {
            const resp = await fetch(`/api/cost/recommendations/${id}/dismiss`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reason: 'User dismissed' })
            });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

            // Remove from local list
            if (this.report?.recommendations) {
                this.report.recommendations = this.report.recommendations.filter(r => r.id !== id);
            }
            this.render();
            this.showToast('Recommendation dismissed', 'success');
        } catch (e) {
            this.showToast('Failed to dismiss: ' + e.message, 'error');
        }
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    getTypeIcon(type) {
        const icons = {
            'drop_unused': '&#128465;',
            'sample_high_volume': '&#127919;',
            'drop_debug_logs': '&#128220;',
            'aggregate_metrics': '&#128202;',
            'drop_high_cardinality_tags': '&#127991;',
            'reduce_retention': '&#128197;',
            'set_quota': '&#128200;',
            'optimize_query': '&#9889;'
        };
        return icons[type] || '&#128161;';
    }

    formatType(type) {
        const labels = {
            'drop_unused': 'Drop Unused',
            'sample_high_volume': 'Sample High Volume',
            'drop_debug_logs': 'Drop Debug Logs',
            'aggregate_metrics': 'Aggregate Metrics',
            'drop_high_cardinality_tags': 'Reduce Cardinality',
            'reduce_retention': 'Reduce Retention',
            'set_quota': 'Set Quota',
            'optimize_query': 'Optimize Query'
        };
        return labels[type] || type?.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()) || 'Unknown';
    }

    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(1)}k`;
        return `$${Math.round(amount).toLocaleString()}`;
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .cost-recommendations {
                background: var(--bg-card, #16181c);
                min-height: 100%;
            }

            .rec-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 600;
            }

            .title-icon { font-size: 1.25rem; }

            .btn-refresh {
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                padding: 0.4rem 0.75rem;
                cursor: pointer;
                font-size: 0.85rem;
            }

            .btn-refresh:hover {
                background: var(--bg-elevated, #1e2128);
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .error { color: var(--error, #f4212e); }

            .savings-summary {
                display: grid;
                grid-template-columns: 2fr 1fr 1fr 1fr 1fr;
                gap: 1rem;
                padding: 1.5rem;
            }

            .summary-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                text-align: center;
            }

            .summary-card.main {
                background: linear-gradient(135deg, #00ba7c 0%, #00875a 100%);
                color: white;
            }

            .summary-card.critical { border-left: 3px solid var(--error, #f4212e); }
            .summary-card.high { border-left: 3px solid #f97316; }
            .summary-card.medium { border-left: 3px solid var(--warning, #ffd400); }

            .summary-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .summary-card.main .summary-label {
                color: rgba(255, 255, 255, 0.85);
            }

            .summary-value {
                font-size: 1.5rem;
                font-weight: 700;
            }

            .summary-detail {
                font-size: 0.8rem;
                opacity: 0.85;
                margin-top: 0.25rem;
            }

            .filters {
                display: flex;
                align-items: center;
                gap: 1.5rem;
                padding: 1rem 1.5rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .filter-group {
                display: flex;
                align-items: center;
                gap: 0.5rem;
            }

            .filter-group label {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .filter-group select {
                padding: 0.4rem 0.75rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }

            .filter-count {
                margin-left: auto;
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .rec-list {
                padding: 1rem 1.5rem;
            }

            .empty-state {
                text-align: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            .empty-hint {
                font-size: 0.85rem;
                margin-top: 0.5rem;
            }

            .rec-item {
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 8px;
                padding: 1rem;
                margin-bottom: 0.75rem;
                border-left: 4px solid var(--border, #2f3336);
            }

            .rec-item.priority-critical { border-left-color: var(--error, #f4212e); }
            .rec-item.priority-high { border-left-color: #f97316; }
            .rec-item.priority-medium { border-left-color: var(--warning, #ffd400); }
            .rec-item.priority-low { border-left-color: var(--success, #00ba7c); }

            .rec-main {
                display: flex;
                gap: 1rem;
                flex: 1;
            }

            .rec-icon {
                font-size: 1.5rem;
                flex-shrink: 0;
            }

            .rec-content {
                flex: 1;
            }

            .rec-title {
                font-weight: 600;
                font-size: 0.95rem;
                margin-bottom: 0.25rem;
            }

            .rec-desc {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .rec-meta {
                display: flex;
                gap: 0.75rem;
                flex-wrap: wrap;
            }

            .rec-type, .rec-data-type, .rec-target {
                font-size: 0.75rem;
                padding: 0.2rem 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                color: var(--text-muted, #71767b);
            }

            .rec-impact {
                text-align: right;
                flex-shrink: 0;
                min-width: 150px;
            }

            .impact-savings {
                margin-bottom: 0.25rem;
            }

            .savings-amount {
                font-size: 1.25rem;
                font-weight: 700;
                color: var(--success, #00ba7c);
            }

            .savings-period {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .impact-reduction {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }

            .rec-actions {
                display: flex;
                gap: 0.5rem;
                justify-content: flex-end;
            }

            .btn-apply, .btn-dismiss {
                padding: 0.35rem 0.75rem;
                border-radius: 4px;
                font-size: 0.8rem;
                cursor: pointer;
                border: 1px solid var(--border, #2f3336);
            }

            .btn-apply {
                background: var(--accent, #1d9bf0);
                color: white;
                border-color: var(--accent, #1d9bf0);
            }

            .btn-apply:hover {
                background: #1a8cd8;
            }

            .btn-dismiss {
                background: transparent;
                color: var(--text-muted, #71767b);
            }

            .btn-dismiss:hover {
                background: var(--bg-card, #16181c);
            }

            @media (max-width: 1000px) {
                .savings-summary {
                    grid-template-columns: repeat(3, 1fr);
                }
                .summary-card.main {
                    grid-column: span 3;
                }
            }

            @media (max-width: 700px) {
                .rec-item {
                    flex-direction: column;
                    gap: 1rem;
                }
                .rec-impact {
                    text-align: left;
                    width: 100%;
                    display: flex;
                    align-items: center;
                    justify-content: space-between;
                }
                .filters {
                    flex-wrap: wrap;
                }
            }
        `;
    }
}

customElements.define('cost-recommendations', CostRecommendations);
