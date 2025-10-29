// Sync progress monitoring
class SyncManager {
    constructor() {
        this.activeSyncs = new Map();
        this.syncHistory = [];
        this.activeSyncsContainer = document.getElementById('active-syncs');
        this.syncHistoryContainer = document.getElementById('sync-history');
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
        if (this.activeSyncs.size === 0) {
            this.activeSyncsContainer.innerHTML = '<p class="empty-message">No active sync operations</p>';
            return;
        }

        this.activeSyncsContainer.innerHTML = '';

        for (const sync of this.activeSyncs.values()) {
            const progressEl = this.createSyncProgress(sync);
            this.activeSyncsContainer.appendChild(progressEl);
        }
    }

    createSyncProgress(sync) {
        const progress = this.syncProgressTemplate.content.cloneNode(true);
        const progressEl = progress.querySelector('.sync-progress');

        progressEl.dataset.deviceId = sync.device_id;

        // Set device name
        progress.querySelector('.sync-device-name').textContent = sync.device_name || sync.device_id;

        // Set status
        progress.querySelector('.sync-status').textContent = 'In Progress';

        // Set current file
        const currentFile = sync.current_file || 'Preparing...';
        progress.querySelector('.sync-current-file').textContent = `Current: ${currentFile}`;

        // Set progress bar
        let progressPercent = 0;
        if (sync.total_files > 0) {
            progressPercent = (sync.completed_files / sync.total_files) * 100;
        }
        progress.querySelector('.progress-fill').style.width = `${progressPercent}%`;

        // Set stats
        const filesText = `Files: ${sync.completed_files || 0}/${sync.total_files || 0}`;
        const bytesText = this.formatBytes(sync.bytes_transferred || 0);
        const speedText = sync.transfer_speed ? this.formatSpeed(sync.transfer_speed) : '';

        progress.querySelector('.sync-stats').textContent =
            `${filesText} • ${bytesText}${speedText ? ' • ' + speedText : ''}`;

        return progress;
    }

    renderSyncHistory() {
        if (this.syncHistory.length === 0) {
            this.syncHistoryContainer.innerHTML = '<p class="empty-message">No sync history</p>';
            return;
        }

        this.syncHistoryContainer.innerHTML = '';

        this.syncHistory.slice(0, 10).forEach(sync => {
            const item = this.createHistoryItem(sync);
            this.syncHistoryContainer.appendChild(item);
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

    formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
    }

    formatSpeed(bytesPerSecond) {
        return this.formatBytes(bytesPerSecond) + '/s';
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
