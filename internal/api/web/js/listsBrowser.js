// Smart Lists File Browser — Explorer-style icon/detail navigation
class ListsBrowser {
    constructor(tree) {
        this.tree = tree;
        this.pathStack = []; // [{id, name}]
        this.viewMode = localStorage.getItem('listsViewMode') || 'icon';
    }

    async init() {
        if (this.tree) {
            this.tree.onListSelected = (listId) => router.navigate(`/lists/${listId}`);
            this.tree.onFolderSelected = (folderId) => {
                const path = this.findPathToFolder(folderId);
                if (path) {
                    this.pathStack = path;
                    this.renderContent();
                }
            };
        }
        this.render();
    }

    findPathToFolder(targetId, items, currentPath) {
        if (!items) items = this.tree ? (this.tree.tree || []) : [];
        if (!currentPath) currentPath = [];
        for (const node of items) {
            if (!node.is_folder) continue;
            const newPath = [...currentPath, { id: node.id, name: node.name }];
            if (node.id === targetId) return newPath;
            const found = this.findPathToFolder(targetId, node.children || [], newPath);
            if (found) return found;
        }
        return null;
    }

    getCurrentItems() {
        let items = this.tree ? (this.tree.tree || []) : [];
        for (const seg of this.pathStack) {
            const folder = items.find(n => n.id === seg.id && n.is_folder);
            if (!folder) return [];
            items = folder.children || [];
        }
        return items;
    }

    countAllLists(items) {
        if (!items) items = this.tree ? (this.tree.tree || []) : [];
        let count = 0;
        for (const node of items) {
            count += node.is_folder ? this.countAllLists(node.children || []) : 1;
        }
        return count;
    }

    navigateToFolder(folderId) {
        const folder = this.getCurrentItems().find(n => n.id === folderId && n.is_folder);
        if (!folder) return;
        this.pathStack = [...this.pathStack, { id: folder.id, name: folder.name }];
        this.renderContent();
        if (this.tree) this.tree.setCurrentPath(this.pathStack);
    }

    navigateToBreadcrumb(index) {
        this.pathStack = this.pathStack.slice(0, index);
        this.renderContent();
        if (this.tree) this.tree.setCurrentPath(this.pathStack);
    }

    setViewMode(mode) {
        this.viewMode = mode;
        localStorage.setItem('listsViewMode', mode);
        this.renderContent();
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="lists-page-with-tree">
                <aside id="lists-tree-sidebar"></aside>
                <main class="lists-main-content filebrowser-main">
                    <div id="filebrowser-content"></div>
                </main>
            </div>
        `;

        navigation.updateBadge('lists', this.countAllLists());

        if (this.tree) {
            setTimeout(() => {
                this.tree.selectedFolderId = null;
                this.tree.selectedListId = null;
                this.tree.render();
                this.tree.setCurrentPath(this.pathStack);
            }, 0);
        }

        this.renderContent();
    }

    renderContent() {
        const container = document.getElementById('filebrowser-content');
        if (!container) return;

        const allItems = this.getCurrentItems();
        const folders = allItems.filter(n => n.is_folder);
        const lists = allItems.filter(n => !n.is_folder);
        const total = folders.length + lists.length;

        const toolbar = `
            <div class="fb-toolbar">
                ${this.renderBreadcrumb()}
                <div class="fb-toolbar-actions">
                    <button id="new-list-btn" class="btn btn-primary btn-small">+ New List</button>
                    <button id="new-folder-btn" class="btn btn-secondary btn-small">+ New Folder</button>
                </div>
                <div class="fb-view-toggle">
                    <button class="fb-view-btn${this.viewMode === 'icon' ? ' active' : ''}" data-view="icon" title="Icon view">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                            <rect x="1" y="1" width="6" height="6" rx="1"/>
                            <rect x="9" y="1" width="6" height="6" rx="1"/>
                            <rect x="1" y="9" width="6" height="6" rx="1"/>
                            <rect x="9" y="9" width="6" height="6" rx="1"/>
                        </svg>
                    </button>
                    <button class="fb-view-btn${this.viewMode === 'detail' ? ' active' : ''}" data-view="detail" title="Details view">
                        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                            <rect x="1" y="2" width="14" height="2" rx="1"/>
                            <rect x="1" y="7" width="14" height="2" rx="1"/>
                            <rect x="1" y="12" width="14" height="2" rx="1"/>
                        </svg>
                    </button>
                </div>
            </div>
        `;

        const body = this.viewMode === 'icon'
            ? this.renderIconGrid(folders, lists)
            : this.renderDetailTable(folders, lists);

        container.innerHTML = `
            ${toolbar}
            ${body}
            <div class="fb-statusbar">${total.toLocaleString()} item${total !== 1 ? 's' : ''}</div>
        `;

        this.attachContentListeners();
    }

    renderBreadcrumb() {
        const isAtRoot = this.pathStack.length === 0;
        let html = '<nav class="fb-breadcrumb">';
        html += `<span class="fb-crumb${isAtRoot ? ' fb-crumb-current' : ' fb-crumb-link'}" data-nav-index="0">Lists</span>`;
        this.pathStack.forEach((seg, i) => {
            const isLast = i === this.pathStack.length - 1;
            html += '<span class="fb-sep">›</span>';
            html += `<span class="fb-crumb${isLast ? ' fb-crumb-current' : ' fb-crumb-link'}" data-nav-index="${i + 1}">${this.escapeHtml(seg.name)}</span>`;
        });
        html += '</nav>';
        return html;
    }

    // ── Icon view ──────────────────────────────────────────────────────────

    renderIconGrid(folders, lists) {
        if (folders.length === 0 && lists.length === 0) {
            return '<div class="fb-empty">This folder is empty.</div>';
        }

        const folderItems = folders.map(f => this.renderIconFolder(f)).join('');
        const listItems = lists.map(l => this.renderIconList(l)).join('');

        // Separator between folders and lists only when both exist
        const sep = (folders.length > 0 && lists.length > 0)
            ? '<div class="fb-icon-sep"></div>'
            : '';

        return `<div class="fb-icon-grid">${folderItems}${sep}${listItems}</div>`;
    }

    renderIconFolder(folder) {
        return `
            <div class="fb-icon-item fb-icon-folder-item" data-folder-id="${folder.id}" title="${this.escapeHtml(folder.name)}">
                <div class="fb-large-folder"></div>
                <span class="fb-icon-label">${this.escapeHtml(folder.name)}</span>
                <div class="fb-item-actions" data-stop>
                    <button class="fb-action-btn" data-action="rename" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Rename">✎</button>
                    <button class="fb-action-btn" data-action="move" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Move">↪</button>
                    <button class="fb-action-btn fb-action-danger" data-action="delete-folder" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Delete">✕</button>
                </div>
            </div>
        `;
    }

    renderIconList(list) {
        const isIdList = list.type && list.type.includes('IdListItem');
        const cls = isIdList ? 'fb-large-idlist' : 'fb-large-list';
        const total = list.book_count || 0;
        const unread = list.unread_count || 0;
        const countHtml = typeof renderCountBadges === 'function'
            ? renderCountBadges(total, unread, false)
            : `<span class="book-count">${total.toLocaleString()}</span>`;
        return `
            <div class="fb-icon-item fb-icon-list-item" data-list-id="${list.id}" title="${this.escapeHtml(list.name)} — ${total.toLocaleString()} comics, ${unread.toLocaleString()} unread">
                <div class="${cls}"></div>
                <span class="fb-icon-label">${this.escapeHtml(list.name)}</span>
                <span class="fb-icon-counts">${countHtml}</span>
                <div class="fb-item-actions" data-stop>
                    <button class="fb-action-btn" data-action="move" data-id="${list.id}" data-name="${this.escapeHtml(list.name)}" title="Move">↪</button>
                </div>
            </div>
        `;
    }

    // ── Detail view ────────────────────────────────────────────────────────

    renderDetailTable(folders, lists) {
        if (folders.length === 0 && lists.length === 0) {
            return '<div class="fb-empty">This folder is empty.</div>';
        }

        const rows = [
            ...folders.map(f => this.renderDetailFolder(f)),
            ...lists.map(l => this.renderDetailList(l)),
        ].join('');

        return `
            <div class="filebrowser">
                <div class="fb-header-row">
                    <span class="fb-col-name">Name</span>
                    <span class="fb-col-type">Type</span>
                    <span class="fb-col-count">Comics</span>
                </div>
                <div class="fb-body">${rows}</div>
            </div>
        `;
    }

    renderDetailFolder(folder) {
        const children = folder.children || [];
        const nF = children.filter(n => n.is_folder).length;
        const nL = children.filter(n => !n.is_folder).length;
        const parts = [];
        if (nF) parts.push(`${nF} folder${nF !== 1 ? 's' : ''}`);
        if (nL) parts.push(`${nL} list${nL !== 1 ? 's' : ''}`);
        const summary = parts.join(', ') || 'Empty';

        return `
            <div class="fb-row fb-folder-row" data-folder-id="${folder.id}">
                <span class="fb-col-name">
                    <span class="fb-row-folder-icon"></span>
                    <span class="fb-row-name">${this.escapeHtml(folder.name)}</span>
                </span>
                <span class="fb-col-type fb-type-folder">Folder</span>
                <span class="fb-col-count fb-col-subdesc">${summary}</span>
                <span class="fb-col-actions" data-stop>
                    <button class="fb-action-btn" data-action="rename" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Rename">✎</button>
                    <button class="fb-action-btn" data-action="move" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Move">↪</button>
                    <button class="fb-action-btn fb-action-danger" data-action="delete-folder" data-id="${folder.id}" data-name="${this.escapeHtml(folder.name)}" title="Delete">✕</button>
                </span>
            </div>
        `;
    }

    renderDetailList(list) {
        const isIdList = list.type && list.type.includes('IdListItem');
        const typeLabel = isIdList ? 'ID List' : 'Smart List';
        const typeClass = isIdList ? 'fb-type-idlist' : 'fb-type-smart';
        const iconClass = isIdList ? 'fb-row-idlist-icon' : 'fb-row-list-icon';
        const total = list.book_count || 0;
        const unread = list.unread_count || 0;
        const countHtml = typeof renderCountBadges === 'function'
            ? renderCountBadges(total, unread, false)
            : `<span class="fb-col-count">${total.toLocaleString()}</span>`;

        return `
            <div class="fb-row fb-list-row" data-list-id="${list.id}">
                <span class="fb-col-name">
                    <span class="${iconClass}"></span>
                    <span class="fb-row-name">${this.escapeHtml(list.name)}</span>
                </span>
                <span class="fb-col-type ${typeClass}">${typeLabel}</span>
                <span class="fb-col-count fb-count-badges">${countHtml}</span>
                <span class="fb-col-actions" data-stop>
                    <button class="fb-action-btn" data-action="move" data-id="${list.id}" data-name="${this.escapeHtml(list.name)}" title="Move">↪</button>
                </span>
            </div>
        `;
    }

    // ── Event listeners ────────────────────────────────────────────────────

    attachContentListeners() {
        // Action buttons stop propagation so they don't trigger folder navigation
        document.querySelectorAll('[data-stop]').forEach(el => {
            el.addEventListener('click', e => e.stopPropagation());
        });

        document.querySelectorAll('[data-folder-id]').forEach(el => {
            el.addEventListener('click', () => this.navigateToFolder(el.dataset.folderId));
        });

        document.querySelectorAll('[data-list-id]').forEach(el => {
            el.addEventListener('click', () => router.navigate(`/lists/${el.dataset.listId}`));
        });

        document.querySelectorAll('.fb-crumb-link').forEach(crumb => {
            crumb.addEventListener('click', () => {
                this.navigateToBreadcrumb(parseInt(crumb.dataset.navIndex));
            });
        });

        document.querySelectorAll('.fb-view-btn').forEach(btn => {
            btn.addEventListener('click', () => this.setViewMode(btn.dataset.view));
        });

        document.getElementById('new-list-btn')?.addEventListener('click', () => this.showNewListDialog());
        document.getElementById('new-folder-btn')?.addEventListener('click', () => this.showNewFolderDialog());

        document.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', e => {
                e.stopPropagation();
                const { action, id, name } = btn.dataset;
                if (action === 'rename') this.showRenameFolderDialog(id, name);
                else if (action === 'delete-folder') this.showDeleteFolderDialog(id, name);
                else if (action === 'move') this.showMoveDialog(id, name);
            });
        });
    }

    async reloadTree() {
        if (this.tree) {
            await this.tree.loadTree();
            this.tree.render();
        }
        this.renderContent();
    }

    async showNewListDialog() {
        const name = prompt('New smart list name:');
        if (!name || !name.trim()) return;

        const parentID = this.currentFolderID();
        try {
            const resp = await fetch('/api/library/lists', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: name.trim(), type: 'ComicSmartListItem', matcher_mode: 'And', matchers: [] })
            });
            if (!resp.ok) throw new Error(await resp.text() || 'Create failed');
            const created = await resp.json();
            // If inside a folder, move the new list there
            if (parentID) {
                await fetch(`/api/library/lists/${created.id}/parent`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ parent_id: parentID })
                });
            }
            router.navigate(`/lists/${created.id}`);
        } catch (e) {
            console.error('Failed to create list:', e);
            alert('Failed to create list: ' + e.message);
        }
    }

    async showNewFolderDialog() {
        const name = prompt('New folder name:');
        if (!name || !name.trim()) return;

        const parentID = this.currentFolderID();
        try {
            const resp = await fetch('/api/library/folders', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: name.trim(), parent_id: parentID || '' })
            });
            if (!resp.ok) throw new Error(await resp.text() || 'Create failed');
            await this.reloadTree();
        } catch (e) {
            console.error('Failed to create folder:', e);
            alert('Failed to create folder: ' + e.message);
        }
    }

    async showRenameFolderDialog(id, currentName) {
        const name = prompt('Rename folder:', currentName);
        if (!name || !name.trim() || name.trim() === currentName) return;

        try {
            // Fetch current list state then update name
            const resp = await fetch(`/api/library/lists/${id}/raw`);
            if (!resp.ok) throw new Error('Failed to fetch folder');
            const folder = await resp.json();
            folder.Name = name.trim();
            const updateResp = await fetch(`/api/library/lists/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: folder.Name, type: folder.Type, matcher_mode: folder.MatcherMode || '', matchers: folder.Matchers || [] })
            });
            if (!updateResp.ok) throw new Error(await updateResp.text() || 'Rename failed');
            await this.reloadTree();
        } catch (e) {
            console.error('Failed to rename folder:', e);
            alert('Failed to rename folder: ' + e.message);
        }
    }

    async showDeleteFolderDialog(id, name) {
        if (!confirm(`Delete folder "${name}" and all its contents? This cannot be undone.`)) return;
        try {
            const resp = await fetch(`/api/library/lists/${id}`, { method: 'DELETE' });
            if (!resp.ok) throw new Error(await resp.text() || 'Delete failed');
            await this.reloadTree();
        } catch (e) {
            console.error('Failed to delete folder:', e);
            alert('Failed to delete folder: ' + e.message);
        }
    }

    async showMoveDialog(id, name) {
        // Build a flat list of all folders the item can be moved to
        const allFolders = this.collectFolders(this.tree ? (this.tree.tree || []) : [], id);
        const options = [{ id: '', name: '(Root)' }, ...allFolders];
        const lines = options.map((f, i) => `${i + 1}. ${f.name}`).join('\n');
        const sel = prompt(`Move "${name}" to:\n\n${lines}\n\nEnter number:`);
        if (!sel) return;
        const idx = parseInt(sel) - 1;
        if (isNaN(idx) || idx < 0 || idx >= options.length) return;

        const parentID = options[idx].id;
        try {
            const resp = await fetch(`/api/library/lists/${id}/parent`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ parent_id: parentID })
            });
            if (!resp.ok) throw new Error(await resp.text() || 'Move failed');
            await this.reloadTree();
        } catch (e) {
            console.error('Failed to move item:', e);
            alert('Failed to move: ' + e.message);
        }
    }

    // Returns a flat list of all folders, excluding the given id and its descendants.
    collectFolders(items, excludeId, prefix) {
        prefix = prefix || '';
        const result = [];
        for (const node of items) {
            if (!node.is_folder || node.id === excludeId) continue;
            const label = prefix ? `${prefix} / ${node.name}` : node.name;
            result.push({ id: node.id, name: label });
            result.push(...this.collectFolders(node.children || [], excludeId, label));
        }
        return result;
    }

    // Returns the ID of the current folder (deepest path segment), or '' for root.
    currentFolderID() {
        if (this.pathStack.length === 0) return '';
        return this.pathStack[this.pathStack.length - 1].id;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
