/**
 * Toast Notifications System
 * Global notification manager for alerts and messages
 */
class ToastContainer extends HTMLElement {
    constructor() {
        super();
        this.toasts = [];
        this.maxToasts = 5;
        this._timeouts = new Map(); // Track timeouts for cleanup
        this._eventHandler = null;
        this._wsUnsubscribe = null;
    }

    connectedCallback() {
        this.render();
        // Expose global function
        window.showToast = this.addToast.bind(this);

        // Listen for custom events
        this._eventHandler = (e) => {
            if (e.detail) {
                this.addToast(e.detail);
            }
        };
        window.addEventListener('toast', this._eventHandler);

        // Listen for WebSocket notifications
        if (window.ws && typeof window.ws.subscribe === 'function') {
            this._wsUnsubscribe = window.ws.subscribe('notification', (data) => {
                if (data && typeof data === 'object') {
                    this.addToast({
                        type: data.severity || 'info',
                        title: data.title,
                        message: data.message,
                        duration: data.duration
                    });
                }
            });
        }
    }

    disconnectedCallback() {
        // Clear all pending timeouts to prevent memory leaks
        this._timeouts.forEach((timeoutId) => {
            clearTimeout(timeoutId);
        });
        this._timeouts.clear();

        // Remove event listener
        if (this._eventHandler) {
            window.removeEventListener('toast', this._eventHandler);
            this._eventHandler = null;
        }

        // Unsubscribe from WebSocket
        if (this._wsUnsubscribe && typeof this._wsUnsubscribe === 'function') {
            this._wsUnsubscribe();
            this._wsUnsubscribe = null;
        }
    }

    addToast(options) {
        // Validate options
        if (!options || typeof options !== 'object') {
            options = { message: String(options) };
        }

        const toast = {
            id: Date.now() + Math.random(),
            type: options.type || 'info', // info, success, warning, error
            title: options.title || '',
            message: options.message || '',
            duration: options.duration !== undefined ? options.duration : 5000,
            action: options.action || null, // { label, callback }
            timestamp: new Date()
        };

        this.toasts.unshift(toast);

        // Limit visible toasts
        if (this.toasts.length > this.maxToasts) {
            // Clear timeouts for removed toasts
            const removedToasts = this.toasts.slice(this.maxToasts);
            removedToasts.forEach(t => {
                if (this._timeouts.has(t.id)) {
                    clearTimeout(this._timeouts.get(t.id));
                    this._timeouts.delete(t.id);
                }
            });
            this.toasts = this.toasts.slice(0, this.maxToasts);
        }

        this.render();

        // Auto dismiss with tracked timeout
        if (toast.duration > 0) {
            const timeoutId = setTimeout(() => {
                this._timeouts.delete(toast.id);
                this.removeToast(toast.id);
            }, toast.duration);
            this._timeouts.set(toast.id, timeoutId);
        }

        return toast.id;
    }

    removeToast(id) {
        // Clear any pending timeout for this toast
        if (this._timeouts.has(id)) {
            clearTimeout(this._timeouts.get(id));
            this._timeouts.delete(id);
        }

        const index = this.toasts.findIndex(t => t.id === id);
        if (index !== -1) {
            // Add exit animation class
            const toastEl = this.querySelector(`[data-toast-id="${id}"]`);
            if (toastEl) {
                toastEl.classList.add('exiting');
                const animationTimeout = setTimeout(() => {
                    this.toasts = this.toasts.filter(t => t.id !== id);
                    this.render();
                }, 300);
                // Track animation timeout separately
                this._timeouts.set(`anim_${id}`, animationTimeout);
            } else {
                this.toasts = this.toasts.filter(t => t.id !== id);
                this.render();
            }
        }
    }

    handleAction(id) {
        const toast = this.toasts.find(t => t.id === id);
        if (toast && toast.action && toast.action.callback) {
            toast.action.callback();
        }
        this.removeToast(id);
    }

    render() {
        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="toast-container">
                ${this.toasts.map(toast => this.renderToast(toast)).join('')}
            </div>
        `;

        // Set up event delegation for toast actions (safer than inline onclick with getRootNode)
        this.querySelector('.toast-container')?.addEventListener('click', (e) => {
            const actionBtn = e.target.closest('[data-toast-action]');
            const closeBtn = e.target.closest('[data-toast-close]');

            if (actionBtn) {
                const id = parseFloat(actionBtn.dataset.toastAction);
                this.handleAction(id);
            } else if (closeBtn) {
                const id = parseFloat(closeBtn.dataset.toastClose);
                this.removeToast(id);
            }
        });
    }

    renderToast(toast) {
        const icon = this.getIcon(toast.type);

        return `
            <div class="toast toast-${toast.type}" data-toast-id="${toast.id}">
                <div class="toast-icon">${icon}</div>
                <div class="toast-content">
                    ${toast.title ? `<div class="toast-title">${this.escapeHtml(toast.title)}</div>` : ''}
                    ${toast.message ? `<div class="toast-message">${this.escapeHtml(toast.message)}</div>` : ''}
                </div>
                <div class="toast-actions">
                    ${toast.action ? `
                        <button class="toast-action-btn" data-toast-action="${toast.id}">
                            ${this.escapeHtml(toast.action.label)}
                        </button>
                    ` : ''}
                    <button class="toast-close" data-toast-close="${toast.id}">×</button>
                </div>
                ${toast.duration > 0 ? `
                    <div class="toast-progress">
                        <div class="toast-progress-bar" style="animation-duration: ${toast.duration}ms"></div>
                    </div>
                ` : ''}
            </div>
        `;
    }

    getIcon(type) {
        switch (type) {
            case 'success': return '✓';
            case 'warning': return '⚠';
            case 'error': return '✗';
            default: return 'ℹ';
        }
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    getStyles() {
        return `
            :host {
                position: fixed;
                top: 1rem;
                right: 1rem;
                z-index: 10000;
                pointer-events: none;
            }

            .toast-container {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
                max-width: 400px;
            }

            .toast {
                display: flex;
                align-items: flex-start;
                gap: 0.75rem;
                padding: 0.875rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                border-left: 4px solid var(--border, #2f3336);
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
                pointer-events: auto;
                animation: slideIn 0.3s ease-out;
                position: relative;
                overflow: hidden;
            }

            .toast.exiting {
                animation: slideOut 0.3s ease-in forwards;
            }

            @keyframes slideIn {
                from {
                    transform: translateX(100%);
                    opacity: 0;
                }
                to {
                    transform: translateX(0);
                    opacity: 1;
                }
            }

            @keyframes slideOut {
                from {
                    transform: translateX(0);
                    opacity: 1;
                }
                to {
                    transform: translateX(100%);
                    opacity: 0;
                }
            }

            .toast-info { border-left-color: var(--accent, #1d9bf0); }
            .toast-success { border-left-color: var(--success, #00ba7c); }
            .toast-warning { border-left-color: var(--warning, #ffd400); }
            .toast-error { border-left-color: var(--error, #f4212e); }

            .toast-icon {
                width: 24px;
                height: 24px;
                border-radius: 50%;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 0.8rem;
                font-weight: bold;
                flex-shrink: 0;
            }

            .toast-info .toast-icon {
                background: rgba(29, 155, 240, 0.2);
                color: var(--accent, #1d9bf0);
            }

            .toast-success .toast-icon {
                background: rgba(0, 186, 124, 0.2);
                color: var(--success, #00ba7c);
            }

            .toast-warning .toast-icon {
                background: rgba(255, 212, 0, 0.2);
                color: var(--warning, #ffd400);
            }

            .toast-error .toast-icon {
                background: rgba(244, 33, 46, 0.2);
                color: var(--error, #f4212e);
            }

            .toast-content {
                flex: 1;
                min-width: 0;
            }

            .toast-title {
                font-weight: 600;
                font-size: 0.9rem;
                margin-bottom: 0.25rem;
            }

            .toast-message {
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
                line-height: 1.4;
            }

            .toast-actions {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                flex-shrink: 0;
            }

            .toast-action-btn {
                background: var(--accent, #1d9bf0);
                border: none;
                border-radius: 4px;
                color: white;
                padding: 0.3rem 0.6rem;
                font-size: 0.75rem;
                cursor: pointer;
                font-weight: 500;
            }

            .toast-action-btn:hover {
                filter: brightness(1.1);
            }

            .toast-close {
                background: none;
                border: none;
                color: var(--text-muted, #71767b);
                font-size: 1.25rem;
                cursor: pointer;
                padding: 0;
                line-height: 1;
            }

            .toast-close:hover {
                color: var(--text, #e7e9ea);
            }

            .toast-progress {
                position: absolute;
                bottom: 0;
                left: 0;
                right: 0;
                height: 3px;
                background: rgba(255, 255, 255, 0.1);
            }

            .toast-progress-bar {
                height: 100%;
                width: 100%;
                transform-origin: left;
                animation: progress linear forwards;
            }

            @keyframes progress {
                from { transform: scaleX(1); }
                to { transform: scaleX(0); }
            }

            .toast-info .toast-progress-bar { background: var(--accent, #1d9bf0); }
            .toast-success .toast-progress-bar { background: var(--success, #00ba7c); }
            .toast-warning .toast-progress-bar { background: var(--warning, #ffd400); }
            .toast-error .toast-progress-bar { background: var(--error, #f4212e); }

            @media (max-width: 500px) {
                :host {
                    left: 1rem;
                    right: 1rem;
                }

                .toast-container {
                    max-width: none;
                }
            }
        `;
    }
}

customElements.define('toast-container', ToastContainer);

// Convenience functions for programmatic use
window.toast = {
    info: (message, title = '', options = {}) => window.showToast({ type: 'info', message, title, ...options }),
    success: (message, title = '', options = {}) => window.showToast({ type: 'success', message, title, ...options }),
    warning: (message, title = '', options = {}) => window.showToast({ type: 'warning', message, title, ...options }),
    error: (message, title = '', options = {}) => window.showToast({ type: 'error', message, title, ...options })
};
