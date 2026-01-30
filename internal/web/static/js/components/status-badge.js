/**
 * Status Badge Web Component
 *
 * Usage:
 *   <dw-badge status="ok"></dw-badge>
 *   <dw-badge status="warning" pulse></dw-badge>
 *   <dw-badge status="error" size="lg">Custom Text</dw-badge>
 *
 * Attributes:
 *   - status: ok|warning|error|info|muted|pending|alerting|healthy|degraded|unhealthy
 *   - pulse: Add pulsing animation
 *   - size: sm|md|lg (default: md)
 */
class StatusBadge extends HTMLElement {
    static get observedAttributes() {
        return ['status', 'pulse', 'size'];
    }

    constructor() {
        super();
        this.attachShadow({ mode: 'open' });
    }

    connectedCallback() {
        this.render();
    }

    attributeChangedCallback() {
        this.render();
    }

    render() {
        const status = this.getAttribute('status') || 'muted';
        const pulse = this.hasAttribute('pulse');
        const size = this.getAttribute('size') || 'md';

        // Map status to colors
        const statusColors = {
            ok: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            success: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            healthy: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            up: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            resolved: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },
            met: { bg: 'rgba(0, 186, 124, 0.2)', color: '#00ba7c' },

            warning: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            pending: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            degraded: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            acknowledged: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },
            at_risk: { bg: 'rgba(255, 212, 0, 0.2)', color: '#ffd400' },

            error: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            alerting: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            unhealthy: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            down: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            triggered: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            critical: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },
            breached: { bg: 'rgba(244, 33, 46, 0.2)', color: '#f4212e' },

            info: { bg: 'rgba(29, 155, 240, 0.2)', color: '#1d9bf0' },

            muted: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' },
            unknown: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' },
            no_data: { bg: 'rgba(47, 51, 54, 1)', color: '#71767b' }
        };

        const colors = statusColors[status.toLowerCase()] || statusColors.muted;

        // Size mappings
        const sizes = {
            sm: { padding: '0.1rem 0.3rem', fontSize: '0.6rem' },
            md: { padding: '0.15rem 0.4rem', fontSize: '0.65rem' },
            lg: { padding: '0.2rem 0.5rem', fontSize: '0.7rem' }
        };
        const sizeStyle = sizes[size] || sizes.md;

        // Get display text
        const text = this.textContent.trim() || this.formatStatus(status);

        this.shadowRoot.innerHTML = `
            <style>
                :host {
                    display: inline-block;
                }
                .badge {
                    padding: ${sizeStyle.padding};
                    border-radius: 3px;
                    font-size: ${sizeStyle.fontSize};
                    font-weight: 600;
                    text-transform: uppercase;
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    background: ${colors.bg};
                    color: ${colors.color};
                    white-space: nowrap;
                    ${pulse ? 'animation: pulse 1s infinite;' : ''}
                }
                @keyframes pulse {
                    0%, 100% { opacity: 1; }
                    50% { opacity: 0.6; }
                }
            </style>
            <span class="badge">${text}</span>
        `;
    }

    formatStatus(status) {
        return status.replace(/_/g, ' ');
    }

    // Public API
    setStatus(status) {
        this.setAttribute('status', status);
    }

    getStatus() {
        return this.getAttribute('status');
    }
}

customElements.define('dw-badge', StatusBadge);
