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

        // Komga sync status is purely informational (comic-server-em05) -
        // folded into a Dashboard panel instead of its own route/tab, so
        // it's loaded once here rather than via the router.
        if (!komgaStatus) {
            komgaStatus = new KomgaStatus();
        }
        komgaStatus.initDashboardPanel();

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
            const [statsRes, listsRes, trashRes] = await Promise.all([
                fetch('/api/stats'),
                fetch('/api/library/lists'),
                fetch('/api/trash')
            ]);
            const stats = await statsRes.json();
            const lists = await listsRes.json();
            // /api/trash returns 200 with entries: [] when trash isn't
            // configured - nothing to show a count for, not an error.
            const trashData = trashRes.ok ? await trashRes.json() : { entries: [] };

            const set = (id, val) => {
                const el = document.getElementById(id);
                if (el) el.textContent = val;
            };

            set('stat-registered-devices', stats.registered_devices ?? '--');
            set('stat-active-syncs', stats.active_syncs ?? '--');
            set('stat-uptime', this.formatUptime(stats.uptime_seconds ?? 0));
            set('stat-smart-lists', (lists.lists ?? []).length);

            // Keeps the nav "Lists" count badge populated on every route, not
            // just after visiting /lists once (listsBrowser.js only updates
            // it from its own render). This runs on every page load/refresh
            // regardless of current route, since dashboard.init() always
            // runs on startup.
            navigation.updateBadge('lists', (lists.lists ?? []).length);

            // Nothing else ever called this for "devices" - the badge was
            // dead code, permanently hidden regardless of route.
            navigation.updateBadge('devices', stats.registered_devices ?? 0);

            navigation.updateBadge('trash', (trashData.entries ?? []).length);

            // Pending ComicVine scrape reviews (comic-server-0nhp); 503
            // when the scraper isn't configured, which just means 0.
            try {
                const reviewRes = await fetch('/api/scrape/review');
                const pending = reviewRes.ok ? await reviewRes.json() : [];
                navigation.updateBadge('scrape-review', (pending || []).length);
            } catch (e) { /* badge stays hidden */ }
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
let komgaStatus = null;
let trashBrowser = null;
let scrapeReviewPage = null;
let duplicatesPage = null;
let physicalPage = null;
let bookDetailPage = null;
let historyPage = null;
let variantCoversPage = null;
let missingIssuesPage = null;
let scanInfoSettings = null;
let dataManagerPage = null;
let dataManagerRulesPage = null;
let workflowPage = null;
let organizePage = null;
let libraryOrganizerProfilesPage = null;
let browsePage = null;
let deviceDetail = null; // Current device detail view
let deviceSettings = null; // Current device settings view
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

    // Resolve this window's theme (server default, unless a per-window
    // toggle override already applied synchronously in index.html's
    // <head>) and wire up the header toggle button.
    themeManager.init();

    // Initialize navigation
    navigation.init();

    // Initialize dashboard
    dashboard.init();

    // Show build version/commit in the header for tying test observations
    // back to a specific deploy. Release builds (GoReleaser injects the real
    // tag via -X cmd.version) show that tag; local/dev builds default to
    // "dev" (see cmd/version.go), which isn't useful to display, so those
    // fall back to the short commit SHA instead.
    fetch('/api/version')
        .then(res => res.json())
        .then(info => {
            const el = document.getElementById('build-version');
            if (el && info.git_commit) {
                el.textContent = info.version && info.version !== 'dev'
                    ? `v${info.version}`
                    : info.git_commit.slice(0, 7);
                el.title = `Version: ${info.version}\nCommit: ${info.git_commit}\nBuilt: ${info.build_date}`;
            }
        })
        .catch(error => console.error('Failed to load version info:', error));

    // Register routes
    router.register('/', () => {
        navigation.setActive('dashboard');
        // Restore dashboard content if it was replaced
        const app = document.getElementById('app');
        const dashboardContent = document.getElementById('dashboard-content');
        if (app && !dashboardContent && dashboardHTML) {
            app.innerHTML = dashboardHTML;
            dashboard.updateStats();
            // Re-render from already-cached data (kept fresh by background
            // polling even while these panels were detached) instead of
            // waiting for the next 5-10s interval tick to repopulate them.
            deviceManager.render();
            syncManager.renderActiveSyncs();
            syncManager.renderSyncHistory();
        }
        dashboard.show();
    });

    router.register('/lists', async (params, ctx) => {
        navigation.setActive('lists');
        dashboard.hide();
        if (!listsTree) {
            // The first-ever visit to /lists this session pays for TWO
            // sequential fetches (the tree, then listsBrowser's own data)
            // before anything renders - show something immediately rather
            // than leaving #app showing whatever the previous page left
            // behind (comic-server-4te).
            document.getElementById('app').innerHTML = '<div class="panel"><p class="empty-message">Loading…</p></div>';
            listsTree = new ListsTree();
            await listsTree.init();
            if (ctx.aborted) return;
        } else {
            // Don't await: this is a background refresh of a persistent,
            // already-rendered sidebar, not part of this navigation's
            // critical path (see refreshTree's comment).
            listsTree.refreshTree();
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
            document.getElementById('app').innerHTML = '<div class="panel"><p class="empty-message">Loading…</p></div>';
            listsTree = new ListsTree();
            await listsTree.init();
            if (ctx.aborted) return;
        } else {
            listsTree.refreshTree();
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

    router.register('/datamanager', async (params, ctx) => {
        navigation.setActive('datamanager');
        dashboard.hide();
        if (!dataManagerPage) {
            dataManagerPage = new DataManagerPage();
        }
        await dataManagerPage.init(ctx);
    });

    router.register('/datamanager/rules', async (params, ctx) => {
        navigation.setActive('datamanager');
        dashboard.hide();
        if (!dataManagerRulesPage) {
            dataManagerRulesPage = new DataManagerRulesPage();
        }
        await dataManagerRulesPage.init(ctx);
    });

    router.register('/workflow', async (params, ctx) => {
        navigation.setActive('workflow');
        dashboard.hide();
        if (!workflowPage) {
            workflowPage = new WorkflowPage();
        }
        await workflowPage.init(ctx);
    });

    router.register('/browse', async (params, ctx) => {
        navigation.setActive('browse');
        dashboard.hide();
        if (!browsePage) {
            browsePage = new BrowsePage();
        }
        await browsePage.init(ctx);
    });

    router.register('/organize', async (params, ctx) => {
        navigation.setActive('workflow');
        dashboard.hide();
        if (!organizePage) {
            organizePage = new OrganizePage();
        }
        await organizePage.init(ctx);
    });

    router.register('/organize/profiles', async (params, ctx) => {
        navigation.setActive('workflow');
        dashboard.hide();
        if (!libraryOrganizerProfilesPage) {
            libraryOrganizerProfilesPage = new LibraryOrganizerProfilesPage();
        }
        await libraryOrganizerProfilesPage.init(ctx);
    });

    router.register('/settings', async (params, ctx) => {
        navigation.setActive('settings');
        dashboard.hide();
        if (!scanInfoSettings) {
            scanInfoSettings = new ScanInfoSettings();
        }
        await scanInfoSettings.init(ctx);
    });

    router.register('/scrape-review', async (params, ctx) => {
        navigation.setActive('scrape-review');
        dashboard.hide();
        if (!scrapeReviewPage) {
            scrapeReviewPage = new ScrapeReviewPage();
        }
        await scrapeReviewPage.init(ctx);
    });

    router.register('/missing-issues', async (params, ctx) => {
        navigation.setActive('missing-issues');
        dashboard.hide();
        if (!missingIssuesPage) {
            missingIssuesPage = new MissingIssuesPage();
        }
        await missingIssuesPage.init(ctx);
    });

    router.register('/variant-covers', async (params, ctx) => {
        navigation.setActive('variant-covers');
        dashboard.hide();
        if (!variantCoversPage) {
            variantCoversPage = new VariantCoversPage();
        }
        await variantCoversPage.init(ctx);
    });

    router.register('/history', async (params, ctx) => {
        navigation.setActive('history');
        dashboard.hide();
        if (!historyPage) {
            historyPage = new HistoryPage();
        }
        await historyPage.init(ctx);
    });

    router.register('/duplicates', async (params, ctx) => {
        navigation.setActive('duplicates');
        dashboard.hide();
        if (!duplicatesPage) {
            duplicatesPage = new DuplicatesPage();
        }
        await duplicatesPage.init(ctx);
    });

    router.register('/physical', async () => {
        navigation.setActive('physical');
        dashboard.hide();
        if (!physicalPage) {
            physicalPage = new PhysicalPage();
        }
        await physicalPage.init();
    });

    router.register('/books/:bookId', async (params, ctx) => {
        navigation.setActive('browse');
        dashboard.hide();
        if (!bookDetailPage) {
            bookDetailPage = new BookDetailPage();
        }
        await bookDetailPage.init(ctx, params.bookId);
    });

    // In-app links (<a href="/books/..." data-route>) navigate without a
    // page load; modifier clicks still open a new tab.
    document.addEventListener('click', (e) => {
        const a = e.target.closest('a[data-route]');
        if (!a || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        e.preventDefault();
        router.navigate(a.getAttribute('href'));
    });

    router.register('/trash', async (params, ctx) => {
        navigation.setActive('trash');
        dashboard.hide();
        if (!trashBrowser) {
            trashBrowser = new TrashBrowser();
        }
        await trashBrowser.init(ctx);
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
        deviceSettings = new DeviceSettings(params.deviceId);
        await deviceSettings.init(ctx);
    });

    console.log('Dashboard initialized with routing');

    // Handle initial route after all routes are registered
    router.handleRoute();
});
