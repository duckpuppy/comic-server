// List Detail Page
class ListDetail {
    constructor(listId, tree) {
        this.listId = listId;
        this.list = null;
        this.devices = [];
        this.preview = [];
        this.previewOffset = 0;
        this.previewLimit = 20;
        this.previewTotal = 0;
        this.tree = tree; // Use provided tree instance
    }

    async init() {
        // Set up tree callback and selection
        if (this.tree) {
            this.tree.onListSelected = (listId) => this.onListSelected(listId);
            // Select current list in tree
            this.tree.selectedListId = this.listId;
        }

        await Promise.all([
            this.loadListDetail(),
            this.loadDevices(),
            this.loadPreview()
        ]);

        this.render();
        this.attachListeners();
    }

    onListSelected(listId) {
        // Navigate to the selected list
        router.navigate(`/lists/${listId}`);
    }

    async loadListDetail() {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}`);
            if (!response.ok) {
                throw new Error('List not found');
            }
            this.list = await response.json();
        } catch (error) {
            console.error('Failed to load list:', error);
            this.list = null;
        }
    }

    async loadDevices() {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}/devices`);
            const data = await response.json();
            this.devices = data.devices || [];
        } catch (error) {
            console.error('Failed to load devices:', error);
            this.devices = [];
        }
    }

    async loadPreview() {
        try {
            const url = `/api/library/lists/${this.listId}/preview?limit=${this.previewLimit}&offset=${this.previewOffset}`;
            const response = await fetch(url);
            const data = await response.json();

            if (this.previewOffset === 0) {
                this.preview = data.comics || [];
            } else {
                this.preview = [...this.preview, ...(data.comics || [])];
            }

            this.previewTotal = data.total || 0;
        } catch (error) {
            console.error('Failed to load preview:', error);
            this.preview = [];
        }
    }

    render() {
        const app = document.getElementById('app');

        if (!this.list) {
            app.innerHTML = `
                <div class="lists-page-with-tree">
                    <aside id="lists-tree-sidebar"></aside>
                    <main class="lists-main-content">
                        <div class="error-page">
                            <h1>List Not Found</h1>
                            <p>The requested list could not be found.</p>
                            <button onclick="router.navigate('/lists')" class="btn btn-primary">
                                Back to Lists
                            </button>
                        </div>
                    </main>
                </div>
            `;
            // Render tree after DOM is ready
            if (this.tree) {
                setTimeout(() => this.tree.render(), 0);
            }
            return;
        }

        app.innerHTML = `
            <div class="lists-page-with-tree">
                <aside id="lists-tree-sidebar"></aside>

                <main class="lists-main-content">
                    <div class="list-detail-page">
                        <!-- Breadcrumb -->
                        <nav class="breadcrumb">
                            <a href="/lists" onclick="router.navigate('/lists'); return false;">Smart Lists</a>
                            <span class="separator">›</span>
                            <span class="current">${this.escapeHtml(this.list.name)}</span>
                        </nav>

                        <!-- Header -->
                        <div class="list-detail-header">
                            <h1>${this.escapeHtml(this.list.name)}</h1>
                            <p class="list-count">
                                ${this.list.book_count.toLocaleString()} comics match this list
                            </p>
                        </div>

                        <!-- Main Content -->
                        <div class="list-detail-content">
                            <!-- Matchers Panel -->
                            <div class="panel matchers-panel">
                                <h2>Matchers</h2>
                                <div class="matcher-mode">
                                    ${this.list.matcher_mode_formatted}
                                </div>
                                <ul class="matchers-list">
                                    ${this.renderMatchers()}
                                </ul>
                            </div>

                            <!-- Device Assignments Panel -->
                            <div class="panel devices-panel">
                                <h2>Device Assignments</h2>
                                <div class="device-assignments">
                                    ${this.renderDeviceAssignments()}
                                </div>
                            </div>
                        </div>

                        <!-- Comics Preview -->
                        <div class="panel preview-panel">
                            <h2>Comics Preview</h2>
                            <p class="preview-info">Showing ${this.preview.length} of ${this.previewTotal.toLocaleString()}</p>
                            <div class="comics-grid">
                                ${this.renderComicsPreview()}
                            </div>
                            ${this.renderLoadMore()}
                        </div>
                    </div>
                </main>
            </div>
        `;

        // Render tree after DOM is ready
        if (this.tree) {
            setTimeout(() => this.tree.render(), 0);
        }
    }

    renderMatchers() {
        if (!this.list.matchers || this.list.matchers.length === 0) {
            return '<li class="empty-message">No matchers defined</li>';
        }

        return this.list.matchers.map(matcher => {
            // Format the matcher display
            let displayText = '';

            if (matcher.field === 'Group') {
                // Group matcher: show operator and value
                displayText = `<strong>${matcher.operator}</strong>: ${matcher.value}`;
            } else {
                // Regular matcher: Field operator "value"
                displayText = `<strong>${this.escapeHtml(matcher.field)}</strong> `;
                displayText += `${this.escapeHtml(matcher.operator)} `;

                // Format value with quotes for readability
                if (matcher.value) {
                    displayText += `<code>"${this.escapeHtml(matcher.value)}"</code>`;
                }

                // Add second value if present (for ranges)
                if (matcher.value2) {
                    displayText += ` and <code>"${this.escapeHtml(matcher.value2)}"</code>`;
                }
            }

            return `
                <li class="matcher-item">
                    <span class="matcher-bullet">•</span>
                    <span class="matcher-text">${displayText}</span>
                </li>
            `;
        }).join('');
    }

    renderDeviceAssignments() {
        if (this.devices.length === 0) {
            return `
                <p class="empty-message">This list is not assigned to any devices.</p>
                <button class="btn btn-primary" id="assign-device-btn">
                    + Assign to Device
                </button>
            `;
        }

        return this.devices.map(device => `
            <div class="device-assignment-card" data-device-id="${device.device_id}">
                <div class="device-assignment-info">
                    <h4>${this.escapeHtml(device.friendly_name)}</h4>
                    <span class="device-status ${device.enabled ? 'enabled' : 'disabled'}">
                        ${device.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                </div>
                <div class="device-assignment-actions">
                    <button class="btn btn-small btn-toggle" data-device-id="${device.device_id}" data-enabled="${device.enabled}">
                        ${device.enabled ? 'Disable' : 'Enable'}
                    </button>
                    <button class="btn btn-small btn-danger btn-unassign" data-device-id="${device.device_id}">
                        Remove
                    </button>
                </div>
            </div>
        `).join('');
    }

    renderComicsPreview() {
        if (this.preview.length === 0) {
            return '<p class="empty-message">No comics to preview</p>';
        }

        return this.preview.map(comic => `
            <div class="comic-card">
                <div class="comic-placeholder">📖</div>
                <div class="comic-info">
                    <div class="comic-series">${this.escapeHtml(comic.series)}</div>
                    <div class="comic-number">#${this.escapeHtml(comic.number)}</div>
                    ${comic.title ? `<div class="comic-title">${this.escapeHtml(comic.title)}</div>` : ''}
                </div>
            </div>
        `).join('');
    }

    renderLoadMore() {
        if (this.preview.length >= this.previewTotal) {
            return '';
        }

        return `
            <div class="load-more-container">
                <button id="load-more-btn" class="btn btn-secondary">
                    Load More
                </button>
            </div>
        `;
    }

    attachListeners() {
        const loadMoreBtn = document.getElementById('load-more-btn');
        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', async () => {
                loadMoreBtn.disabled = true;
                loadMoreBtn.textContent = 'Loading...';

                this.previewOffset += this.previewLimit;
                await this.loadPreview();
                this.render();
            });
        }

        // Assign device button
        const assignBtn = document.getElementById('assign-device-btn');
        if (assignBtn) {
            assignBtn.addEventListener('click', () => this.showAssignDeviceDialog());
        }

        // Toggle enable/disable buttons
        document.querySelectorAll('.btn-toggle').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const deviceId = e.target.dataset.deviceId;
                const enabled = e.target.dataset.enabled === 'true';
                this.toggleDeviceList(deviceId, !enabled);
            });
        });

        // Unassign buttons
        document.querySelectorAll('.btn-unassign').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const deviceId = e.target.dataset.deviceId;
                this.unassignDevice(deviceId);
            });
        });
    }

    async showAssignDeviceDialog() {
        // Fetch all registered devices
        try {
            const response = await fetch('/api/devices');
            const data = await response.json();

            // Filter out devices that already have this list
            const availableDevices = data.devices.filter(d =>
                !this.devices.some(assigned => assigned.device_id === d.id)
            );

            if (availableDevices.length === 0) {
                alert('No available devices to assign. All registered devices already have this list.');
                return;
            }

            // Simple prompt for now - could be improved with a proper modal
            const deviceNames = availableDevices.map((d, i) => `${i + 1}. ${d.friendly_name || d.name} (${d.id})`).join('\n');
            const selection = prompt(`Select a device to assign:\n\n${deviceNames}\n\nEnter device number:`);

            if (selection) {
                const index = parseInt(selection) - 1;
                if (index >= 0 && index < availableDevices.length) {
                    await this.assignDevice(availableDevices[index].id);
                }
            }
        } catch (error) {
            console.error('Failed to fetch devices:', error);
            alert('Failed to load available devices');
        }
    }

    async assignDevice(deviceId) {
        try {
            const response = await fetch(`/api/devices/lists/${deviceId}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    list_id: this.listId,
                    list_name: this.list.name,
                    enabled: true
                })
            });

            if (!response.ok) {
                throw new Error('Failed to assign list');
            }

            // Reload devices
            await this.loadDevices();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to assign device:', error);
            alert('Failed to assign list to device');
        }
    }

    async toggleDeviceList(deviceId, enabled) {
        try {
            const response = await fetch(`/api/devices/lists/${deviceId}/${this.listId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled })
            });

            if (!response.ok) {
                throw new Error('Failed to update list settings');
            }

            // Reload devices
            await this.loadDevices();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to toggle list:', error);
            alert('Failed to update list settings');
        }
    }

    async unassignDevice(deviceId) {
        if (!confirm('Remove this list from the device?')) {
            return;
        }

        try {
            const response = await fetch(`/api/devices/lists/${deviceId}/${this.listId}`, {
                method: 'DELETE'
            });

            if (!response.ok) {
                throw new Error('Failed to remove list');
            }

            // Reload devices
            await this.loadDevices();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to unassign device:', error);
            alert('Failed to remove list from device');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
