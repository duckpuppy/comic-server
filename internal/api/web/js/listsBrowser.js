// Smart Lists Browser
class ListsBrowser {
    constructor() {
        this.lists = [];
        this.filteredLists = [];
        this.searchTerm = '';
        this.sortBy = 'name';
        this.filterAssigned = 'all'; // 'all', 'assigned', 'unassigned'
    }

    async init() {
        await this.loadLists();
        this.render();
        this.attachListeners();
    }

    async loadLists() {
        try {
            const response = await fetch('/api/library/lists');
            const data = await response.json();
            this.lists = data.lists || [];
            this.applyFilters();
        } catch (error) {
            console.error('Failed to load lists:', error);
            this.lists = [];
        }
    }

    applyFilters() {
        let filtered = [...this.lists];

        // Search filter
        if (this.searchTerm) {
            const term = this.searchTerm.toLowerCase();
            filtered = filtered.filter(list =>
                list.name.toLowerCase().includes(term)
            );
        }

        // Assignment filter (would need device data for full implementation)
        // TODO: Implement when device assignment data is available

        // Sort
        filtered.sort((a, b) => {
            switch (this.sortBy) {
                case 'name':
                    return a.name.localeCompare(b.name);
                case 'name-desc':
                    return b.name.localeCompare(a.name);
                case 'count':
                    return b.book_count - a.book_count;
                default:
                    return 0;
            }
        });

        this.filteredLists = filtered;
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="lists-page">
                <div class="lists-header">
                    <h1>Smart Lists</h1>
                    <div class="lists-search">
                        <input
                            type="text"
                            id="lists-search-input"
                            placeholder="Search lists..."
                            value="${this.searchTerm}"
                        >
                    </div>
                </div>

                <div class="lists-content">
                    <aside class="lists-filters">
                        <h3>Filters</h3>
                        <div class="filter-group">
                            <label>Sort by:</label>
                            <select id="lists-sort">
                                <option value="name">Name (A-Z)</option>
                                <option value="name-desc">Name (Z-A)</option>
                                <option value="count">Book Count</option>
                            </select>
                        </div>
                    </aside>

                    <main class="lists-grid">
                        ${this.renderListCards()}
                    </main>
                </div>
            </div>
        `;

        // Update navigation badge
        navigation.updateBadge('lists', this.lists.length);
    }

    renderListCards() {
        if (this.filteredLists.length === 0) {
            return `
                <div class="empty-message">
                    <p>No smart lists found.</p>
                    <p class="help-text">Smart lists are created in ComicRackCE.</p>
                </div>
            `;
        }

        return this.filteredLists.map(list => `
            <div class="list-card" data-list-id="${list.id}">
                <div class="list-card-header">
                    <h3 class="list-name">${this.escapeHtml(list.name)}</h3>
                </div>
                <div class="list-card-body">
                    <div class="list-stat">
                        <span class="stat-icon">📚</span>
                        <span class="stat-value">${list.book_count.toLocaleString()}</span>
                        <span class="stat-label">comics</span>
                    </div>
                    <div class="list-stat">
                        <span class="stat-icon">🔍</span>
                        <span class="stat-value">${list.matcher_count}</span>
                        <span class="stat-label">rules</span>
                    </div>
                </div>
                <div class="list-card-footer">
                    <button class="btn btn-primary btn-small" onclick="router.navigate('/lists/${list.id}')">
                        View Details
                    </button>
                </div>
            </div>
        `).join('');
    }

    attachListeners() {
        // Search input
        const searchInput = document.getElementById('lists-search-input');
        if (searchInput) {
            let debounceTimer;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.searchTerm = e.target.value;
                    this.applyFilters();
                    this.render();
                }, 300);
            });
        }

        // Sort select
        const sortSelect = document.getElementById('lists-sort');
        if (sortSelect) {
            sortSelect.value = this.sortBy;
            sortSelect.addEventListener('change', (e) => {
                this.sortBy = e.target.value;
                this.applyFilters();
                this.render();
            });
        }

        // Card clicks
        const cards = document.querySelectorAll('.list-card');
        cards.forEach(card => {
            card.addEventListener('click', (e) => {
                if (e.target.tagName !== 'BUTTON') {
                    const listId = card.dataset.listId;
                    router.navigate(`/lists/${listId}`);
                }
            });
        });
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
