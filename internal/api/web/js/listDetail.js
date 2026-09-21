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
        this.activeTab = 'matchers';
        this.loading = true;

        // CBL match-correction UI (comic-server-a2hz) - entries are only
        // fetched lazily, on first switch to the Matches tab (cbl_imported
        // lists only), not on every list load. null = not fetched yet.
        this.cblEntries = null;
        this.cblEntriesLoading = false;
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

        // Render once BEFORE these 5 parallel fetches so render()'s
        // existing !this.list check (below) shows a loading state
        // instead of leaving the page blank/stale for however long the
        // slowest of them takes (comic-server-4te).
        this.render();
        await Promise.all([
            this.loadListDetail(),
            this.loadDevices(),
            this.loadKomgaTarget(),
            this.loadPreview(),
            this.loadSchema()
        ]);
        this.loading = false;

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

        if (this.loading) {
            app.innerHTML = `
                <div class="lists-page-with-tree">
                    <aside id="lists-tree-sidebar"></aside>
                    <main class="lists-main-content">
                        <p class="empty-message">Loading…</p>
                    </main>
                </div>
            `;
            if (this.tree) {
                setTimeout(() => this.tree.render(), 0);
            }
            return;
        }

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
        const cblBadge = this.list.cbl_imported
            ? `<span class="cbl-badge" title="${this.escapeHtml(this.list.cbl_source || '')}">CBL</span>`
            : '';
        return `
            <!-- Header -->
            <div class="list-detail-header">
                <div class="list-detail-header-row">
                    <h1>${this.escapeHtml(this.list.name)} ${cblBadge}</h1>
                    ${canEdit ? `
                    <div class="list-header-actions">
                        ${this.list.cbl_imported ? '<button id="cbl-reimport-check-btn" class="btn btn-secondary">Check for Updates</button>' : ''}
                        <button id="edit-list-btn" class="btn btn-secondary">Edit</button>
                        <button id="delete-list-btn" class="btn btn-danger">Delete</button>
                    </div>` : ''}
                </div>
                ${this.list.cbl_imported ? `
                <p class="list-cbl-note">
                    Book membership for this list comes from a CBL import and is reimport-only
                    &mdash; name/description/favorite can still be edited here.
                </p>
                ${this.cblReimportStatusHtml || ''}
                ` : ''}
                <p class="list-count">
                    ${this.list.book_count.toLocaleString()} comics
                    ${this.list.unread_count > 0
                        ? ` &mdash; <span class="list-unread-count">${this.list.unread_count.toLocaleString()} unread</span>`
                        : ' &mdash; <span class="list-all-read">all read</span>'}
                </p>
            </div>

            <!-- Comics Preview - above the tabbed management panels below
                 so the list's actual contents are visible on page load. -->
            <div class="panel preview-panel">
                <h2>Comics Preview</h2>
                <p class="preview-info">Showing ${this.preview.length} of ${this.previewTotal.toLocaleString()}</p>
                <div class="comics-grid">
                    ${this.renderComicsPreview()}
                </div>
                ${this.renderLoadMore()}
            </div>

            <!-- Management Panels - one visible at a time via tabs, so the
                 page doesn't grow linearly as more list features are added
                 (comic-server-030). -->
            <div class="list-detail-tabs">
                ${this.renderTabButton('matchers', 'Details')}
                ${this.list.cbl_imported ? this.renderTabButton('matches', 'Matches') : ''}
                ${this.renderTabButton('devices', 'Devices')}
                ${this.renderTabButton('komga', 'Komga')}
            </div>

            <div class="list-detail-tab-panels">
                <!-- Matchers Panel -->
                <div class="panel matchers-panel${this.tabPanelActiveClass('matchers')}" data-tab-panel="matchers">
                    <h2>Matchers</h2>
                    <div class="matcher-mode">
                        ${this.list.matcher_mode_formatted}
                    </div>
                    <ul class="matchers-list">
                        ${this.renderMatchers()}
                    </ul>
                </div>

                <!-- CBL Match-Correction Panel (comic-server-a2hz) -->
                ${this.list.cbl_imported ? `
                <div class="panel cbl-matches-panel${this.tabPanelActiveClass('matches')}" data-tab-panel="matches">
                    <h2>Match Results</h2>
                    ${this.renderCBLMatchesPanel()}
                </div>
                ` : ''}

                <!-- Device Assignments Panel -->
                <div class="panel devices-panel${this.tabPanelActiveClass('devices')}" data-tab-panel="devices">
                    <h2>Device Assignments</h2>
                    <div class="device-assignments">
                        ${this.renderDeviceAssignments()}
                    </div>
                </div>

                <!-- Komga Sync Panel -->
                <div class="panel komga-panel${this.tabPanelActiveClass('komga')}" data-tab-panel="komga">
                    <h2>Komga Sync</h2>
                    <div class="device-assignments">
                        ${this.renderKomgaTarget()}
                    </div>
                </div>
            </div>
        `;
    }

    renderTabButton(tabId, label) {
        const active = this.activeTab === tabId ? ' active' : '';
        return `<button class="list-detail-tab${active}" data-tab="${tabId}">${label}</button>`;
    }

    tabPanelActiveClass(tabId) {
        return this.activeTab === tabId ? ' active' : '';
    }

    // switchTab toggles which management panel is visible via a class swap
    // on elements already in the DOM, not a full render() - so it doesn't
    // reset scroll position or interrupt any in-progress panel action.
    switchTab(tabId) {
        this.activeTab = tabId;
        document.querySelectorAll('.list-detail-tab').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.tab === tabId);
        });
        document.querySelectorAll('[data-tab-panel]').forEach(panel => {
            panel.classList.toggle('active', panel.dataset.tabPanel === tabId);
        });
    }

    // Reading list membership comes from a CBL import and is never
    // resolved via Matchers (see comic-server-zw0o) - showing the
    // matcher editor for one is a no-op on save and confusing UX
    // (comic-server-d1lk). Reading lists get a small editor scoped to
    // Name/Description/Favorite instead.
    isReadingList() {
        return this.list && this.list.type === 'ComicReadingList';
    }

    renderEditView() {
        const state = this.editState;
        const detailsPanel = `
            <div class="panel matchers-panel">
                <h2>Details</h2>
                <div class="list-edit-field">
                    <label for="edit-list-description">Description</label>
                    <textarea id="edit-list-description" class="list-description-input"
                              placeholder="Description">${this.escapeHtml(state.description)}</textarea>
                </div>
                <label class="list-favorite-toggle">
                    <input type="checkbox" id="edit-list-favorite" ${state.favorite ? 'checked' : ''}> Favorite
                </label>
            </div>
        `;

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

            ${this.isReadingList() ? detailsPanel : `
            ${detailsPanel}
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
            `}
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

        // CustomValues is shaped differently from every other matcher:
        // MatchValue holds the custom field's KEY name (e.g. "Data Manager
        // processed"), not a value to compare - the actual comparison
        // value lives in MatchValue2. Rendering it through the normal
        // single-value layout would show the key where a comparison value
        // belongs and offer no way to see or edit the key itself - the
        // "07 DataManager" list bug (comic-server-arn).
        const isCustomValue = fieldType === 'customvalue';

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
                    ${isCustomValue ? `
                    <input type="text" class="matcher-customfield-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue || '')}" placeholder="custom field name">
                    ` : ''}
                    <select class="matcher-op-select" data-index="${index}">
                        ${opOptions}
                    </select>
                    ${showValue && !isCustomValue ? `
                    <input type="text" class="matcher-value-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue || '')}" placeholder="value">
                    ` : ''}
                    ${showValue2 ? `
                    <span class="matcher-range-and">and</span>
                    <input type="text" class="matcher-value2-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue2 || '')}" placeholder="value 2">
                    ` : ''}
                    ${isCustomValue ? `
                    <input type="text" class="matcher-value2-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue2 || '')}" placeholder="value">
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
                        ${target.sync_read_status ? '<span class="komga-target-type">Read status sync</span>' : ''}
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
            <div class="comic-card${comic.unread ? '' : ' comic-read'}" title="${comic.unread ? '' : 'Read'}">
                ${comic.unread ? '' : '<div class="comic-read-badge" title="Read">✓</div>'}
                <div class="comic-cover">
                    <div class="comic-placeholder">📖</div>
                    <img class="comic-cover-img" alt=""
                         src="/api/library/books/${encodeURIComponent(comic.id)}/cover"
                         loading="lazy"
                         onload="this.classList.add('loaded'); this.previousElementSibling.style.display='none';"
                         onerror="this.style.display='none';">
                </div>
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
                description: rawList ? (rawList.Description || '') : '',
                favorite: rawList ? !!rawList.Favorite : false,
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
        const descInput = document.getElementById('edit-list-description');
        const favInput = document.getElementById('edit-list-favorite');
        const modeSelect = document.getElementById('edit-matcher-mode');
        if (nameInput) this.editState.name = nameInput.value.trim();
        if (descInput) this.editState.description = descInput.value;
        if (favInput) this.editState.favorite = favInput.checked;
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
            dialogs.toast('List name is required', 'error');
            return;
        }

        const body = {
            name: state.name,
            type: this.list.type,
            description: state.description,
            favorite: state.favorite,
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
                throw new Error(friendlyErrorText(resp, text, 'Save failed'));
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
            dialogs.toast('Failed to save list: ' + e.message, 'error');
        }
    }

    async deleteList() {
        const ok = await dialogs.confirm({
            title: 'Delete List',
            message: `Delete "${this.list.name}"? This cannot be undone.`,
            confirmLabel: 'Delete',
            danger: true,
        });
        if (!ok) return;

        try {
            const resp = await fetch(`/api/library/lists/${this.listId}`, { method: 'DELETE' });
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(friendlyErrorText(resp, text, 'Delete failed'));
            }
            router.navigate('/lists');
        } catch (e) {
            console.error('Failed to delete list:', e);
            dialogs.toast('Failed to delete list: ' + e.message, 'error');
        }
    }

    // --- CBL reimport (comic-server-zw0o) ---
    //
    // Two-step flow, matching this codebase's other destructive-adjacent
    // actions (e.g. CBZ convert's confirm()): "Check for Updates" is a
    // read-only preview (calls the bulk reimport-check endpoint and
    // picks out this list); only after that shows a real change does a
    // "Reimport" button appear, and only a confirm() on THAT applies it.
    // Reimport is a full replace of this list's book membership (see
    // storage.CBLReimportPolicy) - there is nothing to merge, since
    // comic-server has no feature that lets a user hand-edit a reading
    // list's membership in the first place.
    async checkCBLReimport() {
        const btn = document.getElementById('cbl-reimport-check-btn');
        if (btn) { btn.disabled = true; btn.textContent = 'Checking...'; }
        try {
            const resp = await fetch('/api/library/cbl-repo/reimport-check');
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(friendlyErrorText(resp, text, 'Check failed'));
            }
            const candidates = await resp.json();
            const mine = (candidates || []).find(c => c.list_id === this.listId);
            this.cblReimportStatusHtml = this.renderCBLReimportStatus(mine);
        } catch (e) {
            console.error('CBL reimport check failed:', e);
            dialogs.toast('Check for updates failed: ' + e.message, 'error');
            this.cblReimportStatusHtml = '';
        }
        this.render();
        this.attachListeners();
    }

    renderCBLReimportStatus(candidate) {
        if (!candidate) {
            return '<p class="list-cbl-status">Could not find this list in the upstream repo check.</p>';
        }
        switch (candidate.status) {
            case 'unchanged':
                return '<p class="list-cbl-status">Up to date with upstream.</p>';
            case 'modified':
                return `
                    <p class="list-cbl-status list-cbl-status-actionable">Upstream file changed.
                        <button id="cbl-reimport-apply-btn" class="btn btn-primary btn-sm">Reimport</button>
                    </p>`;
            case 'renamed':
                return `
                    <p class="list-cbl-status list-cbl-status-actionable">Upstream file was renamed to
                        <code>${this.escapeHtml(candidate.new_path)}</code>.
                        <button id="cbl-reimport-apply-btn" class="btn btn-primary btn-sm">Reimport</button>
                    </p>`;
            case 'orphaned':
                return '<p class="list-cbl-status list-cbl-status-warn">Upstream file was deleted or changed too much for git to trace as a rename. Left as-is - reimport refused rather than guessing a successor.</p>';
            case 'base_unknown':
                return '<p class="list-cbl-status list-cbl-status-warn">Upstream history was rewritten since this list\'s last import. Delete and freshly re-import instead.</p>';
            default:
                return `<p class="list-cbl-status list-cbl-status-warn">${this.escapeHtml(candidate.error || 'Check failed.')}</p>`;
        }
    }

    async applyCBLReimport() {
        const ok = await dialogs.confirm({
            title: 'Reimport CBL List',
            message: `Replace this list's books with the upstream CBL's current contents? Any books currently in the list that are no longer in the upstream file will be removed.`,
            confirmLabel: 'Reimport',
        });
        if (!ok) return;

        try {
            const resp = await fetch(`/api/library/lists/${this.listId}/cbl-reimport`, { method: 'POST' });
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(friendlyErrorText(resp, text, 'Reimport failed'));
            }
            const result = await resp.json();
            dialogs.toast(`Reimported: ${result.matched_cv_id + result.matched_series_number} matched, ${result.unmatched} unmatched`, 'success');
            this.cblReimportStatusHtml = '';
            await this.loadListDetail();
            await this.loadPreview();
            this.render();
            this.attachListeners();
        } catch (e) {
            console.error('CBL reimport failed:', e);
            dialogs.toast('Reimport failed: ' + e.message, 'error');
        }
    }

    // --- CBL match-correction UI (comic-server-a2hz, spec §2d) ---
    //
    // Shows every entry from this list's CBL import - which path matched
    // it (CV-ID / string-fallback / unmatched / manually corrected), and
    // for an ambiguous string-fallback match, the other candidates the
    // matcher's tie-break was choosing among. Lets the user re-point a
    // wrong match to one of those candidates, or unmatch it entirely.
    // Scoped to this one import's results (POST .../correct only ever
    // touches one cbl_import_entries row) - not a general relink tool.

    renderCBLMatchesPanel() {
        if (this.cblEntriesLoading) {
            return '<p class="empty-message">Loading match results…</p>';
        }
        if (this.cblEntries === null) {
            return '<p class="empty-message">Switch to this tab to load match results.</p>';
        }
        if (this.cblEntries.length === 0) {
            return '<p class="empty-message">No entries recorded for this import.</p>';
        }
        return `
            <p class="cbl-matches-info">
                CV-ID matches are direct ID lookups, not heuristics - a manual override
                is available but rarely needed. String-fallback matches can tie between
                similar-looking books; where that happened, the other candidate(s) are
                offered below.
            </p>
            <table class="cbl-matches-table">
                <thead>
                    <tr>
                        <th>Entry</th>
                        <th>Match</th>
                        <th>Matched Book</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody>
                    ${this.cblEntries.map(e => this.renderCBLMatchRow(e)).join('')}
                </tbody>
            </table>
        `;
    }

    renderCBLMatchRow(entry) {
        const entryLabel = `${this.escapeHtml(entry.series)} #${this.escapeHtml(entry.number)}` +
            (entry.year ? ` (${entry.year})` : '');
        const pathBadge = this.renderCBLMatchPathBadge(entry.match_path);
        const bookLabel = entry.book
            ? `${this.escapeHtml(entry.book.series)} #${this.escapeHtml(entry.book.number)}` +
              (entry.book.year ? ` (${entry.book.year})` : '')
            : '<span class="cbl-match-none">Not matched</span>';

        const otherCandidates = (entry.candidates || []).filter(c => !entry.book || c.id !== entry.book.id);
        const candidateButtons = otherCandidates.map(c => `
            <button class="btn btn-small btn-secondary cbl-correct-btn"
                    data-entry-id="${this.escapeHtml(entry.id)}" data-book-id="${this.escapeHtml(c.id)}"
                    title="Re-point this entry to this book instead">
                Use: ${this.escapeHtml(c.series)} #${this.escapeHtml(c.number)}${c.year ? ` (${c.year})` : ''}
            </button>
        `).join('');
        const unmatchButton = entry.book ? `
            <button class="btn btn-small btn-danger cbl-correct-btn"
                    data-entry-id="${this.escapeHtml(entry.id)}" data-book-id=""
                    title="Remove this entry's match">
                Unmatch
            </button>
        ` : '';

        return `
            <tr data-entry-id="${this.escapeHtml(entry.id)}">
                <td>${entryLabel}</td>
                <td>${pathBadge}</td>
                <td>${bookLabel}</td>
                <td class="cbl-match-actions">${candidateButtons}${unmatchButton}</td>
            </tr>
        `;
    }

    renderCBLMatchPathBadge(path) {
        const labels = {
            cv_id: 'CV-ID',
            series_number: 'String match',
            manual: 'Manually corrected',
            none: 'Unmatched',
        };
        return `<span class="cbl-match-badge cbl-match-badge-${path}">${labels[path] || path}</span>`;
    }

    async loadCBLEntriesAndSwitchTab() {
        this.activeTab = 'matches';
        this.cblEntriesLoading = true;
        this.render();
        this.attachListeners();

        try {
            const resp = await fetch(`/api/library/lists/${this.listId}/cbl-import-entries`);
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(friendlyErrorText(resp, text, 'Failed to load match results'));
            }
            const data = await resp.json();
            this.cblEntries = data.entries || [];
        } catch (e) {
            console.error('Failed to load CBL import entries:', e);
            dialogs.toast('Failed to load match results: ' + e.message, 'error');
            this.cblEntries = [];
        }
        this.cblEntriesLoading = false;
        this.render();
        this.attachListeners();
    }

    async correctCBLEntry(entryId, bookId) {
        try {
            const resp = await fetch(`/api/library/lists/${this.listId}/cbl-import-entries/${encodeURIComponent(entryId)}/correct`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_id: bookId }),
            });
            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(friendlyErrorText(resp, text, 'Correction failed'));
            }
            const corrected = await resp.json();
            const idx = this.cblEntries.findIndex(e => e.id === entryId);
            if (idx !== -1) this.cblEntries[idx] = corrected;

            // The list's own book_count/membership changed too.
            this.previewOffset = 0;
            await Promise.all([this.loadListDetail(), this.loadPreview()]);
            dialogs.toast(bookId ? 'Match corrected' : 'Entry unmatched', 'success');
            this.render();
            this.attachListeners();
        } catch (e) {
            console.error('Failed to correct CBL import entry:', e);
            dialogs.toast('Correction failed: ' + e.message, 'error');
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
        document.querySelectorAll('.list-detail-tab').forEach(btn => {
            btn.addEventListener('click', () => {
                // Matches tab content is fetched lazily on first visit
                // (comic-server-a2hz) - switchTab alone only toggles CSS
                // classes on panels already in the DOM.
                if (btn.dataset.tab === 'matches' && this.cblEntries === null) {
                    this.loadCBLEntriesAndSwitchTab();
                } else {
                    this.switchTab(btn.dataset.tab);
                }
            });
        });

        document.querySelectorAll('.cbl-correct-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.correctCBLEntry(btn.dataset.entryId, btn.dataset.bookId);
            });
        });

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

        const cblCheckBtn = document.getElementById('cbl-reimport-check-btn');
        if (cblCheckBtn) {
            cblCheckBtn.addEventListener('click', () => this.checkCBLReimport());
        }

        // Only present after checkCBLReimport() re-renders with an
        // actionable status (modified/renamed) - re-wired each render
        // since renderReadView() replaces the DOM node.
        const cblApplyBtn = document.getElementById('cbl-reimport-apply-btn');
        if (cblApplyBtn) {
            cblApplyBtn.addEventListener('click', () => this.applyCBLReimport());
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

        // CustomValues' "field name" input - writes to MatchValue, same
        // underlying field as matcher-value-input above, but the two never
        // coexist for the same matcher (see renderMatcherEditor's
        // isCustomValue branch), so there's no risk of one clobbering the
        // other.
        document.querySelectorAll('.matcher-customfield-input').forEach(el => {
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
            dialogs.toast('Failed to assign list to device', 'error');
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
            dialogs.toast('Failed to update list settings', 'error');
        }
    }

    async unassignDevice(deviceId) {
        const ok = await dialogs.confirm({
            title: 'Remove from Device',
            message: 'Remove this list from the device?',
            confirmLabel: 'Remove',
            danger: true,
        });
        if (!ok) return;

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
            dialogs.toast('Failed to remove list from device', 'error');
        }
    }

    showKomgaTargetModal(existingTarget) {
        const modal = document.getElementById('komga-target-modal');
        const title = document.getElementById('komga-target-modal-title');
        const typeSelect = document.getElementById('komga-target-type');
        const nameInput = document.getElementById('komga-target-name');
        const enabledCheck = document.getElementById('komga-target-enabled');
        const syncReadStatusCheck = document.getElementById('komga-target-sync-read-status');
        const saveBtn = document.getElementById('komga-target-save-btn');
        const cancelBtn = document.getElementById('komga-target-cancel-btn');
        const closeBtn = document.getElementById('komga-target-modal-close');

        title.textContent = existingTarget ? 'Edit Komga Sync Target' : 'Add Komga Sync Target';
        typeSelect.value = existingTarget ? existingTarget.type : 'collection';
        nameInput.value = existingTarget ? existingTarget.komga_name : this.list.name;
        enabledCheck.checked = existingTarget ? existingTarget.enabled : true;
        syncReadStatusCheck.checked = existingTarget ? !!existingTarget.sync_read_status : false;

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
                dialogs.toast('Komga name is required', 'error');
                return;
            }
            saveBtn.disabled = true;
            await this.saveKomgaTarget({
                type: typeSelect.value,
                komga_name: komgaName,
                enabled: enabledCheck.checked,
                sync_read_status: syncReadStatusCheck.checked
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
                throw new Error(friendlyErrorText(response, text, 'Failed to save Komga target'));
            }

            await this.loadKomgaTarget();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to save Komga target:', error);
            dialogs.toast('Failed to save Komga target: ' + error.message, 'error');
        }
    }

    async removeKomgaTarget() {
        const ok = await dialogs.confirm({
            title: 'Remove Komga Sync',
            message: 'Remove this list from Komga sync?',
            confirmLabel: 'Remove',
            danger: true,
        });
        if (!ok) return;

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
            dialogs.toast('Failed to remove Komga target', 'error');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
