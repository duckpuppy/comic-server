// Shared list-picker modal - a searchable tree of smart lists (with folder
// hierarchy for context) used wherever a page needs the user to pick ONE
// list to assign somewhere. Folders are shown but not selectable; only
// list rows are. Used by deviceDetail.js and deviceSettings.js so the
// picker only needs to be built once.
class ListPicker {
    constructor() {
        this.tree = [];
        this.excludeListIds = [];
        this.onSelect = null;
        this.searchTerm = '';
    }

    async open({ title = 'Select a List', excludeListIds = [], onSelect }) {
        this.excludeListIds = excludeListIds;
        this.onSelect = onSelect;
        this.searchTerm = '';
        this.title = title;

        this.tree = await this.loadTree();

        const modalHTML = `
            <div class="modal-overlay" id="list-picker-modal" onclick="listPicker.close(event)">
                <div class="modal-content" onclick="event.stopPropagation()">
                    <div class="modal-header">
                        <h2>${this.escapeHtml(this.title)}</h2>
                        <button class="modal-close" onclick="listPicker.close()">×</button>
                    </div>
                    <div class="modal-body">
                        <input type="text" class="list-picker-search" id="list-picker-search"
                               placeholder="Search lists..." autofocus>
                        <div class="list-picker-tree" id="list-picker-tree"></div>
                    </div>
                </div>
            </div>
        `;

        document.body.insertAdjacentHTML('beforeend', modalHTML);
        this.renderTree();

        const searchInput = document.getElementById('list-picker-search');
        searchInput.addEventListener('input', () => {
            this.searchTerm = searchInput.value;
            this.renderTree();
        });
    }

    async loadTree() {
        try {
            const response = await fetch('/api/library/lists/tree');
            const data = await response.json();
            return data.tree || [];
        } catch (error) {
            console.error('Failed to load list tree:', error);
            return [];
        }
    }

    close(event) {
        if (event && event.target !== event.currentTarget) {
            return; // clicked inside the modal content
        }
        const modal = document.getElementById('list-picker-modal');
        if (modal) {
            modal.remove();
        }
    }

    select(listId, listName) {
        this.close();
        if (this.onSelect) {
            this.onSelect(listId, listName);
        }
    }

    // Returns a filtered copy of nodes: list nodes matching the search term
    // (and not excluded) are kept; folder nodes are kept only if they have
    // at least one visible descendant after filtering.
    filterTree(nodes) {
        const term = this.searchTerm.trim().toLowerCase();
        const filterNode = (node) => {
            if (node.is_folder) {
                const children = (node.children || [])
                    .map(filterNode)
                    .filter(n => n !== null);
                if (children.length === 0) return null;
                return { ...node, children };
            }
            if (this.excludeListIds.includes(node.id)) return null;
            if (term && !node.name.toLowerCase().includes(term)) return null;
            return node;
        };
        return nodes.map(filterNode).filter(n => n !== null);
    }

    renderTree() {
        const container = document.getElementById('list-picker-tree');
        if (!container) return;

        const filtered = this.filterTree(this.tree);
        if (filtered.length === 0) {
            container.innerHTML = '<p class="empty-message">No matching lists.</p>';
            return;
        }

        container.innerHTML = filtered.map(node => this.renderNode(node)).join('');

        container.querySelectorAll('.list-picker-node.list').forEach(el => {
            el.addEventListener('click', () => {
                this.select(el.dataset.listId, el.dataset.listName);
            });
        });
    }

    renderNode(node, level = 0) {
        const indent = level * 16;

        if (node.is_folder) {
            let html = `
                <div class="list-picker-node folder" style="padding-left: ${indent}px">
                    <span class="list-picker-icon">📁</span>
                    <span class="list-picker-name">${this.escapeHtml(node.name)}</span>
                </div>
            `;
            (node.children || []).forEach(child => {
                html += this.renderNode(child, level + 1);
            });
            return html;
        }

        return `
            <div class="list-picker-node list" style="padding-left: ${indent + 16}px"
                 data-list-id="${this.escapeHtml(node.id)}" data-list-name="${this.escapeHtml(node.name)}">
                <span class="list-picker-icon">📋</span>
                <span class="list-picker-name">${this.escapeHtml(node.name)}</span>
                <span class="list-picker-count">${(node.book_count || 0).toLocaleString()} books</span>
            </div>
        `;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}

// Global shared instance
const listPicker = new ListPicker();
