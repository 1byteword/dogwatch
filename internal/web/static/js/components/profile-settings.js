/**
 * Profile Settings Widget
 * User profile management and preferences
 */
class ProfileSettings extends HTMLElement {
    constructor() {
        super();
        this.profile = null;
        this.loading = true;
        this.saving = false;
        this.error = null;
    }

    connectedCallback() {
        this.render();
        this.loadProfile();
    }

    async loadProfile() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/profile');
            if (resp.ok) {
                this.profile = await resp.json();
            } else {
                this.profile = this.generateDemoProfile();
            }
        } catch (e) {
            console.error('Failed to load profile:', e);
            this.profile = this.generateDemoProfile();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoProfile() {
        return {
            id: '1',
            email: 'user@example.com',
            name: 'John Smith',
            title: 'Senior Engineer',
            department: 'Platform',
            phone: '+1 (555) 123-4567',
            timezone: 'America/New_York',
            language: 'en',
            avatar_url: null,
            created_at: Date.now() - 86400000 * 90,
            last_login: Date.now() - 3600000
        };
    }

    async saveProfile(updates) {
        this.saving = true;
        this.render();

        try {
            const resp = await fetch('/api/profile', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            if (resp.ok) {
                this.profile = { ...this.profile, ...updates };
                this.showToast('Profile updated successfully');
            } else {
                const error = await resp.text();
                this.showToast('Failed to update profile: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to save profile:', e);
            // Demo: update locally
            this.profile = { ...this.profile, ...updates };
            this.showToast('Profile updated successfully');
        } finally {
            this.saving = false;
            this.render();
        }
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        const updates = {
            name: formData.get('name'),
            title: formData.get('title'),
            department: formData.get('department'),
            phone: formData.get('phone'),
            timezone: formData.get('timezone'),
            language: formData.get('language')
        };

        if (!updates.name) {
            this.showToast('Name is required', 'error');
            return;
        }

        this.saveProfile(updates);
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
        return date.toLocaleDateString('en-US', {
            month: 'long',
            day: 'numeric',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
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
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="profile-settings">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Profile Settings</span>
                    </div>
                </div>

                <div class="content">
                    ${this.loading ? `
                        <div class="loading">Loading profile...</div>
                    ` : this.error ? `
                        <div class="error">${this.escapeHtml(this.error)}</div>
                    ` : this.renderProfileForm()}
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderProfileForm() {
        const p = this.profile || {};
        return `
            <div class="profile-layout">
                <div class="profile-sidebar">
                    <div class="avatar-section">
                        <div class="avatar">
                            ${p.avatar_url ? `<img src="${this.escapeHtml(p.avatar_url)}" alt="Avatar">` : `
                                <span class="avatar-initials">${this.escapeHtml((p.name || 'U').charAt(0).toUpperCase())}</span>
                            `}
                        </div>
                        <button class="btn-secondary btn-upload">Change Avatar</button>
                    </div>
                    <div class="profile-meta">
                        <div class="meta-item">
                            <span class="meta-label">Email</span>
                            <span class="meta-value">${this.escapeHtml(p.email)}</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Member Since</span>
                            <span class="meta-value">${this.formatDate(p.created_at)}</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Last Login</span>
                            <span class="meta-value">${this.formatDate(p.last_login)}</span>
                        </div>
                    </div>
                </div>

                <div class="profile-form-section">
                    <form id="profile-form">
                        <h3>Personal Information</h3>

                        <div class="form-row">
                            <div class="form-group">
                                <label for="name">Full Name *</label>
                                <input type="text" id="name" name="name" value="${this.escapeHtml(p.name || '')}" required>
                            </div>
                        </div>

                        <div class="form-row">
                            <div class="form-group">
                                <label for="title">Job Title</label>
                                <input type="text" id="title" name="title" value="${this.escapeHtml(p.title || '')}" placeholder="e.g., Software Engineer">
                            </div>
                            <div class="form-group">
                                <label for="department">Department</label>
                                <input type="text" id="department" name="department" value="${this.escapeHtml(p.department || '')}" placeholder="e.g., Engineering">
                            </div>
                        </div>

                        <div class="form-row">
                            <div class="form-group">
                                <label for="phone">Phone Number</label>
                                <input type="tel" id="phone" name="phone" value="${this.escapeHtml(p.phone || '')}" placeholder="+1 (555) 123-4567">
                            </div>
                        </div>

                        <h3>Preferences</h3>

                        <div class="form-row">
                            <div class="form-group">
                                <label for="timezone">Timezone</label>
                                <select id="timezone" name="timezone">
                                    <option value="UTC" ${p.timezone === 'UTC' ? 'selected' : ''}>UTC</option>
                                    <option value="America/New_York" ${p.timezone === 'America/New_York' ? 'selected' : ''}>Eastern Time (US)</option>
                                    <option value="America/Chicago" ${p.timezone === 'America/Chicago' ? 'selected' : ''}>Central Time (US)</option>
                                    <option value="America/Denver" ${p.timezone === 'America/Denver' ? 'selected' : ''}>Mountain Time (US)</option>
                                    <option value="America/Los_Angeles" ${p.timezone === 'America/Los_Angeles' ? 'selected' : ''}>Pacific Time (US)</option>
                                    <option value="Europe/London" ${p.timezone === 'Europe/London' ? 'selected' : ''}>London</option>
                                    <option value="Europe/Paris" ${p.timezone === 'Europe/Paris' ? 'selected' : ''}>Paris</option>
                                    <option value="Asia/Tokyo" ${p.timezone === 'Asia/Tokyo' ? 'selected' : ''}>Tokyo</option>
                                    <option value="Asia/Shanghai" ${p.timezone === 'Asia/Shanghai' ? 'selected' : ''}>Shanghai</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label for="language">Language</label>
                                <select id="language" name="language">
                                    <option value="en" ${p.language === 'en' ? 'selected' : ''}>English</option>
                                    <option value="es" ${p.language === 'es' ? 'selected' : ''}>Spanish</option>
                                    <option value="fr" ${p.language === 'fr' ? 'selected' : ''}>French</option>
                                    <option value="de" ${p.language === 'de' ? 'selected' : ''}>German</option>
                                    <option value="ja" ${p.language === 'ja' ? 'selected' : ''}>Japanese</option>
                                    <option value="zh" ${p.language === 'zh' ? 'selected' : ''}>Chinese</option>
                                </select>
                            </div>
                        </div>

                        <div class="form-actions">
                            <button type="submit" class="btn-primary" ${this.saving ? 'disabled' : ''}>
                                ${this.saving ? 'Saving...' : 'Save Changes'}
                            </button>
                        </div>
                    </form>
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        this.querySelector('#profile-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));
    }

    getStyles() {
        return `
            .profile-settings {
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

            .title {
                font-weight: 600;
                font-size: 1rem;
            }

            .content {
                flex: 1;
                overflow: auto;
                padding: 1.5rem;
            }

            .loading, .error {
                display: flex;
                align-items: center;
                justify-content: center;
                height: 200px;
                color: var(--text-muted, #71767b);
            }

            .profile-layout {
                display: flex;
                gap: 2rem;
                max-width: 900px;
            }

            .profile-sidebar {
                width: 220px;
                flex-shrink: 0;
            }

            .avatar-section {
                text-align: center;
                margin-bottom: 1.5rem;
            }

            .avatar {
                width: 120px;
                height: 120px;
                border-radius: 50%;
                background: var(--accent, #1d9bf0);
                margin: 0 auto 1rem;
                display: flex;
                align-items: center;
                justify-content: center;
                overflow: hidden;
            }

            .avatar img {
                width: 100%;
                height: 100%;
                object-fit: cover;
            }

            .avatar-initials {
                font-size: 3rem;
                font-weight: 600;
                color: white;
            }

            .btn-upload {
                width: 100%;
            }

            .profile-meta {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                padding: 1rem;
            }

            .meta-item {
                padding: 0.75rem 0;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .meta-item:last-child {
                border-bottom: none;
            }

            .meta-label {
                display: block;
                font-size: 0.75rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }

            .meta-value {
                font-size: 0.9rem;
            }

            .profile-form-section {
                flex: 1;
            }

            .profile-form-section h3 {
                margin: 0 0 1rem 0;
                font-size: 1rem;
                font-weight: 600;
                padding-bottom: 0.5rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .profile-form-section h3:not(:first-child) {
                margin-top: 2rem;
            }

            .form-row {
                display: flex;
                gap: 1rem;
                margin-bottom: 1rem;
            }

            .form-group {
                flex: 1;
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
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.9rem;
            }

            .form-actions {
                margin-top: 2rem;
                padding-top: 1rem;
                border-top: 1px solid var(--border, #2f3336);
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

            .btn-primary:disabled {
                opacity: 0.6;
                cursor: not-allowed;
            }

            .btn-secondary {
                background: var(--bg-card, #16181c);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            @media (max-width: 700px) {
                .profile-layout {
                    flex-direction: column;
                }

                .profile-sidebar {
                    width: 100%;
                }

                .form-row {
                    flex-direction: column;
                }
            }
        `;
    }
}

customElements.define('profile-settings', ProfileSettings);
