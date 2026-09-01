// Smart List Management

const ListManager = {
    currentDeviceId: null,
    allLists: [],
    assignedLists: [],

    init() {
        this.setupModalHandlers();
        this.loadAllLists();
    },

    setupModalHandlers() {
        const modal = document.getElementById('device-config-modal');
        const closeBtn = modal.querySelector('.modal-close');

        // Close modal on X button
        closeBtn.addEventListener('click', () => this.closeModal());

        // Close modal on outside click
        window.addEventListener('click', (e) => {
            if (e.target === modal) {
                this.closeModal();
            }
        });

        // Close modal on Escape key
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && modal.classList.contains('active')) {
                this.closeModal();
            }
        });
    },

    async loadAllLists() {
        try {
            const response = await fetch('/api/library/lists');
            if (!response.ok) throw new Error('Failed to fetch lists');

            const data = await response.json();
            this.allLists = data.lists || [];
        } catch (error) {
            console.error('Error loading lists:', error);
            this.allLists = [];
        }
    },

    async openConfigModal(deviceId, deviceName) {
        this.currentDeviceId = deviceId;

        // Update modal title
        document.getElementById('modal-device-name').textContent = `${deviceName} - Smart Lists`;

        // Show modal
        const modal = document.getElementById('device-config-modal');
        modal.classList.add('active');

        // Load device configuration
        await this.loadDeviceConfig();

        // Render lists
        this.renderAssignedLists();
        this.renderAvailableLists();
    },

    closeModal() {
        const modal = document.getElementById('device-config-modal');
        modal.classList.remove('active');
        this.currentDeviceId = null;
    },

    async loadDeviceConfig() {
        if (!this.currentDeviceId) return;

        try {
            const response = await fetch(`/api/devices/config/${this.currentDeviceId}`);
            if (!response.ok) {
                // Device not configured yet, use empty lists
                this.assignedLists = [];
                return;
            }

            const config = await response.json();
            this.assignedLists = config.lists || [];
        } catch (error) {
            console.error('Error loading device config:', error);
            this.assignedLists = [];
        }
    },

    renderAssignedLists() {
        const container = document.getElementById('assigned-lists');

        if (this.assignedLists.length === 0) {
            container.innerHTML = '<p class="empty-message">No lists assigned</p>';
            return;
        }

        container.innerHTML = this.assignedLists.map(list => `
            <div class="list-item" data-list-id="${list.list_id}">
                <div class="list-info">
                    <div class="list-name">${this.escapeHtml(list.list_name)}</div>
                    <div class="list-meta">
                        Status: ${list.enabled ? 'Enabled' : 'Disabled'}
                    </div>
                </div>
                <div class="list-actions">
                    <button class="btn btn-small btn-remove" onclick="ListManager.removeList('${list.list_id}')">
                        Remove
                    </button>
                </div>
            </div>
        `).join('');
    },

    renderAvailableLists() {
        const container = document.getElementById('available-lists');

        // Filter out already assigned lists
        const assignedIds = new Set(this.assignedLists.map(l => l.list_id));
        const availableLists = this.allLists.filter(l => !assignedIds.has(l.id));

        if (availableLists.length === 0) {
            container.innerHTML = '<p class="empty-message">All lists are already assigned</p>';
            return;
        }

        container.innerHTML = availableLists.map(list => `
            <div class="list-item" data-list-id="${list.id}">
                <div class="list-info">
                    <div class="list-name">${this.escapeHtml(list.name)}</div>
                    <div class="list-meta">
                        ${list.matcher_count} matchers
                        ${list.matcher_mode ? `(${list.matcher_mode})` : ''}
                    </div>
                </div>
                <div class="list-actions">
                    <button class="btn btn-small btn-add" onclick="ListManager.addList('${list.id}', '${this.escapeHtml(list.name)}')">
                        Add
                    </button>
                </div>
            </div>
        `).join('');
    },

    async addList(listId, listName) {
        if (!this.currentDeviceId) return;

        try {
            const response = await fetch(`/api/devices/lists/${this.currentDeviceId}`, {
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
                throw new Error(error);
            }

            // Reload and re-render
            await this.loadDeviceConfig();
            this.renderAssignedLists();
            this.renderAvailableLists();

            console.log(`Added list ${listName} to device`);
        } catch (error) {
            console.error('Error adding list:', error);
            dialogs.toast(`Failed to add list: ${error.message}`, 'error');
        }
    },

    async removeList(listId) {
        if (!this.currentDeviceId) return;

        try {
            const response = await fetch(`/api/devices/lists/${this.currentDeviceId}/${listId}`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error);
            }

            // Reload and re-render
            await this.loadDeviceConfig();
            this.renderAssignedLists();
            this.renderAvailableLists();

            console.log(`Removed list from device`);
        } catch (error) {
            console.error('Error removing list:', error);
            dialogs.toast(`Failed to remove list: ${error.message}`, 'error');
        }
    },

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
};

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => ListManager.init());
} else {
    ListManager.init();
}
