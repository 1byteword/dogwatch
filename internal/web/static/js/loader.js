/**
 * Lazy Loader - Load libraries on demand
 *
 * Usage:
 *   await Loader.load('d3');
 *   await Loader.load('chart');
 *   await Loader.loadMultiple(['d3', 'gridstack']);
 */
const Loader = {
    loaded: {},
    loading: {},

    // Configuration
    config: {
        timeout: 15000,      // 15 second timeout per resource
        maxRetries: 3,       // Max retry attempts
        retryDelay: 1000,    // Base delay between retries (exponential backoff)
    },

    // Library configurations
    libraries: {
        'd3': {
            js: 'https://d3js.org/d3.v7.min.js',
            check: () => typeof d3 !== 'undefined'
        },
        'd3-flamegraph': {
            js: 'https://cdn.jsdelivr.net/npm/d3-flame-graph@4.1.3/dist/d3-flamegraph.min.js',
            css: 'https://cdn.jsdelivr.net/npm/d3-flame-graph@4.1.3/dist/d3-flamegraph.css',
            depends: ['d3'],
            check: () => typeof d3 !== 'undefined' && typeof d3.flamegraph !== 'undefined'
        },
        'chart': {
            js: 'https://cdn.jsdelivr.net/npm/chart.js@4.4.1/dist/chart.umd.min.js',
            check: () => typeof Chart !== 'undefined'
        },
        'chart-adapter': {
            js: 'https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns@3.0.0/dist/chartjs-adapter-date-fns.bundle.min.js',
            depends: ['chart'],
            check: () => typeof Chart !== 'undefined' && Chart._adapters && Chart._adapters._date
        },
        'gridstack': {
            js: 'https://cdn.jsdelivr.net/npm/gridstack@10.0.1/dist/gridstack-all.min.js',
            css: 'https://cdn.jsdelivr.net/npm/gridstack@10.0.1/dist/gridstack.min.css',
            check: () => typeof GridStack !== 'undefined'
        },
        'maplibre': {
            js: 'https://unpkg.com/maplibre-gl@4.1.2/dist/maplibre-gl.js',
            css: 'https://unpkg.com/maplibre-gl@4.1.2/dist/maplibre-gl.css',
            check: () => typeof maplibregl !== 'undefined'
        }
    },

    /**
     * Load a resource with timeout
     */
    _loadWithTimeout(loadFn, url, timeout) {
        return new Promise((resolve, reject) => {
            const timeoutId = setTimeout(() => {
                reject(new Error(`Timeout loading: ${url}`));
            }, timeout);

            loadFn(url)
                .then(result => {
                    clearTimeout(timeoutId);
                    resolve(result);
                })
                .catch(err => {
                    clearTimeout(timeoutId);
                    reject(err);
                });
        });
    },

    /**
     * Load with retry logic
     */
    async _loadWithRetry(loadFn, url, retries = this.config.maxRetries) {
        let lastError;
        for (let attempt = 0; attempt <= retries; attempt++) {
            try {
                return await this._loadWithTimeout(loadFn, url, this.config.timeout);
            } catch (err) {
                lastError = err;
                if (attempt < retries) {
                    const delay = this.config.retryDelay * Math.pow(2, attempt);
                    console.warn(`[Loader] Retry ${attempt + 1}/${retries} for ${url} in ${delay}ms`);
                    await new Promise(r => setTimeout(r, delay));
                }
            }
        }
        throw lastError;
    },

    /**
     * Load a script (internal, no retry)
     */
    _loadScriptOnce(url) {
        return new Promise((resolve, reject) => {
            // Check if already loaded
            const existing = document.querySelector(`script[src="${url}"]`);
            if (existing) {
                resolve();
                return;
            }

            const script = document.createElement('script');
            script.src = url;
            script.async = true;
            script.onload = () => resolve();
            script.onerror = () => {
                // Remove failed script so retry can add a new one
                script.remove();
                reject(new Error(`Failed to load script: ${url}`));
            };
            document.head.appendChild(script);
        });
    },

    /**
     * Load a stylesheet (internal, no retry)
     */
    _loadStyleOnce(url) {
        return new Promise((resolve, reject) => {
            // Check if already loaded
            const existing = document.querySelector(`link[href="${url}"]`);
            if (existing) {
                resolve();
                return;
            }

            const link = document.createElement('link');
            link.rel = 'stylesheet';
            link.href = url;
            link.onload = () => resolve();
            link.onerror = () => {
                // Remove failed link so retry can add a new one
                link.remove();
                reject(new Error(`Failed to load stylesheet: ${url}`));
            };
            document.head.appendChild(link);
        });
    },

    /**
     * Load a script with retry
     */
    loadScript(url) {
        return this._loadWithRetry(this._loadScriptOnce.bind(this), url);
    },

    /**
     * Load a stylesheet with retry
     */
    loadStyle(url) {
        return this._loadWithRetry(this._loadStyleOnce.bind(this), url);
    },

    /**
     * Load a library by name
     * @param {string} name - Library name (d3, chart, gridstack, etc.)
     * @returns {Promise} Resolves when library is loaded
     */
    async load(name) {
        // Already loaded
        if (this.loaded[name]) {
            return Promise.resolve();
        }

        // Currently loading - return existing promise
        if (this.loading[name]) {
            return this.loading[name];
        }

        const lib = this.libraries[name];
        if (!lib) {
            const error = new Error(`Unknown library: ${name}`);
            this._showError(`Unknown library: ${name}`);
            return Promise.reject(error);
        }

        // Already available (maybe loaded differently)
        if (lib.check && lib.check()) {
            this.loaded[name] = true;
            return Promise.resolve();
        }

        // Create loading promise
        this.loading[name] = (async () => {
            try {
                // Load dependencies first
                if (lib.depends) {
                    await this.loadMultiple(lib.depends);
                }

                // Load CSS if present
                if (lib.css) {
                    await this.loadStyle(lib.css);
                }

                // Load JS
                if (lib.js) {
                    await this.loadScript(lib.js);
                }

                // Verify it loaded
                if (lib.check && !lib.check()) {
                    throw new Error(`Library ${name} loaded but check failed`);
                }

                this.loaded[name] = true;
                console.log(`[Loader] Loaded: ${name}`);
            } catch (err) {
                // Show user-friendly error via toast
                this._showError(`Failed to load ${name}: ${err.message}`);
                throw err;
            } finally {
                delete this.loading[name];
            }
        })();

        return this.loading[name];
    },

    /**
     * Show error to user via toast notification
     */
    _showError(message) {
        console.error('[Loader]', message);
        // Use global toast if available
        if (window.showToast) {
            window.showToast({
                type: 'error',
                title: 'Library Load Error',
                message: message,
                duration: 10000
            });
        } else if (window.toast) {
            window.toast.error(message, 'Library Load Error');
        }
    },

    /**
     * Load multiple libraries
     * @param {string[]} names - Array of library names
     * @returns {Promise} Resolves when all libraries are loaded
     */
    async loadMultiple(names) {
        // Group by dependencies to load in correct order
        const noDeps = [];
        const withDeps = [];

        names.forEach(name => {
            const lib = this.libraries[name];
            if (lib && lib.depends && lib.depends.some(d => names.includes(d))) {
                withDeps.push(name);
            } else {
                noDeps.push(name);
            }
        });

        // Load non-dependent libraries in parallel
        await Promise.all(noDeps.map(name => this.load(name)));

        // Load dependent libraries in parallel (their deps are now loaded)
        await Promise.all(withDeps.map(name => this.load(name)));
    },

    /**
     * Check if a library is loaded
     * @param {string} name - Library name
     * @returns {boolean}
     */
    isLoaded(name) {
        if (this.loaded[name]) return true;
        const lib = this.libraries[name];
        return lib && lib.check && lib.check();
    },

    /**
     * Preload libraries in the background
     * @param {string[]} names - Array of library names
     */
    preload(names) {
        // Use requestIdleCallback if available, otherwise setTimeout
        const schedule = window.requestIdleCallback || ((cb) => setTimeout(cb, 1));
        schedule(() => {
            this.loadMultiple(names).catch(e => {
                console.warn('[Loader] Preload failed:', e);
            });
        });
    },

    /**
     * Get load status of all libraries
     */
    status() {
        const result = {};
        Object.keys(this.libraries).forEach(name => {
            result[name] = {
                loaded: this.isLoaded(name),
                loading: !!this.loading[name]
            };
        });
        return result;
    }
};

// Export for module systems if available
if (typeof module !== 'undefined' && module.exports) {
    module.exports = Loader;
}

// Make globally available
window.Loader = Loader;
