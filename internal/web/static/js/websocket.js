/**
 * DogwatchSocket - WebSocket client for real-time updates
 *
 * Usage:
 *   dwSocket.subscribe('system', (msg) => console.log(msg));
 *   dwSocket.unsubscribe('system');
 */
class DogwatchSocket {
    constructor() {
        this.ws = null;
        this.subscriptions = new Map(); // topic -> Set of callbacks
        this.pendingSubscriptions = new Set(); // Topics to subscribe after connect
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 10;
        this.reconnectDelay = 1000;
        this.maxReconnectDelay = 30000;
        this.connected = false;
        this.connecting = false;
        this.messageQueue = []; // Queue messages while disconnected

        // Event callbacks
        this.onConnect = null;
        this.onDisconnect = null;
        this.onError = null;

        // Auto-connect
        this.connect();
    }

    /**
     * Connect to WebSocket server
     */
    connect() {
        if (this.connecting || this.connected) return;

        this.connecting = true;
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const url = `${protocol}//${window.location.host}/api/ws`;

        try {
            this.ws = new WebSocket(url);

            this.ws.onopen = () => {
                console.log('[DogwatchSocket] Connected');
                this.connected = true;
                this.connecting = false;
                this.reconnectAttempts = 0;
                this.reconnectDelay = 1000;

                // Collect all topics to subscribe (copy to avoid modification during iteration)
                const topicsToSubscribe = new Set();

                // Add pending subscriptions
                this.pendingSubscriptions.forEach(topic => topicsToSubscribe.add(topic));

                // Add existing subscriptions
                this.subscriptions.forEach((_, topic) => topicsToSubscribe.add(topic));

                // Clear pending AFTER collecting (prevents race condition on rapid reconnect)
                this.pendingSubscriptions.clear();

                // Subscribe to all topics
                topicsToSubscribe.forEach(topic => {
                    this._sendSubscribe(topic);
                });

                // Flush message queue (copy to avoid modification during iteration)
                const queuedMessages = [...this.messageQueue];
                this.messageQueue = [];
                queuedMessages.forEach(msg => this._send(msg));

                if (this.onConnect) this.onConnect();
            };

            this.ws.onclose = (event) => {
                console.log('[DogwatchSocket] Disconnected', event.code, event.reason);
                this.connected = false;
                this.connecting = false;

                if (this.onDisconnect) this.onDisconnect(event);

                // Auto-reconnect unless explicitly closed
                if (event.code !== 1000) {
                    this._scheduleReconnect();
                }
            };

            this.ws.onerror = (error) => {
                console.error('[DogwatchSocket] Error:', error);
                if (this.onError) this.onError(error);
            };

            this.ws.onmessage = (event) => {
                try {
                    const msg = JSON.parse(event.data);
                    this._handleMessage(msg);
                } catch (e) {
                    console.error('[DogwatchSocket] Failed to parse message:', e);
                }
            };
        } catch (e) {
            console.error('[DogwatchSocket] Failed to connect:', e);
            this.connecting = false;
            this._scheduleReconnect();
        }
    }

    /**
     * Schedule a reconnection with exponential backoff
     */
    _scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('[DogwatchSocket] Max reconnect attempts reached');
            return;
        }

        this.reconnectAttempts++;
        const delay = Math.min(
            this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
            this.maxReconnectDelay
        );

        console.log(`[DogwatchSocket] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
        setTimeout(() => this.connect(), delay);
    }

    /**
     * Send a message to the server
     */
    _send(msg) {
        if (this.connected && this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(msg));
            return true;
        }
        return false;
    }

    /**
     * Send a subscribe message
     */
    _sendSubscribe(topic) {
        return this._send({ action: 'subscribe', topic });
    }

    /**
     * Handle incoming messages
     */
    _handleMessage(msg) {
        const { topic, type, payload } = msg;

        // Get subscribers for this topic
        const callbacks = this.subscriptions.get(topic);
        if (callbacks) {
            callbacks.forEach(callback => {
                try {
                    callback({ topic, type, payload });
                } catch (e) {
                    console.error('[DogwatchSocket] Callback error:', e);
                }
            });
        }
    }

    /**
     * Subscribe to a topic
     * @param {string} topic - Topic name (system, servicemap, traces, logs, watches, alerts, incidents)
     * @param {function} callback - Function to call when messages arrive
     * @returns {function} Unsubscribe function
     */
    subscribe(topic, callback) {
        if (!this.subscriptions.has(topic)) {
            this.subscriptions.set(topic, new Set());
        }
        const callbacks = this.subscriptions.get(topic);
        const wasEmpty = callbacks.size === 0;
        callbacks.add(callback);

        // Subscribe on server if this is the first subscriber
        if (wasEmpty) {
            if (this.connected) {
                this._sendSubscribe(topic);
            } else {
                // Always add to pending, even if connecting (will be sent on connect)
                this.pendingSubscriptions.add(topic);
            }
        }

        // Return unsubscribe function
        return () => this.unsubscribe(topic, callback);
    }

    /**
     * Unsubscribe from a topic
     * @param {string} topic - Topic name
     * @param {function} callback - Specific callback to remove (optional - if not provided, removes all)
     */
    unsubscribe(topic, callback) {
        const callbacks = this.subscriptions.get(topic);
        if (!callbacks) return;

        if (callback) {
            callbacks.delete(callback);
        } else {
            callbacks.clear();
        }

        // Unsubscribe on server if no more subscribers
        if (callbacks.size === 0) {
            this.subscriptions.delete(topic);
            this._send({ action: 'unsubscribe', topic });
        }
    }

    /**
     * Disconnect from the server
     */
    disconnect() {
        if (this.ws) {
            this.ws.close(1000, 'Client disconnect');
            this.ws = null;
        }
        this.connected = false;
        this.connecting = false;
    }

    /**
     * Check if connected
     */
    isConnected() {
        return this.connected;
    }

    /**
     * Get subscription count for a topic
     */
    getSubscriberCount(topic) {
        const callbacks = this.subscriptions.get(topic);
        return callbacks ? callbacks.size : 0;
    }

    /**
     * Get all subscribed topics
     */
    getSubscribedTopics() {
        return Array.from(this.subscriptions.keys());
    }
}

// Create global instance
window.dwSocket = new DogwatchSocket();

// Topic constants for convenience
window.dwSocket.TOPICS = {
    SYSTEM: 'system',
    SERVICE_MAP: 'servicemap',
    TRACES: 'traces',
    LOGS: 'logs',
    WATCHES: 'watches',
    ALERTS: 'alerts',
    INCIDENTS: 'incidents',
    ANOMALIES: 'anomalies'
};

// Helper functions
window.dwSocket.onSystemStats = function(callback) {
    return this.subscribe('system', callback);
};

window.dwSocket.onServiceMapUpdate = function(callback) {
    return this.subscribe('servicemap', callback);
};

window.dwSocket.onNewTrace = function(callback) {
    return this.subscribe('traces', callback);
};

window.dwSocket.onLogEntry = function(callback) {
    return this.subscribe('logs', callback);
};

window.dwSocket.onWatchStateChange = function(callback) {
    return this.subscribe('watches', callback);
};

window.dwSocket.onAlertUpdate = function(callback) {
    return this.subscribe('alerts', callback);
};

window.dwSocket.onIncidentUpdate = function(callback) {
    return this.subscribe('incidents', callback);
};

window.dwSocket.onAnomalyDetected = function(callback) {
    return this.subscribe('anomalies', callback);
};
