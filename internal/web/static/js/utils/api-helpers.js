/**
 * API Response Validation Helpers
 * Provides defensive checks for API responses before accessing nested properties
 */

const APIHelpers = {
    /**
     * Safely fetch and parse JSON with validation
     * @param {string} url - API endpoint URL
     * @param {Object} options - Fetch options
     * @param {Function} validator - Optional validation function
     * @returns {Promise<{data: any, error: string|null}>}
     */
    async fetchJSON(url, options = {}, validator = null) {
        try {
            const resp = await fetch(url, options);

            if (!resp.ok) {
                const errorText = await resp.text().catch(() => 'Unknown error');
                return { data: null, error: `Request failed: ${resp.status} ${errorText}` };
            }

            const data = await resp.json();

            // Validate response structure if validator provided
            if (validator && typeof validator === 'function') {
                if (!validator(data)) {
                    return { data: null, error: 'Invalid response structure' };
                }
            }

            return { data, error: null };
        } catch (e) {
            console.error('API fetch error:', e);
            return { data: null, error: e.message || 'Network error' };
        }
    },

    /**
     * Safely access nested properties
     * @param {Object} obj - Object to access
     * @param {string} path - Dot-notation path (e.g., 'user.profile.name')
     * @param {*} defaultValue - Default value if path doesn't exist
     * @returns {*}
     */
    get(obj, path, defaultValue = undefined) {
        if (!obj || typeof obj !== 'object') return defaultValue;
        if (!path || typeof path !== 'string') return defaultValue;

        const parts = path.split('.');
        let current = obj;

        for (const part of parts) {
            if (current === null || current === undefined) {
                return defaultValue;
            }
            if (typeof current !== 'object') {
                return defaultValue;
            }
            current = current[part];
        }

        return current !== undefined ? current : defaultValue;
    },

    /**
     * Check if object has expected structure
     * @param {Object} obj - Object to check
     * @param {string[]} requiredFields - Array of required field paths
     * @returns {boolean}
     */
    hasFields(obj, requiredFields) {
        if (!obj || typeof obj !== 'object' || !Array.isArray(requiredFields)) {
            return false;
        }
        return requiredFields.every(field => this.get(obj, field) !== undefined);
    },

    /**
     * Validate array response
     * @param {*} data - Response data
     * @param {Function} itemValidator - Optional validator for each item
     * @returns {Array}
     */
    validateArray(data, itemValidator = null) {
        // Handle both direct arrays and { data: [...] } structures
        let arr = data;
        if (data && typeof data === 'object' && !Array.isArray(data)) {
            arr = data.data || data.items || data.results || [];
        }

        if (!Array.isArray(arr)) {
            return [];
        }

        if (itemValidator && typeof itemValidator === 'function') {
            return arr.filter(item => itemValidator(item));
        }

        return arr;
    },

    /**
     * Sanitize string for safe display
     * @param {*} value - Value to sanitize
     * @param {string} defaultValue - Default if value is invalid
     * @returns {string}
     */
    safeString(value, defaultValue = '') {
        if (value === null || value === undefined) return defaultValue;
        const str = String(value);
        // Basic XSS prevention
        return str
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#x27;');
    },

    /**
     * Safely parse a number
     * @param {*} value - Value to parse
     * @param {number} defaultValue - Default if parsing fails
     * @returns {number}
     */
    safeNumber(value, defaultValue = 0) {
        if (value === null || value === undefined) return defaultValue;
        const num = Number(value);
        return isNaN(num) ? defaultValue : num;
    },

    /**
     * Safely parse a boolean
     * @param {*} value - Value to parse
     * @param {boolean} defaultValue - Default if parsing fails
     * @returns {boolean}
     */
    safeBoolean(value, defaultValue = false) {
        if (value === null || value === undefined) return defaultValue;
        if (typeof value === 'boolean') return value;
        if (typeof value === 'string') {
            return value.toLowerCase() === 'true' || value === '1';
        }
        return Boolean(value);
    }
};

// Common response validators
const ResponseValidators = {
    /**
     * Validate user object
     */
    user(data) {
        return data && typeof data === 'object' && (data.id || data.email);
    },

    /**
     * Validate dashboard object
     */
    dashboard(data) {
        return data && typeof data === 'object' && data.id && typeof data.name === 'string';
    },

    /**
     * Validate incident object
     */
    incident(data) {
        return data && typeof data === 'object' && data.id && typeof data.title === 'string';
    },

    /**
     * Validate metrics data
     */
    metrics(data) {
        if (!data || typeof data !== 'object') return false;
        // Accept various metric response formats
        return data.value !== undefined ||
               data.data !== undefined ||
               Array.isArray(data) ||
               data.cpu !== undefined ||
               data.memory !== undefined;
    },

    /**
     * Validate service object
     */
    service(data) {
        return data && typeof data === 'object' && (data.name || data.id);
    },

    /**
     * Validate alert object
     */
    alert(data) {
        return data && typeof data === 'object' && data.id && (data.name || data.title || data.message);
    }
};

// Export for global use
window.APIHelpers = APIHelpers;
window.ResponseValidators = ResponseValidators;
