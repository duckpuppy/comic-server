// Device Settings Page
class DeviceSettings {
    constructor(deviceId) {
        this.deviceId = deviceId;
        this.device = null;
        this.editingListId = null; // Currently editing list
        this.settingsPreview = null; // Preview data for current settings
    }

    async init(ctx) {
        await this.loadDeviceInfo();

        if (ctx && ctx.aborted) return;
        if (this.device) {
            this.render();
            this.attachListeners();
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

    render() {
        const app = document.getElementById('app');

        app.innerHTML = `
            <div class="device-settings-page">
                <!-- Breadcrumb -->
                <nav class="breadcrumb">
                    <a href="/" onclick="router.navigate('/'); return false;">Dashboard</a>
                    <span class="separator">›</span>
                    <a href="/devices/${this.deviceId}" onclick="router.navigate('/devices/${this.deviceId}'); return false;">
                        ${this.escapeHtml(this.device.friendly_name || this.device.name)}
                    </a>
                    <span class="separator">›</span>
                    <span class="current">Settings</span>
                </nav>

                <!-- Page Header -->
                <div class="page-header">
                    <h1>Device Settings</h1>
                    <p class="subtitle">${this.escapeHtml(this.device.friendly_name || this.device.name)}</p>
                </div>

                <!-- Device Info Panel -->
                <div class="panel device-info-panel">
                    <h2>Device Information</h2>
                    ${this.renderDeviceInfo()}
                </div>

                <!-- List Management Panel -->
                <div class="panel list-management-panel">
                    <div class="panel-header">
                        <h2>Smart Lists</h2>
                        <button class="btn btn-primary" onclick="deviceSettings.showAddListModal()">
                            + Assign List
                        </button>
                    </div>
                    ${this.renderListsManagement()}
                </div>

                <!-- Settings Editor (shown when editing a list) -->
                ${this.renderSettingsEditor()}
            </div>
        `;
    }

    renderDeviceInfo() {
        return `
            <div class="device-info-form">
                <div class="form-group">
                    <label for="friendly-name">Friendly Name</label>
                    <div class="form-row">
                        <input type="text"
                               id="friendly-name"
                               class="form-control"
                               value="${this.escapeHtml(this.device.friendly_name || '')}"
                               placeholder="${this.escapeHtml(this.device.name)}">
                        <button class="btn btn-secondary" onclick="deviceSettings.updateFriendlyName()">
                            Save
                        </button>
                    </div>
                    <div class="form-help">A custom name for this device (optional)</div>
                </div>

                <div class="device-info-readonly">
                    <div class="info-item">
                        <span class="info-label">Device ID:</span>
                        <span class="info-value">${this.escapeHtml(this.device.id)}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Model:</span>
                        <span class="info-value">${this.escapeHtml(this.device.model || 'Unknown')}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Edition:</span>
                        <span class="info-value">${this.escapeHtml(this.device.edition || 'Unknown')}</span>
                    </div>
                </div>
            </div>
        `;
    }

    renderListsManagement() {
        const hasLists = this.device.lists && this.device.lists.length > 0;

        if (!hasLists) {
            return `
                <div class="empty-state">
                    <p>No smart lists configured for this device.</p>
                    <p class="help-text">Click "Assign List" to configure smart lists for syncing.</p>
                </div>
            `;
        }

        return `
            <table class="lists-table">
                <thead>
                    <tr>
                        <th>List Name</th>
                        <th>Status</th>
                        <th>Books</th>
                        <th>Settings</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    ${this.device.lists.map(list => this.renderListRow(list)).join('')}
                </tbody>
            </table>
        `;
    }

    renderListRow(list) {
        const settingsSummary = this.getSettingsSummary(list);

        return `
            <tr class="list-row ${list.enabled ? '' : 'disabled'}">
                <td>
                    <a href="/lists/${list.list_id}"
                       onclick="router.navigate('/lists/${list.list_id}'); return false;"
                       class="list-link">
                        ${this.escapeHtml(list.list_name)}
                    </a>
                </td>
                <td>
                    <label class="toggle-switch">
                        <input type="checkbox"
                               ${list.enabled ? 'checked' : ''}
                               onchange="deviceSettings.toggleListEnabled('${list.list_id}', this.checked)">
                        <span class="toggle-slider"></span>
                    </label>
                    <span class="status-text">${list.enabled ? 'Enabled' : 'Disabled'}</span>
                </td>
                <td>${list.book_count.toLocaleString()}</td>
                <td class="settings-summary">${settingsSummary}</td>
                <td class="list-actions">
                    <button class="btn btn-small btn-secondary"
                            onclick="deviceSettings.editListSettings('${list.list_id}')">
                        Configure
                    </button>
                    <button class="btn btn-small btn-danger"
                            onclick="deviceSettings.removeList('${list.list_id}', '${this.escapeHtml(list.list_name)}')">
                        Remove
                    </button>
                </td>
            </tr>
        `;
    }

    getSettingsSummary(list) {
        // No settings stored yet - will be in list.settings
        if (!list.settings) {
            return '<span class="settings-none">Default</span>';
        }

        const parts = [];

        if (list.settings.only_unread) {
            parts.push('Unread only');
        }

        if (list.settings.only_checked) {
            parts.push('Checked only');
        }

        if (list.settings.limit && list.settings.limit_value > 0) {
            const type = list.settings.limit_value_type || 'books';
            parts.push(`Limit: ${list.settings.limit_value} ${type}`);
        }

        if (list.settings.sort && list.settings.list_sort_type) {
            const sortName = this.getSortTypeName(list.settings.list_sort_type);
            parts.push(`Sort: ${sortName}`);
        }

        return parts.length > 0 ? parts.join(', ') : '<span class="settings-none">Default</span>';
    }

    getSortTypeName(sortType) {
        const names = {
            'series': 'Series',
            'random': 'Random',
            'published': 'Published',
            'added': 'Added',
            'story_arc': 'Story Arc',
            'list_order': 'List Order',
            'alternate_series': 'Alternate Series'
        };
        return names[sortType] || sortType;
    }

    renderSettingsEditor() {
        if (!this.editingListId) {
            return '';
        }

        const list = this.device.lists.find(l => l.list_id === this.editingListId);
        if (!list) {
            return '';
        }

        // Get current settings or defaults
        const settings = list.settings || {
            only_unread: false,
            only_checked: false,
            keep_last_read: false,
            keep_last_read_count: 3,
            limit: false,
            limit_value: 0,
            limit_value_type: 'books',
            sort: false,
            list_sort_type: 'series'
        };

        return `
            <div class="settings-editor-overlay" onclick="deviceSettings.closeSettingsEditor(event)">
                <div class="settings-editor-panel" onclick="event.stopPropagation()">
                    <div class="settings-editor-header">
                        <h2>Configure List Settings</h2>
                        <button class="close-btn" onclick="deviceSettings.closeSettingsEditor()">×</button>
                    </div>

                    <div class="settings-editor-body">
                        <h3>${this.escapeHtml(list.list_name)}</h3>

                        <!-- Filtering Options -->
                        <div class="settings-section">
                            <h4>Filtering</h4>

                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox"
                                           id="setting-only-unread"
                                           ${settings.only_unread ? 'checked' : ''}
                                           onchange="deviceSettings.onSettingChange()">
                                    Only sync unread books
                                </label>
                            </div>

                            <div class="form-group form-group-nested ${settings.only_unread ? '' : 'disabled'}">
                                <label class="checkbox-label">
                                    <input type="checkbox"
                                           id="setting-keep-last-read"
                                           ${settings.keep_last_read ? 'checked' : ''}
                                           ${settings.only_unread ? '' : 'disabled'}
                                           onchange="deviceSettings.onSettingChange()">
                                    Keep last read book(s)
                                </label>
                                <div class="form-row ${settings.keep_last_read ? '' : 'hidden'}">
                                    <input type="number"
                                           id="setting-keep-last-read-count"
                                           class="form-control form-control-small"
                                           min="1"
                                           max="10"
                                           value="${settings.keep_last_read_count || 3}"
                                           ${settings.keep_last_read ? '' : 'disabled'}
                                           onchange="deviceSettings.onSettingChange()">
                                    <span class="form-label-inline">book(s)</span>
                                </div>
                            </div>

                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox"
                                           id="setting-only-checked"
                                           ${settings.only_checked ? 'checked' : ''}
                                           onchange="deviceSettings.onSettingChange()">
                                    Only sync checked books
                                </label>
                            </div>
                        </div>

                        <!-- Sorting Options -->
                        <div class="settings-section">
                            <h4>Sorting</h4>

                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox"
                                           id="setting-sort"
                                           ${settings.sort ? 'checked' : ''}
                                           onchange="deviceSettings.onSettingChange()">
                                    Apply custom sort order
                                </label>
                            </div>

                            <div class="form-group ${settings.sort ? '' : 'disabled'}">
                                <label for="setting-sort-type">Sort by:</label>
                                <select id="setting-sort-type"
                                        class="form-control"
                                        ${settings.sort ? '' : 'disabled'}
                                        onchange="deviceSettings.onSettingChange()">
                                    <option value="series" ${settings.list_sort_type === 'series' ? 'selected' : ''}>
                                        Series
                                    </option>
                                    <option value="random" ${settings.list_sort_type === 'random' ? 'selected' : ''}>
                                        Random
                                    </option>
                                    <option value="published" ${settings.list_sort_type === 'published' ? 'selected' : ''}>
                                        Published Date
                                    </option>
                                    <option value="added" ${settings.list_sort_type === 'added' ? 'selected' : ''}>
                                        Added Date
                                    </option>
                                    <option value="story_arc" ${settings.list_sort_type === 'story_arc' ? 'selected' : ''}>
                                        Story Arc
                                    </option>
                                    <option value="list_order" ${settings.list_sort_type === 'list_order' ? 'selected' : ''}>
                                        List Order (Manual)
                                    </option>
                                    <option value="alternate_series" ${settings.list_sort_type === 'alternate_series' ? 'selected' : ''}>
                                        Alternate Series
                                    </option>
                                </select>
                            </div>
                        </div>

                        <!-- Limiting Options -->
                        <div class="settings-section">
                            <h4>Limiting</h4>

                            <div class="form-group">
                                <label class="checkbox-label">
                                    <input type="checkbox"
                                           id="setting-limit"
                                           ${settings.limit ? 'checked' : ''}
                                           onchange="deviceSettings.onSettingChange()">
                                    Limit number of books
                                </label>
                            </div>

                            <div class="form-group form-row ${settings.limit ? '' : 'disabled'}">
                                <input type="number"
                                       id="setting-limit-value"
                                       class="form-control"
                                       min="1"
                                       value="${settings.limit_value || 50}"
                                       ${settings.limit ? '' : 'disabled'}
                                       onchange="deviceSettings.onSettingChange()">
                                <select id="setting-limit-type"
                                        class="form-control"
                                        ${settings.limit ? '' : 'disabled'}
                                        onchange="deviceSettings.onSettingChange()">
                                    <option value="books" ${settings.limit_value_type === 'books' ? 'selected' : ''}>
                                        Books
                                    </option>
                                    <option value="mb" ${settings.limit_value_type === 'mb' ? 'selected' : ''}>
                                        MB
                                    </option>
                                    <option value="gb" ${settings.limit_value_type === 'gb' ? 'selected' : ''}>
                                        GB
                                    </option>
                                </select>
                            </div>
                        </div>

                        <!-- Preview -->
                        <div class="settings-section settings-preview">
                            <h4>Preview</h4>
                            ${this.renderPreview()}
                        </div>
                    </div>

                    <div class="settings-editor-footer">
                        <button class="btn btn-secondary" onclick="deviceSettings.closeSettingsEditor()">
                            Cancel
                        </button>
                        <button class="btn btn-primary" onclick="deviceSettings.saveListSettings()">
                            Save Settings
                        </button>
                    </div>
                </div>
            </div>
        `;
    }

    renderPreview() {
        if (!this.settingsPreview) {
            return `
                <div class="preview-placeholder">
                    <button class="btn btn-secondary" onclick="deviceSettings.loadPreview()">
                        Load Preview
                    </button>
                </div>
            `;
        }

        if (this.settingsPreview.loading) {
            return '<div class="loading-spinner">Loading preview...</div>';
        }

        if (this.settingsPreview.error) {
            return `
                <div class="preview-error">
                    <p>Failed to load preview: ${this.escapeHtml(this.settingsPreview.error)}</p>
                    <button class="btn btn-secondary btn-small" onclick="deviceSettings.loadPreview()">
                        Retry
                    </button>
                </div>
            `;
        }

        const data = this.settingsPreview.data;
        return `
            <div class="preview-results">
                <div class="preview-stat">
                    <span class="preview-label">Books:</span>
                    <span class="preview-value">${data.book_count.toLocaleString()}</span>
                </div>
                ${data.total_size_bytes > 0 ? `
                    <div class="preview-stat">
                        <span class="preview-label">Size:</span>
                        <span class="preview-value">${this.formatBytes(data.total_size_bytes)}</span>
                    </div>
                ` : ''}
                ${data.sample && data.sample.length > 0 ? `
                    <div class="preview-sample">
                        <div class="preview-label">Sample titles:</div>
                        <ul>
                            ${data.sample.map(title => `<li>${this.escapeHtml(title)}</li>`).join('')}
                        </ul>
                    </div>
                ` : ''}
            </div>
        `;
    }

    attachListeners() {
        // Event delegation for dynamic content
        // Listeners are attached via onclick in render methods
    }

    // Device Info Methods
    async updateFriendlyName() {
        const input = document.getElementById('friendly-name');
        if (!input) return;

        const newName = input.value.trim();

        try {
            const response = await fetch(`/api/devices/${this.deviceId}`, {
                method: 'PATCH',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    friendly_name: newName
                })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            this.device.friendly_name = newName;
            alert('Friendly name updated successfully');
        } catch (error) {
            console.error('Failed to update friendly name:', error);
            alert(`Failed to update friendly name: ${error.message}`);
        }
    }

    // List Management Methods
    showAddListModal() {
        const excludeListIds = (this.device.lists || []).map(l => l.list_id);
        listPicker.open({
            title: 'Assign Smart List',
            excludeListIds,
            onSelect: (listId, listName) => this.addList(listId, listName)
        });
    }

    async addList(listId, listName) {
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
            this.render();
            this.attachListeners();

            alert(`Successfully added "${listName}" to this device.`);
        } catch (error) {
            console.error('Failed to add list:', error);
            alert(`Failed to add list: ${error.message}`);
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

            // Reload to show correct state
            await this.loadDeviceInfo();
            this.render();
            this.attachListeners();
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
            this.render();
            this.attachListeners();

            alert(`Successfully removed "${listName}" from this device.`);
        } catch (error) {
            console.error('Failed to remove list:', error);
            alert(`Failed to remove list: ${error.message}`);
        }
    }

    // Settings Editor Methods
    editListSettings(listId) {
        this.editingListId = listId;
        this.settingsPreview = null; // Reset preview
        this.render();
        this.attachListeners();
    }

    closeSettingsEditor(event) {
        if (event && event.target !== event.currentTarget) {
            return; // Clicked inside panel
        }

        this.editingListId = null;
        this.settingsPreview = null;
        this.render();
        this.attachListeners();
    }

    onSettingChange() {
        // Update UI state based on settings
        const onlyUnread = document.getElementById('setting-only-unread')?.checked;
        const keepLastRead = document.getElementById('setting-keep-last-read');
        const keepLastReadCount = document.getElementById('setting-keep-last-read-count')?.parentElement;

        if (keepLastRead) {
            keepLastRead.disabled = !onlyUnread;
            keepLastRead.parentElement.classList.toggle('disabled', !onlyUnread);
        }

        if (keepLastReadCount) {
            keepLastReadCount.classList.toggle('hidden', !keepLastRead?.checked);
        }

        const sort = document.getElementById('setting-sort')?.checked;
        const sortType = document.getElementById('setting-sort-type');
        if (sortType) {
            sortType.disabled = !sort;
            sortType.parentElement.classList.toggle('disabled', !sort);
        }

        const limit = document.getElementById('setting-limit')?.checked;
        const limitValue = document.getElementById('setting-limit-value');
        const limitType = document.getElementById('setting-limit-type');
        if (limitValue) {
            limitValue.disabled = !limit;
            limitValue.parentElement.classList.toggle('disabled', !limit);
        }
        if (limitType) {
            limitType.disabled = !limit;
        }

        // Clear preview when settings change
        this.settingsPreview = null;
        const previewSection = document.querySelector('.settings-preview');
        if (previewSection) {
            const previewContent = previewSection.querySelector('h4').nextElementSibling;
            if (previewContent) {
                previewContent.innerHTML = this.renderPreview();
            }
        }
    }

    getSettingsFromForm() {
        return {
            only_unread: document.getElementById('setting-only-unread')?.checked || false,
            only_checked: document.getElementById('setting-only-checked')?.checked || false,
            keep_last_read: document.getElementById('setting-keep-last-read')?.checked || false,
            keep_last_read_count: parseInt(document.getElementById('setting-keep-last-read-count')?.value) || 3,
            limit: document.getElementById('setting-limit')?.checked || false,
            limit_value: parseInt(document.getElementById('setting-limit-value')?.value) || 0,
            limit_value_type: document.getElementById('setting-limit-type')?.value || 'books',
            sort: document.getElementById('setting-sort')?.checked || false,
            list_sort_type: document.getElementById('setting-sort-type')?.value || 'series'
        };
    }

    async loadPreview() {
        if (!this.editingListId) return;

        this.settingsPreview = { loading: true };

        // Re-render preview section
        const previewSection = document.querySelector('.settings-preview');
        if (previewSection) {
            const previewContent = previewSection.querySelector('h4').nextElementSibling;
            if (previewContent) {
                previewContent.innerHTML = this.renderPreview();
            }
        }

        try {
            const settings = this.getSettingsFromForm();

            const response = await fetch(`/api/devices/${this.deviceId}/lists/${this.editingListId}/preview`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    settings: settings
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();
            this.settingsPreview = { data: data };
        } catch (error) {
            console.error('Failed to load preview:', error);
            this.settingsPreview = { error: error.message };
        }

        // Re-render preview section
        const previewSection2 = document.querySelector('.settings-preview');
        if (previewSection2) {
            const previewContent = previewSection2.querySelector('h4').nextElementSibling;
            if (previewContent) {
                previewContent.innerHTML = this.renderPreview();
            }
        }
    }

    async saveListSettings() {
        if (!this.editingListId) return;

        try {
            const settings = this.getSettingsFromForm();

            const response = await fetch(`/api/devices/lists/${this.deviceId}/${this.editingListId}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    settings: settings
                })
            });

            if (!response.ok) {
                const error = await response.text();
                throw new Error(error || `HTTP ${response.status}`);
            }

            // Update local state
            const list = this.device.lists.find(l => l.list_id === this.editingListId);
            if (list) {
                list.settings = settings;
            }

            // Close editor and re-render
            this.editingListId = null;
            this.settingsPreview = null;
            this.render();
            this.attachListeners();

            alert('Settings saved successfully');
        } catch (error) {
            console.error('Failed to save settings:', error);
            alert(`Failed to save settings: ${error.message}`);
        }
    }

    // Utility Methods
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

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    formatBytes(bytes) {
        if (bytes === 0) return '0 Bytes';

        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));

        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }
}
