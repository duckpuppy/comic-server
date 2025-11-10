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
    }

    async init() {
        await this.loadDeviceInfo();
        if (this.device) {
            this.render();
            this.attachListeners();
            await this.loadSyncHistory();
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
                </div>

                <!-- Device Info Cards -->
                <div class="device-info-cards">
                    ${this.renderInfoCards()}
                </div>

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
    async showAssignListModal() {
        // Load available lists
        const lists = await this.loadAvailableLists();
        if (!lists || lists.length === 0) {
            alert('No smart lists available in the library.');
            return;
        }

        // Filter out already assigned lists
        const assignedListIds = (this.device.lists || []).map(l => l.list_id);
        const availableLists = lists.filter(l => !assignedListIds.includes(l.id));

        if (availableLists.length === 0) {
            alert('All available smart lists are already assigned to this device.');
            return;
        }

        // Create modal
        const modalHTML = `
            <div class="modal-overlay" id="assign-list-modal">
                <div class="modal-content">
                    <div class="modal-header">
                        <h2>Assign Smart List</h2>
                        <button class="modal-close" onclick="deviceDetail.closeAssignListModal()">×</button>
                    </div>
                    <div class="modal-body">
                        <p>Select a smart list to assign to this device:</p>
                        <div class="list-selection">
                            ${availableLists.map(list => `
                                <div class="list-option" onclick="deviceDetail.assignList('${list.id}', '${this.escapeHtml(list.name)}')">
                                    <div class="list-option-name">${this.escapeHtml(list.name)}</div>
                                    <div class="list-option-count">${list.book_count || 0} books</div>
                                </div>
                            `).join('')}
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.insertAdjacentHTML('beforeend', modalHTML);
    }

    closeAssignListModal() {
        const modal = document.getElementById('assign-list-modal');
        if (modal) {
            modal.remove();
        }
    }

    async loadAvailableLists() {
        try {
            const response = await fetch('/api/library/lists');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const data = await response.json();
            return data.lists || [];
        } catch (error) {
            console.error('Failed to load smart lists:', error);
            alert('Failed to load smart lists. Please try again.');
            return [];
        }
    }

    async assignList(listId, listName) {
        this.closeAssignListModal();

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
