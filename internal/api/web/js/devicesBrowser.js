// Devices Browser - Full-page view of all devices
class DevicesBrowser {
    constructor() {
        this.devices = [];
        this.filteredDevices = [];
        this.searchTerm = '';
        this.filterEdition = 'all'; // 'all', 'Android Free', 'Android Full', 'iOS'
        this.filterStatus = 'all'; // 'all', 'online', 'idle', 'offline', 'syncing'
        this.sortBy = 'name'; // 'name', 'name-desc', 'last-seen', 'status'
    }

    async init() {
        await this.loadDevices();
        this.render();
        this.attachListeners();
        this.setupWebSocketListeners();
    }

    async loadDevices() {
        try {
            const response = await fetch('/api/devices');
            const data = await response.json();
            this.devices = data.devices || [];
            this.applyFilters();
        } catch (error) {
            console.error('Failed to load devices:', error);
            this.devices = [];
        }
    }

    getDeviceStatus(device) {
        if (device.is_syncing) {
            return 'syncing';
        }

        const lastSeen = new Date(device.last_seen);
        const now = new Date();
        const diffMinutes = (now - lastSeen) / 1000 / 60;

        if (diffMinutes < 2) return 'online';
        if (diffMinutes < 30) return 'idle';
        return 'offline';
    }

    applyFilters() {
        let filtered = [...this.devices];

        // Search filter
        if (this.searchTerm) {
            const term = this.searchTerm.toLowerCase();
            filtered = filtered.filter(device =>
                device.name.toLowerCase().includes(term) ||
                device.model.toLowerCase().includes(term) ||
                device.ip.toLowerCase().includes(term) ||
                (device.friendly_name && device.friendly_name.toLowerCase().includes(term))
            );
        }

        // Edition filter
        if (this.filterEdition !== 'all') {
            filtered = filtered.filter(device => device.edition === this.filterEdition);
        }

        // Status filter
        if (this.filterStatus !== 'all') {
            filtered = filtered.filter(device => this.getDeviceStatus(device) === this.filterStatus);
        }

        // Sort
        filtered.sort((a, b) => {
            switch (this.sortBy) {
                case 'name':
                    return a.name.localeCompare(b.name);
                case 'name-desc':
                    return b.name.localeCompare(a.name);
                case 'last-seen':
                    return new Date(b.last_seen) - new Date(a.last_seen);
                case 'status':
                    const statusOrder = { syncing: 0, online: 1, idle: 2, offline: 3 };
                    return statusOrder[this.getDeviceStatus(a)] - statusOrder[this.getDeviceStatus(b)];
                default:
                    return 0;
            }
        });

        this.filteredDevices = filtered;
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="devices-browser-page">
                <div class="devices-header">
                    <h1>Devices</h1>
                    <div class="devices-search">
                        <input
                            type="text"
                            id="devices-search-input"
                            placeholder="Search devices..."
                            value="${this.escapeHtml(this.searchTerm)}"
                        >
                    </div>
                </div>

                <div class="devices-content">
                    <aside class="devices-filters">
                        <h3>Filters</h3>

                        <div class="filter-group">
                            <label>Sort by:</label>
                            <select id="devices-sort-select">
                                <option value="name" ${this.sortBy === 'name' ? 'selected' : ''}>Name (A-Z)</option>
                                <option value="name-desc" ${this.sortBy === 'name-desc' ? 'selected' : ''}>Name (Z-A)</option>
                                <option value="last-seen" ${this.sortBy === 'last-seen' ? 'selected' : ''}>Recently Seen</option>
                                <option value="status" ${this.sortBy === 'status' ? 'selected' : ''}>Status</option>
                            </select>
                        </div>

                        <div class="filter-group">
                            <label>Edition:</label>
                            <select id="devices-edition-filter">
                                <option value="all" ${this.filterEdition === 'all' ? 'selected' : ''}>All Editions</option>
                                <option value="Android Free" ${this.filterEdition === 'Android Free' ? 'selected' : ''}>Android Free</option>
                                <option value="Android Full" ${this.filterEdition === 'Android Full' ? 'selected' : ''}>Android Full</option>
                                <option value="iOS" ${this.filterEdition === 'iOS' ? 'selected' : ''}>iOS</option>
                            </select>
                        </div>

                        <div class="filter-group">
                            <label>Status:</label>
                            <select id="devices-status-filter">
                                <option value="all" ${this.filterStatus === 'all' ? 'selected' : ''}>All Status</option>
                                <option value="syncing" ${this.filterStatus === 'syncing' ? 'selected' : ''}>Syncing</option>
                                <option value="online" ${this.filterStatus === 'online' ? 'selected' : ''}>Online</option>
                                <option value="idle" ${this.filterStatus === 'idle' ? 'selected' : ''}>Idle</option>
                                <option value="offline" ${this.filterStatus === 'offline' ? 'selected' : ''}>Offline</option>
                            </select>
                        </div>
                    </aside>

                    <main class="devices-grid">
                        ${this.renderDevicesGrid()}
                    </main>
                </div>
            </div>
        `;
    }

    renderDevicesGrid() {
        if (this.filteredDevices.length === 0) {
            return `
                <div class="empty-state">
                    <p>No devices found</p>
                    ${this.searchTerm || this.filterEdition !== 'all' || this.filterStatus !== 'all'
                        ? '<p class="empty-hint">Try adjusting your search or filters</p>'
                        : '<p class="empty-hint">Devices will appear here once they connect to the server</p>'}
                </div>
            `;
        }

        return this.filteredDevices.map(device => this.renderDeviceCard(device)).join('');
    }

    renderDeviceCard(device) {
        const status = this.getDeviceStatus(device);
        const statusClass = `status-${status}`;
        const displayName = device.friendly_name || device.name;
        const lastSeen = this.formatTimestamp(device.last_seen);
        const isRegistered = device.is_registered || false;

        return `
            <div class="device-card ${isRegistered ? 'registered' : 'unregistered'}" data-device-id="${device.id}">
                <div class="device-card-header">
                    <h3>${this.escapeHtml(displayName)}</h3>
                    <div class="device-badges">
                        <span class="device-status-badge ${statusClass}">${status}</span>
                        ${isRegistered
                            ? '<span class="registration-badge registered">Registered</span>'
                            : '<span class="registration-badge unregistered">Not Registered</span>'}
                    </div>
                </div>
                <div class="device-card-body">
                    <div class="device-info-row">
                        <span class="device-label">Model:</span>
                        <span class="device-value">${this.escapeHtml(device.model)}</span>
                    </div>
                    <div class="device-info-row">
                        <span class="device-label">Edition:</span>
                        <span class="device-value">${this.escapeHtml(device.edition)}</span>
                    </div>
                    <div class="device-info-row">
                        <span class="device-label">IP:</span>
                        <span class="device-value">${this.escapeHtml(device.ip)}</span>
                    </div>
                    <div class="device-info-row">
                        <span class="device-label">Last Seen:</span>
                        <span class="device-value">${lastSeen}</span>
                    </div>
                </div>
                <div class="device-card-footer">
                    ${isRegistered ? `
                        <button class="btn btn-secondary view-device-btn" data-device-id="${device.id}">
                            View Details
                        </button>
                        <button class="btn btn-danger btn-unregister" data-device-id="${device.id}">
                            Unregister
                        </button>
                    ` : `
                        <button class="btn btn-primary btn-register" data-device-id="${device.id}">
                            Register
                        </button>
                    `}
                </div>
            </div>
        `;
    }

    attachListeners() {
        // Search input
        const searchInput = document.getElementById('devices-search-input');
        if (searchInput) {
            let searchTimeout;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(searchTimeout);
                searchTimeout = setTimeout(() => {
                    this.searchTerm = e.target.value;
                    this.applyFilters();
                    this.updateGrid();
                }, 300);
            });
        }

        // Sort select
        const sortSelect = document.getElementById('devices-sort-select');
        if (sortSelect) {
            sortSelect.addEventListener('change', (e) => {
                this.sortBy = e.target.value;
                this.applyFilters();
                this.updateGrid();
            });
        }

        // Edition filter
        const editionFilter = document.getElementById('devices-edition-filter');
        if (editionFilter) {
            editionFilter.addEventListener('change', (e) => {
                this.filterEdition = e.target.value;
                this.applyFilters();
                this.updateGrid();
            });
        }

        // Status filter
        const statusFilter = document.getElementById('devices-status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', (e) => {
                this.filterStatus = e.target.value;
                this.applyFilters();
                this.updateGrid();
            });
        }

        // Attach button listeners
        this.attachButtonListeners();
    }

    updateGrid() {
        const grid = document.querySelector('.devices-grid');
        if (grid) {
            grid.innerHTML = this.renderDevicesGrid();
            // Reattach all listeners
            this.attachButtonListeners();
        }
    }

    attachButtonListeners() {
        // Register buttons
        const registerButtons = document.querySelectorAll('.btn-register');
        registerButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const deviceId = btn.dataset.deviceId;
                this.registerDevice(deviceId);
            });
        });

        // Unregister buttons
        const unregisterButtons = document.querySelectorAll('.btn-unregister');
        unregisterButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const deviceId = btn.dataset.deviceId;
                this.unregisterDevice(deviceId);
            });
        });

        // View Details buttons
        const viewButtons = document.querySelectorAll('.view-device-btn');
        viewButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const deviceId = btn.dataset.deviceId;
                router.navigate(`/devices/${deviceId}`);
            });
        });

        // Make entire unregistered card clickable to register
        const unregisteredCards = document.querySelectorAll('.device-card.unregistered');
        unregisteredCards.forEach(card => {
            card.style.cursor = 'pointer';
            card.addEventListener('click', (e) => {
                if (e.target.tagName !== 'BUTTON') {
                    const deviceId = card.dataset.deviceId;
                    this.registerDevice(deviceId);
                }
            });
        });

        // Make registered cards clickable to view details
        const registeredCards = document.querySelectorAll('.device-card.registered');
        registeredCards.forEach(card => {
            card.style.cursor = 'pointer';
            card.addEventListener('click', (e) => {
                if (e.target.tagName !== 'BUTTON') {
                    const deviceId = card.dataset.deviceId;
                    router.navigate(`/devices/${deviceId}`);
                }
            });
        });
    }

    formatTimestamp(timestamp) {
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;

        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

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

    async registerDevice(deviceId) {
        try {
            const response = await fetch('/api/devices/register', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_id: deviceId })
            });

            if (response.ok) {
                console.log('Device registered successfully');
                await this.loadDevices();
                this.updateGrid();
            } else {
                const error = await response.text();
                console.error('Failed to register device:', error);
                alert(`Failed to register device: ${error}`);
            }
        } catch (error) {
            console.error('Error registering device:', error);
            alert(`Error registering device: ${error.message}`);
        }
    }

    async unregisterDevice(deviceId) {
        const device = this.devices.find(d => d.id === deviceId);
        const deviceName = device ? (device.friendly_name || device.name) : deviceId;

        if (!confirm(`Unregister device "${deviceName}"?\n\nThis will remove all list assignments and sync settings.`)) {
            return;
        }

        try {
            const response = await fetch('/api/devices/unregister', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ device_id: deviceId })
            });

            if (response.ok) {
                console.log('Device unregistered successfully');
                await this.loadDevices();
                this.updateGrid();
            } else {
                const error = await response.text();
                console.error('Failed to unregister device:', error);
                alert(`Failed to unregister device: ${error}`);
            }
        } catch (error) {
            console.error('Error unregistering device:', error);
            alert(`Error unregistering device: ${error.message}`);
        }
    }

    setupWebSocketListeners() {
        if (!window.wsClient) {
            console.warn('WebSocket client not available');
            return;
        }

        // Listen for device events
        window.wsClient.on('device_discovered', () => this.handleDeviceUpdate());
        window.wsClient.on('device_connected', () => this.handleDeviceUpdate());
        window.wsClient.on('device_disconnected', () => this.handleDeviceUpdate());
        window.wsClient.on('device_registered', () => this.handleDeviceUpdate());
        window.wsClient.on('device_unregistered', () => this.handleDeviceUpdate());

        // Listen for sync events (to update device status)
        window.wsClient.on('sync_started', () => this.handleDeviceUpdate());
        window.wsClient.on('sync_completed', () => this.handleDeviceUpdate());
        window.wsClient.on('sync_failed', () => this.handleDeviceUpdate());
    }

    async handleDeviceUpdate() {
        console.log('Device update received, reloading devices...');
        await this.loadDevices();
        this.updateGrid();
    }
}
