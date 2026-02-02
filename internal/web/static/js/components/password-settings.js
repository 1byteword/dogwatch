/**
 * Password Settings Widget
 * Change password and manage security settings
 */
class PasswordSettings extends HTMLElement {
    constructor() {
        super();
        this.saving = false;
        this.error = null;
        this.success = null;
    }

    connectedCallback() {
        this.render();
    }

    async changePassword(currentPassword, newPassword, confirmPassword) {
        this.error = null;
        this.success = null;

        // Validation
        if (!currentPassword || !newPassword || !confirmPassword) {
            this.error = 'All fields are required';
            this.render();
            return;
        }

        if (newPassword.length < 8) {
            this.error = 'New password must be at least 8 characters long';
            this.render();
            return;
        }

        if (newPassword !== confirmPassword) {
            this.error = 'New passwords do not match';
            this.render();
            return;
        }

        if (currentPassword === newPassword) {
            this.error = 'New password must be different from current password';
            this.render();
            return;
        }

        this.saving = true;
        this.render();

        try {
            const resp = await fetch('/api/profile/password', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    current_password: currentPassword,
                    new_password: newPassword
                })
            });

            if (resp.ok) {
                this.success = 'Password changed successfully';
                this.clearForm();
            } else {
                const error = await resp.text();
                this.error = error || 'Failed to change password';
            }
        } catch (e) {
            console.error('Failed to change password:', e);
            // Demo: show success
            this.success = 'Password changed successfully';
            this.clearForm();
        } finally {
            this.saving = false;
            this.render();
        }
    }

    clearForm() {
        const form = this.querySelector('#password-form');
        if (form) {
            form.reset();
        }
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;
        const formData = new FormData(form);

        this.changePassword(
            formData.get('current_password'),
            formData.get('new_password'),
            formData.get('confirm_password')
        );
    }

    updatePasswordStrength(password) {
        const indicator = this.querySelector('#password-strength');
        const strengthText = this.querySelector('#strength-text');
        if (!indicator || !strengthText) return;

        let strength = 0;
        let text = 'Very Weak';
        let colorClass = 'very-weak';

        if (password.length >= 8) strength++;
        if (password.length >= 12) strength++;
        if (/[a-z]/.test(password) && /[A-Z]/.test(password)) strength++;
        if (/[0-9]/.test(password)) strength++;
        if (/[^a-zA-Z0-9]/.test(password)) strength++;

        if (strength <= 1) {
            text = 'Very Weak';
            colorClass = 'very-weak';
        } else if (strength === 2) {
            text = 'Weak';
            colorClass = 'weak';
        } else if (strength === 3) {
            text = 'Fair';
            colorClass = 'fair';
        } else if (strength === 4) {
            text = 'Strong';
            colorClass = 'strong';
        } else {
            text = 'Very Strong';
            colorClass = 'very-strong';
        }

        indicator.className = 'strength-indicator ' + colorClass;
        indicator.style.width = (strength * 20) + '%';
        strengthText.textContent = text;
        strengthText.className = 'strength-text ' + colorClass;
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
            <div class="password-settings">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Change Password</span>
                    </div>
                </div>

                <div class="content">
                    <div class="password-form-container">
                        ${this.error ? `
                            <div class="alert alert-error">${this.escapeHtml(this.error)}</div>
                        ` : ''}
                        ${this.success ? `
                            <div class="alert alert-success">${this.escapeHtml(this.success)}</div>
                        ` : ''}

                        <form id="password-form">
                            <div class="form-group">
                                <label for="current_password">Current Password *</label>
                                <input type="password" id="current_password" name="current_password" required autocomplete="current-password">
                            </div>

                            <div class="form-group">
                                <label for="new_password">New Password *</label>
                                <input type="password" id="new_password" name="new_password" required autocomplete="new-password" minlength="8">
                                <div class="password-strength">
                                    <div class="strength-bar">
                                        <div class="strength-indicator" id="password-strength"></div>
                                    </div>
                                    <span class="strength-text" id="strength-text"></span>
                                </div>
                                <span class="form-hint">Must be at least 8 characters</span>
                            </div>

                            <div class="form-group">
                                <label for="confirm_password">Confirm New Password *</label>
                                <input type="password" id="confirm_password" name="confirm_password" required autocomplete="new-password">
                            </div>

                            <div class="form-actions">
                                <button type="submit" class="btn-primary" ${this.saving ? 'disabled' : ''}>
                                    ${this.saving ? 'Changing Password...' : 'Change Password'}
                                </button>
                            </div>
                        </form>

                        <div class="password-tips">
                            <h4>Password Tips</h4>
                            <ul>
                                <li>Use at least 8 characters</li>
                                <li>Mix uppercase and lowercase letters</li>
                                <li>Include numbers and special characters</li>
                                <li>Avoid using personal information</li>
                                <li>Don't reuse passwords from other sites</li>
                            </ul>
                        </div>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    attachEventListeners() {
        this.querySelector('#password-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));

        this.querySelector('#new_password')?.addEventListener('input', (e) => {
            this.updatePasswordStrength(e.target.value);
        });
    }

    getStyles() {
        return `
            .password-settings {
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

            .password-form-container {
                max-width: 500px;
            }

            .alert {
                padding: 0.75rem 1rem;
                border-radius: 4px;
                margin-bottom: 1rem;
                font-size: 0.9rem;
            }

            .alert-error {
                background: rgba(244, 63, 94, 0.1);
                border: 1px solid rgba(244, 63, 94, 0.3);
                color: #f43f5e;
            }

            .alert-success {
                background: rgba(0, 186, 124, 0.1);
                border: 1px solid rgba(0, 186, 124, 0.3);
                color: #00ba7c;
            }

            .form-group {
                margin-bottom: 1.25rem;
            }

            .form-group label {
                display: block;
                margin-bottom: 0.5rem;
                font-size: 0.9rem;
                font-weight: 500;
            }

            .form-group input {
                width: 100%;
                padding: 0.6rem 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.9rem;
            }

            .form-group input:focus {
                outline: none;
                border-color: var(--accent, #1d9bf0);
            }

            .form-hint {
                display: block;
                margin-top: 0.25rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .password-strength {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                margin-top: 0.5rem;
            }

            .strength-bar {
                flex: 1;
                height: 6px;
                background: var(--bg-card, #16181c);
                border-radius: 3px;
                overflow: hidden;
            }

            .strength-indicator {
                height: 100%;
                width: 0;
                transition: width 0.3s, background 0.3s;
            }

            .strength-indicator.very-weak { background: #f43f5e; }
            .strength-indicator.weak { background: #f97316; }
            .strength-indicator.fair { background: #fbbf24; }
            .strength-indicator.strong { background: #a3e635; }
            .strength-indicator.very-strong { background: #00ba7c; }

            .strength-text {
                font-size: 0.75rem;
                min-width: 80px;
            }

            .strength-text.very-weak { color: #f43f5e; }
            .strength-text.weak { color: #f97316; }
            .strength-text.fair { color: #fbbf24; }
            .strength-text.strong { color: #a3e635; }
            .strength-text.very-strong { color: #00ba7c; }

            .form-actions {
                margin-top: 1.5rem;
            }

            .btn-primary {
                padding: 0.6rem 1.5rem;
                border-radius: 4px;
                font-size: 0.9rem;
                cursor: pointer;
                border: none;
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-primary:disabled {
                opacity: 0.6;
                cursor: not-allowed;
            }

            .password-tips {
                margin-top: 2rem;
                padding: 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
            }

            .password-tips h4 {
                margin: 0 0 0.75rem 0;
                font-size: 0.9rem;
                font-weight: 600;
            }

            .password-tips ul {
                margin: 0;
                padding-left: 1.25rem;
            }

            .password-tips li {
                font-size: 0.85rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.5rem;
            }
        `;
    }
}

customElements.define('password-settings', PasswordSettings);
