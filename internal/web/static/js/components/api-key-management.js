/**
 * API Key Management Widget
 * Generate, view, and revoke API keys
 */
class ApiKeyManagement extends HTMLElement {
    constructor() {
        super();
        this.apiKeys = [];
        this.loading = true;
        this.error = null;
        this.newKey = null; // Temporarily stores newly generated key
    }

    connectedCallback() {
        this.render();
        this.loadApiKeys();
    }

    async loadApiKeys() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/apikeys');
            if (resp.ok) {
                this.apiKeys = await resp.json();
            } else if (resp.status === 401 || resp.status === 403) {
                this.error = 'You do not have permission to view API keys';
            } else {
                this.apiKeys = this.generateDemoKeys();
            }
        } catch (e) {
            console.error('Failed to load API keys:', e);
            this.apiKeys = this.generateDemoKeys();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoKeys() {
        return [
            {
                id: '1',
                name: 'Production Agent',
                prefix: 'dw_prod_abc',
                scopes: ['traces:write', 'metrics:write', 'logs:write'],
                created_at: Date.now() - 86400000 * 30,
                last_used: Date.now() - 3600000,
                expires_at: null,
                status: 'active'
            },
            {
                id: '2',
                name: 'CI/CD Pipeline',
                prefix: 'dw_ci_def',
                scopes: ['traces:write', 'metrics:write'],
                created_at: Date.now() - 86400000 * 15,
                last_used: Date.now() - 86400000,
                expires_at: Date.now() + 86400000 * 30,
                status: 'active'
            },
            {
                id: '3',
                name: 'Read-Only Dashboard',
                prefix: 'dw_dash_ghi',
                scopes: ['traces:read', 'metrics:read', 'logs:read'],
                created_at: Date.now() - 86400000 * 7,
                last_used: null,
                expires_at: null,
                status: 'active'
            },
            {
                id: '4',
                name: 'Old Integration',
                prefix: 'dw_old_jkl',
                scopes: ['traces:write'],
                created_at: Date.now() - 86400000 * 60,
                last_used: Date.now() - 86400000 * 45,
                expires_at: Date.now() - 86400000 * 5,
                status: 'expired'
            }
        ];
    }

    async generateKey(keyData) {
        try {
            const resp = await fetch('/api/apikeys', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(keyData)
            });

            if (resp.ok) {
                const result = await resp.json();
                this.newKey = result.key; // The full key is only shown once
                await this.loadApiKeys();
                this.showNewKeyModal();
            } else {
                const error = await resp.text();
                this.showToast('Failed to generate API key: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to generate API key:', e);
            // Demo: generate a fake key
            const prefix = 'dw_' + keyData.name.toLowerCase().replace(/[^a-z0-9]/g, '').slice(0, 4) + '_';
            const fullKey = prefix + this.generateRandomString(32);
            this.newKey = fullKey;

            const newApiKey = {
                id: String(Date.now()),
                name: keyData.name,
                prefix: prefix + fullKey.slice(prefix.length, prefix.length + 3),
                scopes: keyData.scopes || [],
                created_at: Date.now(),
                last_used: null,
                expires_at: keyData.expires_at || null,
                status: 'active'
            };
            this.apiKeys.unshift(newApiKey);
            this.closeCreateModal();
            this.showNewKeyModal();
        }
    }

    generateRandomString(length) {
        const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
        let result = '';
        for (let i = 0; i < length; i++) {
            result += chars.charAt(Math.floor(Math.random() * chars.length));
        }
        return result;
    }

    async revokeKey(keyId) {
        const key = this.apiKeys.find(k => k.id === keyId);
        if (!key) return;

        if (!confirm(`Are you sure you want to revoke the API key "${key.name}"? This action cannot be undone.`)) {
            return;
        }

        try {
            const resp = await fetch(`/api/apikeys/${encodeURIComponent(keyId)}`, {
                method: 'DELETE'
            });

            if (resp.ok) {
                await this.loadApiKeys();
                this.showToast('API key revoked successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to revoke API key: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to revoke API key:', e);
            // Demo: remove locally
            this.apiKeys = this.apiKeys.filter(k => k.id !== keyId);
            this.showToast('API key revoked successfully');
            this.render();
        }
    }

    openCreateModal() {
        const modal = this.querySelector('#create-modal');
        if (modal) {
            modal.style.display = 'flex';
            // Reset form
            const form = modal.querySelector('form');
            if (form) form.reset();
        }
    }

    closeCreateModal() {
        const modal = this.querySelector('#create-modal');
        if (modal) modal.style.display = 'none';
    }

    showNewKeyModal() {
        const modal = this.querySelector('#new-key-modal');
        const keyDisplay = this.querySelector('#new-key-display');
        if (modal && keyDisplay) {
            keyDisplay.textContent = this.newKey;
            modal.style.display = 'flex';
        }
        this.render();
    }

    closeNewKeyModal() {
        const modal = this.querySelector('#new-key-modal');
        if (modal) modal.style.display = 'none';
        this.newKey = null;
    }

    copyToClipboard() {
        if (this.newKey) {
            navigator.clipboard.writeText(this.newKey).then(() => {
                this.showToast('API key copied to clipboard');
            }).catch(() => {
                this.showToast('Failed to copy to clipboard', 'error');
            });
        }
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        const selectedScopes = [];
        form.querySelectorAll('input[name="scopes"]:checked').forEach(cb => {
            selectedScopes.push(cb.value);
        });

        const keyData = {
            name: formData.get('name'),
            scopes: selectedScopes,
            expires_at: formData.get('expires') ? new Date(formData.get('expires')).getTime() : null
        };

        if (!keyData.name) {
            this.showToast('Please enter a key name', 'error');
            return;
        }

        if (selectedScopes.length === 0) {
            this.showToast('Please select at least one scope', 'error');
            return;
        }

        this.generateKey(keyData);
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatDate(timestamp) {
        if (!timestamp) return 'Never';
        const date = new Date(timestamp);
        return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    }

    formatRelativeTime(timestamp) {
        if (!timestamp) return 'Never';
        const diff = Date.now() - timestamp;
        if (diff < 0) return 'in ' + this.formatDuration(-diff);
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        if (diff < 604800000) return Math.floor(diff / 86400000) + 'd ago';
        return this.formatDate(timestamp);
    }

    formatDuration(ms) {
        const days = Math.floor(ms / 86400000);
        if (days > 0) return days + ' days';
        const hours = Math.floor(ms / 3600000);
        if (hours > 0) return hours + ' hours';
        return Math.floor(ms / 60000) + ' minutes';
    }

    getExpirationStatus(key) {
        if (key.status === 'revoked') return { class: 'status-revoked', text: 'Revoked' };
        if (!key.expires_at) return { class: 'status-active', text: 'Active' };
        if (key.expires_at < Date.now()) return { class: 'status-expired', text: 'Expired' };
        if (key.expires_at < Date.now() + 86400000 * 7) return { class: 'status-expiring', text: 'Expiring Soon' };
        return { class: 'status-active', text: 'Active' };
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    getScopeLabel(scope) {
        const labels = {
            'traces:read': 'Read Traces',
            'traces:write': 'Write Traces',
            'metrics:read': 'Read Metrics',
            'metrics:write': 'Write Metrics',
            'logs:read': 'Read Logs',
            'logs:write': 'Write Logs',
            'admin': 'Admin Access'
        };
        return labels[scope] || scope;
    }

    render() {
        const activeKeys = this.apiKeys.filter(k => k.status === 'active' && (!k.expires_at || k.expires_at > Date.now()));
        const expiredKeys = this.apiKeys.filter(k => k.status !== 'active' || (k.expires_at && k.expires_at <= Date.now()));

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="api-key-management">
                <div class="header">
                    <div class="header-left">
                        <span class="title">API Keys</span>
                        <span class="key-count">${activeKeys.length} active</span>
                    </div>
                    <div class="header-right">
                        <button class="btn-primary" id="btn-generate">+ Generate New Key</button>
                    </div>
                </div>

                <div class="info-banner">
                    <span class="info-icon">i</span>
                    <span>API keys are used to authenticate agents and integrations. Keep your keys secure and rotate them regularly.</span>
                </div>

                <div class="content">
                    ${this.loading ? `
                        <div class="loading">Loading API keys...</div>
                    ` : this.error ? `
                        <div class="error">${this.escapeHtml(this.error)}</div>
                    ` : this.apiKeys.length === 0 ? `
                        <div class="empty-state">
                            <div class="empty-icon">&#128273;</div>
                            <div class="empty-text">No API keys found</div>
                            <div class="empty-hint">Generate your first API key to start sending data to dogwatch</div>
                        </div>
                    ` : `
                        ${activeKeys.length > 0 ? `
                            <div class="section">
                                <h3>Active Keys</h3>
                                <div class="keys-list">
                                    ${activeKeys.map(key => this.renderKeyCard(key)).join('')}
                                </div>
                            </div>
                        ` : ''}

                        ${expiredKeys.length > 0 ? `
                            <div class="section">
                                <h3>Expired / Revoked Keys</h3>
                                <div class="keys-list">
                                    ${expiredKeys.map(key => this.renderKeyCard(key)).join('')}
                                </div>
                            </div>
                        ` : ''}
                    `}
                </div>

                <div class="modal" id="create-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span>Generate API Key</span>
                            <button class="btn-close" id="btn-close-create">&times;</button>
                        </div>
                        <form id="create-form">
                            <div class="modal-body">
                                <div class="form-group">
                                    <label for="key-name">Key Name *</label>
                                    <input type="text" id="key-name" name="name" placeholder="e.g., Production Agent" required>
                                    <span class="form-hint">A descriptive name to identify this key</span>
                                </div>

                                <div class="form-group">
                                    <label>Permissions *</label>
                                    <div class="scopes-grid">
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="traces:write" checked>
                                            <span>Write Traces</span>
                                        </label>
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="traces:read">
                                            <span>Read Traces</span>
                                        </label>
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="metrics:write" checked>
                                            <span>Write Metrics</span>
                                        </label>
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="metrics:read">
                                            <span>Read Metrics</span>
                                        </label>
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="logs:write" checked>
                                            <span>Write Logs</span>
                                        </label>
                                        <label class="scope-checkbox">
                                            <input type="checkbox" name="scopes" value="logs:read">
                                            <span>Read Logs</span>
                                        </label>
                                    </div>
                                </div>

                                <div class="form-group">
                                    <label for="key-expires">Expiration (Optional)</label>
                                    <input type="date" id="key-expires" name="expires">
                                    <span class="form-hint">Leave empty for no expiration</span>
                                </div>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn-secondary" id="btn-cancel-create">Cancel</button>
                                <button type="submit" class="btn-primary">Generate Key</button>
                            </div>
                        </form>
                    </div>
                </div>

                <div class="modal" id="new-key-modal" style="display: none;">
                    <div class="modal-content">
                        <div class="modal-header">
                            <span>API Key Generated</span>
                            <button class="btn-close" id="btn-close-new-key">&times;</button>
                        </div>
                        <div class="modal-body">
                            <div class="warning-banner">
                                <span class="warning-icon">!</span>
                                <span>Make sure to copy your API key now. You won't be able to see it again!</span>
                            </div>
                            <div class="key-display-container">
                                <code id="new-key-display" class="key-display"></code>
                                <button class="btn-copy" id="btn-copy-key">Copy</button>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn-primary" id="btn-done-new-key">Done</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderKeyCard(key) {
        const status = this.getExpirationStatus(key);
        const isDisabled = status.class !== 'status-active';

        return `
            <div class="key-card ${isDisabled ? 'disabled' : ''}">
                <div class="key-header">
                    <div class="key-info">
                        <div class="key-name">${this.escapeHtml(key.name)}</div>
                        <code class="key-prefix">${this.escapeHtml(key.prefix)}...</code>
                    </div>
                    <span class="status-badge ${status.class}">${status.text}</span>
                </div>
                <div class="key-scopes">
                    ${(key.scopes || []).map(scope => `
                        <span class="scope-tag">${this.escapeHtml(this.getScopeLabel(scope))}</span>
                    `).join('')}
                </div>
                <div class="key-meta">
                    <span>Created: ${this.formatDate(key.created_at)}</span>
                    <span>Last used: ${this.formatRelativeTime(key.last_used)}</span>
                    ${key.expires_at ? `<span>Expires: ${this.formatRelativeTime(key.expires_at)}</span>` : ''}
                </div>
                <div class="key-actions">
                    ${!isDisabled ? `
                        <button class="btn-danger" data-revoke="${this.escapeHtml(key.id)}">Revoke</button>
                    ` : ''}
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        // Generate button
        this.querySelector('#btn-generate')?.addEventListener('click', () => this.openCreateModal());

        // Modal close buttons
        this.querySelector('#btn-close-create')?.addEventListener('click', () => this.closeCreateModal());
        this.querySelector('#btn-cancel-create')?.addEventListener('click', () => this.closeCreateModal());
        this.querySelector('#btn-close-new-key')?.addEventListener('click', () => this.closeNewKeyModal());
        this.querySelector('#btn-done-new-key')?.addEventListener('click', () => this.closeNewKeyModal());

        // Copy button
        this.querySelector('#btn-copy-key')?.addEventListener('click', () => this.copyToClipboard());

        // Form submit
        this.querySelector('#create-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));

        // Revoke buttons
        this.querySelectorAll('[data-revoke]').forEach(btn => {
            btn.addEventListener('click', () => this.revokeKey(btn.dataset.revoke));
        });
    }

    getStyles() {
        return `
            .api-key-management {
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

            .key-count {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
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

            .btn-danger {
                background: rgba(244, 63, 94, 0.2);
                color: #f43f5e;
                border: 1px solid rgba(244, 63, 94, 0.3);
                padding: 0.25rem 0.75rem;
                border-radius: 4px;
                font-size: 0.8rem;
                cursor: pointer;
            }

            .info-banner {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem 1rem;
                background: rgba(29, 155, 240, 0.1);
                border-bottom: 1px solid var(--border, #2f3336);
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
            }

            .info-icon {
                width: 20px;
                height: 20px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                color: white;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.75rem;
                font-weight: 600;
            }

            .content {
                flex: 1;
                overflow: auto;
                padding: 1rem;
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 200px;
                color: var(--text-muted, #71767b);
            }

            .error {
                color: var(--error, #f4212e);
            }

            .empty-state {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                text-align: center;
            }

            .empty-icon {
                font-size: 3rem;
                margin-bottom: 1rem;
            }

            .empty-text {
                font-size: 1.1rem;
                font-weight: 500;
                margin-bottom: 0.5rem;
            }

            .empty-hint {
                color: var(--text-muted, #71767b);
                font-size: 0.9rem;
            }

            .section {
                margin-bottom: 1.5rem;
            }

            .section h3 {
                font-size: 0.85rem;
                font-weight: 600;
                color: var(--text-muted, #71767b);
                margin: 0 0 0.75rem 0;
                text-transform: uppercase;
            }

            .keys-list {
                display: flex;
                flex-direction: column;
                gap: 0.75rem;
            }

            .key-card {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
                border: 1px solid var(--border, #2f3336);
            }

            .key-card.disabled {
                opacity: 0.6;
            }

            .key-header {
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
                margin-bottom: 0.75rem;
            }

            .key-name {
                font-weight: 600;
                font-size: 0.95rem;
                margin-bottom: 0.25rem;
            }

            .key-prefix {
                font-family: monospace;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                background: var(--bg-card, #16181c);
                padding: 0.2rem 0.4rem;
                border-radius: 4px;
            }

            .status-badge {
                padding: 0.2rem 0.5rem;
                border-radius: 4px;
                font-size: 0.75rem;
                font-weight: 500;
            }

            .status-active { background: rgba(0, 186, 124, 0.2); color: #00ba7c; }
            .status-expired { background: rgba(113, 118, 123, 0.2); color: #71767b; }
            .status-expiring { background: rgba(251, 191, 36, 0.2); color: #fbbf24; }
            .status-revoked { background: rgba(244, 63, 94, 0.2); color: #f43f5e; }

            .key-scopes {
                display: flex;
                flex-wrap: wrap;
                gap: 0.5rem;
                margin-bottom: 0.75rem;
            }

            .scope-tag {
                font-size: 0.75rem;
                padding: 0.2rem 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                color: var(--text-muted, #71767b);
            }

            .key-meta {
                display: flex;
                gap: 1.5rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.75rem;
            }

            .key-actions {
                display: flex;
                justify-content: flex-end;
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
                width: 450px;
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

            .form-group input {
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

            .scopes-grid {
                display: grid;
                grid-template-columns: repeat(2, 1fr);
                gap: 0.5rem;
            }

            .scope-checkbox {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                padding: 0.5rem;
                background: var(--bg-card, #16181c);
                border-radius: 4px;
                cursor: pointer;
                font-size: 0.85rem;
            }

            .scope-checkbox input {
                width: auto;
            }

            .modal-footer {
                display: flex;
                justify-content: flex-end;
                gap: 0.5rem;
                padding: 1rem;
                border-top: 1px solid var(--border, #2f3336);
            }

            .warning-banner {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: rgba(251, 191, 36, 0.1);
                border: 1px solid rgba(251, 191, 36, 0.3);
                border-radius: 6px;
                margin-bottom: 1rem;
                font-size: 0.85rem;
                color: #fbbf24;
            }

            .warning-icon {
                width: 20px;
                height: 20px;
                border-radius: 50%;
                background: #fbbf24;
                color: #16181c;
                display: flex;
                align-items: center;
                justify-content: center;
                font-weight: 700;
                font-size: 0.8rem;
            }

            .key-display-container {
                display: flex;
                gap: 0.5rem;
            }

            .key-display {
                flex: 1;
                padding: 0.75rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                font-family: monospace;
                font-size: 0.85rem;
                word-break: break-all;
            }

            .btn-copy {
                padding: 0.5rem 1rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                cursor: pointer;
                font-size: 0.85rem;
            }

            .btn-copy:hover {
                background: var(--bg-elevated, #1e2128);
            }
        `;
    }
}

customElements.define('api-key-management', ApiKeyManagement);
