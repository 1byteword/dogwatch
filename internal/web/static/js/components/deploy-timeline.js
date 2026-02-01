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
