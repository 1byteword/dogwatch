/**
 * Base Web Component with performance optimizations:
 * - Shadow DOM with adoptedStyleSheets (CSS parsed once, shared)
 * - Template caching (HTML parsed once)
 * - Batched updates via requestAnimationFrame
 */

// Shared stylesheet cache - parse CSS once per component class
const styleCache = new Map();

// Template cache - parse HTML structure once
const templateCache = new Map();

class BaseComponent extends HTMLElement {
    constructor() {
        super();
        this._updateScheduled = false;
        this._mounted = false;
    }

    // Override in subclass - return CSS string
    static get styles() { return ''; }

    // Override in subclass - return component name for caching
    static get componentName() { return 'base-component'; }

    connectedCallback() {
        // Create shadow root if using shadow DOM
        if (this.constructor.useShadowDom !== false) {
            if (!this.shadowRoot) {
                this.attachShadow({ mode: 'open' });
                this._applyStyles();
            }
        }
        this._mounted = true;
        this.onMount();
    }

    disconnectedCallback() {
        this._mounted = false;
        this.onUnmount();
    }

    // Override in subclass
    onMount() {}
    onUnmount() {}

    _applyStyles() {
        const name = this.constructor.componentName;

        // Check if we already have a parsed stylesheet
        if (!styleCache.has(name)) {
            const css = this.constructor.styles;
            if (css) {
                // Use adoptedStyleSheets if supported (much faster)
                if ('adoptedStyleSheets' in Document.prototype) {
                    const sheet = new CSSStyleSheet();
                    sheet.replaceSync(css);
                    styleCache.set(name, sheet);
                } else {
                    // Fallback: cache the CSS string
                    styleCache.set(name, css);
                }
            }
        }

        const cached = styleCache.get(name);
        if (cached instanceof CSSStyleSheet) {
            this.shadowRoot.adoptedStyleSheets = [cached];
        } else if (cached) {
            // Fallback for older browsers
            const style = document.createElement('style');
            style.textContent = cached;
            this.shadowRoot.appendChild(style);
        }
    }

    // Schedule batched render via rAF
    scheduleUpdate() {
        if (this._updateScheduled || !this._mounted) return;
        this._updateScheduled = true;
        requestAnimationFrame(() => {
            this._updateScheduled = false;
            if (this._mounted) this.render();
        });
    }

    // Get or create root element for rendering
    get root() {
        return this.shadowRoot || this;
    }

    // Efficient DOM update - only updates changed content
    updateContent(selector, html) {
        const el = this.root.querySelector(selector);
        if (el && el.innerHTML !== html) {
            el.innerHTML = html;
        }
    }

    // Update text content only (faster than innerHTML)
    updateText(selector, text) {
        const el = this.root.querySelector(selector);
        if (el && el.textContent !== text) {
            el.textContent = text;
        }
    }

    // Toggle class efficiently
    toggleClass(selector, className, force) {
        const el = this.root.querySelector(selector);
        if (el) el.classList.toggle(className, force);
    }

    // Override in subclass
    render() {}

    // Utility: escape HTML
    escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }

    // Utility: format duration
    formatDuration(ms) {
        if (ms < 0) ms = 0;
        const s = Math.floor(ms / 1000);
        if (s < 60) return `${s}s`;
        const m = Math.floor(s / 60);
        if (m < 60) return `${m}m`;
        const h = Math.floor(m / 60);
        if (h < 24) return `${h}h ${m % 60}m`;
        const d = Math.floor(h / 24);
        return `${d}d ${h % 24}h`;
    }

    // Utility: format relative time
    formatRelativeTime(timestamp) {
        if (!timestamp) return '—';
        const d = new Date(timestamp);
        const diff = Date.now() - d.getTime();
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    // Utility: format currency
    formatCurrency(amount) {
        if (!amount || amount === 0) return '$0';
        if (amount >= 1000000) return `$${(amount / 1000000).toFixed(1)}M`;
        if (amount >= 1000) return `$${(amount / 1000).toFixed(0)}k`;
        return `$${amount.toFixed(0)}`;
    }

    // Utility: format bytes
    formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
    }

    // Utility: show error toast notification
    showError(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'error', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.error(message, title);
        } else {
            console.error(`[${this.constructor.componentName}] ${title}: ${message}`);
        }
    }

    // Utility: show success toast notification
    showSuccess(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'success', title, message, duration: 3000 });
        } else if (window.toast) {
            window.toast.success(message, title);
        }
    }

    // Utility: show warning toast notification
    showWarning(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'warning', title, message, duration: 5000 });
        } else if (window.toast) {
            window.toast.warning(message, title);
        }
    }

    // Utility: show info toast notification
    showInfo(title, message) {
        if (window.showToast) {
            window.showToast({ type: 'info', title, message, duration: 4000 });
        } else if (window.toast) {
            window.toast.info(message, title);
        }
    }

    // Utility: fetch with error handling
    async fetchWithErrorHandling(url, options = {}) {
        try {
            const response = await fetch(url, options);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            return response;
        } catch (e) {
            const errorTitle = options.errorTitle || 'Request Failed';
            this.showError(errorTitle, e.message);
            throw e;
        }
    }
}

// Export for use
window.BaseComponent = BaseComponent;
