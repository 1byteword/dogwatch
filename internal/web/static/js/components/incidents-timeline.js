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
