/**
 * Geo Map Component
 * Geographic visualization using MapLibre GL JS (open-source, no API key needed)
 */
class GeoMap extends HTMLElement {
    constructor() {
        super();
        this.map = null;
        this.data = null;
        this.markers = [];
        this.resizeObserver = null;
    }

    connectedCallback() {
        this.render();
        this.initMap();

        this.resizeObserver = new ResizeObserver(() => {
            if (this.map) {
                this.map.resize();
            }
        });
        this.resizeObserver.observe(this);
    }

    disconnectedCallback() {
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
        }
        if (this.map) {
            this.map.remove();
            this.map = null;
        }
    }

    static get observedAttributes() {
        return ['metric', 'time-range', 'style-url'];
    }

    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue && this.isConnected) {
            if (name === 'style-url' && this.map) {
                this.map.setStyle(this.styleUrl);
            } else {
                this.loadData();
            }
        }
    }

    get metric() { return this.getAttribute('metric') || 'requests'; }
    get timeRange() { return this.getAttribute('time-range') || '1h'; }
    get styleUrl() {
        return this.getAttribute('style-url') ||
               // CartoDB Dark Matter - free, no API key, perfect for dark UIs
               'https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json';
    }

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
                .geomap-controls {
                    display: flex;
                    gap: 0.5rem;
                }
                .geomap-controls select {
                    background: var(--bg-primary, #0f1419);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 4px;
                    padding: 0.3rem 0.5rem;
                    color: var(--text-primary, #e7e9ea);
                    font-size: 0.75rem;
                    cursor: pointer;
                }
                .geomap-body {
                    flex: 1;
                    position: relative;
                    min-height: 300px;
                }
                .geomap-map {
                    width: 100%;
                    height: 100%;
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
                .geomap-loading {
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    height: 100%;
                    color: var(--text-muted, #71767b);
                }
                .geomap-marker {
                    border-radius: 50%;
                    display: flex;
                    align-items: center;
                    justify-content: center;
                    color: white;
                    font-size: 10px;
                    font-weight: 600;
                    cursor: pointer;
                    transition: transform 0.2s ease;
                    box-shadow: 0 2px 8px rgba(0,0,0,0.4);
                    border: 2px solid rgba(255,255,255,0.3);
                }
                .geomap-marker:hover {
                    transform: scale(1.2);
                    z-index: 10;
                }
                .maplibregl-popup-content {
                    background: var(--bg-elevated, #1a1f2e) !important;
                    color: var(--text-primary, #e7e9ea) !important;
                    border-radius: 8px !important;
                    padding: 12px !important;
                    box-shadow: 0 4px 16px rgba(0,0,0,0.4) !important;
                    border: 1px solid var(--border-color, #2f3336) !important;
                }
                .maplibregl-popup-tip {
                    border-top-color: var(--bg-elevated, #1a1f2e) !important;
                }
                .maplibregl-popup-close-button {
                    color: var(--text-muted, #71767b) !important;
                    font-size: 18px !important;
                    padding: 4px 8px !important;
                }
                .maplibregl-popup-close-button:hover {
                    color: var(--text-primary, #e7e9ea) !important;
                    background: transparent !important;
                }
                .geomap-popup-title {
                    font-weight: 600;
                    margin-bottom: 8px;
                    font-size: 14px;
                }
                .geomap-popup-row {
                    display: flex;
                    justify-content: space-between;
                    gap: 16px;
                    font-size: 12px;
                    margin: 4px 0;
                }
                .geomap-popup-label {
                    color: var(--text-muted, #71767b);
                }
                .geomap-popup-value {
                    font-weight: 500;
                }
                .geomap-popup-error {
                    color: var(--color-error, #f43f5e);
                }
                .geomap-legend {
                    position: absolute;
                    bottom: 24px;
                    left: 12px;
                    background: var(--bg-elevated, #1a1f2e);
                    border: 1px solid var(--border-color, #2f3336);
                    border-radius: 6px;
                    padding: 10px;
                    font-size: 11px;
                    z-index: 1;
                }
                .geomap-legend-title {
                    font-weight: 600;
                    margin-bottom: 6px;
                }
                .geomap-legend-gradient {
                    width: 100px;
                    height: 8px;
                    background: linear-gradient(to right, #22c55e, #f59e0b, #f43f5e);
                    border-radius: 4px;
                    margin-bottom: 4px;
                }
                .geomap-legend-labels {
                    display: flex;
                    justify-content: space-between;
                    color: var(--text-muted, #71767b);
                }
                .maplibregl-ctrl-group {
                    background: var(--bg-elevated, #1a1f2e) !important;
                    border: 1px solid var(--border-color, #2f3336) !important;
                }
                .maplibregl-ctrl-group button {
                    background-color: var(--bg-elevated, #1a1f2e) !important;
                }
                .maplibregl-ctrl-group button:hover {
                    background-color: var(--bg-primary, #0f1419) !important;
                }
                .maplibregl-ctrl-group button span {
                    filter: invert(1);
                }
            </style>
            <div class="geomap-container">
                <div class="geomap-header">
                    <div class="geomap-title">
                        <span>🌐</span>
                        <span>Geographic Distribution</span>
                    </div>
                    <div class="geomap-controls">
                        <select id="style-select">
                            <option value="dark">Dark</option>
                            <option value="light">Light</option>
                            <option value="voyager">Voyager</option>
                        </select>
                    </div>
                </div>
                <div class="geomap-body" id="body">
                    <div class="geomap-loading">Loading map...</div>
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
            </div>
        `;

        // Style switcher
        const styleSelect = this.querySelector('#style-select');
        styleSelect.addEventListener('change', (e) => {
            const styles = {
                dark: 'https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json',
                light: 'https://basemaps.cartocdn.com/gl/positron-gl-style/style.json',
                voyager: 'https://basemaps.cartocdn.com/gl/voyager-gl-style/style.json'
            };
            if (this.map) {
                this.map.setStyle(styles[e.target.value]);
                // Re-add markers after style loads
                this.map.once('styledata', () => this.updateMarkers());
            }
        });
    }

    async initMap() {
        const body = this.querySelector('#body');
        if (!body) return;

        // Load MapLibre library
        if (!window.maplibregl) {
            try {
                if (window.LibLoader && window.LibLoader.load) {
                    await Promise.all([
                        window.LibLoader.load('maplibre'),
                        window.LibLoader.load('maplibre-css')
                    ]);
                } else if (window.Loader && window.Loader.load) {
                    await window.Loader.load('maplibre');
                } else {
                    // Fallback: load directly
                    await this.loadMapLibreDirect();
                }
            } catch (e) {
                console.error('Failed to load MapLibre:', e);
                body.innerHTML = `<div class="geomap-loading">Failed to load map library: ${e.message}</div>`;
                return;
            }
        }

        // Create map container
        body.innerHTML = `
            <div class="geomap-map" id="map"></div>
            <div class="geomap-legend">
                <div class="geomap-legend-title">Traffic Volume</div>
                <div class="geomap-legend-gradient"></div>
                <div class="geomap-legend-labels">
                    <span>Low</span>
                    <span>High</span>
                </div>
            </div>
        `;

        const mapContainer = this.querySelector('#map');

        try {
            this.map = new maplibregl.Map({
                container: mapContainer,
                style: this.styleUrl,
                center: [0, 20],
                zoom: 1.3,
                attributionControl: false
            });

            // Add compact attribution
            this.map.addControl(new maplibregl.AttributionControl({
                compact: true
            }));

            // Add navigation controls
            this.map.addControl(new maplibregl.NavigationControl({
                showCompass: false
            }), 'top-right');

            this.map.on('load', () => {
                this.loadData();
            });

        } catch (e) {
            console.error('Failed to initialize MapLibre:', e);
            body.innerHTML = `<div class="geomap-loading">Failed to initialize map: ${e.message}</div>`;
        }
    }

    async loadData() {
        try {
            const resp = await fetch(`/api/geo/distribution?metric=${this.metric}&range=${this.timeRange}`);
            if (!resp.ok) {
                this.data = this.generateDemoData();
            } else {
                this.data = await resp.json();
            }
            this.updateMarkers();
        } catch (e) {
            this.data = this.generateDemoData();
            this.updateMarkers();
        }
    }

    generateDemoData() {
        return {
            regions: [
                { code: 'US-W', name: 'US West (Oregon)', lat: 45.5152, lng: -122.6784, requests: 45000, errors: 120, latency: 45 },
                { code: 'US-E', name: 'US East (Virginia)', lat: 37.4316, lng: -78.6569, requests: 38000, errors: 95, latency: 52 },
                { code: 'EU-W', name: 'EU West (Ireland)', lat: 53.1424, lng: -7.6921, requests: 28000, errors: 85, latency: 120 },
                { code: 'EU-C', name: 'EU Central (Frankfurt)', lat: 50.1109, lng: 8.6821, requests: 22000, errors: 65, latency: 115 },
                { code: 'APAC', name: 'Asia Pacific (Tokyo)', lat: 35.6762, lng: 139.6503, requests: 18000, errors: 45, latency: 180 },
                { code: 'SA', name: 'South America (São Paulo)', lat: -23.5505, lng: -46.6333, requests: 8000, errors: 25, latency: 200 },
                { code: 'AU', name: 'Australia (Sydney)', lat: -33.8688, lng: 151.2093, requests: 5000, errors: 12, latency: 220 },
                { code: 'APAC-S', name: 'Asia Pacific (Singapore)', lat: 1.3521, lng: 103.8198, requests: 12000, errors: 35, latency: 160 },
            ],
            total: 176000
        };
    }

    updateMarkers() {
        if (!this.map || !this.data) return;

        // Remove existing markers
        this.markers.forEach(m => m.remove());
        this.markers = [];

        const { regions, total } = this.data;

        if (!regions || regions.length === 0) return;

        const maxRequests = Math.max(...regions.map(r => r.requests));

        regions.forEach(r => {
            const intensity = r.requests / maxRequests;
            const color = this.getHeatColor(intensity);
            const size = 18 + intensity * 22;

            // Create marker element
            const el = document.createElement('div');
            el.className = 'geomap-marker';
            el.style.width = size + 'px';
            el.style.height = size + 'px';
            el.style.backgroundColor = color;

            const errorRate = (r.errors / r.requests * 100).toFixed(2);
            const errorClass = errorRate > 1 ? 'geomap-popup-error' : '';

            // Create popup
            const popup = new maplibregl.Popup({
                offset: 25,
                closeButton: true,
                closeOnClick: false,
                maxWidth: '280px'
            }).setHTML(`
                <div class="geomap-popup-title">${r.name}</div>
                <div class="geomap-popup-row">
                    <span class="geomap-popup-label">Requests</span>
                    <span class="geomap-popup-value">${r.requests.toLocaleString()}</span>
                </div>
                <div class="geomap-popup-row">
                    <span class="geomap-popup-label">Errors</span>
                    <span class="geomap-popup-value ${errorClass}">${r.errors.toLocaleString()} (${errorRate}%)</span>
                </div>
                <div class="geomap-popup-row">
                    <span class="geomap-popup-label">Avg Latency</span>
                    <span class="geomap-popup-value">${r.latency}ms</span>
                </div>
                <div class="geomap-popup-row">
                    <span class="geomap-popup-label">% of Total</span>
                    <span class="geomap-popup-value">${(r.requests / total * 100).toFixed(1)}%</span>
                </div>
            `);

            const marker = new maplibregl.Marker({ element: el })
                .setLngLat([r.lng, r.lat])
                .setPopup(popup)
                .addTo(this.map);

            this.markers.push(marker);
        });

        // Update stats
        const topRegion = regions.reduce((a, b) => a.requests > b.requests ? a : b);
        const statTotal = this.querySelector('#stat-total');
        const statRegions = this.querySelector('#stat-regions');
        const statTop = this.querySelector('#stat-top');

        if (statTotal) statTotal.textContent = total.toLocaleString();
        if (statRegions) statRegions.textContent = regions.length;
        if (statTop) statTop.textContent = `${topRegion.name.split('(')[0].trim()} (${(topRegion.requests / total * 100).toFixed(0)}%)`;
    }

    getHeatColor(intensity) {
        // Smooth gradient: green -> yellow -> red
        if (intensity < 0.33) {
            return '#22c55e'; // Green
        } else if (intensity < 0.66) {
            return '#f59e0b'; // Yellow/Orange
        } else {
            return '#f43f5e'; // Red
        }
    }

    // Fallback direct loader if no loader available
    async loadMapLibreDirect() {
        return new Promise((resolve, reject) => {
            // Load CSS
            const link = document.createElement('link');
            link.rel = 'stylesheet';
            link.href = 'https://unpkg.com/maplibre-gl@4.1.2/dist/maplibre-gl.css';
            document.head.appendChild(link);

            // Load JS
            const script = document.createElement('script');
            script.src = 'https://unpkg.com/maplibre-gl@4.1.2/dist/maplibre-gl.js';
            script.onload = () => resolve();
            script.onerror = () => reject(new Error('Failed to load MapLibre script'));
            document.head.appendChild(script);
        });
    }
}

customElements.define('geo-map', GeoMap);
