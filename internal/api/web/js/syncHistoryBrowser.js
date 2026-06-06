// Sync History Browser - Full-page view of all sync operations
class SyncHistoryBrowser {
    constructor() {
        this.history = [];
        this.filteredHistory = [];
        this.devices = []; // For device filter dropdown
        this.filterDevice = 'all'; // 'all' or device ID
        this.filterStatus = 'all'; // 'all', 'completed', 'failed', 'aborted'
        this.sortBy = 'recent'; // 'recent', 'oldest', 'duration'
        this.currentPage = 1;
        this.itemsPerPage = 20;
        this.totalItems = 0;
    }

    async init(ctx) {
        await Promise.all([
            this.loadDevices(),
            this.loadHistory()
        ]);
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async loadDevices() {
        try {
            const response = await fetch('/api/devices');
            const data = await response.json();
            this.devices = data.devices || [];
        } catch (error) {
            console.error('Failed to load devices:', error);
            this.devices = [];
        }
    }

    async loadHistory() {
        try {
            const params = new URLSearchParams();
            params.set('limit', '100'); // Load last 100 syncs

            const response = await fetch(`/api/sync/history?${params}`);
            const data = await response.json();
            this.history = data.history || [];
            this.applyFilters();
        } catch (error) {
            console.error('Failed to load sync history:', error);
            this.history = [];
        }
    }

    applyFilters() {
        let filtered = [...this.history];

        // Device filter
        if (this.filterDevice !== 'all') {
            filtered = filtered.filter(item => item.device_id === this.filterDevice);
        }

        // Status filter
        if (this.filterStatus !== 'all') {
            filtered = filtered.filter(item => item.status === this.filterStatus);
        }

        // Sort
        filtered.sort((a, b) => {
            switch (this.sortBy) {
                case 'recent':
                    return new Date(b.start_time) - new Date(a.start_time);
                case 'oldest':
                    return new Date(a.start_time) - new Date(b.start_time);
                case 'duration':
                    const aDuration = new Date(a.end_time) - new Date(a.start_time);
                    const bDuration = new Date(b.end_time) - new Date(b.start_time);
                    return bDuration - aDuration;
                default:
                    return 0;
            }
        });

        this.filteredHistory = filtered;
        this.totalItems = filtered.length;
    }

    getPaginatedHistory() {
        const start = (this.currentPage - 1) * this.itemsPerPage;
        const end = start + this.itemsPerPage;
        return this.filteredHistory.slice(start, end);
    }

    getTotalPages() {
        return Math.ceil(this.totalItems / this.itemsPerPage);
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="sync-history-page">
                <div class="sync-history-header">
                    <h1>Sync History</h1>
                    <p class="sync-history-subtitle">${this.totalItems} sync operation${this.totalItems !== 1 ? 's' : ''}</p>
                </div>

                <div class="sync-history-content">
                    <aside class="sync-history-filters">
                        <h3>Filters</h3>

                        <div class="filter-group">
                            <label>Sort by:</label>
                            <select id="sync-sort-select">
                                <option value="recent" ${this.sortBy === 'recent' ? 'selected' : ''}>Most Recent</option>
                                <option value="oldest" ${this.sortBy === 'oldest' ? 'selected' : ''}>Oldest First</option>
                                <option value="duration" ${this.sortBy === 'duration' ? 'selected' : ''}>Longest Duration</option>
                            </select>
                        </div>

                        <div class="filter-group">
                            <label>Device:</label>
                            <select id="sync-device-filter">
                                <option value="all" ${this.filterDevice === 'all' ? 'selected' : ''}>All Devices</option>
                                ${this.devices.map(device => `
                                    <option value="${device.id}" ${this.filterDevice === device.id ? 'selected' : ''}>
                                        ${this.escapeHtml(device.friendly_name || device.name)}
                                    </option>
                                `).join('')}
                            </select>
                        </div>

                        <div class="filter-group">
                            <label>Status:</label>
                            <select id="sync-status-filter">
                                <option value="all" ${this.filterStatus === 'all' ? 'selected' : ''}>All Status</option>
                                <option value="completed" ${this.filterStatus === 'completed' ? 'selected' : ''}>Completed</option>
                                <option value="failed" ${this.filterStatus === 'failed' ? 'selected' : ''}>Failed</option>
                                <option value="aborted" ${this.filterStatus === 'aborted' ? 'selected' : ''}>Aborted</option>
                            </select>
                        </div>
                    </aside>

                    <main class="sync-history-list">
                        ${this.renderHistoryList()}
                        ${this.renderPagination()}
                    </main>
                </div>
            </div>
        `;
    }

    renderHistoryList() {
        const paginatedHistory = this.getPaginatedHistory();

        if (paginatedHistory.length === 0) {
            return `
                <div class="empty-state">
                    <p>No sync history found</p>
                    ${this.filterDevice !== 'all' || this.filterStatus !== 'all'
                        ? '<p class="empty-hint">Try adjusting your filters</p>'
                        : '<p class="empty-hint">Sync operations will appear here once devices sync</p>'}
                </div>
            `;
        }

        return `
            <div class="sync-items">
                ${paginatedHistory.map(item => this.renderSyncItem(item)).join('')}
            </div>
        `;
    }

    renderSyncItem(item) {
        const device = this.devices.find(d => d.id === item.device_id);
        const deviceName = device ? (device.friendly_name || device.name) : item.device_id;
        const statusClass = `sync-status-${item.status}`;
        const duration = this.formatDuration(new Date(item.end_time) - new Date(item.start_time));
        const timestamp = this.formatTimestamp(item.start_time);

        return `
            <div class="sync-item ${statusClass}">
                <div class="sync-item-header">
                    <div class="sync-device-name">
                        <strong>${this.escapeHtml(deviceName)}</strong>
                        <span class="sync-status-badge">${item.status}</span>
                    </div>
                    <div class="sync-timestamp">${timestamp}</div>
                </div>
                <div class="sync-item-body">
                    <div class="sync-stats">
                        <div class="sync-stat">
                            <span class="sync-stat-label">Duration:</span>
                            <span class="sync-stat-value">${duration}</span>
                        </div>
                        <div class="sync-stat">
                            <span class="sync-stat-label">Files Added:</span>
                            <span class="sync-stat-value">${item.files_added}</span>
                        </div>
                        <div class="sync-stat">
                            <span class="sync-stat-label">Files Updated:</span>
                            <span class="sync-stat-value">${item.files_updated}</span>
                        </div>
                        <div class="sync-stat">
                            <span class="sync-stat-label">Files Deleted:</span>
                            <span class="sync-stat-value">${item.files_deleted}</span>
                        </div>
                    </div>
                    ${item.error ? `
                        <div class="sync-error">
                            <strong>Error:</strong> ${this.escapeHtml(item.error)}
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    renderPagination() {
        const totalPages = this.getTotalPages();

        if (totalPages <= 1) {
            return '';
        }

        const pages = [];
        const maxPagesToShow = 5;
        let startPage = Math.max(1, this.currentPage - Math.floor(maxPagesToShow / 2));
        let endPage = Math.min(totalPages, startPage + maxPagesToShow - 1);

        if (endPage - startPage + 1 < maxPagesToShow) {
            startPage = Math.max(1, endPage - maxPagesToShow + 1);
        }

        for (let i = startPage; i <= endPage; i++) {
            pages.push(i);
        }

        return `
            <div class="pagination">
                <button
                    class="pagination-btn"
                    data-page="${this.currentPage - 1}"
                    ${this.currentPage === 1 ? 'disabled' : ''}
                >
                    Previous
                </button>

                ${pages.map(page => `
                    <button
                        class="pagination-btn ${page === this.currentPage ? 'active' : ''}"
                        data-page="${page}"
                    >
                        ${page}
                    </button>
                `).join('')}

                <button
                    class="pagination-btn"
                    data-page="${this.currentPage + 1}"
                    ${this.currentPage === totalPages ? 'disabled' : ''}
                >
                    Next
                </button>
            </div>
        `;
    }

    attachListeners() {
        // Sort select
        const sortSelect = document.getElementById('sync-sort-select');
        if (sortSelect) {
            sortSelect.addEventListener('change', (e) => {
                this.sortBy = e.target.value;
                this.currentPage = 1;
                this.applyFilters();
                this.updateList();
            });
        }

        // Device filter
        const deviceFilter = document.getElementById('sync-device-filter');
        if (deviceFilter) {
            deviceFilter.addEventListener('change', (e) => {
                this.filterDevice = e.target.value;
                this.currentPage = 1;
                this.applyFilters();
                this.updateList();
            });
        }

        // Status filter
        const statusFilter = document.getElementById('sync-status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', (e) => {
                this.filterStatus = e.target.value;
                this.currentPage = 1;
                this.applyFilters();
                this.updateList();
            });
        }

        // Pagination buttons
        this.attachPaginationListeners();
    }

    attachPaginationListeners() {
        const paginationBtns = document.querySelectorAll('.pagination-btn');
        paginationBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                if (btn.disabled) return;
                const page = parseInt(btn.dataset.page);
                if (page >= 1 && page <= this.getTotalPages()) {
                    this.currentPage = page;
                    this.updateList();
                    // Scroll to top of list
                    document.querySelector('.sync-history-list').scrollIntoView({ behavior: 'smooth' });
                }
            });
        });
    }

    updateList() {
        const listContainer = document.querySelector('.sync-history-list');
        if (listContainer) {
            listContainer.innerHTML = this.renderHistoryList() + this.renderPagination();
            this.attachPaginationListeners();
        }

        // Update subtitle
        const subtitle = document.querySelector('.sync-history-subtitle');
        if (subtitle) {
            subtitle.textContent = `${this.totalItems} sync operation${this.totalItems !== 1 ? 's' : ''}`;
        }
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

    formatTimestamp(timestamp) {
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;

        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

        if (days > 7) {
            return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
        }
        if (days > 1) return `${days} days ago`;
        if (days === 1) return '1 day ago';
        if (hours > 1) return `${hours} hours ago`;
        if (hours === 1) return '1 hour ago';
        if (minutes > 1) return `${minutes} minutes ago`;
        if (minutes === 1) return '1 minute ago';
        return 'just now';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
