// Device Detail Page
class DeviceDetail {
    constructor(deviceId) {
        this.deviceId = deviceId;
        this.device = null;
        this.syncHistory = [];
        this.historyOffset = 0;
        this.historyLimit = 10;
        this.historyTotal = 0;
        this.historyLoading = false;
        this.syncProgress = null; // Track active sync progress
    }

    async init(ctx) {
        await this.loadDeviceInfo();
        if (ctx && ctx.aborted) return;
        if (this.device) {
            this.render();
            this.attachListeners();
            this.startSyncPolling(ctx);
            await this.loadSyncHistory();
            if (ctx && ctx.aborted) return;
        }
    }

    // This used to rely on sync_started/sync_progress/sync_completed/
    // sync_failed WebSocket events, but those are never actually
    // broadcast by the server - nothing in cmd/server.go's sync path calls
    // wsHub.Broadcast for them - so this.syncProgress was permanently null
    // and renderSyncProgress() never showed anything (comic-server-p0x).
    // Poll the same GET /api/sync/status the dashboard already uses
    // instead, filtered to this device, until a real event pipeline exists
    // to replace it.
    //
    // ctx is the router navigation context (see router.js): unlike
    // dashboard.js/devices.js's singleton, session-lifetime setIntervals,
    // this DeviceDetail instance is recreated on every navigation to
    // /devices/:deviceId and there's no page-teardown hook to clear this
    // timer from the outside - so pollSyncStatus checks ctx.aborted itself
    // on every tick and self-cancels once the router has moved on,
    // instead of leaking a timer that would otherwise keep clobbering
    // whatever page the user navigated to next with stale device markup.
    startSyncPolling(ctx) {
        this.pollCtx = ctx;
        this.pollSyncStatus();
        this.syncPollTimer = setInterval(() => this.pollSyncStatus(), 5000);
    }

    stopSyncPolling() {
        if (this.syncPollTimer) {
            clearInterval(this.syncPollTimer);
            this.syncPollTimer = null;
        }
    }

    async pollSyncStatus() {
        if (this.pollCtx && this.pollCtx.aborted) {
            this.stopSyncPolling();
            return;
        }

        try {
            const response = await fetch('/api/sync/status');
            if (this.pollCtx && this.pollCtx.aborted) {
                this.stopSyncPolling();
                return;
            }
            if (!response.ok) return;
            const data = await response.json();
            const mine = (data.active_syncs || []).find(s => s.device_id === this.deviceId) || null;

            if (JSON.stringify(mine) === JSON.stringify(this.syncProgress)) {
                return; // nothing changed - skip the re-render
            }

            const hadPanel = !!this.syncProgress;
            this.syncProgress = mine;
            this.render();
            this.attachListeners();

            if (hadPanel && !mine) {
                // A sync for this device just ended - refresh history
                // instead of waiting for the next scheduled reload.
                this.loadSyncHistory();
            }
        } catch (error) {
            console.error('Failed to poll sync status:', error);
        }
    }

    async loadDeviceInfo() {
        try {
            const response = await fetch(`/api/devices/${this.deviceId}`);

            if (response.status === 404) {
                this.showError("Device not found. It may have been unregistered.");
                return;
            }

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            this.device = await response.json();
        } catch (error) {
            console.error('Failed to load device:', error);
            this.showError("Failed to load device information. Please try again.");
        }
    }

    async loadSyncHistory() {
        if (this.historyLoading) return;

        this.historyLoading = true;
        this.renderHistoryLoading();

        try {
            const url = `/api/devices/${this.deviceId}/sync-history?limit=${this.historyLimit}&offset=${this.historyOffset}`;
            const response = await fetch(url);

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();

            if (this.historyOffset === 0) {
                this.syncHistory = data.history || [];
            } else {
                this.syncHistory = [...this.syncHistory, ...(data.history || [])];
            }

            this.historyTotal = data.total || 0;
            this.historyHasMore = data.has_more || false;
            this.historyNextOffset = data.next_offset;

            this.renderSyncHistory();
        } catch (error) {
            console.error('Failed to load sync history:', error);
            this.renderHistoryError();
        } finally {
            this.historyLoading = false;
        }
    }

    render() {
        const app = document.getElementById('app');

        app.innerHTML = `
            <div class="device-detail-page">
                <!-- Breadcrumb -->
                <nav class="breadcrumb">
                    <a href="/" onclick="router.navigate('/'); return false;">Dashboard</a>
                    <span class="separator">›</span>
                    <span class="current">${this.escapeHtml(this.device.friendly_name || this.device.name)}</span>
                </nav>

                <!-- Device Header -->
                <div class="device-detail-header">
                    <div class="device-header-main">
                        <h1>${this.escapeHtml(this.device.friendly_name || this.device.name)}</h1>
                        ${this.renderStatusBadge()}
                    </div>
                    <div class="device-header-actions">
                        ${this.renderSyncNowButton()}
                        <button class="btn btn-secondary" onclick="router.navigate('/devices/${this.deviceId}/settings'); return false;">
                            ⚙️ Settings
                        </button>
                    </div>
                </div>

                <!-- Device Info Cards -->
                <div class="device-info-cards">
                    ${this.renderInfoCards()}
                </div>

                <!-- Current Sync Panel (shown only when syncing) -->
                ${this.renderSyncProgress()}

                <!-- Assigned Lists Panel -->
                <div class="panel assigned-lists-panel">
                    <h2>Assigned Smart Lists</h2>
                    ${this.renderAssignedLists()}
                </div>

                <!-- Sync History Panel -->
                <div class="panel sync-history-panel">
                    <h2>Sync History</h2>
                    <div id="sync-history-content">
                        <div class="loading-spinner">Loading history...</div>
                    </div>
                </div>
            </div>
        `;
    }

    // Sync Now is only offered when the device is actually connected
    // (device.ip is only populated from the registry - see
    // handleGetDeviceDetail - so an offline/config-only device has
    // nothing to dial) and not already mid-sync. comic-server-yfp.
    renderSyncNowButton() {
        if (!this.device.ip) {
            return '';
        }
        if (this.device.is_syncing) {
            return `<button class="btn btn-primary" disabled>⏳ Syncing…</button>`;
        }
        return `<button class="btn btn-primary" onclick="deviceDetail.triggerSyncNow()">🔄 Sync Now</button>`;
    }

    async triggerSyncNow() {
        try {
            const response = await fetch(`/api/devices/${this.deviceId}/sync`, { method: 'POST' });

            if (response.status === 202) {
                // Sync accepted. The next pollSyncStatus tick (up to 5s
                // away) will also flip this via the sync progress panel,
                // but set it immediately too so the button disables
                // without waiting on that round trip.
                this.device.is_syncing = true;
                this.render();
                this.attachListeners();
                return;
            }

            if (response.status === 409) {
                alert('This device is already syncing.');
                return;
            }

            if (response.status === 404) {
                alert('This device is not currently connected.');
                await this.loadDeviceInfo();
                if (this.device) {
                    this.render();
                    this.attachListeners();
                }
                return;
            }

            const text = await response.text();
            throw new Error(text || `HTTP ${response.status}`);
        } catch (error) {
            console.error('Failed to trigger sync:', error);
            alert('Failed to start sync. Please try again.');
        }
    }

    renderStatusBadge() {
        if (this.device.is_syncing) {
            return '<span class="status-badge syncing">Syncing</span>';
        }

        // Check last_seen to determine online/offline
        if (this.device.last_seen) {
            const lastSeen = new Date(this.device.last_seen);
            const minutesAgo = (Date.now() - lastSeen.getTime()) / 1000 / 60;

            if (minutesAgo < 2) {
                return '<span class="status-badge online">Online</span>';
            } else if (minutesAgo < 30) {
                return '<span class="status-badge idle">Idle</span>';
            }
        }

        return '<span class="status-badge offline">Offline</span>';
    }

    renderInfoCards() {
        return `
            <div class="info-card">
                <div class="info-card-label">Model</div>
                <div class="info-card-value">${this.escapeHtml(this.device.model || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Manufacturer</div>
                <div class="info-card-value">${this.escapeHtml(this.device.manufacturer || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Edition</div>
                <div class="info-card-value">${this.escapeHtml(this.device.edition || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">IP Address</div>
                <div class="info-card-value">${this.escapeHtml(this.device.ip || 'Unknown')}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Last Seen</div>
                <div class="info-card-value">${this.formatTimestamp(this.device.last_seen)}</div>
            </div>
            <div class="info-card">
                <div class="info-card-label">Device ID</div>
                <div class="info-card-value device-id">${this.escapeHtml(this.device.id)}</div>
            </div>
        `;
    }

    // this.syncProgress is a syncstate.SyncState from GET /api/sync/status
    // (see pollSyncStatus) - progress/books_total/books_added/
    // books_updated/books_deleted/error_count/detail.
    renderSyncProgress() {
        // Only show if device is currently syncing
        if (!this.syncProgress) {
            return '';
        }

        const progressPercent = this.syncProgress.progress || 0;
        const added = this.syncProgress.books_added || 0;
        const updated = this.syncProgress.books_updated || 0;
        const deleted = this.syncProgress.books_deleted || 0;
        const total = this.syncProgress.books_total || 0;
        const done = added + updated + deleted;
        const detail = this.syncProgress.detail
            || (total > 0 ? 'Syncing...' : 'Preparing...');

        return `
            <div class="panel sync-progress-panel">
                <h2>Current Sync</h2>
                <div class="sync-progress-content">
                    <div class="sync-progress-info">
                        <div class="sync-current-file">
                            <span class="label">Status:</span>
                            <span class="value">${this.escapeHtml(detail)}</span>
                        </div>
                        <div class="sync-file-count">
                            ${done} / ${total} books (${added} added, ${updated} updated, ${deleted} deleted)
                            ${this.syncProgress.error_count ? ` • ${this.syncProgress.error_count} error${this.syncProgress.error_count !== 1 ? 's' : ''}` : ''}
                        </div>
                    </div>
                    <div class="sync-progress-bar">
                        <div class="progress-bar-container">
                            <div class="progress-bar-fill" style="width: ${progressPercent}%"></div>
                        </div>
                        <div class="progress-percentage">${progressPercent}%</div>
                    </div>
                </div>
            </div>
        `;
    }

    renderAssignedLists() {
        const hasLists = this.device.lists && this.device.lists.length > 0;

        return `
            <div class="assigned-lists-header">
                <button class="btn btn-primary" onclick="deviceDetail.showAssignListModal()">
                    + Assign List
                </button>
            </div>

            ${hasLists ? `
                <table class="assigned-lists-table">
                    <thead>
                        <tr>
                            <th>List Name</th>
                            <th>Books</th>
                            <th>Status</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${this.device.lists.map(list => `
                            <tr>
                                <td>
                                    <a href="/lists/${list.list_id}"
                                       onclick="router.navigate('/lists/${list.list_id}'); return false;"
                                       class="list-link">
                                        ${this.escapeHtml(list.list_name)}
                                    </a>
                                </td>
                                <td>${list.book_count.toLocaleString()}</td>
                                <td>
                                    <label class="toggle-switch">
                                        <input type="checkbox"
                                               ${list.enabled ? 'checked' : ''}
                                               onchange="deviceDetail.toggleListEnabled('${list.list_id}', this.checked)">
                                        <span class="toggle-slider"></span>
                                    </label>
                                    <span class="status-text">${list.enabled ? 'Enabled' : 'Disabled'}</span>
                                </td>
                                <td>
                                    <button class="btn btn-small btn-secondary"
                                            onclick="router.navigate('/lists/${list.list_id}'); return false;">
                                        View
                                    </button>
                                    <button class="btn btn-small btn-danger"
                                            onclick="deviceDetail.removeList('${list.list_id}', '${this.escapeHtml(list.list_name)}')">
                                        Remove
                                    </button>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            ` : `
                <div class="empty-state">
                    <p>No smart lists assigned to this device.</p>
                    <p class="help-text">Click "Assign List" to add smart lists for syncing.</p>
                </div>
            `}
        `;
    }

    renderHistoryLoading() {
        const content = document.getElementById('sync-history-content');
        if (content) {
            content.innerHTML = '<div class="loading-spinner">Loading history...</div>';
        }
    }

    renderHistoryError() {
        const content = document.getElementById('sync-history-content');
        if (content) {
            content.innerHTML = `
                <div class="error-message">
                    <p>Failed to load sync history.</p>
                    <button class="btn btn-secondary" onclick="location.reload()">Retry</button>
                </div>
            `;
        }
    }

    renderSyncHistory() {
        const content = document.getElementById('sync-history-content');
        if (!content) return;

        if (this.syncHistory.length === 0) {
            content.innerHTML = `
                <div class="empty-state">
                    <p>No sync history yet.</p>
                    <p class="help-text">Sync history will appear here after the first sync.</p>
                </div>
            `;
            return;
        }

        content.innerHTML = `
            <div class="sync-history-list">
                ${this.syncHistory.map(session => this.renderHistoryItem(session)).join('')}
            </div>
            ${this.renderLoadMoreButton()}
        `;
    }

    renderHistoryItem(session) {
        const startTime = new Date(session.start_time);
        const endTime = new Date(session.end_time);
        const duration = Math.round((endTime - startTime) / 1000); // seconds

        const statusClass = session.status === 'completed' ? 'success' :
                          session.status === 'failed' ? 'error' : 'warning';

        return `
            <div class="history-item">
                <div class="history-item-header">
                    <span class="history-timestamp">${this.formatTimestamp(session.start_time)}</span>
                    <span class="history-status status-${statusClass}">${session.status}</span>
                </div>
                <div class="history-item-stats">
                    <span class="history-stat">
                        <span class="stat-icon">📚</span>
                        ${session.files_added || 0} added
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">🔄</span>
                        ${session.files_updated || 0} updated
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">🗑️</span>
                        ${session.files_deleted || 0} deleted
                    </span>
                    <span class="history-stat">
                        <span class="stat-icon">⏱️</span>
                        ${this.formatDuration(duration)}
                    </span>
                </div>
            </div>
        `;
    }

    renderLoadMoreButton() {
        if (!this.historyHasMore) {
            return '';
        }

        return `
            <div class="load-more-container">
                <button id="load-more-history" class="btn btn-secondary">
                    Load More
                </button>
            </div>
        `;
    }

    attachListeners() {
        // Will attach load more listener after history renders
        this.attachHistoryListeners();
    }

    attachHistoryListeners() {
        const loadMoreBtn = document.getElementById('load-more-history');
        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', async () => {
                loadMoreBtn.disabled = true;
                loadMoreBtn.textContent = 'Loading...';

                this.historyOffset = this.historyNextOffset;
                await this.loadSyncHistory();

                // Button will be re-rendered by renderSyncHistory
            });
        }
    }

    showError(message) {
        document.getElementById('app').innerHTML = `
            <div class="error-page">
                <h1>Error</h1>
                <p>${this.escapeHtml(message)}</p>
                <button onclick="router.navigate('/')" class="btn btn-primary">
                    Back to Dashboard
                </button>
            </div>
        `;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return 'Never';

        const date = new Date(timestamp);
        const now = new Date();
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / 60000);

        if (diffMins < 1) return 'Just now';
        if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;

        const diffHours = Math.floor(diffMins / 60);
        if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;

        const diffDays = Math.floor(diffHours / 24);
        if (diffDays < 7) return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;

        // Format as date
        return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }

    formatDuration(seconds) {
        if (seconds < 60) return `${seconds}s`;

        const mins = Math.floor(seconds / 60);
        const secs = seconds % 60;
        return `${mins}m ${secs}s`;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    // List assignment methods
    showAssignListModal() {
        const excludeListIds = (this.device.lists || []).map(l => l.list_id);
        listPicker.open({
            title: 'Assign Smart List',
            excludeListIds,
            onSelect: (listId, listName) => this.assignList(listId, listName)
        });
    }

    async assignList(listId, listName) {
        try {
            const response = await fetch(`/api/devices/lists/${this.deviceId}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    list_id: listId,
                    list_name: listName,
                    enabled: true
                })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            // Reload device info to show the new list
            await this.loadDeviceInfo();
            if (this.device) {
                this.render();
                this.attachListeners();
            }

            alert(`Successfully assigned "${listName}" to this device.`);
        } catch (error) {
            console.error('Failed to assign list:', error);
            alert(`Failed to assign list: ${error.message}`);
        }
    }

    async toggleListEnabled(listId, enabled) {
        try {
            const response = await fetch(`/api/devices/lists/${this.deviceId}/${listId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    enabled: enabled
                })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            // Update local state
            const list = this.device.lists.find(l => l.list_id === listId);
            if (list) {
                list.enabled = enabled;
            }

            console.log(`List ${enabled ? 'enabled' : 'disabled'} successfully`);
        } catch (error) {
            console.error('Failed to toggle list:', error);
            alert(`Failed to update list: ${error.message}`);

            // Revert the checkbox
            await this.loadDeviceInfo();
            if (this.device) {
                this.render();
                this.attachListeners();
            }
        }
    }

    async removeList(listId, listName) {
        if (!confirm(`Remove "${listName}" from this device?\n\nThis will stop syncing this list to the device.`)) {
            return;
        }

        try {
            const response = await fetch(`/api/devices/lists/${this.deviceId}/${listId}`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            // Reload device info to show updated lists
            await this.loadDeviceInfo();
            if (this.device) {
                this.render();
                this.attachListeners();
            }

            alert(`Successfully removed "${listName}" from this device.`);
        } catch (error) {
            console.error('Failed to remove list:', error);
            alert(`Failed to remove list: ${error.message}`);
        }
    }

}
