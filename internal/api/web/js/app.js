// Main application initialization
class Dashboard {
    constructor() {
        this.statsUpdateInterval = 5000; // 5 seconds
        this.startTime = new Date();
    }

    init() {
        // Initialize WebSocket client
        wsClient.connect();

        // Initialize managers
        deviceManager.init();
        syncManager.init();

        // Set up stats updates
        this.updateStats();
        setInterval(() => this.updateStats(), this.statsUpdateInterval);
    }

    async updateStats() {
        try {
            // Update total devices count
            const devicesCount = deviceManager.devices.size;
            document.getElementById('stat-total-devices').textContent = devicesCount;

            // Update active syncs count
            const activeSyncsCount = syncManager.activeSyncs.size;
            document.getElementById('stat-active-syncs').textContent = activeSyncsCount;

            // Update uptime
            const uptime = this.calculateUptime();
            document.getElementById('stat-uptime').textContent = uptime;

            // Update WebSocket clients count (if available from server)
            const wsClientsElement = document.getElementById('stat-ws-clients');
            if (wsClient.connected) {
                wsClientsElement.textContent = '1'; // This client is connected
            } else {
                wsClientsElement.textContent = '0';
            }
        } catch (error) {
            console.error('Failed to update stats:', error);
        }
    }

    calculateUptime() {
        const now = new Date();
        const diff = now - this.startTime;

        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

        if (days > 0) {
            return `${days}d ${hours % 24}h`;
        } else if (hours > 0) {
            return `${hours}h ${minutes % 60}m`;
        } else if (minutes > 0) {
            return `${minutes}m ${seconds % 60}s`;
        } else {
            return `${seconds}s`;
        }
    }
}

// Global dashboard instance
const dashboard = new Dashboard();

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    console.log('Comic Server Dashboard initializing...');
    dashboard.init();
    console.log('Dashboard initialized');
});
