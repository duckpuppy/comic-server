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

    show() {
        // Show dashboard content
        const dashboardContent = document.getElementById('dashboard-content');
        if (dashboardContent) {
            dashboardContent.style.display = 'block';
        }
    }

    hide() {
        // Hide dashboard content
        const dashboardContent = document.getElementById('dashboard-content');
        if (dashboardContent) {
            dashboardContent.style.display = 'none';
        }
    }

    async updateStats() {
        try {
            // Update total devices count
            const devicesCount = deviceManager.devices.size;
            const statElement = document.getElementById('stat-total-devices');
            if (statElement) {
                statElement.textContent = devicesCount;
            }

            // Update active syncs count
            const activeSyncsCount = syncManager.activeSyncs.size;
            const syncElement = document.getElementById('stat-active-syncs');
            if (syncElement) {
                syncElement.textContent = activeSyncsCount;
            }

            // Update uptime
            const uptime = this.calculateUptime();
            const uptimeElement = document.getElementById('stat-uptime');
            if (uptimeElement) {
                uptimeElement.textContent = uptime;
            }

            // Update WebSocket clients count (if available from server)
            const wsClientsElement = document.getElementById('stat-ws-clients');
            if (wsClientsElement) {
                if (wsClient.connected) {
                    wsClientsElement.textContent = '1'; // This client is connected
                } else {
                    wsClientsElement.textContent = '0';
                }
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

// Global browser instances (created on demand)
let listsBrowser = null;
let devicesBrowser = null;
let syncHistoryBrowser = null;
let listsTree = null; // Shared tree instance for lists pages

// Store original dashboard HTML
let dashboardHTML = null;

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    console.log('Comic Server Dashboard initializing...');

    // Store original dashboard content before it can be destroyed
    const app = document.getElementById('app');
    if (app) {
        dashboardHTML = app.innerHTML;
    }

    // Initialize navigation
    navigation.init();

    // Initialize dashboard
    dashboard.init();

    // Register routes
    router.register('/', () => {
        navigation.setActive('dashboard');
        // Restore dashboard content if it was replaced
        const app = document.getElementById('app');
        const dashboardContent = document.getElementById('dashboard-content');
        if (app && !dashboardContent && dashboardHTML) {
            app.innerHTML = dashboardHTML;
        }
        dashboard.show();
    });

    router.register('/lists', async () => {
        navigation.setActive('lists');
        dashboard.hide();
        // Initialize shared tree if needed
        if (!listsTree) {
            listsTree = new ListsTree();
            await listsTree.init();
        }
        if (!listsBrowser) {
            listsBrowser = new ListsBrowser(listsTree);
        }
        await listsBrowser.init();
    });

    router.register('/lists/:listId', async (params) => {
        navigation.setActive('lists');
        dashboard.hide();
        // Initialize shared tree if needed
        if (!listsTree) {
            listsTree = new ListsTree();
            await listsTree.init();
        }
        const listDetail = new ListDetail(params.listId, listsTree);
        await listDetail.init();
    });

    router.register('/devices', async () => {
        navigation.setActive('devices');
        dashboard.hide();
        if (!devicesBrowser) {
            devicesBrowser = new DevicesBrowser();
        }
        await devicesBrowser.init();
    });

    router.register('/sync', async () => {
        navigation.setActive('sync');
        dashboard.hide();
        if (!syncHistoryBrowser) {
            syncHistoryBrowser = new SyncHistoryBrowser();
        }
        await syncHistoryBrowser.init();
    });

    router.register('/devices/:deviceId', async (params) => {
        navigation.setActive('devices');
        dashboard.hide();
        const deviceDetail = new DeviceDetail(params.deviceId);
        await deviceDetail.init();
    });

    console.log('Dashboard initialized with routing');

    // Handle initial route after all routes are registered
    router.handleRoute();
});
