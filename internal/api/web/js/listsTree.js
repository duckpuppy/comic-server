// Smart Lists Tree Sidebar
class ListsTree {
    constructor() {
        this.tree = [];
        this.expandedFolders = new Set();
        this.selectedListId = null;
        this.onListSelected = null; // Callback for when a list is selected
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
        this.render();
        if (this.onListSelected) {
            this.onListSelected(listId);
        }
    }

    renderNode(node, level = 0) {
        const indent = level * 16; // 16px per level
        const isExpanded = this.expandedFolders.has(node.id);
        const isSelected = this.selectedListId === node.id;

        if (node.is_folder) {
            const expandIcon = isExpanded ? '▼' : '▶';
            const folderIcon = isExpanded ? '📂' : '📁';

            let html = `
                <div class="tree-node folder ${isExpanded ? 'expanded' : ''}" style="padding-left: ${indent}px">
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
            // Smart list node
            const selectedClass = isSelected ? 'selected' : '';
            return `
                <div class="tree-node list ${selectedClass}"
                     style="padding-left: ${indent + 16}px"
                     data-list-id="${node.id}">
                    <span class="list-icon">📋</span>
                    <span class="node-name">${node.name}</span>
                    <span class="book-count">${node.book_count || 0}</span>
                </div>
            `;
        }
    }

    render() {
        const container = document.getElementById('lists-tree-sidebar');
        if (!container) return;

        let html = '<div class="tree-container">';
        this.tree.forEach(node => {
            html += this.renderNode(node);
        });
        html += '</div>';

        container.innerHTML = html;
        this.attachListeners();
    }

    attachListeners() {
        // Folder expand/collapse
        document.querySelectorAll('.expand-icon').forEach(icon => {
            icon.addEventListener('click', (e) => {
                e.stopPropagation();
                const folderId = icon.dataset.folderId;
                this.toggleFolder(folderId);
            });
        });

        // List selection
        document.querySelectorAll('.tree-node.list').forEach(node => {
            node.addEventListener('click', (e) => {
                const listId = node.dataset.listId;
                this.selectList(listId);
            });
        });
    }
}

// Export for use in other modules
window.ListsTree = ListsTree;
