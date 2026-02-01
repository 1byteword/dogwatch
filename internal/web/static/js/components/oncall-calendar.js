/**
 * On-Call Calendar Widget
 * Who's on-call now, schedule view, shift swaps
 */
class OncallCalendar extends HTMLElement {
    constructor() {
        super();
        this.schedules = [];
        this.currentOnCall = [];
        this.view = 'current'; // current, week, month
    }

    connectedCallback() {
        this.render();
        this.loadData();
    }

    async loadData() {
        try {
            const [schedulesResp, currentResp] = await Promise.all([
                fetch('/api/oncall/schedules'),
                fetch('/api/oncall/current')
            ]);

            if (schedulesResp.ok) this.schedules = await schedulesResp.json() || [];
            if (currentResp.ok) this.currentOnCall = await currentResp.json() || [];

            this.renderContent();
        } catch (e) {
            console.error('Failed to load on-call data:', e);
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
            <div class="oncall-calendar">
                <div class="oncall-header">
                    <div class="header-title">
                        <span class="title-icon">📞</span>
                        <span>On-Call</span>
                    </div>
                    <div class="header-views">
                        <button class="view-btn active" data-view="current" onclick="this.getRootNode().host.setView('current')">Now</button>
                        <button class="view-btn" data-view="week" onclick="this.getRootNode().host.setView('week')">Week</button>
                        <button class="view-btn" data-view="month" onclick="this.getRootNode().host.setView('month')">Month</button>
                    </div>
                </div>
                <div class="oncall-content" id="oncall-content">
                    <div class="loading">Loading...</div>
                </div>
            </div>
        `;
    }

    renderContent() {
        const container = this.querySelector('#oncall-content');
        if (!container) return;

        switch (this.view) {
            case 'current':
                container.innerHTML = this.renderCurrentOnCall();
                break;
            case 'week':
                container.innerHTML = this.renderWeekView();
                break;
            case 'month':
                container.innerHTML = this.renderMonthView();
                break;
        }
    }

    renderCurrentOnCall() {
        if (this.currentOnCall.length === 0 && this.schedules.length === 0) {
            return `
                <div class="empty-state">
                    <span class="icon">📞</span>
                    <p>No on-call schedules configured</p>
                </div>
            `;
        }

        // Group by schedule/team
        const bySchedule = new Map();
        this.currentOnCall.forEach(oc => {
            const key = oc.schedule_name || oc.team || 'Default';
            if (!bySchedule.has(key)) bySchedule.set(key, []);
            bySchedule.get(key).push(oc);
        });

        return `
            <div class="current-oncall">
                ${Array.from(bySchedule.entries()).map(([name, people]) => `
                    <div class="schedule-card">
                        <div class="schedule-name">${this.escapeHtml(name)}</div>
                        <div class="oncall-people">
                            ${people.map((p, i) => `
                                <div class="person-card ${i === 0 ? 'primary' : 'backup'}">
                                    <div class="person-avatar">${this.getInitials(p.user_name || p.name)}</div>
                                    <div class="person-info">
                                        <div class="person-name">${this.escapeHtml(p.user_name || p.name || 'Unknown')}</div>
                                        <div class="person-role">${i === 0 ? 'Primary' : 'Backup'}</div>
                                        <div class="shift-time">Until ${this.formatTime(p.end_time)}</div>
                                    </div>
                                    <div class="person-actions">
                                        <a href="tel:${p.phone || ''}" class="btn-contact" title="Call">📞</a>
                                        <a href="mailto:${p.email || ''}" class="btn-contact" title="Email">✉️</a>
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                `).join('')}
                ${this.schedules.filter(s => !Array.from(bySchedule.keys()).includes(s.name)).map(s => `
                    <div class="schedule-card empty">
                        <div class="schedule-name">${this.escapeHtml(s.name)}</div>
                        <div class="no-oncall">No one currently on-call</div>
                    </div>
                `).join('')}
            </div>
        `;
    }

    renderWeekView() {
        const days = this.getWeekDays();

        return `
            <div class="week-view">
                <div class="week-header">
                    ${days.map(d => `
                        <div class="day-header ${d.isToday ? 'today' : ''}">
                            <span class="day-name">${d.name}</span>
                            <span class="day-date">${d.date}</span>
                        </div>
                    `).join('')}
                </div>
                <div class="week-body">
                    ${this.schedules.slice(0, 3).map(schedule => `
                        <div class="schedule-row">
                            <div class="schedule-label">${this.escapeHtml(schedule.name)}</div>
                            <div class="schedule-shifts">
                                ${days.map(d => `
                                    <div class="shift-cell ${d.isToday ? 'today' : ''}">
                                        ${this.getShiftForDay(schedule, d.fullDate) || '<span class="no-shift">—</span>'}
                                    </div>
                                `).join('')}
                            </div>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderMonthView() {
        const today = new Date();
        const year = today.getFullYear();
        const month = today.getMonth();
        const firstDay = new Date(year, month, 1);
        const lastDay = new Date(year, month + 1, 0);
        const startPadding = firstDay.getDay();

        const days = [];
        for (let i = 0; i < startPadding; i++) {
            days.push({ empty: true });
        }
        for (let d = 1; d <= lastDay.getDate(); d++) {
            days.push({
                date: d,
                isToday: d === today.getDate(),
                fullDate: new Date(year, month, d)
            });
        }

        return `
            <div class="month-view">
                <div class="month-header">
                    <span class="month-name">${today.toLocaleDateString('en-US', { month: 'long', year: 'numeric' })}</span>
                </div>
                <div class="month-grid">
                    <div class="weekday-row">
                        ${['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map(d => `<div class="weekday">${d}</div>`).join('')}
                    </div>
                    <div class="days-grid">
                        ${days.map(d => d.empty ? '<div class="day-cell empty"></div>' : `
                            <div class="day-cell ${d.isToday ? 'today' : ''}">
                                <span class="day-num">${d.date}</span>
                                ${this.getShiftIndicator(d.fullDate)}
                            </div>
                        `).join('')}
                    </div>
                </div>
            </div>
        `;
    }

    getWeekDays() {
        const days = [];
        const today = new Date();
        const dayOfWeek = today.getDay();
        const startOfWeek = new Date(today);
        startOfWeek.setDate(today.getDate() - dayOfWeek);

        for (let i = 0; i < 7; i++) {
            const d = new Date(startOfWeek);
            d.setDate(startOfWeek.getDate() + i);
            days.push({
                name: d.toLocaleDateString('en-US', { weekday: 'short' }),
                date: d.getDate(),
                isToday: d.toDateString() === today.toDateString(),
                fullDate: d
            });
        }
        return days;
    }

    getShiftForDay(schedule, date) {
        // In a real implementation, this would look up actual shifts
        const person = this.currentOnCall.find(oc =>
            (oc.schedule_name === schedule.name || oc.schedule_id === schedule.id)
        );

        if (person) {
            return `<span class="shift-person">${this.getInitials(person.user_name || person.name)}</span>`;
        }
        return '';
    }

    getShiftIndicator(date) {
        const hasShift = this.currentOnCall.length > 0;
        return hasShift ? '<div class="shift-indicator"></div>' : '';
    }

    getInitials(name) {
        if (!name) return '?';
        return name.split(' ').map(n => n[0]).join('').toUpperCase().substring(0, 2);
    }

    formatTime(timestamp) {
        if (!timestamp) return 'N/A';
        const d = new Date(timestamp);
        return d.toLocaleString('en-US', {
            weekday: 'short',
            hour: 'numeric',
            minute: '2-digit'
        });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            .oncall-calendar {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .oncall-header {
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

            .header-views { display: flex; gap: 0.25rem; }

            .view-btn {
                background: transparent;
                border: none;
                color: var(--text-muted, #71767b);
                padding: 0.4rem 0.6rem;
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.8rem;
            }

            .view-btn:hover { background: var(--bg-card, #16181c); }
            .view-btn.active { background: var(--bg-card, #16181c); color: var(--text, #e7e9ea); }

            .oncall-content {
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

            .empty-state .icon { font-size: 2rem; margin-bottom: 0.5rem; }

            /* Current On-Call */
            .current-oncall { display: flex; flex-direction: column; gap: 1rem; }

            .schedule-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .schedule-card.empty { opacity: 0.6; }

            .schedule-name {
                font-weight: 600;
                font-size: 0.85rem;
                margin-bottom: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .oncall-people { display: flex; flex-direction: column; gap: 0.5rem; }

            .person-card {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                border-left: 3px solid var(--accent, #1d9bf0);
            }

            .person-card.backup {
                border-left-color: var(--text-muted, #71767b);
                opacity: 0.8;
            }

            .person-avatar {
                width: 40px;
                height: 40px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
                font-size: 0.9rem;
            }

            .person-info { flex: 1; }

            .person-name { font-weight: 500; }

            .person-role {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .shift-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .person-actions { display: flex; gap: 0.5rem; }

            .btn-contact {
                width: 32px;
                height: 32px;
                display: flex;
                align-items: center;
                justify-content: center;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                text-decoration: none;
                font-size: 0.9rem;
            }

            .no-oncall {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            /* Week View */
            .week-view { display: flex; flex-direction: column; }

            .week-header {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
                margin-bottom: 0.5rem;
            }

            .day-header {
                text-align: center;
                padding: 0.5rem;
            }

            .day-header.today {
                background: var(--accent, #1d9bf0);
                border-radius: 6px;
                color: white;
            }

            .day-name { display: block; font-size: 0.7rem; color: var(--text-muted, #71767b); }
            .day-header.today .day-name { color: rgba(255,255,255,0.8); }

            .day-date { font-weight: 600; }

            .schedule-row {
                display: flex;
                align-items: center;
                margin-bottom: 0.5rem;
            }

            .schedule-label {
                width: 80px;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .schedule-shifts {
                flex: 1;
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
            }

            .shift-cell {
                background: var(--bg-elevated, #1e2128);
                padding: 0.5rem;
                border-radius: 4px;
                text-align: center;
                min-height: 40px;
                display: flex;
                align-items: center;
                justify-content: center;
            }

            .shift-cell.today { border: 1px solid var(--accent, #1d9bf0); }

            .shift-person {
                width: 28px;
                height: 28px;
                background: var(--accent, #1d9bf0);
                color: white;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.7rem;
                font-weight: 600;
            }

            .no-shift { color: var(--text-muted, #71767b); }

            /* Month View */
            .month-view { }

            .month-header {
                text-align: center;
                margin-bottom: 1rem;
            }

            .month-name { font-weight: 600; font-size: 1.1rem; }

            .weekday-row {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
                margin-bottom: 0.25rem;
            }

            .weekday {
                text-align: center;
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                padding: 0.25rem;
            }

            .days-grid {
                display: grid;
                grid-template-columns: repeat(7, 1fr);
                gap: 0.25rem;
            }

            .day-cell {
                aspect-ratio: 1;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                padding: 0.25rem;
                position: relative;
            }

            .day-cell.empty { background: transparent; }

            .day-cell.today {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .day-num { font-size: 0.75rem; }

            .shift-indicator {
                position: absolute;
                bottom: 4px;
                left: 50%;
                transform: translateX(-50%);
                width: 6px;
                height: 6px;
                background: var(--success, #00ba7c);
                border-radius: 50%;
            }
        `;
    }
}

customElements.define('oncall-calendar', OncallCalendar);
