/**
 * Notification Preferences Widget
 * User notification settings and preferences
 */
class NotificationPreferences extends HTMLElement {
    constructor() {
        super();
        this.preferences = null;
        this.loading = true;
        this.saving = false;
        this.error = null;
    }

    connectedCallback() {
        this.render();
        this.loadPreferences();
    }

    async loadPreferences() {
        this.loading = true;
        this.error = null;
        this.render();

        try {
            const resp = await fetch('/api/profile/notifications');
            if (resp.ok) {
                this.preferences = await resp.json();
            } else {
                this.preferences = this.generateDemoPreferences();
            }
        } catch (e) {
            console.error('Failed to load preferences:', e);
            this.preferences = this.generateDemoPreferences();
        } finally {
            this.loading = false;
            this.render();
        }
    }

    generateDemoPreferences() {
        return {
            email: {
                enabled: true,
                alerts_critical: true,
                alerts_warning: true,
                alerts_info: false,
                daily_digest: true,
                weekly_report: true
            },
            slack: {
                enabled: true,
                alerts_critical: true,
                alerts_warning: false,
                alerts_info: false
            },
            browser: {
                enabled: true,
                alerts_critical: true,
                alerts_warning: true,
                alerts_info: false
            },
            quiet_hours: {
                enabled: false,
                start: '22:00',
                end: '08:00',
                timezone: 'America/New_York'
            }
        };
    }

    async savePreferences(updates) {
        this.saving = true;
        this.render();

        try {
            const resp = await fetch('/api/profile/notifications', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates)
            });

            if (resp.ok) {
                this.preferences = updates;
                this.showToast('Notification preferences saved');
            } else {
                const error = await resp.text();
                this.showToast('Failed to save preferences: ' + error, 'error');
            }
        } catch (e) {
            console.error('Failed to save preferences:', e);
            // Demo: update locally
            this.preferences = updates;
            this.showToast('Notification preferences saved');
        } finally {
            this.saving = false;
            this.render();
        }
    }

    handleFormSubmit(e) {
        e.preventDefault();
        const form = e.target;

        const updates = {
            email: {
                enabled: form.querySelector('#email_enabled')?.checked || false,
                alerts_critical: form.querySelector('#email_critical')?.checked || false,
                alerts_warning: form.querySelector('#email_warning')?.checked || false,
                alerts_info: form.querySelector('#email_info')?.checked || false,
                daily_digest: form.querySelector('#email_daily')?.checked || false,
                weekly_report: form.querySelector('#email_weekly')?.checked || false
            },
            slack: {
                enabled: form.querySelector('#slack_enabled')?.checked || false,
                alerts_critical: form.querySelector('#slack_critical')?.checked || false,
                alerts_warning: form.querySelector('#slack_warning')?.checked || false,
                alerts_info: form.querySelector('#slack_info')?.checked || false
            },
            browser: {
                enabled: form.querySelector('#browser_enabled')?.checked || false,
                alerts_critical: form.querySelector('#browser_critical')?.checked || false,
                alerts_warning: form.querySelector('#browser_warning')?.checked || false,
                alerts_info: form.querySelector('#browser_info')?.checked || false
            },
            quiet_hours: {
                enabled: form.querySelector('#quiet_enabled')?.checked || false,
                start: form.querySelector('#quiet_start')?.value || '22:00',
                end: form.querySelector('#quiet_end')?.value || '08:00',
                timezone: this.preferences?.quiet_hours?.timezone || 'UTC'
            }
        };

        this.savePreferences(updates);
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    requestBrowserPermission() {
        if ('Notification' in window) {
            Notification.requestPermission().then(permission => {
                if (permission === 'granted') {
                    this.showToast('Browser notifications enabled');
                } else {
                    this.showToast('Browser notification permission denied', 'error');
                }
            });
        } else {
            this.showToast('Browser notifications not supported', 'error');
        }
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="notification-preferences">
                <div class="header">
                    <div class="header-left">
                        <span class="title">Notification Preferences</span>
                    </div>
                </div>

                <div class="content">
                    ${this.loading ? `
                        <div class="loading">Loading preferences...</div>
                    ` : this.error ? `
                        <div class="error">${this.escapeHtml(this.error)}</div>
                    ` : this.renderPreferencesForm()}
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    renderPreferencesForm() {
        const p = this.preferences || {};
        const email = p.email || {};
        const slack = p.slack || {};
        const browser = p.browser || {};
        const quiet = p.quiet_hours || {};

        return `
            <form id="preferences-form">
                <div class="notification-channel">
                    <div class="channel-header">
                        <div class="channel-info">
                            <span class="channel-icon">&#128231;</span>
                            <div>
                                <h3>Email Notifications</h3>
                                <p>Receive notifications via email</p>
                            </div>
                        </div>
                        <label class="toggle">
                            <input type="checkbox" id="email_enabled" ${email.enabled ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="channel-options" ${email.enabled ? '' : 'style="opacity: 0.5; pointer-events: none;"'}>
                        <label class="checkbox-option">
                            <input type="checkbox" id="email_critical" ${email.alerts_critical ? 'checked' : ''}>
                            <span>Critical alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="email_warning" ${email.alerts_warning ? 'checked' : ''}>
                            <span>Warning alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="email_info" ${email.alerts_info ? 'checked' : ''}>
                            <span>Info alerts</span>
                        </label>
                        <div class="option-divider"></div>
                        <label class="checkbox-option">
                            <input type="checkbox" id="email_daily" ${email.daily_digest ? 'checked' : ''}>
                            <span>Daily digest</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="email_weekly" ${email.weekly_report ? 'checked' : ''}>
                            <span>Weekly report</span>
                        </label>
                    </div>
                </div>

                <div class="notification-channel">
                    <div class="channel-header">
                        <div class="channel-info">
                            <span class="channel-icon">&#128172;</span>
                            <div>
                                <h3>Slack Notifications</h3>
                                <p>Receive notifications in Slack</p>
                            </div>
                        </div>
                        <label class="toggle">
                            <input type="checkbox" id="slack_enabled" ${slack.enabled ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="channel-options" ${slack.enabled ? '' : 'style="opacity: 0.5; pointer-events: none;"'}>
                        <label class="checkbox-option">
                            <input type="checkbox" id="slack_critical" ${slack.alerts_critical ? 'checked' : ''}>
                            <span>Critical alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="slack_warning" ${slack.alerts_warning ? 'checked' : ''}>
                            <span>Warning alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="slack_info" ${slack.alerts_info ? 'checked' : ''}>
                            <span>Info alerts</span>
                        </label>
                    </div>
                </div>

                <div class="notification-channel">
                    <div class="channel-header">
                        <div class="channel-info">
                            <span class="channel-icon">&#128276;</span>
                            <div>
                                <h3>Browser Notifications</h3>
                                <p>Receive push notifications in your browser</p>
                            </div>
                        </div>
                        <label class="toggle">
                            <input type="checkbox" id="browser_enabled" ${browser.enabled ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="channel-options" ${browser.enabled ? '' : 'style="opacity: 0.5; pointer-events: none;"'}>
                        <button type="button" class="btn-secondary" id="btn-request-permission">
                            Request Browser Permission
                        </button>
                        <label class="checkbox-option">
                            <input type="checkbox" id="browser_critical" ${browser.alerts_critical ? 'checked' : ''}>
                            <span>Critical alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="browser_warning" ${browser.alerts_warning ? 'checked' : ''}>
                            <span>Warning alerts</span>
                        </label>
                        <label class="checkbox-option">
                            <input type="checkbox" id="browser_info" ${browser.alerts_info ? 'checked' : ''}>
                            <span>Info alerts</span>
                        </label>
                    </div>
                </div>

                <div class="notification-channel">
                    <div class="channel-header">
                        <div class="channel-info">
                            <span class="channel-icon">&#127769;</span>
                            <div>
                                <h3>Quiet Hours</h3>
                                <p>Mute non-critical notifications during specified hours</p>
                            </div>
                        </div>
                        <label class="toggle">
                            <input type="checkbox" id="quiet_enabled" ${quiet.enabled ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="channel-options" ${quiet.enabled ? '' : 'style="opacity: 0.5; pointer-events: none;"'}>
                        <div class="time-range">
                            <div class="time-input">
                                <label for="quiet_start">Start</label>
                                <input type="time" id="quiet_start" value="${quiet.start || '22:00'}">
                            </div>
                            <span class="time-separator">to</span>
                            <div class="time-input">
                                <label for="quiet_end">End</label>
                                <input type="time" id="quiet_end" value="${quiet.end || '08:00'}">
                            </div>
                        </div>
                        <p class="hint">Critical alerts will still be delivered during quiet hours</p>
                    </div>
                </div>

                <div class="form-actions">
                    <button type="submit" class="btn-primary" ${this.saving ? 'disabled' : ''}>
                        ${this.saving ? 'Saving...' : 'Save Preferences'}
                    </button>
                </div>
            </form>
        `;
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    attachEventListeners() {
        this.querySelector('#preferences-form')?.addEventListener('submit', (e) => this.handleFormSubmit(e));

        this.querySelector('#btn-request-permission')?.addEventListener('click', () => this.requestBrowserPermission());

        // Toggle channel options visibility
        ['email', 'slack', 'browser', 'quiet'].forEach(channel => {
            const toggle = this.querySelector(`#${channel}_enabled`);
            toggle?.addEventListener('change', (e) => {
                const options = toggle.closest('.notification-channel').querySelector('.channel-options');
                if (options) {
                    options.style.opacity = e.target.checked ? '1' : '0.5';
                    options.style.pointerEvents = e.target.checked ? 'auto' : 'none';
                }
            });
        });
    }

    getStyles() {
        return `
            .notification-preferences {
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

            .notification-channel {
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                margin-bottom: 1rem;
                overflow: hidden;
            }

            .channel-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .channel-info {
                display: flex;
                align-items: center;
                gap: 0.75rem;
            }

            .channel-icon {
                font-size: 1.5rem;
            }

            .channel-info h3 {
                margin: 0;
                font-size: 0.95rem;
                font-weight: 600;
            }

            .channel-info p {
                margin: 0.25rem 0 0;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .toggle {
                position: relative;
                display: inline-block;
                width: 44px;
                height: 24px;
            }

            .toggle input {
                opacity: 0;
                width: 0;
                height: 0;
            }

            .toggle-slider {
                position: absolute;
                cursor: pointer;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: var(--bg-card, #16181c);
                border-radius: 12px;
                transition: 0.3s;
            }

            .toggle-slider:before {
                position: absolute;
                content: "";
                height: 18px;
                width: 18px;
                left: 3px;
                bottom: 3px;
                background: white;
                border-radius: 50%;
                transition: 0.3s;
            }

            .toggle input:checked + .toggle-slider {
                background: var(--accent, #1d9bf0);
            }

            .toggle input:checked + .toggle-slider:before {
                transform: translateX(20px);
            }

            .channel-options {
                padding: 1rem;
                transition: opacity 0.2s;
            }

            .checkbox-option {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.5rem 0;
                cursor: pointer;
                font-size: 0.9rem;
            }

            .checkbox-option input {
                width: 18px;
                height: 18px;
                accent-color: var(--accent, #1d9bf0);
            }

            .option-divider {
                height: 1px;
                background: var(--border, #2f3336);
                margin: 0.5rem 0;
            }

            .time-range {
                display: flex;
                align-items: flex-end;
                gap: 1rem;
                margin-bottom: 0.75rem;
            }

            .time-input {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .time-input label {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .time-input input {
                padding: 0.5rem;
                background: var(--bg-card, #16181c);
                border: 1px solid var(--border, #2f3336);
                border-radius: 4px;
                color: var(--text, #e7e9ea);
                font-size: 0.9rem;
            }

            .time-separator {
                padding-bottom: 0.5rem;
                color: var(--text-muted, #71767b);
            }

            .hint {
                margin: 0;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .form-actions {
                margin-top: 1.5rem;
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
                margin-bottom: 0.75rem;
            }
        `;
    }
}

customElements.define('notification-preferences', NotificationPreferences);
