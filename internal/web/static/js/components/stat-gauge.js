/**
 * Stat Gauge Component
 * Big number display with optional gauge arc
 */
class StatGauge extends HTMLElement {
    constructor() {
        super();
        this.value = 0;
        this.animationFrame = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Auto-refresh
        this.refreshInterval = setInterval(() => this.loadData(), 10000);
    }

    disconnectedCallback() {
        if (this.refreshInterval) clearInterval(this.refreshInterval);
        if (this.animationFrame) cancelAnimationFrame(this.animationFrame);
    }

    static get observedAttributes() {
        return ['metric', 'title', 'unit', 'min', 'max', 'thresholds', 'show-gauge'];
    }

    get metric() { return this.getAttribute('metric') || ''; }
    get title() { return this.getAttribute('title') || 'Metric'; }
    get unit() { return this.getAttribute('unit') || ''; }
    get min() { return parseFloat(this.getAttribute('min')) || 0; }
    get max() { return parseFloat(this.getAttribute('max')) || 100; }
    get showGauge() { return this.getAttribute('show-gauge') !== 'false'; }
    get thresholds() {
        try {
            return JSON.parse(this.getAttribute('thresholds') || '{}');
        } catch { return {}; }
    }

    render() {
        this.innerHTML = `
            <style>
                .stat-gauge-container {
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    padding: 1rem;
                }
                .stat-gauge-title {
                    font-size: 0.85rem;
                    color: var(--text-muted, #71767b);
                    margin-bottom: 0.5rem;
                }
                .stat-gauge-value-container {
                    position: relative;
                    display: flex;
                    flex-direction: column;
                    align-items: center;
                }
                .stat-gauge-svg {
                    width: 120px;
                    height: 70px;
                }
                .stat-gauge-bg {
                    fill: none;
                    stroke: var(--border-color, #2f3336);
                    stroke-width: 8;
                    stroke-linecap: round;
                }
                .stat-gauge-fill {
                    fill: none;
                    stroke-width: 8;
                    stroke-linecap: round;
                    transition: stroke-dashoffset 0.5s ease, stroke 0.3s ease;
                }
                .stat-gauge-value {
                    font-size: 2rem;
                    font-weight: 700;
                    line-height: 1;
                    margin-top: 0.5rem;
                }
                .stat-gauge-unit {
                    font-size: 0.9rem;
                    color: var(--text-muted, #71767b);
                    margin-top: 0.25rem;
                }
                .stat-gauge-trend {
                    display: flex;
                    align-items: center;
                    gap: 0.25rem;
                    font-size: 0.8rem;
                    margin-top: 0.5rem;
                }
                .stat-gauge-trend.up { color: #22c55e; }
                .stat-gauge-trend.down { color: #f43f5e; }
                .stat-gauge-trend.neutral { color: var(--text-muted, #71767b); }
            </style>
            <div class="stat-gauge-container">
                <div class="stat-gauge-title">${this.title}</div>
                <div class="stat-gauge-value-container">
                    ${this.showGauge ? `
                    <svg class="stat-gauge-svg" viewBox="0 0 120 70">
                        <path class="stat-gauge-bg" d="M 10 60 A 50 50 0 0 1 110 60"></path>
                        <path class="stat-gauge-fill" id="gauge-fill" d="M 10 60 A 50 50 0 0 1 110 60"></path>
                    </svg>
                    ` : ''}
                    <div class="stat-gauge-value" id="value">--</div>
                    <div class="stat-gauge-unit">${this.unit}</div>
                </div>
                <div class="stat-gauge-trend neutral" id="trend">
                    <span id="trend-icon">→</span>
                    <span id="trend-value">--</span>
                </div>
            </div>
        `;
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/metrics/current?metric=${this.metric}`);
            let data;
            if (!resp.ok) {
                data = this.generateDemoData();
            } else {
                data = await resp.json();
            }
            this.updateDisplay(data);
        } catch (e) {
            this.updateDisplay(this.generateDemoData());
        }
    }

    generateDemoData() {
        const metrics = {
            'cpu_usage': { value: 45 + Math.random() * 30, trend: Math.random() * 10 - 5 },
            'memory_usage': { value: 60 + Math.random() * 20, trend: Math.random() * 5 - 2 },
            'requests_per_second': { value: 800 + Math.random() * 400, trend: Math.random() * 100 - 50 },
            'error_rate': { value: Math.random() * 2, trend: Math.random() * 0.5 - 0.25 },
            'active_connections': { value: Math.floor(100 + Math.random() * 200), trend: Math.floor(Math.random() * 20 - 10) },
        };
        return metrics[this.metric] || { value: 50 + Math.random() * 50, trend: Math.random() * 10 - 5 };
    }

    updateDisplay(data) {
        const { value, trend } = data;
        const prevValue = this.value;
        this.value = value;

        // Animate value
        this.animateValue(prevValue, value);

        // Update gauge
        if (this.showGauge) {
            this.updateGauge(value);
        }

        // Update trend
        const trendEl = this.querySelector('#trend');
        const trendIcon = this.querySelector('#trend-icon');
        const trendValue = this.querySelector('#trend-value');

        if (trend > 0.5) {
            trendEl.className = 'stat-gauge-trend up';
            trendIcon.textContent = '↑';
            trendValue.textContent = `+${this.formatValue(trend)}`;
        } else if (trend < -0.5) {
            trendEl.className = 'stat-gauge-trend down';
            trendIcon.textContent = '↓';
            trendValue.textContent = this.formatValue(trend);
        } else {
            trendEl.className = 'stat-gauge-trend neutral';
            trendIcon.textContent = '→';
            trendValue.textContent = 'stable';
        }
    }

    animateValue(from, to) {
        const valueEl = this.querySelector('#value');
        if (!valueEl) return;

        const duration = 500;
        const start = performance.now();

        const animate = (now) => {
            const elapsed = now - start;
            const progress = Math.min(elapsed / duration, 1);
            const eased = 1 - Math.pow(1 - progress, 3); // ease-out cubic

            const current = from + (to - from) * eased;
            valueEl.textContent = this.formatValue(current);
            valueEl.style.color = this.getValueColor(current);

            if (progress < 1) {
                this.animationFrame = requestAnimationFrame(animate);
            }
        };

        this.animationFrame = requestAnimationFrame(animate);
    }

    updateGauge(value) {
        const fill = this.querySelector('#gauge-fill');
        if (!fill) return;

        const percentage = Math.min(1, Math.max(0, (value - this.min) / (this.max - this.min)));

        // Arc length calculation
        const arcLength = Math.PI * 50; // radius 50, semicircle
        const dashOffset = arcLength * (1 - percentage);

        fill.style.strokeDasharray = arcLength;
        fill.style.strokeDashoffset = dashOffset;
        fill.style.stroke = this.getValueColor(value);
    }

    getValueColor(value) {
        const { warning, critical } = this.thresholds;

        if (critical !== undefined && value >= critical) return '#f43f5e';
        if (warning !== undefined && value >= warning) return '#f59e0b';
        return '#22c55e';
    }

    formatValue(value) {
        if (value >= 1000000) return (value / 1000000).toFixed(1) + 'M';
        if (value >= 1000) return (value / 1000).toFixed(1) + 'K';
        if (Number.isInteger(value)) return value.toString();
        return value.toFixed(1);
    }
}

customElements.define('stat-gauge', StatGauge);
