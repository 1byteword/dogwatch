/**
 * Safe localStorage utilities with error handling and validation
 * Provides defensive wrappers around localStorage operations
 */

const SafeStorage = {
    /**
     * Safely get and parse JSON from localStorage
     * @param {string} key - localStorage key
     * @param {*} defaultValue - Default value if key doesn't exist or parsing fails
     * @param {Function} validator - Optional validation function that receives parsed data
     * @returns {*} Parsed value or defaultValue
     */
    getJSON(key, defaultValue = null, validator = null) {
        try {
            const raw = localStorage.getItem(key);
            if (raw === null || raw === undefined) {
                return defaultValue;
            }

            const parsed = JSON.parse(raw);

            // If a validator is provided, use it to validate the data structure
            if (validator && typeof validator === 'function') {
                if (!validator(parsed)) {
                    console.warn(`SafeStorage: Invalid data structure for key "${key}", using default`);
                    return defaultValue;
                }
            }

            return parsed;
        } catch (e) {
            console.warn(`SafeStorage: Failed to parse JSON for key "${key}":`, e.message);
            return defaultValue;
        }
    },

    /**
     * Safely set JSON in localStorage
     * @param {string} key - localStorage key
     * @param {*} value - Value to stringify and store
     * @returns {boolean} True if successful
     */
    setJSON(key, value) {
        try {
            localStorage.setItem(key, JSON.stringify(value));
            return true;
        } catch (e) {
            console.warn(`SafeStorage: Failed to set JSON for key "${key}":`, e.message);
            return false;
        }
    },

    /**
     * Safely get a string from localStorage
     * @param {string} key - localStorage key
     * @param {string} defaultValue - Default value if key doesn't exist
     * @returns {string}
     */
    getString(key, defaultValue = '') {
        try {
            const value = localStorage.getItem(key);
            return value !== null ? value : defaultValue;
        } catch (e) {
            console.warn(`SafeStorage: Failed to get string for key "${key}":`, e.message);
            return defaultValue;
        }
    },

    /**
     * Safely set a string in localStorage
     * @param {string} key - localStorage key
     * @param {string} value - Value to store
     * @returns {boolean} True if successful
     */
    setString(key, value) {
        try {
            localStorage.setItem(key, String(value));
            return true;
        } catch (e) {
            console.warn(`SafeStorage: Failed to set string for key "${key}":`, e.message);
            return false;
        }
    },

    /**
     * Safely remove an item from localStorage
     * @param {string} key - localStorage key
     * @returns {boolean} True if successful
     */
    remove(key) {
        try {
            localStorage.removeItem(key);
            return true;
        } catch (e) {
            console.warn(`SafeStorage: Failed to remove key "${key}":`, e.message);
            return false;
        }
    },

    /**
     * Check if localStorage is available
     * @returns {boolean}
     */
    isAvailable() {
        try {
            const testKey = '__storage_test__';
            localStorage.setItem(testKey, testKey);
            localStorage.removeItem(testKey);
            return true;
        } catch (e) {
            return false;
        }
    }
};

// Validators for specific data structures
const StorageValidators = {
    /**
     * Validate dashboard layout structure
     * @param {*} data - Data to validate
     * @returns {boolean}
     */
    dashboardLayout(data) {
        if (!Array.isArray(data)) return false;
        return data.every(item =>
            item &&
            typeof item === 'object' &&
            typeof item.id === 'string'
        );
    },

    /**
     * Validate user data structure
     * @param {*} data - Data to validate
     * @returns {boolean}
     */
    userData(data) {
        if (!data || typeof data !== 'object') return false;
        // Accept either a string (username) or an object with name/id
        if (typeof data === 'string') return data.length > 0 && data.length < 256;
        return typeof data.name === 'string' || typeof data.email === 'string' || typeof data.id === 'string';
    },

    /**
     * Validate a simple string value
     * @param {*} data - Data to validate
     * @returns {boolean}
     */
    nonEmptyString(data) {
        return typeof data === 'string' && data.length > 0 && data.length < 1000;
    },

    /**
     * Validate numeric width value
     * @param {*} data - Data to validate
     * @returns {boolean}
     */
    sidebarWidth(data) {
        const num = parseInt(data, 10);
        return !isNaN(num) && num >= 100 && num <= 800;
    }
};

// Allowed OAuth providers whitelist
const ALLOWED_OAUTH_PROVIDERS = ['google', 'github', 'microsoft', 'okta', 'azure', 'saml'];

/**
 * Validate OAuth provider against whitelist
 * @param {string} provider - Provider name to validate
 * @returns {boolean}
 */
function isValidOAuthProvider(provider) {
    if (!provider || typeof provider !== 'string') return false;
    // Sanitize: only alphanumeric and hyphens
    const sanitized = provider.toLowerCase().replace(/[^a-z0-9-]/g, '');
    return ALLOWED_OAUTH_PROVIDERS.includes(sanitized) && sanitized === provider.toLowerCase();
}

// Export for use
window.SafeStorage = SafeStorage;
window.StorageValidators = StorageValidators;
window.isValidOAuthProvider = isValidOAuthProvider;
window.ALLOWED_OAUTH_PROVIDERS = ALLOWED_OAUTH_PROVIDERS;
