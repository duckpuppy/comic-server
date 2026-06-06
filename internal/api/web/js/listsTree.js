// Smart Lists Tree Sidebar
class ListsTree {
    constructor() {
        this.tree = [];
        this.expandedFolders = new Set();
        this.selectedListId = null;
        this.selectedFolderId = null;
        this.onListSelected = null;
        this.onFolderSelected = null; // called with (folderId) when a folder is clicked
    }

    async init() {
        await this.loadTree();
        this.render();
    }

    async loadTree() {
        try {
            const response = await fetch('/api/library/lists/tree');
            const data = await response.json();
            this.tree = data.tree || [];
        } catch (error) {
            console.error('Failed to load list tree:', error);
            this.tree = [];
        }
    }

    // Called from file browser when navigating; expands ancestors and highlights folder
    setCurrentPath(pathStack) {
        for (const seg of pathStack) {
            this.expandedFolders.add(seg.id);
        }
        this.selectedFolderId = pathStack.length > 0
            ? pathStack[pathStack.length - 1].id
            : null;
        this.selectedListId = null;
        this.render();
    }

    // Returns the array of ancestor folder nodes (root → immediate parent) for any node ID,
    // or null if the target is not found.
    findAncestors(targetId, items, ancestors) {
        if (!items) items = this.tree || [];
        if (!ancestors) ancestors = [];
        for (const node of items) {
            if (node.id === targetId) return ancestors;
            if (node.is_folder && node.children) {
                const found = this.findAncestors(targetId, node.children, [...ancestors, node]);
                if (found) return found;
            }
        }
        return null;
    }

    toggleFolder(folderId) {
        if (this.expandedFolders.has(folderId)) {
            this.expandedFolders.delete(folderId);
        } else {
            this.expandedFolders.add(folderId);
        }
        this.render();
    }

    selectList(listId) {
        this.selectedListId = listId;
        this.selectedFolderId = null;
        this.render();
        if (this.onListSelected) {
            this.onListSelected(listId);
        }
    }

    navigateFolder(folderId) {
        this.expandedFolders.add(folderId);
        this.selectedFolderId = folderId;
        this.selectedListId = null;
        this.render();
        if (this.onFolderSelected) {
            this.onFolderSelected(folderId);
        }
    }

    renderNode(node, level = 0) {
        const indent = level * 16;
        const isExpanded = this.expandedFolders.has(node.id);

        if (node.is_folder) {
            const isSelected = this.selectedFolderId === node.id;
            const expandIcon = isExpanded ? '▼' : '▶';
            const folderIcon = isExpanded ? '📂' : '📁';

            let html = `
                <div class="tree-node folder${isSelected ? ' selected' : ''}" data-folder-id="${node.id}" style="padding-left: ${indent}px">
                    <span class="expand-icon" data-folder-id="${node.id}">${expandIcon}</span>
                    <span class="folder-icon">${folderIcon}</span>
                    <span class="node-name">${node.name}</span>
                </div>
            `;

            if (isExpanded && node.children) {
                node.children.forEach(child => {
                    html += this.renderNode(child, level + 1);
                });
            }

            return html;
        } else {
            const isSelected = this.selectedListId === node.id;
            const total = node.book_count || 0;
            const unread = node.unread_count || 0;
            return `
                <div class="tree-node list${isSelected ? ' selected' : ''}"
                     style="padding-left: ${indent + 16}px"
                     data-list-id="${node.id}">
                    <span class="list-icon">📋</span>
                    <span class="node-name">${node.name}</span>
                    ${renderCountBadges(total, unread, isSelected)}
                </div>
            `;
        }
    }

    render() {
        const container = document.getElementById('lists-tree-sidebar');
        if (!container) return;

        const savedScroll = container.scrollTop;

        let html = '<div class="tree-container">';
        this.tree.forEach(node => {
            html += this.renderNode(node);
        });
        html += '</div>';

        container.innerHTML = html;
        container.scrollTop = savedScroll;

        this.attachListeners();
    }

    attachListeners() {
        // Expand toggle — only expand/collapse, no navigation
        document.querySelectorAll('.expand-icon').forEach(icon => {
            icon.addEventListener('click', (e) => {
                e.stopPropagation();
                this.toggleFolder(icon.dataset.folderId);
            });
        });

        // Folder row — navigate to folder in file browser AND expand
        document.querySelectorAll('.tree-node.folder').forEach(node => {
            node.addEventListener('click', () => {
                this.navigateFolder(node.dataset.folderId);
            });
        });

        // List selection
        document.querySelectorAll('.tree-node.list').forEach(node => {
            node.addEventListener('click', () => {
                this.selectList(node.dataset.listId);
            });
        });
    }
}

window.ListsTree = ListsTree;

// Shared helper: renders count badge(s) for a list node.
// When all books are read (unread === 0): single green badge showing total.
// When some are unread: amber badge showing unread + neutral badge showing total.
// isSelected: when true the node has a blue background, so use white-tinted badges.
function renderCountBadges(total, unread, isSelected) {
    if (total === 0) {
        return `<span class="book-count count-zero${isSelected ? ' count-selected' : ''}">${total}</span>`;
    }
    if (unread === 0) {
        return `<span class="book-count count-read${isSelected ? ' count-selected' : ''}">${total.toLocaleString()}</span>`;
    }
    return `
        <span class="book-count count-unread${isSelected ? ' count-selected' : ''}" title="${unread.toLocaleString()} unread">${unread.toLocaleString()}</span>
        <span class="book-count count-total${isSelected ? ' count-selected' : ''}" title="${total.toLocaleString()} total">${total.toLocaleString()}</span>
    `;
}
