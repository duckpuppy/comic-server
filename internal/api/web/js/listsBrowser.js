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
            </div>
        `;
    }

    // ── Event listeners ────────────────────────────────────────────────────

    attachContentListeners() {
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
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
