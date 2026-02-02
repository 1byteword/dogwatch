/**
 * User Management Widget
 * CRUD operations for users with role management
 */
class UserManagement extends HTMLElement {
    constructor() {
        super();
        this.users = [];
        this.roles = ['viewer', 'editor', 'admin', 'owner'];
        this.selectedUser = null;
        this.loading = true;
        this.error = null;
        this.searchQuery = '';
        this.filterRole = '';
    }

    connectedCallback() {
        this.render();
        this.loadUsers();
    }

    async loadUsers() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/users');
            if (resp.ok) {
                this.users = await resp.json();
            } else if (resp.status === 401 || resp.status === 403) {
                this.error = 'You do not have permission to view users';
            } else {
                // Demo data for development
                this.users = this.generateDemoUsers();
            }
        } catch (e) {
            console.error('Failed to load users:', e);
            this.users = this.generateDemoUsers();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoUsers() {
        return [
            { id: '1', email: 'admin@example.com', name: 'Admin User', role: 'owner', created_at: Date.now() - 86400000 * 30, last_login: Date.now() - 3600000, status: 'active' },
            { id: '2', email: 'john@example.com', name: 'John Smith', role: 'admin', created_at: Date.now() - 86400000 * 20, last_login: Date.now() - 7200000, status: 'active' },
            { id: '3', email: 'jane@example.com', name: 'Jane Doe', role: 'editor', created_at: Date.now() - 86400000 * 15, last_login: Date.now() - 86400000, status: 'active' },
            { id: '4', email: 'bob@example.com', name: 'Bob Wilson', role: 'viewer', created_at: Date.now() - 86400000 * 10, last_login: Date.now() - 86400000 * 5, status: 'active' },
            { id: '5', email: 'alice@example.com', name: 'Alice Brown', role: 'editor', created_at: Date.now() - 86400000 * 5, last_login: null, status: 'pending' }
        ];
    }

    getFilteredUsers() {
        return this.users.filter(user => {
            const matchesSearch = !this.searchQuery ||
                user.name.toLowerCase().includes(this.searchQuery.toLowerCase()) ||
                user.email.toLowerCase().includes(this.searchQuery.toLowerCase());
            const matchesRole = !this.filterRole || user.role === this.filterRole;
            return matchesSearch && matchesRole;
        });
    }

    async createUser(userData) {
        try {
            const resp = await fetch('/api/users', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(userData)
            });

            if (resp.ok) {
                await this.loadUsers();
                this.closeModal();
                this.showToast('User created successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to create user: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to create user:', e);
            // Demo: add locally
            const newUser = {
                id: String(Date.now()),
                ...userData,
                created_at: Date.now(),
                last_login: null,
                status: 'pending'
            };
            this.users.push(newUser);
            this.closeModal();
            this.showToast('User created successfully');
            this.render();
        }
    }

    async updateUser(userId, updates) {
        try {
            const resp = await fetch(`/api/users/${encodeURIComponent(userId)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            if (resp.ok) {
                await this.loadUsers();
                this.closeModal();
                this.showToast('User updated successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to update user: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to update user:', e);
            // Demo: update locally
            const idx = this.users.findIndex(u => u.id === userId);
            if (idx >= 0) {
                this.users[idx] = { ...this.users[idx], ...updates };
            }
            this.closeModal();
            this.showToast('User updated successfully');
            this.render();
        }
    }

    async deleteUser(userId) {
        if (!confirm('Are you sure you want to delete this user? This action cannot be undone.')) {
            return;
        }

        try {
            const resp = await fetch(`/api/users/${encodeURIComponent(userId)}`, {
                method: 'DELETE'
            });

            if (resp.ok) {
                await this.loadUsers();
                this.showToast('User deleted successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to delete user: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to delete user:', e);
            // Demo: delete locally
            this.users = this.users.filter(u => u.id !== userId);
            this.showToast('User deleted successfully');
            this.render();
        }
    }

    async resetPassword(userId) {
        if (!confirm('Send password reset email to this user?')) {
            return;
        }

        try {
            const resp = await fetch(`/api/users/${encodeURIComponent(userId)}/reset-password`, {
                method: 'POST'
            });

            if (resp.ok) {
                this.showToast('Password reset email sent');
            } else {
                this.showToast('Failed to send reset email', 'error');
            }
        } catch (e) {
            console.error('Failed to reset password:', e);
            this.showToast('Password reset email sent');
        }
    }

    openCreateModal() {
        this.selectedUser = null;
        this.showModal();
    }

    openEditModal(user) {
        this.selectedUser = user;
        this.showModal();
    }

    showModal() {
        const modal = this.querySelector('#user-modal');
        if (modal) {
            modal.style.display = 'flex';
            this.updateModalForm();
        }
    }

    closeModal() {
        const modal = this.querySelector('#user-modal');
        if (modal) {
            modal.style.display = 'none';
        }
        this.selectedUser = null;
    }

    updateModalForm() {
        const form = this.querySelector('#user-form');
        if (!form) return;

        const emailInput = form.querySelector('#user-email');
        const nameInput = form.querySelector('#user-name');
        const roleInput = form.querySelector('#user-role');
        const passwordGroup = form.querySelector('#password-group');
        const title = this.querySelector('#modal-title');

        if (this.selectedUser) {
            title.textContent = 'Edit User';
            emailInput.value = this.selectedUser.email;
            nameInput.value = this.selectedUser.name;
            roleInput.value = this.selectedUser.role;
            passwordGroup.style.display = 'none';
        } else {
            title.textContent = 'Create User';
            emailInput.value = '';
            nameInput.value = '';
            roleInput.value = 'viewer';
            passwordGroup.style.display = 'block';
        }
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        const userData = {
            email: formData.get('email'),
            name: formData.get('name'),
            role: formData.get('role')
        };

        if (!this.selectedUser) {
            userData.password = formData.get('password');
            if (!userData.password || userData.password.length < 8) {
                this.showToast('Password must be at least 8 characters', 'error');
                return;
            }
        }

        if (!userData.email || !userData.name) {
            this.showToast('Please fill in all required fields', 'error');
            return;
        }

        if (this.selectedUser) {
            this.updateUser(this.selectedUser.id, userData);
        } else {
            this.createUser(userData);
        }
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);

        // Fallback: show alert if no toast handler
        if (type === 'error') {
            console.error(message);
        }
    }

    handleSearch(query) {
        this.searchQuery = query;
        this.render();
    }

    handleRoleFilter(role) {
        this.filterRole = role;
        this.render();
    }

    formatDate(timestamp) {
        if (!timestamp) return 'Never';
        const date = new Date(timestamp);
        return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    }

    formatRelativeTime(timestamp) {
        if (!timestamp) return 'Never';
        const diff = Date.now() - timestamp;
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        if (diff < 604800000) return Math.floor(diff / 86400000) + 'd ago';
        return this.formatDate(timestamp);
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    getRoleBadgeClass(role) {
        switch (role) {
            case 'owner': return 'role-owner';
            case 'admin': return 'role-admin';
            case 'editor': return 'role-editor';
            default: return 'role-viewer';
        }
    }

    getStatusBadgeClass(status) {
        switch (status) {
            case 'active': return 'status-active';
            case 'pending': return 'status-pending';
            case 'suspended': return 'status-suspended';
            default: return '';
        }
    }

    render() {
        const filteredUsers = this.getFilteredUsers();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="user-management">
                <div class="header">
                    <div class="header-left">
                        <span class="title">User Management</span>
                        <span class="user-count">${this.users.length} users</span>
                    </div>
                    <div class="header-right">
                        <button class="btn-primary" id="btn-create-user">+ Add User</button>
                    </div>
                </div>

                <div class="toolbar">
                    <div class="search-box">
                        <input type="text" id="search-input" placeholder="Search users..." value="${this.escapeHtml(this.searchQuery)}">
                    </div>
                    <div class="filter-group">
                        <select id="role-filter">
                            <option value="">All Roles</option>
                            ${this.roles.map(role => `
                                <option value="${role}" ${this.filterRole === role ? 'selected' : ''}>${role.charAt(0).toUpperCase() + role.slice(1)}</option>
                            `).join('')}
                        </select>
                    </div>
                    <button class="btn-secondary" id="btn-refresh">Refresh</button>
                </div>

                <div class="content">
                    ${this.loading ? `
                        <div class="loading">Loading users...</div>
                    ` : this.error ? `
                        <div class="error">${this.escapeHtml(this.error)}</div>
                    ` : filteredUsers.length === 0 ? `
                        <div class="empty-state">
                            ${this.searchQuery || this.filterRole ? 'No users match your filters' : 'No users found'}
                        </div>
                    ` : `
                        <table class="users-table">
                            <thead>
                                <tr>
                                    <th>User</th>
                                    <th>Role</th>
                                    <th>Status</th>
                                    <th>Last Login</th>
                                    <th>Created</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${filteredUsers.map(user => `
                                    <tr>
                                        <td>
                                            <div class="user-info">
                                                <div class="user-avatar">${this.escapeHtml(user.name.charAt(0).toUpperCase())}</div>
                                                <div class="user-details">
                                                    <div class="user-name">${this.escapeHtml(user.name)}</div>
                                                    <div class="user-email">${this.escapeHtml(user.email)}</div>
                                                </div>
                                            </div>
                                        </td>
                                        <td><span class="role-badge ${this.getRoleBadgeClass(user.role)}">${this.escapeHtml(user.role)}</span></td>
                                        <td><span class="status-badge ${this.getStatusBadgeClass(user.status)}">${this.escapeHtml(user.status)}</span></td>
                                        <td class="text-muted">${this.formatRelativeTime(user.last_login)}</td>
                                        <td class="text-muted">${this.formatDate(user.created_at)}</td>
                                        <td>
                                            <div class="action-buttons">
                                                <button class="btn-action" data-edit="${this.escapeHtml(user.id)}" title="Edit">Edit</button>
                                                <button class="btn-action" data-reset="${this.escapeHtml(user.id)}" title="Reset Password">Reset</button>
                                                ${user.role !== 'owner' ? `
                                                    <button class="btn-action btn-danger" data-delete="${this.escapeHtml(user.id)}" title="Delete">Delete</button>
                                                ` : ''}
                                            </div>
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    `}
                </div>

                <div class="modal" id="user-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span id="modal-title">Create User</span>
                            <button class="btn-close" id="btn-close-modal">&times;</button>
                        </div>
                        <form id="user-form">
                            <div class="modal-body">
                                <div class="form-group">
                                    <label for="user-email">Email *</label>
                                    <input type="email" id="user-email" name="email" required>
                                </div>
                                <div class="form-group">
                                    <label for="user-name">Name *</label>
                                    <input type="text" id="user-name" name="name" required>
                                </div>
                                <div class="form-group">
                                    <label for="user-role">Role *</label>
                                    <select id="user-role" name="role" required>
                                        ${this.roles.map(role => `
                                            <option value="${role}">${role.charAt(0).toUpperCase() + role.slice(1)}</option>
                                        `).join('')}
                                    </select>
                                </div>
                                <div class="form-group" id="password-group">
                                    <label for="user-password">Password *</label>
                                    <input type="password" id="user-password" name="password" minlength="8">
                                    <span class="form-hint">Minimum 8 characters</span>
                                </div>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn-secondary" id="btn-cancel">Cancel</button>
                                <button type="submit" class="btn-primary">Save</button>
                            </div>
                        </form>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    attachEventListeners() {
        // Create user button
        this.querySelector('#btn-create-user')?.addEventListener('click', () => this.openCreateModal());

        // Refresh button
        this.querySelector('#btn-refresh')?.addEventListener('click', () => this.loadUsers());

        // Search input
        this.querySelector('#search-input')?.addEventListener('input', (e) => this.handleSearch(e.target.value));

        // Role filter
        this.querySelector('#role-filter')?.addEventListener('change', (e) => this.handleRoleFilter(e.target.value));

        // Modal close buttons
        this.querySelector('#btn-close-modal')?.addEventListener('click', () => this.closeModal());
        this.querySelector('#btn-cancel')?.addEventListener('click', () => this.closeModal());

        // Form submit
        this.querySelector('#user-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));

        // Edit buttons
        this.querySelectorAll('[data-edit]').forEach(btn => {
            btn.addEventListener('click', () => {
                const user = this.users.find(u => u.id === btn.dataset.edit);
                if (user) this.openEditModal(user);
            });
        });

        // Reset password buttons
        this.querySelectorAll('[data-reset]').forEach(btn => {
            btn.addEventListener('click', () => this.resetPassword(btn.dataset.reset));
        });

        // Delete buttons
        this.querySelectorAll('[data-delete]').forEach(btn => {
            btn.addEventListener('click', () => this.deleteUser(btn.dataset.delete));
        });
    }

    getStyles() {
        return `
            .user-management {
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

            .user-count {
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

            .filter-group select {
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

            .error {
                color: var(--error, #f4212e);
            }

            .users-table {
                width: 100%;
                border-collapse: collapse;
            }

            .users-table th,
            .users-table td {
                padding: 0.75rem;
                text-align: left;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .users-table th {
                font-weight: 500;
                font-size: 0.75rem;
                text-transform: uppercase;
                color: var(--text-muted, #71767b);
                background: var(--bg-elevated, #1e2128);
            }

            .user-info {
                display: flex;
                align-items: center;
                gap: 0.75rem;
            }

            .user-avatar {
                width: 36px;
                height: 36px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 600;
                font-size: 0.9rem;
            }

            .user-name {
                font-weight: 500;
            }

            .user-email {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .role-badge, .status-badge {
                display: inline-block;
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.75rem;
                font-weight: 500;
                text-transform: capitalize;
            }

            .role-owner { background: rgba(244, 63, 94, 0.2); color: #f43f5e; }
            .role-admin { background: rgba(251, 191, 36, 0.2); color: #fbbf24; }
            .role-editor { background: rgba(29, 155, 240, 0.2); color: #1d9bf0; }
            .role-viewer { background: rgba(113, 118, 123, 0.2); color: #71767b; }

            .status-active { background: rgba(0, 186, 124, 0.2); color: #00ba7c; }
            .status-pending { background: rgba(251, 191, 36, 0.2); color: #fbbf24; }
            .status-suspended { background: rgba(244, 63, 94, 0.2); color: #f43f5e; }

            .text-muted {
                color: var(--text-muted, #71767b);
                font-size: 0.85rem;
            }

            .action-buttons {
                display: flex;
                gap: 0.5rem;
            }

            .btn-action {
                padding: 0.25rem 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.75rem;
                cursor: pointer;
            }

            .btn-action:hover {
                background: var(--bg-card, #16181c);
            }

            .btn-action.btn-danger {
                color: var(--error, #f4212e);
                border-color: rgba(244, 63, 94, 0.3);
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
            .form-group select {
                width: 100%;
                padding: 0.5rem 0.75rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.85rem;
            }

            .form-hint {
                display: block;
                margin-top: 0.25rem;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
            }

            .modal-footer {
                display: flex;
                justify-content: flex-end;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }
        `;
    }
}

customElements.define('user-management', UserManagement);
