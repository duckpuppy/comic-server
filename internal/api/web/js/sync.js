// Sync progress monitoring
class SyncManager {
    constructor() {
        this.activeSyncs = new Map();
        this.syncHistory = [];
        this.syncProgressTemplate = document.getElementById('sync-progress-template');
        this.historyItemTemplate = document.getElementById('history-item-template');
    }

    init() {
        // Load initial data
        this.loadActiveSyncs();
        this.loadSyncHistory();

        // Set up WebSocket event handlers
        wsClient.on('sync_started', (data) => this.handleSyncStarted(data));
        wsClient.on('sync_progress', (data) => this.handleSyncProgress(data));
        wsClient.on('sync_completed', (data) => this.handleSyncCompleted(data));
        wsClient.on('sync_failed', (data) => this.handleSyncFailed(data));

        // Refresh periodically
        setInterval(() => {
            this.loadActiveSyncs();
            this.loadSyncHistory();
        }, 5000);
    }

    async loadActiveSyncs() {
        try {
            const response = await fetch('/api/sync/status');
            const data = await response.json();

            // Update active syncs
            const currentIds = new Set(data.active_syncs.map(s => s.device_id));

            // Remove syncs that are no longer active
            for (const id of this.activeSyncs.keys()) {
                if (!currentIds.has(id)) {
                    this.activeSyncs.delete(id);
                }
            }

            // Update or add syncs
            data.active_syncs.forEach(sync => {
                this.activeSyncs.set(sync.device_id, sync);
            });

            this.renderActiveSyncs();
        } catch (error) {
            console.error('Failed to load active syncs:', error);
        }
    }

    async loadSyncHistory() {
        try {
            const response = await fetch('/api/sync/history?limit=10');
            const data = await response.json();

            this.syncHistory = data.history || [];
            this.renderSyncHistory();
        } catch (error) {
            console.error('Failed to load sync history:', error);
        }
    }

    handleSyncStarted(data) {
        console.log('Sync started:', data);
        this.loadActiveSyncs();
    }

    handleSyncProgress(data) {
        console.log('Sync progress:', data);

        const sync = this.activeSyncs.get(data.device_id);
        if (sync) {
            // Update progress data
            Object.assign(sync, data);
            this.renderActiveSyncs();
        }
    }

    handleSyncCompleted(data) {
        console.log('Sync completed:', data);
        this.activeSyncs.delete(data.device_id);
        this.renderActiveSyncs();
        this.loadSyncHistory();
    }

    handleSyncFailed(data) {
        console.log('Sync failed:', data);
        this.activeSyncs.delete(data.device_id);
        this.renderActiveSyncs();
        this.loadSyncHistory();
    }

    renderActiveSyncs() {
        const container = document.getElementById('active-syncs');
        if (!container) return;

        if (this.activeSyncs.size === 0) {
            container.innerHTML = '<p class="empty-message">No active sync operations</p>';
            return;
        }

        container.innerHTML = '';
        for (const sync of this.activeSyncs.values()) {
            container.appendChild(this.createSyncProgress(sync));
        }
    }

    // sync is a syncstate.SyncState (see GET /api/sync/status) -
    // device_id/device_name/status/progress/books_total/books_added/
    // books_updated/books_deleted/error_count/detail. Progress used to
    // read fictional fields (total_files/completed_files/
    // bytes_transferred/transfer_speed) that nothing on the server ever
    // populated, so this bar always sat at 0% for an entire sync despite
    // real per-operation progress in the server logs (comic-server-p0x) -
    // fixed at the source by syncstate.Manager.UpdateProgress now
    // actually being called from the forward-sync loop; this just reads
    // the fields that are really there.
    createSyncProgress(sync) {
        const progress = this.syncProgressTemplate.content.cloneNode(true);
        const progressEl = progress.querySelector('.sync-progress');

        progressEl.dataset.deviceId = sync.device_id;

        // Set device name
        progress.querySelector('.sync-device-name').textContent = sync.device_name || sync.device_id;

        // Set status
        progress.querySelector('.sync-status').textContent =
            sync.status === 'starting' ? 'Starting' : 'In Progress';

        // Set status detail - e.g. "device not responding yet, retrying"
        // (comic-server-134) while nothing else is visibly moving yet.
        progress.querySelector('.sync-current-file').textContent =
            sync.detail || (sync.books_total > 0 ? 'Syncing...' : 'Preparing...');

        // Set progress bar - the server already reports 0-100.
        const progressPercent = sync.progress || 0;
        progress.querySelector('.progress-fill').style.width = `${progressPercent}%`;

        // Set stats - same "total (added/updated/deleted)" shape as
        // history's file stats, for the same sync while it's still running.
        const added = sync.books_added || 0;
        const updated = sync.books_updated || 0;
        const deleted = sync.books_deleted || 0;
        const total = sync.books_total || 0;
        const done = added + updated + deleted;
        let statsText = `Books: ${done}/${total} (${added}/${updated}/${deleted})`;
        if (sync.error_count) {
            statsText += ` • ${sync.error_count} error${sync.error_count !== 1 ? 's' : ''}`;
        }
        progress.querySelector('.sync-stats').textContent = statsText;

        return progress;
    }

    renderSyncHistory() {
        const container = document.getElementById('sync-history');
        if (!container) return;

        if (this.syncHistory.length === 0) {
            container.innerHTML = '<p class="empty-message">No sync history</p>';
            return;
        }

        container.innerHTML = '';
        this.syncHistory.slice(0, 5).forEach(sync => {
            container.appendChild(this.createHistoryItem(sync));
        });
    }

    createHistoryItem(sync) {
        const item = this.historyItemTemplate.content.cloneNode(true);

        // Set device name
        item.querySelector('.history-device').textContent = sync.device_name || sync.device_id;

        // Set timestamp
        const timestamp = new Date(sync.end_time || sync.start_time);
        item.querySelector('.history-timestamp').textContent = this.formatTimestamp(timestamp);

        // Set stats
        const filesAdded = sync.files_added || 0;
        const filesUpdated = sync.files_updated || 0;
        const filesDeleted = sync.files_deleted || 0;
        const totalFiles = filesAdded + filesUpdated + filesDeleted;

        item.querySelector('.history-files').textContent = `${totalFiles} (${filesAdded}/${filesUpdated}/${filesDeleted})`;

        // Calculate duration
        if (sync.start_time && sync.end_time) {
            const duration = new Date(sync.end_time) - new Date(sync.start_time);
            item.querySelector('.history-duration').textContent = this.formatDuration(duration);
        } else {
            item.querySelector('.history-duration').textContent = '--';
        }

        // Set status
        const statusEl = item.querySelector('.history-status');
        if (sync.status === 'completed') {
            statusEl.textContent = 'Success';
            statusEl.className = 'history-status success';
        } else {
            statusEl.textContent = 'Failed';
            statusEl.className = 'history-status failed';
        }

        return item;
    }

    formatDuration(ms) {
        const seconds = Math.floor(ms / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);

        if (hours > 0) {
            return `${hours}h ${minutes % 60}m`;
        } else if (minutes > 0) {
            return `${minutes}m ${seconds % 60}s`;
        } else {
            return `${seconds}s`;
        }
    }

    formatTimestamp(date) {
        const now = new Date();
        const diff = now - date;
        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);

        if (seconds < 60) return 'just now';
        if (minutes < 60) return `${minutes}m ago`;
        if (hours < 24) return `${hours}h ago`;
        return date.toLocaleDateString();
    }
}

// Global sync manager instance
const syncManager = new SyncManager();
