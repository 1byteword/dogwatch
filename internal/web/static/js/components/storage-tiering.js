/**
 * Storage Tiering Widget
 * Visualizes hot/warm/cold data tiers and provides management controls
 */
class StorageTiering extends HTMLElement {
    constructor() {
        super();
        this.stats = null;
        this.state = null;
        this.warmFiles = [];
        this.coldArchives = [];
        this.backends = [];
        this.walStats = null;
        this.partitions = [];
        this.downsampleRules = [];
        this.loading = true;
        this.refreshInterval = null;
        this.animationFrame = null;
        this._mounted = false;
    }

    connectedCallback() {
        this._mounted = true;
        this.render();
        this.loadData();
        // Auto-refresh every 30 seconds
        this.refreshInterval = setInterval(() => {
            if (this._mounted) this.loadData();
        }, 30000);
    }

    disconnectedCallback() {
        this._mounted = false;
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
        if (this.animationFrame) {
            cancelAnimationFrame(this.animationFrame);
            this.animationFrame = null;
        }
    }

    async loadData() {
        try {
            const [statsResp, stateResp, warmResp, coldResp, backendsResp, walResp, partitionsResp, downsampleResp] = await Promise.all([
                fetch('/api/storage/tiering/stats'),
                fetch('/api/storage/tiering/state'),
                fetch('/api/storage/tiering/warm'),
                fetch('/api/storage/tiering/cold'),
                fetch('/api/storage/backends'),
                fetch('/api/storage/wal/stats'),
                fetch('/api/timeseries/partitions').catch(() => ({ ok: false })),
                fetch('/api/timeseries/downsample').catch(() => ({ ok: false }))
            ]);

            if (statsResp.ok) this.stats = await statsResp.json();
            if (stateResp.ok) this.state = await stateResp.json();
            if (warmResp.ok) this.warmFiles = await warmResp.json() || [];
            if (coldResp.ok) this.coldArchives = await coldResp.json() || [];
            if (backendsResp.ok) this.backends = await backendsResp.json() || [];
            if (walResp.ok) this.walStats = await walResp.json();
            if (partitionsResp.ok) this.partitions = await partitionsResp.json() || [];
            if (downsampleResp.ok) this.downsampleRules = await downsampleResp.json() || [];
        } catch (e) {
            console.error('Failed to load storage tiering data:', e);
        } finally {
            this.loading = false;
            this.render();
        }
    }

    async forceCompact() {
        try {
            const resp = await fetch('/api/storage/tiering/compact', { method: 'POST' });
            if (resp.ok) {
                const result = await resp.json();
                this.showToast('Compaction completed in ' + result.duration_ms + 'ms');
                this.loadData();
            } else {
                this.showToast('Compaction failed', 'error');
            }
        } catch (e) {
            this.showToast('Compaction error: ' + e.message, 'error');
        }
    }

    async forceTier() {
        try {
            const resp = await fetch('/api/storage/tiering/tier', { method: 'POST' });
            if (resp.ok) {
                const result = await resp.json();
                this.showToast('Tiering completed - moved ' + this.formatBytes(result.bytes_moved_to_warm + result.bytes_moved_to_cold));
                this.loadData();
            } else {
                this.showToast('Tiering failed', 'error');
            }
        } catch (e) {
            this.showToast('Tiering error: ' + e.message, 'error');
        }
    }

    async restoreFromCold(key) {
        try {
            const resp = await fetch('/api/storage/tiering/restore', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ key })
            });
            if (resp.ok) {
                this.showToast('Restore initiated for ' + key);
                this.loadData();
            } else {
                this.showToast('Restore failed', 'error');
            }
        } catch (e) {
            this.showToast('Restore error: ' + e.message, 'error');
        }
    }

    showToast(message, type = 'success') {
        const event = new CustomEvent('toast', {
            detail: { message, type },
            bubbles: true
        });
        this.dispatchEvent(event);
    }

    formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    }

    formatRelativeTime(timestamp) {
        if (!timestamp) return '-';
        const d = new Date(timestamp);
        const diff = Date.now() - d.getTime();
        if (diff < 60000) return 'just now';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }

    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    calculateTierPercentages() {
        const stats = this.stats || {};
        const hot = stats.hot_data_size || 0;
        const warm = stats.warm_data_size || 0;
        const cold = stats.cold_data_size || 0;
        const total = hot + warm + cold || 1;

        return {
            hot: (hot / total * 100).toFixed(1),
            warm: (warm / total * 100).toFixed(1),
            cold: (cold / total * 100).toFixed(1)
        };
    }

    render() {
        if (this.loading) {
            this.innerHTML = `
                <style>${this.getStyles()}</style>
                <div class="storage-tiering">
                    <div class="header">
                        <div class="title">Storage Tiering</div>
                    </div>
                    <div class="loading">Loading storage data...</div>
                </div>
            `;
            return;
        }

        const stats = this.stats || {};
        const state = this.state || {};
        const pct = this.calculateTierPercentages();

        this.innerHTML = `
            <style>${this.getStyles()}</style>
            <div class="storage-tiering">
                <div class="header">
                    <div class="title">Storage Tiering</div>
                    <div class="header-actions">
                        <button class="btn btn-secondary" onclick="this.getRootNode().host.loadData()">Refresh</button>
                    </div>
                </div>

                <!-- Tier Visualization -->
                <div class="tier-visualization">
                    <div class="tier-bar">
                        <div class="tier-segment hot" style="width: ${pct.hot}%">
                            <span class="tier-label">HOT</span>
                        </div>
                        <div class="tier-segment warm" style="width: ${pct.warm}%">
                            <span class="tier-label">WARM</span>
                        </div>
                        <div class="tier-segment cold" style="width: ${pct.cold}%">
                            <span class="tier-label">COLD</span>
                        </div>
                    </div>
                    <div class="tier-legend">
                        <div class="legend-item">
                            <span class="legend-dot hot"></span>
                            <span>Hot: ${stats.hot_data_size_human || '0 B'} (${pct.hot}%)</span>
                        </div>
                        <div class="legend-item">
                            <span class="legend-dot warm"></span>
                            <span>Warm: ${stats.warm_data_size_human || '0 B'} (${pct.warm}%)</span>
                        </div>
                        <div class="legend-item">
                            <span class="legend-dot cold"></span>
                            <span>Cold: ${stats.cold_data_size_human || '0 B'} (${pct.cold}%)</span>
                        </div>
                    </div>
                </div>

                <!-- Data Flow Animation -->
                <div class="data-flow">
                    <div class="flow-tier hot-tier">
                        <div class="tier-icon">DB</div>
                        <div class="tier-name">Hot (SQLite)</div>
                        <div class="tier-metric">${stats.hot_data_size_human || '0 B'}</div>
                    </div>
                    <div class="flow-arrow">
                        <div class="arrow-line"></div>
                        <div class="arrow-head"></div>
                        <div class="flow-particles">
                            <span class="particle"></span>
                            <span class="particle"></span>
                            <span class="particle"></span>
                        </div>
                    </div>
                    <div class="flow-tier warm-tier">
                        <div class="tier-icon">GZ</div>
                        <div class="tier-name">Warm (Compressed)</div>
                        <div class="tier-metric">${stats.warm_data_size_human || '0 B'}</div>
                    </div>
                    <div class="flow-arrow">
                        <div class="arrow-line"></div>
                        <div class="arrow-head"></div>
                        <div class="flow-particles">
                            <span class="particle"></span>
                            <span class="particle"></span>
                            <span class="particle"></span>
                        </div>
                    </div>
                    <div class="flow-tier cold-tier">
                        <div class="tier-icon">S3</div>
                        <div class="tier-name">Cold (Object Store)</div>
                        <div class="tier-metric">${stats.cold_data_size_human || '0 B'}</div>
                    </div>
                </div>

                <!-- Stats Grid -->
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-value">${stats.total_compactions || 0}</div>
                        <div class="stat-label">Total Compactions</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">${stats.total_tierings || 0}</div>
                        <div class="stat-label">Total Tierings</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">${this.formatBytes(stats.bytes_tiered_to_warm || 0)}</div>
                        <div class="stat-label">Tiered to Warm</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">${this.formatBytes(stats.bytes_tiered_to_cold || 0)}</div>
                        <div class="stat-label">Tiered to Cold</div>
                    </div>
                </div>

                <!-- Manual Controls -->
                <div class="controls-section">
                    <h3>Manual Controls</h3>
                    <div class="controls-grid">
                        <button class="btn btn-primary" onclick="this.getRootNode().host.forceCompact()">
                            Force Compact
                        </button>
                        <button class="btn btn-primary" onclick="this.getRootNode().host.forceTier()">
                            Force Tier
                        </button>
                    </div>
                </div>

                <!-- WAL Status -->
                ${this.walStats ? `
                <div class="wal-section">
                    <h3>WAL Status</h3>
                    <div class="wal-stats">
                        <div class="wal-stat">
                            <span class="wal-label">Entries Written:</span>
                            <span class="wal-value">${this.walStats.entries_written || 0}</span>
                        </div>
                        <div class="wal-stat">
                            <span class="wal-label">Bytes Written:</span>
                            <span class="wal-value">${this.formatBytes(this.walStats.bytes_written || 0)}</span>
                        </div>
                        <div class="wal-stat">
                            <span class="wal-label">Pending Entries:</span>
                            <span class="wal-value ${this.walStats.pending_entries > 100 ? 'warning' : ''}">${this.walStats.pending_entries || 0}</span>
                        </div>
                        <div class="wal-stat">
                            <span class="wal-label">Checkpoints:</span>
                            <span class="wal-value">${this.walStats.checkpoints || 0}</span>
                        </div>
                        <div class="wal-stat">
                            <span class="wal-label">Current Segment:</span>
                            <span class="wal-value">#${this.walStats.current_segment_id || 0}</span>
                        </div>
                        <div class="wal-stat">
                            <span class="wal-label">Last Checkpoint:</span>
                            <span class="wal-value">${this.formatRelativeTime(this.walStats.last_checkpoint)}</span>
                        </div>
                    </div>
                </div>
                ` : ''}

                <!-- Backend Status -->
                <div class="backends-section">
                    <h3>Storage Backends</h3>
                    ${this.backends.length > 0 ? `
                    <div class="backends-list">
                        ${this.backends.map(b => `
                            <div class="backend-item">
                                <div class="backend-icon ${b.type}">${b.type === 'local' ? 'FS' : b.type === 's3' ? 'S3' : 'GCS'}</div>
                                <div class="backend-info">
                                    <div class="backend-name">${this.escapeHtml(b.name)}</div>
                                    <div class="backend-type">${this.escapeHtml(b.type)}</div>
                                </div>
                                <div class="backend-status connected">Connected</div>
                            </div>
                        `).join('')}
                    </div>
                    ` : '<div class="empty-state">No backends configured</div>'}
                </div>

                <!-- Warm Files Table -->
                <div class="files-section">
                    <h3>Warm Tier Files (${this.warmFiles.length})</h3>
                    ${this.warmFiles.length > 0 ? `
                    <div class="table-wrapper">
                        <table class="files-table">
                            <thead>
                                <tr>
                                    <th>Path</th>
                                    <th>Size</th>
                                    <th>Time Range</th>
                                    <th>Tables</th>
                                    <th>Compressed</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${this.warmFiles.slice(0, 10).map(f => `
                                    <tr>
                                        <td class="path-cell" title="${this.escapeHtml(f.path)}">${this.escapeHtml(f.path.split('/').pop())}</td>
                                        <td>${f.size_human || this.formatBytes(f.size)}</td>
                                        <td>${this.formatRelativeTime(f.time_range?.start)} - ${this.formatRelativeTime(f.time_range?.end)}</td>
                                        <td>${(f.tables || []).join(', ')}</td>
                                        <td>${f.compressed ? 'Yes' : 'No'}</td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    </div>
                    ` : '<div class="empty-state">No warm tier files</div>'}
                </div>

                <!-- Cold Archives Table -->
                <div class="files-section">
                    <h3>Cold Tier Archives (${this.coldArchives.length})</h3>
                    ${this.coldArchives.length > 0 ? `
                    <div class="table-wrapper">
                        <table class="files-table">
                            <thead>
                                <tr>
                                    <th>Key</th>
                                    <th>Size</th>
                                    <th>Time Range</th>
                                    <th>Tables</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${this.coldArchives.slice(0, 10).map(a => `
                                    <tr>
                                        <td class="path-cell" title="${this.escapeHtml(a.key)}">${this.escapeHtml(a.key.split('/').pop())}</td>
                                        <td>${a.size_human || this.formatBytes(a.size)}</td>
                                        <td>${this.formatRelativeTime(a.time_range?.start)} - ${this.formatRelativeTime(a.time_range?.end)}</td>
                                        <td>${(a.tables || []).join(', ')}</td>
                                        <td>
                                            <button class="btn btn-sm btn-secondary" onclick="this.getRootNode().host.restoreFromCold('${this.escapeHtml(a.key)}')">
                                                Restore
                                            </button>
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    </div>
                    ` : '<div class="empty-state">No cold tier archives</div>'}
                </div>

                <!-- Downsampling Rules -->
                ${this.downsampleRules.length > 0 ? `
                <div class="downsample-section">
                    <h3>Downsampling Rules</h3>
                    <div class="rules-list">
                        ${this.downsampleRules.map(r => `
                            <div class="rule-item ${r.enabled ? 'enabled' : 'disabled'}">
                                <div class="rule-info">
                                    <div class="rule-name">${this.escapeHtml(r.source_table)} -> ${this.escapeHtml(r.target_table)}</div>
                                    <div class="rule-details">${r.aggregate_func}(${r.aggregate_column}) @ ${r.target_interval}</div>
                                </div>
                                <div class="rule-status">${r.enabled ? 'Active' : 'Disabled'}</div>
                            </div>
                        `).join('')}
                    </div>
                </div>
                ` : ''}

                <!-- Error Display -->
                ${stats.last_error ? `
                <div class="error-banner">
                    <div class="error-title">Last Error</div>
                    <div class="error-message">${this.escapeHtml(stats.last_error)}</div>
                    <div class="error-time">${this.formatRelativeTime(stats.last_error_time)}</div>
                </div>
                ` : ''}
            </div>
        `;
    }

    getStyles() {
        return `
            .storage-tiering {
                background: var(--bg-card, #16181c);
                border-radius: 8px;
                overflow: hidden;
                height: 100%;
                display: flex;
                flex-direction: column;
            }

            .header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem 1rem;
                background: var(--bg-elevated, #1e2128);
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .title {
                font-weight: 600;
                font-size: 1rem;
            }

            .header-actions {
                display: flex;
                gap: 0.5rem;
            }

            .loading {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 3rem;
                color: var(--text-muted, #71767b);
            }

            /* Tier Visualization */
            .tier-visualization {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .tier-bar {
                display: flex;
                height: 32px;
                border-radius: 4px;
                overflow: hidden;
                background: var(--bg-elevated, #1e2128);
            }

            .tier-segment {
                display: flex;
                align-items: center;
                justify-content: center;
                min-width: 40px;
                transition: width 0.3s ease;
            }

            .tier-segment.hot {
                background: linear-gradient(135deg, #f97316, #ea580c);
            }

            .tier-segment.warm {
                background: linear-gradient(135deg, #eab308, #ca8a04);
            }

            .tier-segment.cold {
                background: linear-gradient(135deg, #3b82f6, #2563eb);
            }

            .tier-label {
                font-size: 0.7rem;
                font-weight: 600;
                color: white;
                text-shadow: 0 1px 2px rgba(0,0,0,0.3);
            }

            .tier-legend {
                display: flex;
                gap: 1rem;
                margin-top: 0.75rem;
                flex-wrap: wrap;
            }

            .legend-item {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-size: 0.8rem;
                color: var(--text-muted, #71767b);
            }

            .legend-dot {
                width: 10px;
                height: 10px;
                border-radius: 2px;
            }

            .legend-dot.hot { background: #f97316; }
            .legend-dot.warm { background: #eab308; }
            .legend-dot.cold { background: #3b82f6; }

            /* Data Flow Animation */
            .data-flow {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 1.5rem 1rem;
                gap: 0.5rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .flow-tier {
                display: flex;
                flex-direction: column;
                align-items: center;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 8px;
                min-width: 100px;
            }

            .tier-icon {
                width: 36px;
                height: 36px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 8px;
                font-weight: 700;
                font-size: 0.75rem;
                margin-bottom: 0.5rem;
            }

            .hot-tier .tier-icon { background: rgba(249, 115, 22, 0.2); color: #f97316; }
            .warm-tier .tier-icon { background: rgba(234, 179, 8, 0.2); color: #eab308; }
            .cold-tier .tier-icon { background: rgba(59, 130, 246, 0.2); color: #3b82f6; }

            .tier-name {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-bottom: 0.25rem;
            }

            .tier-metric {
                font-size: 0.85rem;
                font-weight: 600;
            }

            .flow-arrow {
                position: relative;
                width: 40px;
                height: 20px;
            }

            .arrow-line {
                position: absolute;
                top: 50%;
                left: 0;
                right: 10px;
                height: 2px;
                background: var(--border, #2f3336);
            }

            .arrow-head {
                position: absolute;
                right: 0;
                top: 50%;
                transform: translateY(-50%);
                width: 0;
                height: 0;
                border-left: 8px solid var(--border, #2f3336);
                border-top: 5px solid transparent;
                border-bottom: 5px solid transparent;
            }

            .flow-particles {
                position: absolute;
                top: 50%;
                left: 0;
                right: 0;
                transform: translateY(-50%);
            }

            .particle {
                position: absolute;
                width: 4px;
                height: 4px;
                background: var(--accent, #1d9bf0);
                border-radius: 50%;
                animation: flowParticle 2s infinite;
            }

            .particle:nth-child(2) { animation-delay: 0.6s; }
            .particle:nth-child(3) { animation-delay: 1.2s; }

            @keyframes flowParticle {
                0% { left: 0; opacity: 0; }
                10% { opacity: 1; }
                90% { opacity: 1; }
                100% { left: 100%; opacity: 0; }
            }

            /* Stats Grid */
            .stats-grid {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 0.5rem;
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .stat-card {
                background: var(--bg-elevated, #1e2128);
                padding: 0.75rem;
                border-radius: 6px;
                text-align: center;
            }

            .stat-value {
                font-size: 1.25rem;
                font-weight: 600;
                color: var(--accent, #1d9bf0);
            }

            .stat-label {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            /* Controls Section */
            .controls-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .controls-section h3 {
                font-size: 0.85rem;
                margin: 0 0 0.75rem 0;
                color: var(--text-muted, #71767b);
            }

            .controls-grid {
                display: flex;
                gap: 0.5rem;
            }

            /* Buttons */
            .btn {
                padding: 0.5rem 1rem;
                border-radius: 4px;
                border: none;
                cursor: pointer;
                font-size: 0.8rem;
                font-weight: 500;
                transition: background 0.2s;
            }

            .btn-primary {
                background: var(--accent, #1d9bf0);
                color: white;
            }

            .btn-primary:hover {
                background: var(--accent-hover, #1a8cd8);
            }

            .btn-secondary {
                background: var(--bg-elevated, #1e2128);
                color: var(--text, #e7e9ea);
                border: 1px solid var(--border, #2f3336);
            }

            .btn-secondary:hover {
                background: var(--bg-hover, #2a2e35);
            }

            .btn-sm {
                padding: 0.25rem 0.5rem;
                font-size: 0.7rem;
            }

            /* WAL Section */
            .wal-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .wal-section h3 {
                font-size: 0.85rem;
                margin: 0 0 0.75rem 0;
                color: var(--text-muted, #71767b);
            }

            .wal-stats {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 0.5rem;
            }

            .wal-stat {
                display: flex;
                justify-content: space-between;
                padding: 0.5rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 4px;
                font-size: 0.8rem;
            }

            .wal-label {
                color: var(--text-muted, #71767b);
            }

            .wal-value {
                font-weight: 500;
            }

            .wal-value.warning {
                color: var(--warning, #ffd400);
            }

            /* Backends Section */
            .backends-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .backends-section h3 {
                font-size: 0.85rem;
                margin: 0 0 0.75rem 0;
                color: var(--text-muted, #71767b);
            }

            .backends-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .backend-item {
                display: flex;
                align-items: center;
                gap: 0.75rem;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
            }

            .backend-icon {
                width: 32px;
                height: 32px;
                display: flex;
                align-items: center;
                justify-content: center;
                border-radius: 6px;
                font-weight: 700;
                font-size: 0.7rem;
                background: var(--bg-card, #16181c);
            }

            .backend-icon.local { color: #10b981; }
            .backend-icon.s3 { color: #f97316; }
            .backend-icon.gcs { color: #3b82f6; }

            .backend-info {
                flex: 1;
            }

            .backend-name {
                font-weight: 500;
                font-size: 0.85rem;
            }

            .backend-type {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .backend-status {
                font-size: 0.75rem;
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
            }

            .backend-status.connected {
                background: rgba(16, 185, 129, 0.1);
                color: #10b981;
            }

            /* Files Section */
            .files-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .files-section h3 {
                font-size: 0.85rem;
                margin: 0 0 0.75rem 0;
                color: var(--text-muted, #71767b);
            }

            .table-wrapper {
                overflow-x: auto;
            }

            .files-table {
                width: 100%;
                border-collapse: collapse;
                font-size: 0.8rem;
            }

            .files-table th,
            .files-table td {
                padding: 0.5rem;
                text-align: left;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .files-table th {
                font-weight: 500;
                color: var(--text-muted, #71767b);
                background: var(--bg-elevated, #1e2128);
            }

            .path-cell {
                max-width: 150px;
                overflow: hidden;
                text-overflow: ellipsis;
                white-space: nowrap;
            }

            .empty-state {
                padding: 1rem;
                text-align: center;
                color: var(--text-muted, #71767b);
                font-size: 0.8rem;
            }

            /* Downsample Section */
            .downsample-section {
                padding: 1rem;
                border-bottom: 1px solid var(--border, #2f3336);
            }

            .downsample-section h3 {
                font-size: 0.85rem;
                margin: 0 0 0.75rem 0;
                color: var(--text-muted, #71767b);
            }

            .rules-list {
                display: flex;
                flex-direction: column;
                gap: 0.5rem;
            }

            .rule-item {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 0.75rem;
                background: var(--bg-elevated, #1e2128);
                border-radius: 6px;
                border-left: 3px solid var(--border, #2f3336);
            }

            .rule-item.enabled {
                border-left-color: var(--success, #00ba7c);
            }

            .rule-item.disabled {
                border-left-color: var(--text-muted, #71767b);
                opacity: 0.7;
            }

            .rule-name {
                font-weight: 500;
                font-size: 0.85rem;
            }

            .rule-details {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
            }

            .rule-status {
                font-size: 0.75rem;
                padding: 0.25rem 0.5rem;
                border-radius: 4px;
                background: var(--bg-card, #16181c);
            }

            /* Error Banner */
            .error-banner {
                margin: 1rem;
                padding: 0.75rem;
                background: rgba(244, 33, 46, 0.1);
                border: 1px solid var(--error, #f4212e);
                border-radius: 6px;
            }

            .error-title {
                font-weight: 600;
                color: var(--error, #f4212e);
                font-size: 0.85rem;
            }

            .error-message {
                font-size: 0.8rem;
                margin-top: 0.25rem;
            }

            .error-time {
                font-size: 0.7rem;
                color: var(--text-muted, #71767b);
                margin-top: 0.25rem;
            }

            @media (max-width: 800px) {
                .stats-grid {
                    grid-template-columns: repeat(2, 1fr);
                }

                .wal-stats {
                    grid-template-columns: repeat(2, 1fr);
                }

                .data-flow {
                    flex-direction: column;
                }

                .flow-arrow {
                    transform: rotate(90deg);
                }
            }
        `;
    }
}

customElements.define('storage-tiering', StorageTiering);
