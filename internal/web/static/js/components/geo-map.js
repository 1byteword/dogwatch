/**
 * Geo Map Component
 * Geographic visualization of traffic/errors by region
 */
class GeoMap extends HTMLElement {
    constructor() {
        super();
        this.data = null;
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.loadData();

        // Handle resize
        this.resizeObserver = new ResizeObserver(() => {
            if (this.data) {
                this.updateMap();
            }
        });
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
    }

    static get observedAttributes() {
        return ['metric', 'time-range'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            this.loadData();
        }
    }

    get metric() { return this.getAttribute('metric') || 'requests'; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }

    render() {
        this.innerHTML = `
            <style>
                .geomap-container {
                    display: flex;
                    flex-direction: column;
                    height: 100%;
                    background: var(--bg-card, #16181c);
                    border-radius: 8px;
                    overflow: hidden;
                }
                .geomap-header {
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-bottom: 1px solid var(--border-color, #2f3336);
                }
                .geomap-title {
                    font-weight: 600;
                    font-size: 0.9rem;
                    display: flex;
                    align-items: center;
                    gap: 0.5rem;
                }
                .geomap-body {
                    flex: 1;
                    position: relative;
                    min-height: 300px;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                }
                .geomap-svg {
                    width: 100%;
                    height: 100%;
                }
                .geomap-region {
                    fill: var(--bg-elevated, #1a1f2e);
                    stroke: var(--border-color, #2f3336);
                    stroke-width: 0.5;
                    transition: fill 0.2s ease;
                }
                .geomap-region:hover {
                    stroke: var(--color-info, #1d9bf0);
                    stroke-width: 1;
                }
                .geomap-dot {
                    cursor: pointer;
                    transition: r 0.2s ease;
                }
                .geomap-dot:hover {
                    r: 12;
                }
                .geomap-tooltip {
                    position: fixed;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.8rem;
                    pointer-events: none;
                    z-index: 1000;
                    display: none;
                }
                .geomap-legend {
                    position: absolute;
                    bottom: 1rem;
                    left: 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 0.75rem;
                    font-size: 0.75rem;
                }
                .geomap-legend-gradient {
                    width: 100px;
                    height: 8px;
                    background: linear-gradient(to right, #22c55e, #f59e0b, #f43f5e);
                    border-radius: 4px;
                    margin-bottom: 0.5rem;
                }
                .geomap-legend-labels {
                    display: flex;
                    justify-content: space-between;
                    color: var(--text-muted, #71767b);
                }
                .geomap-empty {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
                .geomap-stats {
                    display: flex;
                    gap: 1.5rem;
                    padding: 0.75rem 1rem;
                    background: var(--bg-elevated, #1a1f2e);
                    border-top: 1px solid var(--border-color, #2f3336);
                    font-size: 0.8rem;
                }
                .geomap-stat {
                    display: flex;
                    gap: 0.5rem;
                }
                .geomap-stat-label {
                    color: var(--text-muted, #71767b);
                }
            </style>
            <div class="geomap-container">
                <div class="geomap-header">
                    <div class="geomap-title">
                        <span>&#127758;</span>
                        <span>Geographic Distribution</span>
                    </div>
                </div>
                <div class="geomap-body" id="body">
                    <svg class="geomap-svg" id="svg" viewBox="0 0 800 400"></svg>
                    <div class="geomap-legend">
                        <div class="geomap-legend-gradient"></div>
                        <div class="geomap-legend-labels">
                            <span>Low</span>
                            <span>High</span>
                        </div>
                    </div>
                </div>
                <div class="geomap-stats">
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Total Requests:</span>
                        <span id="stat-total">0</span>
                    </div>
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Regions:</span>
                        <span id="stat-regions">0</span>
                    </div>
                    <div class="geomap-stat">
                        <span class="geomap-stat-label">Top Region:</span>
                        <span id="stat-top">-</span>
                    </div>
                </div>
                <div class="geomap-tooltip" id="tooltip"></div>
            </div>
        `;

        this.renderMap();
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/geo/distribution?metric=${this.metric}&range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateMap();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateMap();
        }
    }

    generateDemoData() {
        return {
            regions: [
                { code: 'US-W', name: 'US West', lat: 37.7749, lng: -122.4194, requests: 45000, errors: 120, latency: 45 },
                { code: 'US-E', name: 'US East', lat: 40.7128, lng: -74.0060, requests: 38000, errors: 95, latency: 52 },
                { code: 'EU-W', name: 'EU West', lat: 51.5074, lng: -0.1278, requests: 28000, errors: 85, latency: 120 },
                { code: 'EU-C', name: 'EU Central', lat: 52.5200, lng: 13.4050, requests: 22000, errors: 65, latency: 115 },
                { code: 'APAC', name: 'Asia Pacific', lat: 35.6762, lng: 139.6503, requests: 18000, errors: 45, latency: 180 },
                { code: 'SA', name: 'South America', lat: -23.5505, lng: -46.6333, requests: 8000, errors: 25, latency: 200 },
                { code: 'AU', name: 'Australia', lat: -33.8688, lng: 151.2093, requests: 5000, errors: 12, latency: 220 },
            ],
            total: 164000
        };
    }

    renderMap() {
        const svg = this.querySelector('#svg');
        if (!svg) return;

        // Simple world map outline (simplified paths)
        svg.innerHTML = `
            <defs>
                <radialGradient id="dotGradient">
                    <stop offset="0%" stop-color="rgba(59, 130, 246, 0.8)"/>
                    <stop offset="100%" stop-color="rgba(59, 130, 246, 0.2)"/>
                </radialGradient>
            </defs>
            <g id="regions"></g>
            <g id="dots"></g>
        `;
    }

    updateMap() {
        if (!this.data) return;

        const svg = this.querySelector('#svg');
        const dotsGroup = svg?.querySelector('#dots');
        const tooltip = this.querySelector('#tooltip');

        if (!dotsGroup) return;

        const { regions, total } = this.data;

        if (!regions || regions.length === 0) {
            const body = this.querySelector('.geomap-body');
            if (body) {
                body.innerHTML = '<div class="geomap-empty">No geographic data available</div>';
            }
            return;
        }
        const maxRequests = Math.max(...regions.map(r => r.requests));

        // Convert lat/lng to SVG coordinates (simple equirectangular projection)
        const toX = lng => ((lng + 180) / 360) * 800;
        const toY = lat => ((90 - lat) / 180) * 400;

        dotsGroup.innerHTML = regions.map(r => {
            const x = toX(r.lng);
            const y = toY(r.lat);
            const radius = 5 + (r.requests / maxRequests) * 15;
            const color = this.getHeatColor(r.requests / maxRequests);

            return `
                <circle class="geomap-dot" cx="${x}" cy="${y}" r="${radius}"
                        fill="${color}" fill-opacity="0.7"
                        data-region="${r.code}"/>
            `;
        }).join('');

        // Tooltip events
        dotsGroup.querySelectorAll('.geomap-dot').forEach((dot, i) => {
            const r = regions[i];

            dot.addEventListener('mouseenter', (e) => {
                tooltip.innerHTML = `
                    <div style="font-weight:600;margin-bottom:0.5rem">${r.name}</div>
                    <div>Requests: ${r.requests.toLocaleString()}</div>
                    <div>Errors: ${r.errors} (${(r.errors/r.requests*100).toFixed(2)}%)</div>
                    <div>Avg Latency: ${r.latency}ms</div>
                `;
                tooltip.style.display = 'block';
            });

            dot.addEventListener('mousemove', (e) => {
                tooltip.style.left = (e.clientX + 10) + 'px';
                tooltip.style.top = (e.clientY + 10) + 'px';
            });

            dot.addEventListener('mouseleave', () => {
                tooltip.style.display = 'none';
            });
        });

        // Update stats
        const topRegion = regions.reduce((a, b) => a.requests > b.requests ? a : b);
        this.querySelector('#stat-total').textContent = total.toLocaleString();
        this.querySelector('#stat-regions').textContent = regions.length;
        this.querySelector('#stat-top').textContent = `${topRegion.name} (${(topRegion.requests/total*100).toFixed(0)}%)`;
    }

    getHeatColor(intensity) {
        if (intensity < 0.3) return '#22c55e';
        if (intensity < 0.6) return '#f59e0b';
        return '#f43f5e';
    }
}

customElements.define('geo-map', GeoMap);
