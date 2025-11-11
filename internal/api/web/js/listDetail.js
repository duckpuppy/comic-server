// List Detail Page
class ListDetail {
    constructor(listId) {
        this.listId = listId;
        this.list = null;
        this.devices = [];
        this.preview = [];
        this.previewOffset = 0;
        this.previewLimit = 20;
        this.previewTotal = 0;
        this.tree = null;
    }

    async init() {
        // Initialize tree sidebar
        this.tree = new ListsTree();
        this.tree.onListSelected = (listId) => this.onListSelected(listId);

        await Promise.all([
            this.loadListDetail(),
            this.loadDevices(),
            this.loadPreview(),
            this.tree.init()
        ]);

        // Select current list in tree
        this.tree.selectedListId = this.listId;

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

        return this.list.matchers.map(matcher => `
            <li class="matcher-item">
                <span class="matcher-bullet">•</span>
                <span class="matcher-text">${this.escapeHtml(matcher)}</span>
            </li>
        `).join('');
    }

    renderDeviceAssignments() {
        if (this.devices.length === 0) {
            return `
                <p class="empty-message">This list is not assigned to any devices.</p>
                <button class="btn btn-primary" onclick="alert('Assign to device - TODO')">
                    + Assign to Device
                </button>
            `;
        }

        return this.devices.map(device => `
            <div class="device-assignment-card">
                <div class="device-assignment-info">
                    <h4>${this.escapeHtml(device.friendly_name)}</h4>
                    <span class="device-status ${device.enabled ? 'enabled' : 'disabled'}">
                        ${device.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                </div>
                <div class="device-assignment-actions">
                    <button class="btn btn-small" onclick="router.navigate('/devices/${device.device_id}')">
                        View Device
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
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }
}
