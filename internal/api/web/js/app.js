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
            const [statsRes, listsRes] = await Promise.all([
                fetch('/api/stats'),
                fetch('/api/library/lists')
            ]);
            const stats = await statsRes.json();
            const lists = await listsRes.json();

            const set = (id, val) => {
                const el = document.getElementById(id);
                if (el) el.textContent = val;
            };

            set('stat-registered-devices', stats.registered_devices ?? '--');
            set('stat-active-syncs', stats.active_syncs ?? '--');
            set('stat-uptime', this.formatUptime(stats.uptime_seconds ?? 0));
            set('stat-smart-lists', (lists.lists ?? []).length);
        } catch (error) {
            console.error('Failed to update stats:', error);
        }
    }

    formatUptime(seconds) {
        if (seconds < 60) return `${seconds}s`;
        const m = Math.floor(seconds / 60);
        if (m < 60) return `${m}m ${seconds % 60}s`;
        const h = Math.floor(m / 60);
        if (h < 24) return `${h}h ${m % 60}m`;
        const d = Math.floor(h / 24);
        return `${d}d ${h % 24}h`;
    }
}

// Global dashboard instance
const dashboard = new Dashboard();

// Global browser instances (created on demand)
let listsBrowser = null;
let devicesBrowser = null;
let syncHistoryBrowser = null;
let deviceDetail = null; // Current device detail view
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

    router.register('/lists', async (params, ctx) => {
        navigation.setActive('lists');
        dashboard.hide();
        if (!listsTree) {
            listsTree = new ListsTree();
            await listsTree.init();
            if (ctx.aborted) return;
        }
        if (!listsBrowser) {
            listsBrowser = new ListsBrowser(listsTree);
        }
        await listsBrowser.init(); // synchronous render, no check needed
    });

    router.register('/lists/:listId', async (params, ctx) => {
        navigation.setActive('lists');
        dashboard.hide();
        if (!listsTree) {
            listsTree = new ListsTree();
            await listsTree.init();
            if (ctx.aborted) return;
        }
        const listDetail = new ListDetail(params.listId, listsTree);
        await listDetail.init(ctx);
    });

    router.register('/devices', async (params, ctx) => {
        navigation.setActive('devices');
        dashboard.hide();
        if (!devicesBrowser) {
            devicesBrowser = new DevicesBrowser();
        }
        await devicesBrowser.init(ctx);
    });

    router.register('/sync', async (params, ctx) => {
        navigation.setActive('sync');
        dashboard.hide();
        if (!syncHistoryBrowser) {
            syncHistoryBrowser = new SyncHistoryBrowser();
        }
        await syncHistoryBrowser.init(ctx);
    });

    router.register('/devices/:deviceId', async (params, ctx) => {
        navigation.setActive('devices');
        dashboard.hide();
        deviceDetail = new DeviceDetail(params.deviceId);
        await deviceDetail.init(ctx);
    });

    router.register('/devices/:deviceId/settings', async (params, ctx) => {
        navigation.setActive('devices');
        dashboard.hide();
        const deviceSettings = new DeviceSettings(params.deviceId);
        await deviceSettings.init(ctx);
    });

    console.log('Dashboard initialized with routing');

    // Handle initial route after all routes are registered
    router.handleRoute();
});
