// List Detail Page
class ListDetail {
    constructor(listId, tree) {
        this.listId = listId;
        this.list = null;
        this.devices = [];
        this.komga = null; // { komga_enabled, target } - see loadKomgaTarget()
        this.preview = [];
        this.previewOffset = 0;
        this.previewLimit = 20;
        this.previewTotal = 0;
        this.tree = tree; // Use provided tree instance
        this.editMode = false;
        this.editState = null;
        this.schema = null;
    }

    async init(ctx) {
        if (this.tree) {
            this.tree.onListSelected = (listId) => router.navigate(`/lists/${listId}`);
            this.tree.onFolderSelected = (folderId) => this.navigateToFolder(folderId);
            this.tree.selectedListId = this.listId;
            this.tree.selectedFolderId = null;

            // Compute ancestors for breadcrumb and auto-expand in tree
            this.ancestors = this.tree.findAncestors(this.listId) || [];
            for (const folder of this.ancestors) {
                this.tree.expandedFolders.add(folder.id);
            }
        } else {
            this.ancestors = [];
        }

        await Promise.all([
            this.loadListDetail(),
            this.loadDevices(),
            this.loadKomgaTarget(),
            this.loadPreview(),
            this.loadSchema()
        ]);

        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    // Navigate to the file browser focused on a specific folder
    navigateToFolder(folderId) {
        if (typeof listsBrowser !== 'undefined' && listsBrowser) {
            const path = listsBrowser.findPathToFolder(folderId);
            if (path) listsBrowser.pathStack = path;
        }
        router.navigate('/lists');
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

    async loadKomgaTarget() {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}/komga`);
            this.komga = await response.json();
        } catch (error) {
            console.error('Failed to load Komga target:', error);
            this.komga = { komga_enabled: false, target: null };
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

    async loadSchema() {
        if (this.schema) return;
        try {
            const resp = await fetch('/api/library/lists/schema');
            this.schema = await resp.json();
        } catch (e) {
            console.error('Failed to load list schema:', e);
            this.schema = { matcherTypes: [], operators: {}, matcherModes: [] };
        }
    }

    renderBreadcrumb() {
        let html = `<a href="/lists" onclick="router.navigate('/lists'); return false;">Lists</a>`;
        (this.ancestors || []).forEach(folder => {
            html += `<span class="separator">›</span>`;
            html += `<a href="/lists" class="breadcrumb-folder-link" data-folder-id="${folder.id}">${this.escapeHtml(folder.name)}</a>`;
        });
        html += `<span class="separator">›</span>`;
        html += `<span class="current">${this.escapeHtml(this.list ? this.list.name : '')}</span>`;
        return `<nav class="breadcrumb">${html}</nav>`;
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
                        ${this.renderBreadcrumb()}

                        ${this.editMode ? this.renderEditView() : this.renderReadView()}
                    </div>
                </main>
            </div>
        `;

        // Render tree after DOM is ready
        if (this.tree) {
            setTimeout(() => this.tree.render(), 0);
        }
    }

    renderReadView() {
        const canEdit = true; // Only disable when backend signals read-only
        return `
            <!-- Header -->
            <div class="list-detail-header">
                <div class="list-detail-header-row">
                    <h1>${this.escapeHtml(this.list.name)}</h1>
                    ${canEdit ? `
                    <div class="list-header-actions">
                        <button id="edit-list-btn" class="btn btn-secondary">Edit</button>
                        <button id="delete-list-btn" class="btn btn-danger">Delete</button>
                    </div>` : ''}
                </div>
                <p class="list-count">
                    ${this.list.book_count.toLocaleString()} comics
                    ${this.list.unread_count > 0
                        ? ` &mdash; <span class="list-unread-count">${this.list.unread_count.toLocaleString()} unread</span>`
                        : ' &mdash; <span class="list-all-read">all read</span>'}
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

                <!-- Komga Sync Panel -->
                <div class="panel komga-panel">
                    <h2>Komga Sync</h2>
                    <div class="device-assignments">
                        ${this.renderKomgaTarget()}
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
        `;
    }

    renderEditView() {
        const state = this.editState;
        return `
            <!-- Edit Header -->
            <div class="list-detail-header">
                <div class="list-detail-header-row">
                    <input type="text" id="edit-list-name" class="list-name-input"
                           value="${this.escapeHtml(state.name)}" placeholder="List name">
                    <div class="list-header-actions">
                        <button id="save-list-btn" class="btn btn-primary">Save</button>
                        <button id="cancel-edit-btn" class="btn btn-secondary">Cancel</button>
                    </div>
                </div>
            </div>

            <!-- Matchers Editor -->
            <div class="panel matchers-panel">
                <div class="matchers-editor-header">
                    <h2>Matchers</h2>
                    <div class="matcher-mode-selector">
                        <label>Match:
                            <select id="edit-matcher-mode">
                                <option value="And" ${state.matcherMode === 'And' ? 'selected' : ''}>ALL conditions (AND)</option>
                                <option value="Or" ${state.matcherMode === 'Or' ? 'selected' : ''}>ANY condition (OR)</option>
                            </select>
                        </label>
                    </div>
                </div>

                <ul class="matchers-list matchers-editor-list" id="matchers-editor-list">
                    ${state.matchers.map((m, i) => this.renderMatcherEditor(m, i)).join('')}
                </ul>

                <button id="add-matcher-btn" class="btn btn-secondary btn-add-matcher">+ Add Matcher</button>
            </div>
        `;
    }

    renderMatcherEditor(matcher, index) {
        const schema = this.schema || { matcherTypes: [], operators: {} };
        const typeInfo = schema.matcherTypes.find(t => t.id === matcher.Type) || { fieldType: 'string', label: matcher.Type };
        const fieldType = typeInfo.fieldType;
        const ops = (schema.operators && schema.operators[fieldType]) || [];

        // Group types by category for the select
        const typeOptions = this.renderTypeOptions(matcher.Type);
        const opOptions = ops.map(op =>
            `<option value="${op.value}" ${matcher.MatchOperator === op.value ? 'selected' : ''}>${this.escapeHtml(op.label)}</option>`
        ).join('');

        const selectedOp = ops.find(o => o.value === matcher.MatchOperator) || ops[0] || {};
        const showValue = selectedOp.hasValue !== false;
        const showValue2 = !!selectedOp.hasValue2;

        const isYesNo = fieldType === 'yesno' || fieldType === 'manga';

        return `
            <li class="matcher-editor-row" data-index="${index}">
                <div class="matcher-editor-controls">
                    <label class="matcher-not-toggle" title="Negate this matcher">
                        <input type="checkbox" class="matcher-not-check" data-index="${index}"
                               ${matcher.Not ? 'checked' : ''}> NOT
                    </label>
                    <select class="matcher-type-select" data-index="${index}">
                        ${typeOptions}
                    </select>
                    ${!isYesNo ? `
                    <select class="matcher-op-select" data-index="${index}">
                        ${opOptions}
                    </select>
                    ` : ''}
                    ${showValue && !isYesNo ? `
                    <input type="text" class="matcher-value-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue || '')}" placeholder="value">
                    ` : ''}
                    ${showValue2 ? `
                    <span class="matcher-range-and">and</span>
                    <input type="text" class="matcher-value2-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue2 || '')}" placeholder="value 2">
                    ` : ''}
                </div>
                <button class="btn btn-small btn-danger matcher-remove-btn" data-index="${index}" title="Remove">✕</button>
            </li>
        `;
    }

    renderTypeOptions(selectedType) {
        const schema = this.schema || { matcherTypes: [] };
        const groups = {};
        for (const t of schema.matcherTypes) {
            if (!groups[t.category]) groups[t.category] = [];
            groups[t.category].push(t);
        }
        return Object.entries(groups).map(([cat, types]) => `
            <optgroup label="${this.escapeHtml(cat)}">
                ${types.map(t =>
                    `<option value="${t.id}" ${t.id === selectedType ? 'selected' : ''}>${this.escapeHtml(t.label)}</option>`
                ).join('')}
            </optgroup>
        `).join('');
    }

    renderMatchers(matchers, depth = 0) {
        const list = matchers || this.list.matchers;
        if (!list || list.length === 0) {
            return '<li class="empty-message">No matchers defined</li>';
        }

        return list.map(matcher => {
            if (matcher.field === 'Group' && matcher.children && matcher.children.length > 0) {
                return `
                    <li class="matcher-item matcher-group" style="--depth:${depth}">
                        <details${depth === 0 ? ' open' : ''}>
                            <summary>
                                <span class="matcher-bullet"></span>
                                <span class="matcher-text">
                                    <strong>${this.escapeHtml(matcher.operator)}</strong>:
                                    ${this.escapeHtml(matcher.value)}
                                </span>
                            </summary>
                            <ul class="matchers-list nested-matchers">
                                ${this.renderMatchers(matcher.children, depth + 1)}
                            </ul>
                        </details>
                    </li>
                `;
            }

            // Regular matcher
            let displayText = `<strong>${this.escapeHtml(matcher.field)}</strong> `;
            displayText += `<em>${this.escapeHtml(matcher.operator)}</em>`;
            if (matcher.value) {
                displayText += ` <code>"${this.escapeHtml(matcher.value)}"</code>`;
            }
            if (matcher.value2) {
                displayText += ` and <code>"${this.escapeHtml(matcher.value2)}"</code>`;
            }

            return `
                <li class="matcher-item" style="--depth:${depth}">
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

    renderKomgaTarget() {
        const komga = this.komga || {};
        const target = komga.target;

        const disabledNote = !komga.komga_enabled
            ? `<p class="komga-disabled-note">Komga integration is disabled in config.yaml - a target can still be saved here, but it won't sync until Komga is enabled.</p>`
            : '';

        if (!target) {
            return `
                ${disabledNote}
                <p class="empty-message">This list is not synced to Komga.</p>
                <button class="btn btn-primary" id="add-komga-target-btn">
                    + Add Komga Target
                </button>
            `;
        }

        return `
            ${disabledNote}
            <div class="device-assignment-card" data-list-id="${this.escapeHtml(target.list_id)}">
                <div class="device-assignment-info">
                    <h4>${this.escapeHtml(target.komga_name)}
                        <span class="komga-target-type">${target.type === 'readlist' ? 'Read List' : 'Collection'}</span>
                    </h4>
                    <span class="device-status ${target.enabled ? 'enabled' : 'disabled'}">
                        ${target.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                </div>
                <div class="device-assignment-actions">
                    <button class="btn btn-small" id="edit-komga-target-btn">Edit</button>
                    <button class="btn btn-small btn-toggle" id="toggle-komga-target-btn" data-enabled="${target.enabled}">
                        ${target.enabled ? 'Disable' : 'Enable'}
                    </button>
                    <button class="btn btn-small btn-danger" id="remove-komga-target-btn">Remove</button>
                </div>
            </div>
        `;
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

    // --- Edit mode helpers ---

    enterEditMode() {
        this.fetchRawList().then(rawList => {
            this.editState = {
                name: rawList ? rawList.Name : this.list.name,
                matcherMode: rawList ? (rawList.MatcherMode || 'And') : (this.list.matcher_mode || 'And'),
                matchers: rawList ? (rawList.Matchers || []) : []
            };
            this.editMode = true;
            this.render();
            this.attachListeners();
        });
    }

    async fetchRawList() {
        try {
            const resp = await fetch(`/api/library/lists/${this.listId}/raw`);
            if (!resp.ok) throw new Error('Failed to fetch raw list');
            return await resp.json();
        } catch (e) {
            console.error('Failed to fetch raw list:', e);
            return null;
        }
    }

    cancelEdit() {
        this.editMode = false;
        this.editState = null;
        this.render();
        this.attachListeners();
    }

    collectEditState() {
        const nameInput = document.getElementById('edit-list-name');
        const modeSelect = document.getElementById('edit-matcher-mode');
        if (nameInput) this.editState.name = nameInput.value.trim();
        if (modeSelect) this.editState.matcherMode = modeSelect.value;
    }

    addMatcher() {
        const schema = this.schema || { matcherTypes: [] };
        const firstType = schema.matcherTypes[0] || { id: 'ComicBookSeriesMatcher' };
        this.editState.matchers.push({
            Type: firstType.id,
            Not: false,
            MatchOperator: '0',
            MatchValue: '',
            MatchValue2: ''
        });
        this.collectEditState();
        this.renderMatcherList();
    }

    removeMatcher(index) {
        this.collectEditState();
        this.editState.matchers.splice(index, 1);
        this.renderMatcherList();
    }

    updateMatcherField(index, field, value) {
        this.collectEditState();
        const matcher = this.editState.matchers[index];
        if (!matcher) return;
        matcher[field] = value;

        // When type changes, reset operator and values
        if (field === 'Type') {
            matcher.MatchOperator = '0';
            matcher.MatchValue = '';
            matcher.MatchValue2 = '';
        }

        // Re-render just the matcher row to update visible inputs
        this.renderMatcherList();
    }

    renderMatcherList() {
        const list = document.getElementById('matchers-editor-list');
        if (!list) return;
        list.innerHTML = this.editState.matchers.map((m, i) => this.renderMatcherEditor(m, i)).join('');
        this.attachMatcherListeners();
    }

    async saveList() {
        this.collectEditState();
        const state = this.editState;

        if (!state.name) {
            alert('List name is required');
            return;
        }

        const body = {
            name: state.name,
            type: this.list.type,
            matcher_mode: state.matcherMode,
            matchers: state.matchers
        };

        try {
            const resp = await fetch(`/api/library/lists/${this.listId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });

            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(text || 'Save failed');
            }

            // Reload and exit edit mode
            this.editMode = false;
            this.editState = null;
            this.previewOffset = 0;
            await Promise.all([this.loadListDetail(), this.loadPreview()]);
            this.render();
            this.attachListeners();
        } catch (e) {
            console.error('Failed to save list:', e);
            alert('Failed to save list: ' + e.message);
        }
    }

    async deleteList() {
        if (!confirm(`Delete "${this.list.name}"? This cannot be undone.`)) return;

        try {
            const resp = await fetch(`/api/library/lists/${this.listId}`, { method: 'DELETE' });
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(text || 'Delete failed');
            }
            router.navigate('/lists');
        } catch (e) {
            console.error('Failed to delete list:', e);
            alert('Failed to delete list: ' + e.message);
        }
    }

    // --- Event wiring ---

    attachListeners() {
        // Breadcrumb folder links — navigate to that folder in the file browser
        document.querySelectorAll('.breadcrumb-folder-link').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                this.navigateToFolder(link.dataset.folderId);
            });
        });

        if (this.editMode) {
            this.attachEditListeners();
        } else {
            this.attachReadListeners();
        }
    }

    attachReadListeners() {
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

        const editBtn = document.getElementById('edit-list-btn');
        if (editBtn) {
            editBtn.addEventListener('click', () => this.enterEditMode());
        }

        const deleteBtn = document.getElementById('delete-list-btn');
        if (deleteBtn) {
            deleteBtn.addEventListener('click', () => this.deleteList());
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

        // Komga sync target controls
        const addKomgaBtn = document.getElementById('add-komga-target-btn');
        if (addKomgaBtn) {
            addKomgaBtn.addEventListener('click', () => this.showKomgaTargetModal());
        }

        const editKomgaBtn = document.getElementById('edit-komga-target-btn');
        if (editKomgaBtn) {
            editKomgaBtn.addEventListener('click', () => this.showKomgaTargetModal(this.komga.target));
        }

        const toggleKomgaBtn = document.getElementById('toggle-komga-target-btn');
        if (toggleKomgaBtn) {
            toggleKomgaBtn.addEventListener('click', () => {
                const target = this.komga.target;
                this.saveKomgaTarget({
                    type: target.type,
                    komga_name: target.komga_name,
                    enabled: toggleKomgaBtn.dataset.enabled !== 'true'
                }, { isUpdate: true });
            });
        }

        const removeKomgaBtn = document.getElementById('remove-komga-target-btn');
        if (removeKomgaBtn) {
            removeKomgaBtn.addEventListener('click', () => this.removeKomgaTarget());
        }
    }

    attachEditListeners() {
        const saveBtn = document.getElementById('save-list-btn');
        if (saveBtn) saveBtn.addEventListener('click', () => this.saveList());

        const cancelBtn = document.getElementById('cancel-edit-btn');
        if (cancelBtn) cancelBtn.addEventListener('click', () => this.cancelEdit());

        const addBtn = document.getElementById('add-matcher-btn');
        if (addBtn) addBtn.addEventListener('click', () => this.addMatcher());

        this.attachMatcherListeners();
    }

    attachMatcherListeners() {
        document.querySelectorAll('.matcher-not-check').forEach(el => {
            el.addEventListener('change', () => {
                const i = parseInt(el.dataset.index);
                this.updateMatcherField(i, 'Not', el.checked);
            });
        });

        document.querySelectorAll('.matcher-type-select').forEach(el => {
            el.addEventListener('change', () => {
                const i = parseInt(el.dataset.index);
                this.updateMatcherField(i, 'Type', el.value);
            });
        });

        document.querySelectorAll('.matcher-op-select').forEach(el => {
            el.addEventListener('change', () => {
                const i = parseInt(el.dataset.index);
                this.updateMatcherField(i, 'MatchOperator', el.value);
            });
        });

        document.querySelectorAll('.matcher-value-input').forEach(el => {
            el.addEventListener('input', () => {
                const i = parseInt(el.dataset.index);
                if (this.editState.matchers[i]) this.editState.matchers[i].MatchValue = el.value;
            });
        });

        document.querySelectorAll('.matcher-value2-input').forEach(el => {
            el.addEventListener('input', () => {
                const i = parseInt(el.dataset.index);
                if (this.editState.matchers[i]) this.editState.matchers[i].MatchValue2 = el.value;
            });
        });

        document.querySelectorAll('.matcher-remove-btn').forEach(el => {
            el.addEventListener('click', () => {
                const i = parseInt(el.dataset.index);
                this.removeMatcher(i);
            });
        });
    }

    async showAssignDeviceDialog() {
        const modal = document.getElementById('assign-device-modal');
        const listEl = document.getElementById('assign-device-list');
        const confirmBtn = document.getElementById('assign-device-confirm-btn');
        const cancelBtn = document.getElementById('assign-device-cancel-btn');
        const closeBtn = document.getElementById('assign-device-modal-close');

        const close = () => modal.classList.remove('active');
        const onKeydown = (e) => {
            if (e.key === 'Escape') close();
        };
        const onBackdropClick = (e) => {
            if (e.target === modal) close();
        };

        closeBtn.onclick = close;
        cancelBtn.onclick = close;
        document.addEventListener('keydown', onKeydown, { once: true });
        modal.addEventListener('click', onBackdropClick, { once: true });

        listEl.innerHTML = '<p class="empty-message">Loading...</p>';
        confirmBtn.disabled = true;
        modal.classList.add('active');

        try {
            const response = await fetch('/api/devices');
            const data = await response.json();

            // Filter out devices that already have this list
            const availableDevices = data.devices.filter(d =>
                !this.devices.some(assigned => assigned.device_id === d.id)
            );

            if (availableDevices.length === 0) {
                listEl.innerHTML = '<p class="empty-message">All registered devices already have this list assigned.</p>';
                return;
            }

            listEl.innerHTML = availableDevices.map(d => `
                <label class="assign-device-item">
                    <input type="checkbox" value="${d.id}">
                    <div>
                        <div class="device-name">${d.friendly_name || d.name}</div>
                        <div class="device-meta">${d.id}</div>
                    </div>
                </label>
            `).join('');
            confirmBtn.disabled = false;

            confirmBtn.onclick = async () => {
                const selectedIds = Array.from(listEl.querySelectorAll('input[type="checkbox"]:checked'))
                    .map(cb => cb.value);
                if (selectedIds.length === 0) {
                    close();
                    return;
                }
                confirmBtn.disabled = true;
                await Promise.all(selectedIds.map(id => this.assignDevice(id, { refresh: false })));
                close();
                await this.loadDevices();
                this.render();
                this.attachListeners();
            };
        } catch (error) {
            console.error('Failed to fetch devices:', error);
            listEl.innerHTML = '<p class="empty-message">Failed to load available devices.</p>';
        }
    }

    async assignDevice(deviceId, { refresh = true } = {}) {
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

            if (refresh) {
                await this.loadDevices();
                this.render();
                this.attachListeners();
            }
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

    showKomgaTargetModal(existingTarget) {
        const modal = document.getElementById('komga-target-modal');
        const title = document.getElementById('komga-target-modal-title');
        const typeSelect = document.getElementById('komga-target-type');
        const nameInput = document.getElementById('komga-target-name');
        const enabledCheck = document.getElementById('komga-target-enabled');
        const saveBtn = document.getElementById('komga-target-save-btn');
        const cancelBtn = document.getElementById('komga-target-cancel-btn');
        const closeBtn = document.getElementById('komga-target-modal-close');

        title.textContent = existingTarget ? 'Edit Komga Sync Target' : 'Add Komga Sync Target';
        typeSelect.value = existingTarget ? existingTarget.type : 'collection';
        nameInput.value = existingTarget ? existingTarget.komga_name : this.list.name;
        enabledCheck.checked = existingTarget ? existingTarget.enabled : true;

        const close = () => modal.classList.remove('active');
        const onKeydown = (e) => {
            if (e.key === 'Escape') close();
        };
        const onBackdropClick = (e) => {
            if (e.target === modal) close();
        };

        closeBtn.onclick = close;
        cancelBtn.onclick = close;
        document.addEventListener('keydown', onKeydown, { once: true });
        modal.addEventListener('click', onBackdropClick, { once: true });

        saveBtn.onclick = async () => {
            const komgaName = nameInput.value.trim();
            if (!komgaName) {
                alert('Komga name is required');
                return;
            }
            saveBtn.disabled = true;
            await this.saveKomgaTarget({
                type: typeSelect.value,
                komga_name: komgaName,
                enabled: enabledCheck.checked
            }, { isUpdate: !!existingTarget });
            saveBtn.disabled = false;
            close();
        };

        modal.classList.add('active');
    }

    async saveKomgaTarget(body, { isUpdate = false } = {}) {
        try {
            const response = await fetch(`/api/library/lists/${this.listId}/komga`, {
                method: isUpdate ? 'PUT' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body)
            });

            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || 'Failed to save Komga target');
            }

            await this.loadKomgaTarget();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to save Komga target:', error);
            alert('Failed to save Komga target: ' + error.message);
        }
    }

    async removeKomgaTarget() {
        if (!confirm('Remove this list from Komga sync?')) {
            return;
        }

        try {
            const response = await fetch(`/api/library/lists/${this.listId}/komga`, { method: 'DELETE' });
            if (!response.ok) {
                throw new Error('Failed to remove Komga target');
            }

            await this.loadKomgaTarget();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to remove Komga target:', error);
            alert('Failed to remove Komga target');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
