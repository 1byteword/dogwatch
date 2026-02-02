/**
 * Incident helper functions with safe localStorage handling
 * These functions provide additional safety wrappers around incident operations
 */

// Helper function to safely get stored username
function getSavedUsername() {
    if (typeof SafeStorage === 'undefined') {
        // Fallback if SafeStorage not loaded
        try {
            const user = localStorage.getItem('dogwatch_user');
            if (user && typeof user === 'string' && user.length > 0 && user.length < 256) {
                return user.replace(/[<>"'&]/g, '');
            }
        } catch (e) {
            console.warn('Failed to get username from localStorage:', e);
        }
        return 'operator';
    }
    const user = SafeStorage.getString('dogwatch_user', 'operator');
    // Validate and sanitize the username
    if (user && typeof user === 'string' && user.length > 0 && user.length < 256) {
        // Only allow safe characters
        return user.replace(/[<>"'&]/g, '');
    }
    return 'operator';
}

// Helper function to safely save username
function setSavedUsername(user) {
    if (!user || typeof user !== 'string' || user.length === 0 || user.length >= 256) {
        return false;
    }

    if (typeof SafeStorage === 'undefined') {
        // Fallback if SafeStorage not loaded
        try {
            localStorage.setItem('dogwatch_user', user);
            return true;
        } catch (e) {
            console.warn('Failed to save username to localStorage:', e);
            return false;
        }
    }
    return SafeStorage.setString('dogwatch_user', user);
}

// Validate incident ID format
function isValidIncidentId(id) {
    if (!id || typeof id !== 'string') return false;
    // Allow alphanumeric IDs, UUIDs, and common ID formats
    return /^[a-zA-Z0-9\-_]{1,128}$/.test(id);
}

// Export for global use
window.getSavedUsername = getSavedUsername;
window.setSavedUsername = setSavedUsername;
window.isValidIncidentId = isValidIncidentId;
