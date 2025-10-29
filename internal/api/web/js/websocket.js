// WebSocket connection management
class WebSocketClient {
    constructor() {
        this.ws = null;
        this.reconnectInterval = 3000;
        this.reconnectTimer = null;
        this.eventHandlers = {};
        this.connected = false;
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        console.log('Connecting to WebSocket:', wsUrl);
        this.updateStatus('connecting');

        try {
            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                console.log('WebSocket connected');
                this.connected = true;
                this.updateStatus('connected');

                // Clear reconnect timer
                if (this.reconnectTimer) {
                    clearTimeout(this.reconnectTimer);
                    this.reconnectTimer = null;
                }
            };

            this.ws.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    this.handleEvent(data);
                } catch (error) {
                    console.error('Failed to parse WebSocket message:', error);
                }
            };

            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.updateStatus('disconnected');
            };

            this.ws.onclose = () => {
                console.log('WebSocket disconnected');
                this.connected = false;
                this.updateStatus('disconnected');
                this.scheduleReconnect();
            };
        } catch (error) {
            console.error('Failed to create WebSocket:', error);
            this.updateStatus('disconnected');
            this.scheduleReconnect();
        }
    }

    scheduleReconnect() {
        if (this.reconnectTimer) return;

        console.log(`Reconnecting in ${this.reconnectInterval}ms...`);
        this.reconnectTimer = setTimeout(() => {
            this.reconnectTimer = null;
            this.connect();
        }, this.reconnectInterval);
    }

    disconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }

        this.connected = false;
        this.updateStatus('disconnected');
    }

    on(eventType, handler) {
        if (!this.eventHandlers[eventType]) {
            this.eventHandlers[eventType] = [];
        }
        this.eventHandlers[eventType].push(handler);
    }

    handleEvent(event) {
        console.log('WebSocket event:', event);

        const handlers = this.eventHandlers[event.type];
        if (handlers) {
            handlers.forEach(handler => {
                try {
                    handler(event.data, event.timestamp);
                } catch (error) {
                    console.error(`Error in event handler for ${event.type}:`, error);
                }
            });
        }
    }

    updateStatus(status) {
        const indicator = document.getElementById('ws-status');
        const text = document.getElementById('ws-status-text');

        if (!indicator || !text) return;

        indicator.className = `status-indicator ${status}`;

        switch (status) {
            case 'connected':
                text.textContent = 'Connected';
                break;
            case 'connecting':
                text.textContent = 'Connecting...';
                break;
            case 'disconnected':
                text.textContent = 'Disconnected';
                break;
        }
    }
}

// Global WebSocket client instance
const wsClient = new WebSocketClient();
