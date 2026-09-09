// Library Browser (comic-server-joj) - ad-hoc filtered issue browsing.
// Exposes the exact same smart-list matcher engine every saved smart list
// already uses (internal/library/smartlist.go), but for throwaway
// exploration: pick matchers, see results immediately, without first
// having to create-and-save a named list just to find out what it would
// match. Distinct from the smart-list CRUD UI (listDetail.js) - nothing
// built here is ever persisted.
//
// Reuses the same matcher-editor markup/CSS/schema (GET
// /api/library/lists/schema) listDetail.js's edit mode already uses, and
// the same matcher JSON shape (Type/Not/MatchOperator/MatchValue/
// MatchValue2) the existing raw-matcher endpoint already speaks - a
// matcher object built here is wire-compatible with a saved smart list's,
// even though this page never saves one.
class BrowsePage {
    constructor() {
        this.schema = null;
        this.matcherMode = 'And';
        this.matchers = [];
        this.results = [];
        this.total = 0;
        this.limit = 20;
        this.offset = 0;
        this.hasMore = false;
        this.loading = false;
        this.error = null;
        this.searchDebounce = null;
    }

    async init(ctx) {
        await this.loadSchema();
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async loadSchema() {
        try {
            const response = await fetch('/api/library/lists/schema');
            this.schema = await response.json();
        } catch (error) {
            console.error('Failed to load matcher schema:', error);
            this.schema = { matcherTypes: [], operators: {}, matcherModes: [] };
        }
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Browse</h1>
                    <p class="empty-message">Build a filter on the fly and see matching books immediately - nothing here is saved. Use "Save as Smart List" on the Lists page if you want to keep a filter.</p>
                </div>
                <div class="panel matchers-panel">
                    <div class="matchers-editor-header">
                        <h2>Matchers</h2>
                        <div class="matcher-mode-selector">
                            <label>Match:
                                <select id="browse-matcher-mode">
                                    <option value="And" ${this.matcherMode === 'And' ? 'selected' : ''}>ALL conditions (AND)</option>
                                    <option value="Or" ${this.matcherMode === 'Or' ? 'selected' : ''}>ANY condition (OR)</option>
                                </select>
                            </label>
                        </div>
                    </div>
                    <ul class="matchers-list matchers-editor-list" id="browse-matchers-list">
                        ${this.matchers.map((m, i) => this.renderMatcherEditor(m, i)).join('')}
                    </ul>
                    <button id="browse-add-matcher-btn" class="btn btn-secondary btn-add-matcher">+ Add Matcher</button>
                </div>
                <div class="panel">
                    ${this.renderResults()}
                </div>
            </div>
        `;
    }

    renderMatcherEditor(matcher, index) {
        const schema = this.schema || { matcherTypes: [], operators: {} };
        const typeInfo = schema.matcherTypes.find(t => t.id === matcher.Type) || { fieldType: 'string', label: matcher.Type };
        const fieldType = typeInfo.fieldType;
        const ops = (schema.operators && schema.operators[fieldType]) || [];

        const typeOptions = this.renderTypeOptions(matcher.Type);
        const opOptions = ops.map(op =>
            `<option value="${op.value}" ${matcher.MatchOperator === op.value ? 'selected' : ''}>${this.escapeHtml(op.label)}</option>`
        ).join('');

        const selectedOp = ops.find(o => o.value === matcher.MatchOperator) || ops[0] || {};
        const showValue = selectedOp.hasValue !== false;
        const showValue2 = !!selectedOp.hasValue2;
        const isCustomValue = fieldType === 'customvalue';

        return `
            <li class="matcher-editor-row" data-index="${index}">
                <div class="matcher-editor-controls">
                    <label class="matcher-not-toggle" title="Negate this matcher">
                        <input type="checkbox" class="browse-matcher-not-check" data-index="${index}"
                               ${matcher.Not ? 'checked' : ''}> NOT
                    </label>
                    <select class="matcher-type-select browse-matcher-type-select" data-index="${index}">
                        ${typeOptions}
                    </select>
                    ${isCustomValue ? `
                    <input type="text" class="matcher-customfield-input browse-matcher-customfield-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue || '')}" placeholder="custom field name">
                    ` : ''}
                    <select class="matcher-op-select browse-matcher-op-select" data-index="${index}">
                        ${opOptions}
                    </select>
                    ${showValue && !isCustomValue ? `
                    <input type="text" class="matcher-value-input browse-matcher-value-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue || '')}" placeholder="value">
                    ` : ''}
                    ${showValue2 ? `
                    <span class="matcher-range-and">and</span>
                    <input type="text" class="matcher-value2-input browse-matcher-value2-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue2 || '')}" placeholder="value 2">
                    ` : ''}
                    ${isCustomValue ? `
                    <input type="text" class="matcher-value2-input browse-matcher-value2-input" data-index="${index}"
                           value="${this.escapeHtml(matcher.MatchValue2 || '')}" placeholder="value">
                    ` : ''}
                </div>
                <button class="btn btn-small btn-danger matcher-remove-btn browse-matcher-remove-btn" data-index="${index}" title="Remove">✕</button>
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

    renderResults() {
        if (this.error) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.error)}</p>`;
        }
        if (this.matchers.length === 0) {
            return `<p class="empty-message">Add a matcher above to start browsing.</p>`;
        }
        if (this.loading && this.results.length === 0) {
            return `<p class="empty-message">Searching…</p>`;
        }

        let html = `<p class="datamanager-summary">${this.total} book${this.total === 1 ? '' : 's'} matched.</p>`;
        if (this.results.length === 0) {
            return html;
        }

        html += '<table class="datamanager-diff-table"><thead><tr><th>Series</th><th>Number</th><th>Title</th><th>Publisher</th><th>Year</th></tr></thead><tbody>';
        for (const c of this.results) {
            html += `<tr><td>${this.escapeHtml(c.series)}</td><td>${this.escapeHtml(c.number)}</td><td>${this.escapeHtml(c.title)}</td><td>${this.escapeHtml(c.publisher)}</td><td>${c.year || ''}</td></tr>`;
        }
        html += '</tbody></table>';

        if (this.hasMore) {
            html += `<div class="load-more-container"><button id="browse-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
        }
        return html;
    }

    attachListeners() {
        const modeSelect = document.getElementById('browse-matcher-mode');
        if (modeSelect) modeSelect.addEventListener('change', () => {
            this.matcherMode = modeSelect.value;
            this.search(true);
        });

        const addBtn = document.getElementById('browse-add-matcher-btn');
        if (addBtn) addBtn.addEventListener('click', () => this.addMatcher());

        const loadMoreBtn = document.getElementById('browse-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.search(false));

        this.attachMatcherListeners();
    }

    attachMatcherListeners() {
        document.querySelectorAll('.browse-matcher-not-check').forEach(el => {
            el.addEventListener('change', () => {
                this.updateMatcherField(parseInt(el.dataset.index), 'Not', el.checked);
            });
        });
        document.querySelectorAll('.browse-matcher-type-select').forEach(el => {
            el.addEventListener('change', () => {
                this.updateMatcherField(parseInt(el.dataset.index), 'Type', el.value);
            });
        });
        document.querySelectorAll('.browse-matcher-op-select').forEach(el => {
            el.addEventListener('change', () => {
                this.updateMatcherField(parseInt(el.dataset.index), 'MatchOperator', el.value);
            });
        });
        document.querySelectorAll('.browse-matcher-value-input, .browse-matcher-customfield-input').forEach(el => {
            el.addEventListener('input', () => {
                const i = parseInt(el.dataset.index);
                if (this.matchers[i]) this.matchers[i].MatchValue = el.value;
                this.debouncedSearch();
            });
        });
        document.querySelectorAll('.browse-matcher-value2-input').forEach(el => {
            el.addEventListener('input', () => {
                const i = parseInt(el.dataset.index);
                if (this.matchers[i]) this.matchers[i].MatchValue2 = el.value;
                this.debouncedSearch();
            });
        });
        document.querySelectorAll('.browse-matcher-remove-btn').forEach(el => {
            el.addEventListener('click', () => this.removeMatcher(parseInt(el.dataset.index)));
        });
    }

    addMatcher() {
        const schema = this.schema || { matcherTypes: [] };
        const firstType = schema.matcherTypes[0] || { id: 'ComicBookSeriesMatcher' };
        this.matchers.push({ Type: firstType.id, Not: false, MatchOperator: '0', MatchValue: '', MatchValue2: '' });
        this.renderAndReattach();
        this.search(true);
    }

    removeMatcher(index) {
        this.matchers.splice(index, 1);
        this.renderAndReattach();
        this.search(true);
    }

    updateMatcherField(index, field, value) {
        const matcher = this.matchers[index];
        if (!matcher) return;
        matcher[field] = value;
        if (field === 'Type') {
            matcher.MatchOperator = '0';
            matcher.MatchValue = '';
            matcher.MatchValue2 = '';
        }
        this.renderAndReattach();
        this.search(true);
    }

    renderAndReattach() {
        this.render();
        this.attachListeners();
    }

    // debouncedSearch waits for a pause in typing (free-text value inputs)
    // before actually querying, so "browse live" doesn't mean one request
    // per keystroke.
    debouncedSearch() {
        if (this.searchDebounce) clearTimeout(this.searchDebounce);
        this.searchDebounce = setTimeout(() => this.search(true), 400);
    }

    // reset=true starts a fresh search from offset 0 (any matcher edit);
    // reset=false appends the next page (Load More).
    async search(reset) {
        if (this.matchers.length === 0) {
            this.results = [];
            this.total = 0;
            this.render();
            this.attachListeners();
            return;
        }

        this.loading = true;
        this.error = null;
        if (reset) {
            this.offset = 0;
            this.results = [];
        }

        try {
            const url = `/api/library/browse?limit=${this.limit}&offset=${this.offset}`;
            const response = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ matcher_mode: this.matcherMode, matchers: this.matchers }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to browse library'));
            }
            const result = JSON.parse(text);
            this.total = result.total || 0;
            this.hasMore = !!result.has_more;
            const page = result.comics || [];
            this.results = reset ? page : this.results.concat(page);
            this.offset += this.limit;
        } catch (error) {
            console.error('Failed to browse library:', error);
            this.error = `Failed: ${error.message}`;
        } finally {
            this.loading = false;
            this.render();
            this.attachListeners();
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text === undefined || text === null ? '' : String(text);
        return div.innerHTML;
    }
}

window.BrowsePage = BrowsePage;
