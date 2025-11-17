// Device management
class DeviceManager {
    constructor() {
        this.devices = new Map();
        this.container = document.getElementById('devices-list');
        this.template = document.getElementById('device-card-template');
    }

    init() {
        // Load initial devices
        this.loadDevices();

        // Set up WebSocket event handlers
        wsClient.on('device_discovered', (data) => this.handleDeviceDiscovered(data));
        wsClient.on('device_connected', (data) => this.handleDeviceConnected(data));
        wsClient.on('device_disconnected', (data) => this.handleDeviceDisconnected(data));
        wsClient.on('device_registered', (data) => this.handleDeviceRegistered(data));
        wsClient.on('device_unregistered', (data) => this.handleDeviceUnregistered(data));

        // Refresh devices periodically
        setInterval(() => this.loadDevices(), 10000);
    }

    async loadDevices() {
        try {
            const response = await fetch('/api/devices');
            const data = await response.json();

            data.devices.forEach(device => {
                this.updateDevice(device);
            });

            this.render();
        } catch (error) {
            console.error('Failed to load devices:', error);
        }
    }

    updateDevice(device) {
        this.devices.set(device.id, device);
    }

    removeDevice(deviceId) {
        this.devices.delete(deviceId);
        this.render();
    }

    handleDeviceDiscovered(data) {
        console.log('Device discovered:', data);
        this.loadDevices(); // Reload to get fresh data
    }

    handleDeviceConnected(data) {
        console.log('Device connected:', data);
        const device = this.devices.get(data.device_id);
        if (device) {
            device.is_syncing = false;
            this.render();
        }
    }

    handleDeviceDisconnected(data) {
        console.log('Device disconnected:', data);
        const device = this.devices.get(data.device_id);
        if (device) {
            device.is_syncing = false;
            this.render();
        }
    }

    handleDeviceRegistered(data) {
        console.log('Device registered:', data);
        this.loadDevices();
    }

    handleDeviceUnregistered(data) {
        console.log('Device unregistered:', data);
        this.loadDevices();
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
                this.loadDevices();
            } else {
                console.error('Failed to register device:', await response.text());
            }
        } catch (error) {
            console.error('Error registering device:', error);
        }
    }

    async unregisterDevice(deviceId) {
        if (!confirm('Are you sure you want to unregister this device?')) {
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
                this.loadDevices();
            } else {
                console.error('Failed to unregister device:', await response.text());
            }
        } catch (error) {
            console.error('Error unregistering device:', error);
        }
    }

    render() {
        // Only render if container exists (dashboard page)
        if (!this.container) return;

        if (this.devices.size === 0) {
            this.container.innerHTML = '<p class="empty-message">No devices discovered</p>';
            return;
        }

        // Clear container
        this.container.innerHTML = '';

        // Sort devices: syncing first, then by name
        const sortedDevices = Array.from(this.devices.values()).sort((a, b) => {
            if (a.is_syncing !== b.is_syncing) {
                return b.is_syncing ? 1 : -1;
            }
            return a.name.localeCompare(b.name);
        });

        // Render device cards
        sortedDevices.forEach(device => {
            const card = this.createDeviceCard(device);
            this.container.appendChild(card);
        });
    }

    createDeviceCard(device) {
        const card = this.template.content.cloneNode(true);
        const cardEl = card.querySelector('.device-card');

        cardEl.dataset.deviceId = device.id;

        // Set device name
        card.querySelector('.device-name').textContent = device.name;

        // Set status
        const statusEl = card.querySelector('.device-status');
        if (device.is_syncing) {
            statusEl.textContent = 'Syncing';
            statusEl.className = 'device-status syncing';
        } else {
            const lastSeen = new Date(device.last_seen);
            const now = new Date();
            const minutesAgo = (now - lastSeen) / 1000 / 60;

            if (minutesAgo < 2) {
                statusEl.textContent = 'Connected';
                statusEl.className = 'device-status connected';
            } else if (minutesAgo < 30) {
                statusEl.textContent = 'Idle';
                statusEl.className = 'device-status idle';
            } else {
                statusEl.textContent = 'Offline';
                statusEl.className = 'device-status offline';
            }
        }

        // Set device info
        card.querySelector('.device-model').textContent = `${device.manufacturer} ${device.model}`;
        card.querySelector('.device-ip').textContent = `IP: ${device.ip}`;

        const lastSeenDate = new Date(device.last_seen);
        card.querySelector('.device-last-seen').textContent =
            `Last seen: ${this.formatTimestamp(lastSeenDate)}`;

        // Set up action buttons
        const registerBtn = card.querySelector('.btn-register');
        const unregisterBtn = card.querySelector('.btn-unregister');
        const configureBtn = card.querySelector('.btn-configure');

        registerBtn.onclick = () => this.registerDevice(device.id);
        unregisterBtn.onclick = () => this.unregisterDevice(device.id);
        configureBtn.onclick = () => {
            if (typeof ListManager !== 'undefined') {
                ListManager.openConfigModal(device.id, device.name);
            }
        };

        // Enable/disable buttons based on registration status
        if (device.is_registered) {
            registerBtn.style.display = 'none';
            unregisterBtn.style.display = 'inline-block';
            configureBtn.disabled = false;
        } else {
            registerBtn.style.display = 'inline-block';
            unregisterBtn.style.display = 'none';
            configureBtn.disabled = true;
        }

        // Add click handler to navigate to device detail page
        cardEl.addEventListener('click', (e) => {
            // Don't navigate if clicking buttons
            if (e.target.tagName === 'BUTTON') {
                return;
            }
            router.navigate(`/devices/${device.id}`);
        });

        return card;
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

// Global device manager instance
const deviceManager = new DeviceManager();
