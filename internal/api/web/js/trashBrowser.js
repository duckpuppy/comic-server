// Trash Browser - lists quarantined files (internal/trash) with Restore,
// per-item or multi-select (comic-server-tfs).
class TrashBrowser {
    constructor() {
        this.entries = [];
        this.selectedIds = new Set();
        this.notConfigured = false;
    }

    async init(ctx) {
        await this.loadEntries();
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async loadEntries() {
        try {
            const response = await fetch('/api/trash');
            if (response.status === 503) {
                this.notConfigured = true;
                this.entries = [];
                return;
            }
            this.notConfigured = false;
            const data = await response.json();
            this.entries = data.entries || [];
            // Loading may have dropped entries that were selected (e.g.
            // restored from another tab) - drop stale selections rather
            // than silently keep them checked against nothing.
            const liveIds = new Set(this.entries.map(e => e.id));
            for (const id of this.selectedIds) {
                if (!liveIds.has(id)) this.selectedIds.delete(id);
            }
        } catch (error) {
            console.error('Failed to load trash entries:', error);
            this.entries = [];
        }
    }

    formatSize(bytes) {
        if (bytes < 1024) return `${bytes} B`;
        const units = ['KB', 'MB', 'GB'];
        let value = bytes / 1024;
        let unit = 0;
        while (value >= 1024 && unit < units.length - 1) {
            value /= 1024;
            unit++;
        }
        return `${value.toFixed(1)} ${units[unit]}`;
    }

    formatRelativeTime(isoString) {
        const then = new Date(isoString);
        const diffSeconds = (Date.now() - then.getTime()) / 1000;
        if (diffSeconds < 60) return 'just now';
        const diffMinutes = diffSeconds / 60;
        if (diffMinutes < 60) return `${Math.floor(diffMinutes)}m ago`;
        const diffHours = diffMinutes / 60;
        if (diffHours < 24) return `${Math.floor(diffHours)}h ago`;
        const diffDays = diffHours / 24;
        return `${Math.floor(diffDays)}d ago`;
    }

    render() {
        const app = document.getElementById('app');

        if (this.notConfigured) {
            app.innerHTML = `
                <div class="trash-browser-page">
                    <h1>Trash</h1>
                    <p class="empty-message">Trash isn't configured (server.trash_path is empty) - nothing has been quarantined.</p>
                </div>
            `;
            return;
        }

        app.innerHTML = `
            <div class="trash-browser-page">
                <div class="trash-header">
                    <h1>Trash</h1>
                    <p class="trash-subtitle">Files replaced by features like Convert to CBZ are quarantined here, not deleted.</p>
                </div>

                <div class="trash-toolbar${this.selectedIds.size > 0 ? ' active' : ''}" id="trash-toolbar">
                    <span class="trash-selection-count">${this.selectedIds.size} selected</span>
                    <button class="btn btn-primary" id="restore-selected-btn" ${this.selectedIds.size === 0 ? 'disabled' : ''}>
                        Restore Selected
                    </button>
                </div>

                ${this.entries.length === 0
                    ? '<p class="empty-message">Trash is empty.</p>'
                    : this.renderTable()}
            </div>
        `;
    }

    renderTable() {
        return `
            <table class="trash-table">
                <thead>
                    <tr>
                        <th class="trash-col-checkbox">
                            <input type="checkbox" id="trash-select-all"
                                   ${this.selectedIds.size === this.entries.length ? 'checked' : ''}>
                        </th>
                        <th>Original Path</th>
                        <th>Size</th>
                        <th>Quarantined</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody>
                    ${this.entries.map(e => this.renderRow(e)).join('')}
                </tbody>
            </table>
        `;
    }

    renderRow(entry) {
        return `
            <tr data-id="${this.escapeHtml(entry.id)}">
                <td class="trash-col-checkbox">
                    <input type="checkbox" class="trash-row-check" value="${this.escapeHtml(entry.id)}"
                           ${this.selectedIds.has(entry.id) ? 'checked' : ''}>
                </td>
                <td class="trash-original-path" title="${this.escapeHtml(entry.original_path)}">${this.escapeHtml(entry.original_path)}</td>
                <td>${this.formatSize(entry.size)}</td>
                <td title="${this.escapeHtml(entry.quarantined_at)}">${this.formatRelativeTime(entry.quarantined_at)}</td>
                <td>
                    <button class="btn btn-small trash-restore-btn" data-id="${this.escapeHtml(entry.id)}">Restore</button>
                </td>
            </tr>
        `;
    }

    attachListeners() {
        const selectAll = document.getElementById('trash-select-all');
        if (selectAll) {
            selectAll.addEventListener('change', () => {
                if (selectAll.checked) {
                    this.entries.forEach(e => this.selectedIds.add(e.id));
                } else {
                    this.selectedIds.clear();
                }
                this.render();
                this.attachListeners();
            });
        }

        document.querySelectorAll('.trash-row-check').forEach(cb => {
            cb.addEventListener('change', () => {
                if (cb.checked) {
                    this.selectedIds.add(cb.value);
                } else {
                    this.selectedIds.delete(cb.value);
                }
                this.render();
                this.attachListeners();
            });
        });

        document.querySelectorAll('.trash-restore-btn').forEach(btn => {
            btn.addEventListener('click', () => this.confirmAndRestore([btn.dataset.id]));
        });

        const restoreSelectedBtn = document.getElementById('restore-selected-btn');
        if (restoreSelectedBtn) {
            restoreSelectedBtn.addEventListener('click', () => {
                this.confirmAndRestore(Array.from(this.selectedIds));
            });
        }
    }

    async confirmAndRestore(ids) {
        if (ids.length === 0) return;
        const label = ids.length === 1 ? '1 item' : `${ids.length} items`;
        if (!confirm(`Restore ${label} from trash? If a file now occupies the original spot, it will be quarantined in its place.`)) {
            return;
        }
        try {
            const response = await fetch('/api/trash/restore', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ids })
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(text || 'Failed to restore');
            }
            const result = JSON.parse(text);
            if (result.errors && result.errors.length > 0) {
                alert(`Restored ${result.restored}, but some failed:\n${result.errors.join('\n')}`);
            }
            ids.forEach(id => this.selectedIds.delete(id));
            await this.loadEntries();
            this.render();
            this.attachListeners();
        } catch (error) {
            console.error('Failed to restore trash entries:', error);
            alert('Failed to restore: ' + error.message);
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
