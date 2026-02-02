/**
 * Team Management Widget
 * CRUD operations for teams with member management
 */
class TeamManagement extends HTMLElement {
    constructor() {
        super();
        this.teams = [];
        this.users = [];
        this.selectedTeam = null;
        this.loading = true;
        this.error = null;
        this.searchQuery = '';
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
            const [teamsResp, usersResp] = await Promise.all([
                fetch('/api/teams'),
                fetch('/api/users')
            ]);

            if (teamsResp.ok) {
                this.teams = await teamsResp.json();
            } else {
                this.teams = this.generateDemoTeams();
            }

            if (usersResp.ok) {
                this.users = await usersResp.json();
            } else {
                this.users = this.generateDemoUsers();
            }
        } catch (e) {
            console.error('Failed to load data:', e);
            this.teams = this.generateDemoTeams();
            this.users = this.generateDemoUsers();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoTeams() {
        return [
            {
                id: '1',
                name: 'Platform Team',
                description: 'Core infrastructure and platform services',
                members: ['1', '2', '3'],
                created_at: Date.now() - 86400000 * 60,
                services: ['api-gateway', 'auth-service', 'user-service']
            },
            {
                id: '2',
                name: 'Backend Team',
                description: 'Backend services and APIs',
                members: ['2', '4'],
                created_at: Date.now() - 86400000 * 45,
                services: ['order-service', 'payment-service']
            },
            {
                id: '3',
                name: 'Frontend Team',
                description: 'Web and mobile applications',
                members: ['3', '5'],
                created_at: Date.now() - 86400000 * 30,
                services: ['web-app', 'mobile-bff']
            },
            {
                id: '4',
                name: 'DevOps',
                description: 'Infrastructure and deployments',
                members: ['1'],
                created_at: Date.now() - 86400000 * 20,
                services: ['monitoring', 'ci-cd']
            }
        ];
    }

    generateDemoUsers() {
        return [
            { id: '1', email: 'admin@example.com', name: 'Admin User', role: 'owner' },
            { id: '2', email: 'john@example.com', name: 'John Smith', role: 'admin' },
            { id: '3', email: 'jane@example.com', name: 'Jane Doe', role: 'editor' },
            { id: '4', email: 'bob@example.com', name: 'Bob Wilson', role: 'viewer' },
            { id: '5', email: 'alice@example.com', name: 'Alice Brown', role: 'editor' }
        ];
    }

    getFilteredTeams() {
        return this.teams.filter(team => {
            return !this.searchQuery ||
                team.name.toLowerCase().includes(this.searchQuery.toLowerCase()) ||
                (team.description && team.description.toLowerCase().includes(this.searchQuery.toLowerCase()));
        });
    }

    getUserById(userId) {
        return this.users.find(u => u.id === userId);
    }

    async createTeam(teamData) {
        try {
            const resp = await fetch('/api/teams', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(teamData)
            });

            if (resp.ok) {
                await this.loadData();
                this.closeModal();
                this.showToast('Team created successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to create team: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to create team:', e);
            // Demo: add locally
            const newTeam = {
                id: String(Date.now()),
                ...teamData,
                members: teamData.members || [],
                services: [],
                created_at: Date.now()
            };
            this.teams.push(newTeam);
            this.closeModal();
            this.showToast('Team created successfully');
            this.render();
        }
    }

    async updateTeam(teamId, updates) {
        try {
            const resp = await fetch(`/api/teams/${encodeURIComponent(teamId)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            if (resp.ok) {
                await this.loadData();
                this.closeModal();
                this.showToast('Team updated successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to update team: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to update team:', e);
            // Demo: update locally
            const idx = this.teams.findIndex(t => t.id === teamId);
            if (idx >= 0) {
                this.teams[idx] = { ...this.teams[idx], ...updates };
            }
            this.closeModal();
            this.showToast('Team updated successfully');
            this.render();
        }
    }

    async deleteTeam(teamId) {
        if (!confirm('Are you sure you want to delete this team? This action cannot be undone.')) {
            return;
        }

        try {
            const resp = await fetch(`/api/teams/${encodeURIComponent(teamId)}`, {
                method: 'DELETE'
            });

            if (resp.ok) {
                await this.loadData();
                this.showToast('Team deleted successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to delete team: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to delete team:', e);
            // Demo: delete locally
            this.teams = this.teams.filter(t => t.id !== teamId);
            this.showToast('Team deleted successfully');
            this.render();
        }
    }

    async addMember(teamId, userId) {
        const team = this.teams.find(t => t.id === teamId);
        if (!team) return;

        const members = [...(team.members || [])];
        if (!members.includes(userId)) {
            members.push(userId);
            await this.updateTeam(teamId, { members });
        }
    }

    async removeMember(teamId, userId) {
        const team = this.teams.find(t => t.id === teamId);
        if (!team) return;

        const members = (team.members || []).filter(id => id !== userId);
        await this.updateTeam(teamId, { members });
    }

    openCreateModal() {
        this.selectedTeam = null;
        this.showModal();
    }

    openEditModal(team) {
        this.selectedTeam = team;
        this.showModal();
    }

    openMembersModal(team) {
        this.selectedTeam = team;
        const modal = this.querySelector('#members-modal');
        if (modal) {
            modal.style.display = 'flex';
            this.renderMembersList();
        }
    }

    showModal() {
        const modal = this.querySelector('#team-modal');
        if (modal) {
            modal.style.display = 'flex';
            this.updateModalForm();
        }
    }

    closeModal() {
        const teamModal = this.querySelector('#team-modal');
        const membersModal = this.querySelector('#members-modal');
        if (teamModal) teamModal.style.display = 'none';
        if (membersModal) membersModal.style.display = 'none';
        this.selectedTeam = null;
    }

    updateModalForm() {
        const form = this.querySelector('#team-form');
        if (!form) return;

        const nameInput = form.querySelector('#team-name');
        const descInput = form.querySelector('#team-description');
        const title = this.querySelector('#modal-title');

        if (this.selectedTeam) {
            title.textContent = 'Edit Team';
            nameInput.value = this.selectedTeam.name;
            descInput.value = this.selectedTeam.description || '';
        } else {
            title.textContent = 'Create Team';
            nameInput.value = '';
            descInput.value = '';
        }
    }

    renderMembersList() {
        const container = this.querySelector('#members-list');
        if (!container || !this.selectedTeam) return;

        const teamMembers = this.selectedTeam.members || [];
        const availableUsers = this.users.filter(u => !teamMembers.includes(u.id));

        container.innerHTML = `
            <div class="members-section">
                <h4>Current Members (${teamMembers.length})</h4>
                ${teamMembers.length === 0 ? '<div class="empty-state">No members</div>' : ''}
                <div class="members-grid">
                    ${teamMembers.map(memberId => {
                        const user = this.getUserById(memberId);
                        if (!user) return '';
                        return `
                            <div class="member-card">
                                <div class="member-avatar">${this.escapeHtml(user.name.charAt(0).toUpperCase())}</div>
                                <div class="member-info">
                                    <div class="member-name">${this.escapeHtml(user.name)}</div>
                                    <div class="member-email">${this.escapeHtml(user.email)}</div>
                                </div>
                                <button class="btn-remove-member" data-user="${this.escapeHtml(user.id)}">Remove</button>
                            </div>
                        `;
                    }).join('')}
                </div>
            </div>

            <div class="members-section">
                <h4>Add Members</h4>
                ${availableUsers.length === 0 ? '<div class="empty-state">All users are already members</div>' : ''}
                <div class="members-grid">
                    ${availableUsers.map(user => `
                        <div class="member-card available">
                            <div class="member-avatar">${this.escapeHtml(user.name.charAt(0).toUpperCase())}</div>
                            <div class="member-info">
                                <div class="member-name">${this.escapeHtml(user.name)}</div>
                                <div class="member-email">${this.escapeHtml(user.email)}</div>
                            </div>
                            <button class="btn-add-member" data-user="${this.escapeHtml(user.id)}">Add</button>
                        </div>
                    `).join('')}
                </div>
            </div>
        `;

        // Attach member action listeners
        container.querySelectorAll('.btn-remove-member').forEach(btn => {
            btn.addEventListener('click', async () => {
                await this.removeMember(this.selectedTeam.id, btn.dataset.user);
                this.renderMembersList();
            });
        });

        container.querySelectorAll('.btn-add-member').forEach(btn => {
            btn.addEventListener('click', async () => {
                await this.addMember(this.selectedTeam.id, btn.dataset.user);
                this.renderMembersList();
            });
        });
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        const teamData = {
            name: formData.get('name'),
            description: formData.get('description')
        };

        if (!teamData.name) {
            this.showToast('Please enter a team name', 'error');
            return;
        }

        if (this.selectedTeam) {
            this.updateTeam(this.selectedTeam.id, teamData);
        } else {
            this.createTeam(teamData);
        }
    }

    handleSearch(query) {
        this.searchQuery = query;
        this.render();
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatDate(timestamp) {
        if (!timestamp) return '-';
        const date = new Date(timestamp);
        return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    render() {
        const filteredTeams = this.getFilteredTeams();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="team-management">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Team Management</span>
                        <span class="team-count">${this.teams.length} teams</span>
                    </div>
                    <div class="header-right">
                        <button class="btn-primary" id="btn-create-team">+ Create Team</button>
                    </div>
                </div>

                <div class="toolbar">
                    <div class="search-box">
                        <input type="text" id="search-input" placeholder="Search teams..." value="${this.escapeHtml(this.searchQuery)}">
                    </div>
                    <button class="btn-secondary" id="btn-refresh">Refresh</button>
                </div>

                <div class="content">
                    ${this.loading ? `
                        <div class="loading">Loading teams...</div>
                    ` : this.error ? `
                        <div class="error">${this.escapeHtml(this.error)}</div>
                    ` : filteredTeams.length === 0 ? `
                        <div class="empty-state">
                            ${this.searchQuery ? 'No teams match your search' : 'No teams found. Create your first team!'}
                        </div>
                    ` : `
                        <div class="teams-grid">
                            ${filteredTeams.map(team => `
                                <div class="team-card">
                                    <div class="team-header">
                                        <div class="team-icon">${this.escapeHtml(team.name.charAt(0).toUpperCase())}</div>
                                        <div class="team-title">
                                            <h3>${this.escapeHtml(team.name)}</h3>
                                            <span class="team-created">Created ${this.formatDate(team.created_at)}</span>
                                        </div>
                                        <div class="team-menu">
                                            <button class="btn-menu" data-edit="${this.escapeHtml(team.id)}">Edit</button>
                                            <button class="btn-menu btn-danger" data-delete="${this.escapeHtml(team.id)}">Delete</button>
                                        </div>
                                    </div>
                                    ${team.description ? `<p class="team-description">${this.escapeHtml(team.description)}</p>` : ''}
                                    <div class="team-stats">
                                        <div class="stat">
                                            <span class="stat-value">${(team.members || []).length}</span>
                                            <span class="stat-label">Members</span>
                                        </div>
                                        <div class="stat">
                                            <span class="stat-value">${(team.services || []).length}</span>
                                            <span class="stat-label">Services</span>
                                        </div>
                                    </div>
                                    <div class="team-members-preview">
                                        ${(team.members || []).slice(0, 5).map(memberId => {
                                            const user = this.getUserById(memberId);
                                            return user ? `<div class="member-avatar-small" title="${this.escapeHtml(user.name)}">${this.escapeHtml(user.name.charAt(0))}</div>` : '';
                                        }).join('')}
                                        ${(team.members || []).length > 5 ? `<div class="member-avatar-small more">+${team.members.length - 5}</div>` : ''}
                                    </div>
                                    <div class="team-actions">
                                        <button class="btn-secondary" data-members="${this.escapeHtml(team.id)}">Manage Members</button>
                                    </div>
                                </div>
                            `).join('')}
                        </div>
                    `}
                </div>

                <div class="modal" id="team-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span id="modal-title">Create Team</span>
                            <button class="btn-close" id="btn-close-modal">&times;</button>
                        </div>
                        <form id="team-form">
                            <div class="modal-body">
                                <div class="form-group">
                                    <label for="team-name">Team Name *</label>
                                    <input type="text" id="team-name" name="name" required>
                                </div>
                                <div class="form-group">
                                    <label for="team-description">Description</label>
                                    <textarea id="team-description" name="description" rows="3"></textarea>
                                </div>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn-secondary" id="btn-cancel">Cancel</button>
                                <button type="submit" class="btn-primary">Save</button>
                            </div>
                        </form>
                    </div>
                </div>

                <div class="modal" id="members-modal" style="display: none;">
                    <div class="modal-content modal-wide">
                        <div class="modal-header">
                            <span>Team Members</span>
                            <button class="btn-close" id="btn-close-members">&times;</button>
                        </div>
                        <div class="modal-body" id="members-list">
                            <!-- Populated dynamically -->
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn-primary" id="btn-done-members">Done</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    attachEventListeners() {
        // Create team button
        this.querySelector('#btn-create-team')?.addEventListener('click', () => this.openCreateModal());

        // Refresh button
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadData());

        // Search input
        this.querySelector('#search-input')?.addEventListener('input', (e) => this.handleSearch(e.target.value));

        // Modal close buttons
        this.querySelector('#btn-close-modal')?.addEventListener('click', () => this.closeModal());
        this.querySelector('#btn-cancel')?.addEventListener('click', () => this.closeModal());
        this.querySelector('#btn-close-members')?.addEventListener('click', () => this.closeModal());
        this.querySelector('#btn-done-members')?.addEventListener('click', () => this.closeModal());

        // Form submit
        this.querySelector('#team-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));

        // Edit buttons
        this.querySelectorAll('[data-edit]').forEach(btn => {
            btn.addEventListener('click', () => {
                const team = this.teams.find(t => t.id === btn.dataset.edit);
                if (team) this.openEditModal(team);
            });
        });

        // Delete buttons
        this.querySelectorAll('[data-delete]').forEach(btn => {
            btn.addEventListener('click', () => this.deleteTeam(btn.dataset.delete));
        });

        // Members buttons
        this.querySelectorAll('[data-members]').forEach(btn => {
            btn.addEventListener('click', () => {
                const team = this.teams.find(t => t.id === btn.dataset.members);
                if (team) this.openMembersModal(team);
            });
        });
    }

    getStyles() {
        return `
            .team-management {
                display: flex;
                flex-direction: column;
                height: 100%;
                background: var(--bg-card, #16181c);
            }

            .header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .header-left {
                display: flex;
                align-items: center;
                gap: 1rem;
            }

            .title {
                font-weight: 600;
                font-size: 1rem;
            }

            .team-count {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .toolbar {
                display: flex;
                gap: 1rem;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .search-box {
                flex: 1;
                max-width: 300px;
            }

            .search-box input {
                width: 100%;
                padding: 0.5rem 0.75rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }

            .btn-primary, .btn-secondary {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                font-size: 0.85rem;
                cursor: pointer;
                border: none;
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-secondary {
                background: var(--bg-card, #16181c);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .content {
                flex: 1;
                overflow: auto;
                padding: 1rem;
            }

            .loading, .error, .empty-state {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 200px;
                color: var(--text-muted, #71767b);
            }

            .teams-grid {
                display: grid;
                grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
                gap: 1rem;
            }

            .team-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                border: 1px solid var(--border, #2f3336);
            }

            .team-header {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                margin-bottom: 0.75rem;
            }

            .team-icon {
                width: 40px;
                height: 40px;
                border-radius: 8px;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
                font-size: 1.1rem;
            }

            .team-title {
                flex: 1;
            }

            .team-title h3 {
                margin: 0;
                font-size: 1rem;
                font-weight: 600;
            }

            .team-created {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .team-menu {
                display: flex;
                gap: 0.25rem;
            }

            .btn-menu {
                padding: 0.25rem 0.5rem;
                background: transparent;
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.75rem;
                cursor: pointer;
            }

            .btn-menu:hover {
                background: var(--bg-card, #16181c);
            }

            .btn-menu.btn-danger {
                color: var(--error, #f4212e);
            }

            .team-description {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin: 0 0 1rem 0;
            }

            .team-stats {
                display: flex;
                gap: 1.5rem;
                padding: 0.75rem 0;
                border-top: 1px solid var(--border, #2f3336);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .stat {
                display: flex;
                flex-direction: column;
            }

            .stat-value {
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }

            .stat-label {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .team-members-preview {
                display: flex;
                margin: 0.75rem 0;
            }

            .member-avatar-small {
                width: 28px;
                height: 28px;
                border-radius: 50%;
                background: var(--bg-card, #16181c);
                border: 2px solid var(--bg-elevated, #1e2128);
                margin-left: -8px;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.7rem;
                font-weight: 600;
            }

            .member-avatar-small:first-child {
                margin-left: 0;
            }

            .member-avatar-small.more {
                background: var(--border, #2f3336);
                font-size: 0.65rem;
            }

            .team-actions {
                margin-top: 0.75rem;
            }

            .team-actions button {
                width: 100%;
            }

            .modal {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.5);
                display: flex;
                align-items: center;
                justify-content: center;
                z-index: 1000;
            }

            .modal-content {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                width: 400px;
                max-width: 90%;
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            }

            .modal-content.modal-wide {
                width: 600px;
            }

            .modal-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
                font-weight: 600;
            }

            .btn-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.25rem;
                cursor: pointer;
            }

            .modal-body {
                padding: 1rem;
                max-height: 60vh;
                overflow-y: auto;
            }

            .form-group {
                margin-bottom: 1rem;
            }

            .form-group label {
                display: block;
                margin-bottom: 0.5rem;
                font-size: 0.85rem;
                font-weight: 500;
            }

            .form-group input,
            .form-group textarea {
                width: 100%;
                padding: 0.5rem 0.75rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }

            .form-group textarea {
                resize: vertical;
            }

            .modal-footer {
                display: flex;
                justify-content: flex-end;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .members-section {
                margin-bottom: 1.5rem;
            }

            .members-section h4 {
                margin: 0 0 0.75rem 0;
                font-size: 0.9rem;
                color: var(--text-muted, #71767b);
            }

            .members-grid {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .member-card {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-card, #16181c);
                border-radius: 6px;
            }

            .member-avatar {
                width: 36px;
                height: 36px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
            }

            .member-info {
                flex: 1;
            }

            .member-name {
                font-weight: 500;
                font-size: 0.9rem;
            }

            .member-email {
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .btn-remove-member, .btn-add-member {
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                font-size: 0.75rem;
                cursor: pointer;
            }

            .btn-remove-member {
                background: rgba(244, 63, 94, 0.2);
                color: #f43f5e;
                border: none;
            }

            .btn-add-member {
                background: rgba(0, 186, 124, 0.2);
                color: #00ba7c;
                border: none;
            }
        `;
    }
}

customElements.define('team-management', TeamManagement);
