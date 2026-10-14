// Library Browser (comic-server-joj) - ad-hoc filtered issue browsing.
// Exposes the exact same smart-list matcher engine every saved smart list
// already uses (internal/library/smartlist.go), but for throwaway
// exploration: pick matchers, see results immediately, without first
// having to create-and-save a named list just to find out what it would
// match. Distinct from the smart-list CRUD UI (listDetail.js) - nothing
// built here is persisted UNLESS the user explicitly clicks "Save as
// Smart List" (comic-server-hil), which bridges the current in-progress
// filter into that same CRUD rather than duplicating it.
//
// Reuses the same matcher-editor markup/CSS/schema (GET
// /api/library/lists/schema) listDetail.js's edit mode already uses, and
// the same matcher JSON shape (Type/Not/MatchOperator/MatchValue/
// MatchValue2) the existing raw-matcher endpoint already speaks - a
// matcher object built here is wire-compatible with a saved smart list's,
// which is exactly what makes Save as Smart List a plain POST with no
// transformation needed.
//
// comic-server-w7ig adds Browse's first bulk action: Data Manager,
// scoped to whatever the matcher list (or the explicit "Show all books"
// opt-in) currently means - this REPLACES the old standalone whole-
// library Data Manager tab, which dataManagerPage.js used to be. The
// Data Manager section below ports that page's own job/diff-table/
// polling logic essentially unchanged, just pointed at the Browse-scoped
// /api/library/browse/datamanager-* endpoints instead of the whole-
// library ones - both still share the same single server-side job slot
// (GET /api/library/datamanager-job), so only one Data Manager run of
// either kind is ever in flight at a time.
class BrowsePage {
    constructor() {
        this.schema = null;
        this.matcherMode = 'And';
        this.matchers = [];
        this.showAll = false;
        this.query = '';      // quick search: words plus #N, (YYYY), VN tokens (comic-server-mzkf)
        this.sortField = '';  // '' = library order
        this.sortDesc = false;
        // Filter rail (comic-server-j6lk): selected values per facet, the
        // server's option counts, and the result view.
        this.facets = { publishers: [], formats: [], stages: [], flags: [] };
        this.facetCounts = null;
        this.view = 'table';
        try { this.view = localStorage.getItem('browse-view') || 'table'; } catch (e) { /* storage blocked */ }
        this.results = [];
        this.total = 0;
        this.limit = 20;
        this.offset = 0;
        this.hasMore = false;
        this.loading = false;
        this.error = null;
        this.searchDebounce = null;

        // Data Manager section state (comic-server-w7ig) - field names
        // prefixed dm* throughout to keep them distinct from the browse
        // table's own state above.
        this.dmBooks = [];
        this.dmSelected = new Map();
        this.dmLimit = 20;
        this.dmOffset = 0;
        this.dmHasMore = false;
        this.dmSummary = null;
        this.dmLastResult = null;
        this.dmError = null;
        this.dmJobKind = null; // 'preview' | 'apply' | null
        this.dmJobTotal = 0;
        this.dmJobProcessed = 0;
        this.dmPollTimer = null;
        this.dmPollCtx = null;
        this.dmLastHandledJobId = null;
    }

    async init(ctx) {
        await this.loadSchema();
        if (ctx && ctx.aborted) return;
        const fromUrl = this.readUrl();
        this.render();
        this.attachListeners();
        if (fromUrl) this.search(true);
        else this.writeUrl(); // reached via the nav: show the kept filter in the URL
        await this.resumeDMJobIfAny(ctx);
    }

    // The filter lives in the URL (comic-server-j6lk) so Back from a book
    // page, a reload or a bookmark returns to the same results. Returns
    // whether the URL carried a filter.
    readUrl() {
        const p = new URLSearchParams(window.location.search);
        if (![...p.keys()].length) return false;
        this.showAll = p.get('all') === '1';
        this.query = p.get('q') || '';
        this.sortField = p.get('sort') || '';
        this.sortDesc = p.get('desc') === '1';
        this.matcherMode = p.get('mode') === 'Or' ? 'Or' : 'And';
        try { this.matchers = JSON.parse(p.get('m') || '[]'); } catch (e) { this.matchers = []; }
        if (!Array.isArray(this.matchers)) this.matchers = [];
        this.facets = {
            publishers: p.getAll('pub'), formats: p.getAll('fmt'),
            stages: p.getAll('stage'), flags: p.getAll('flag'),
        };
        return true;
    }

    writeUrl() {
        if (window.location.pathname !== '/browse') return; // navigated away mid-search
        const p = new URLSearchParams();
        if (this.showAll) p.set('all', '1');
        if (this.matchers.length) p.set('m', JSON.stringify(this.matchers));
        if (this.matcherMode === 'Or') p.set('mode', 'Or');
        if (this.query) p.set('q', this.query);
        if (this.sortField) p.set('sort', this.sortField);
        if (this.sortDesc) p.set('desc', '1');
        for (const [key, facet] of [['pub', 'publishers'], ['fmt', 'formats'], ['stage', 'stages'], ['flag', 'flags']]) {
            for (const v of this.facets[facet]) p.append(key, v);
        }
        const qs = p.toString();
        const url = '/browse' + (qs ? '?' + qs : '');
        if (url !== window.location.pathname + window.location.search) {
            window.history.replaceState({}, '', url);
            if (typeof router !== 'undefined') router.currentPath = '/browse';
        }
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
        // The page re-renders wholesale after every search; remember which
        // text input had focus (and the caret) so typing in the quick search
        // is not interrupted.
        const active = document.activeElement;
        const focusId = active && active.id && app.contains(active) ? active.id : null;
        const caret = focusId && typeof active.selectionStart === 'number' ? active.selectionStart : null;
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Browse</h1>
                    <p class="empty-message">Build a filter on the fly and see matching books immediately. Nothing is saved unless you click "Save as Smart List" below.</p>
                </div>
                <div class="panel matchers-panel">
                    <div class="matchers-editor-header">
                        <h2>Matchers</h2>
                        <div class="matcher-mode-selector">
                            <label>Match:
                                <select id="browse-matcher-mode" ${this.showAll ? 'disabled' : ''}>
                                    <option value="And" ${this.matcherMode === 'And' ? 'selected' : ''}>ALL conditions (AND)</option>
                                    <option value="Or" ${this.matcherMode === 'Or' ? 'selected' : ''}>ANY condition (OR)</option>
                                </select>
                            </label>
                            <label class="browse-show-all-toggle">
                                <input type="checkbox" id="browse-show-all" ${this.showAll ? 'checked' : ''}>
                                Show all books (ignore matchers)
                            </label>
                        </div>
                    </div>
                    <ul class="matchers-list matchers-editor-list" id="browse-matchers-list">
                        ${this.matchers.map((m, i) => this.renderMatcherEditor(m, i)).join('')}
                    </ul>
                    <div class="datamanager-actions">
                        <button id="browse-add-matcher-btn" class="btn btn-secondary btn-add-matcher" ${this.showAll ? 'disabled' : ''}>+ Add Matcher</button>
                        <button id="browse-save-as-list-btn" class="btn btn-primary" ${this.matchers.length === 0 || this.showAll ? 'disabled' : ''}>Save as Smart List</button>
                    </div>
                </div>
                <div class="panel browse-results-panel">
                  ${this.renderRail()}
                  <div class="browse-main">
                    <div class="browse-search-row">
                        <input type="text" id="browse-query" class="browse-query-input"
                               value="${this.escapeHtml(this.query)}"
                               placeholder="Quick search, e.g. Batman V2017 #15 (2016)">
                        <select id="browse-sort" title="Sort results">
                            ${[['', 'Library order'], ['series', 'Series'], ['number', 'Issue number'], ['title', 'Title'],
                               ['publisher', 'Publisher'], ['year', 'Year'], ['volume', 'Volume'], ['pages', 'Page count']]
                                .map(([v, l]) => `<option value="${v}" ${this.sortField === v ? 'selected' : ''}>${l}</option>`).join('')}
                        </select>
                        <label class="browse-sort-desc"><input type="checkbox" id="browse-sort-desc" ${this.sortDesc ? 'checked' : ''}> Descending</label>
                        <div class="browse-view-toggle" role="group" aria-label="View">
                            ${[['table', 'Table'], ['grid', 'Grid'], ['compact', 'Compact']].map(([v, l]) =>
                                `<button class="btn btn-small ${this.view === v ? 'btn-primary' : 'btn-secondary'} browse-view-btn" data-view="${v}" aria-pressed="${this.view === v}">${l}</button>`).join('')}
                        </div>
                    </div>
                    ${this.renderChips()}
                    ${this.renderResults()}
                  </div>
                </div>
                ${this.renderDataManagerPanel()}
            </div>
        `;
        if (focusId) {
            const el = document.getElementById(focusId);
            if (el) {
                el.focus();
                if (caret !== null && el.setSelectionRange) {
                    try { el.setSelectionRange(caret, caret); } catch (e) { /* not a text input */ }
                }
            }
        }
    }

    static FACETS = [
        ['publishers', 'Publisher'],
        ['formats', 'Format'],
        ['stages', 'Stage'],
        ['flags', 'Needs attention'],
    ];

    facetLabel(facet, value) {
        const opt = this.facetCounts && (this.facetCounts[facet] || []).find(o => o.value.toLowerCase() === value.toLowerCase());
        if (opt) return opt.label;
        return value === '' ? (facet === 'stages' ? 'Not staged' : '(none)') : value;
    }

    activeFacetCount() {
        return Object.values(this.facets).reduce((n, v) => n + v.length, 0);
    }

    // Filter rail: one checkbox list per facet with the count each option
    // would keep. Counts ignore the rail's own selections, so options do
    // not disappear as you tick them. Collapsed on a phone.
    renderRail() {
        if (this.matchers.length === 0 && !this.showAll) return '';
        const fc = this.facetCounts;
        const sections = BrowsePage.FACETS.map(([facet, title]) => {
            const opts = fc ? (fc[facet] || []) : [];
            const selected = new Set(this.facets[facet].map(v => v.toLowerCase()));
            // A selected value the current result no longer has stays listed.
            const extra = this.facets[facet].filter(v => !opts.some(o => o.value.toLowerCase() === v.toLowerCase()))
                .map(v => ({ value: v, label: this.facetLabel(facet, v), count: 0 }));
            const all = opts.concat(extra);
            if (!all.length) return '';
            const filter = all.length > 12
                ? `<input type="search" class="browse-facet-filter" data-facet="${facet}" placeholder="Filter ${title.toLowerCase()}…" aria-label="Filter ${title}">` : '';
            return `
                <details class="browse-facet" ${selected.size || facet !== 'flags' ? 'open' : ''}>
                    <summary>${this.escapeHtml(title)}${selected.size ? ` (${selected.size})` : ''}</summary>
                    ${filter}
                    <div class="browse-facet-options">
                        ${all.map(o => `
                            <label class="browse-facet-option" data-label="${this.escapeAttr(o.label.toLowerCase())}">
                                <input type="checkbox" class="browse-facet-check" data-facet="${facet}" data-value="${this.escapeAttr(o.value)}"
                                       ${selected.has(o.value.toLowerCase()) ? 'checked' : ''}>
                                <span class="browse-facet-name">${this.escapeHtml(o.label)}</span>
                                <span class="browse-facet-count">${o.count.toLocaleString()}</span>
                            </label>`).join('')}
                    </div>
                </details>`;
        }).join('');
        const n = this.activeFacetCount();
        return `
            <details class="browse-rail" ${(this.railOpen ?? window.innerWidth > 760) ? 'open' : ''}>
                <summary class="browse-rail-toggle">Filters${n ? ` (${n})` : ''}</summary>
                ${sections || '<p class="empty-message">Search to see filters.</p>'}
            </details>`;
    }

    renderChips() {
        if (!this.activeFacetCount()) return '';
        const chips = [];
        for (const [facet, title] of BrowsePage.FACETS) {
            for (const v of this.facets[facet]) {
                chips.push(`<span class="browse-chip">${this.escapeHtml(title)}: ${this.escapeHtml(this.facetLabel(facet, v))}
                    <button class="browse-chip-remove" data-facet="${facet}" data-value="${this.escapeAttr(v)}" aria-label="Remove filter">&times;</button></span>`);
            }
        }
        return `<div class="browse-chips">${chips.join('')}<button class="btn btn-small btn-secondary" id="browse-reset-facets">Reset filters</button></div>`;
    }

    setFacet(facet, value, on) {
        const list = this.facets[facet].filter(v => v.toLowerCase() !== value.toLowerCase());
        if (on) list.push(value);
        this.facets[facet] = list;
        this.search(true);
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
        if (this.matchers.length === 0 && !this.showAll) {
            return `<p class="empty-message">Add a matcher above to start browsing, or check "Show all books".</p>`;
        }
        if (this.loading && this.results.length === 0) {
            return `<p class="empty-message">Searching…</p>`;
        }

        let html = `<p class="datamanager-summary">${this.total} book${this.total === 1 ? '' : 's'} matched.</p>`;
        if (this.results.length === 0) {
            return html;
        }

        const link = (c, inner) => `<a class="book-link" href="/books/${encodeURIComponent(c.id)}" data-route>${inner}</a>`;
        if (this.view === 'grid') {
            html += '<div class="comics-grid browse-grid">' + this.results.map(c => `
                <div class="comic-card${c.unread ? '' : ' comic-read'}">
                    <div class="comic-cover">
                        <div class="comic-placeholder">📖</div>
                        <img class="comic-cover-img" alt="" loading="lazy" src="/api/library/books/${encodeURIComponent(c.id)}/cover"
                             onload="this.classList.add('loaded'); this.previousElementSibling.style.display='none';"
                             onerror="this.style.display='none';">
                    </div>
                    <div class="comic-info">
                        <div class="comic-series">${link(c, this.escapeHtml(c.series))}</div>
                        <div class="comic-number">#${this.escapeHtml(c.number)}${c.year ? ` (${c.year})` : ''}</div>
                    </div>
                </div>`).join('') + '</div>';
        } else if (this.view === 'compact') {
            html += '<ul class="browse-compact">' + this.results.map(c =>
                `<li>${link(c, `${this.escapeHtml(c.series)}${c.number ? ' #' + this.escapeHtml(c.number) : ''}`)}${c.year ? ` <span class="browse-compact-meta">(${c.year})</span>` : ''}${c.publisher ? ` <span class="browse-compact-meta">${this.escapeHtml(c.publisher)}</span>` : ''}</li>`).join('') + '</ul>';
        }
        if (this.view === 'grid' || this.view === 'compact') {
            if (this.hasMore) html += `<div class="load-more-container"><button id="browse-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
            return html;
        }
        html += '<div class="browse-table-wrap"><table class="datamanager-diff-table"><thead><tr><th>Series</th><th>Number</th><th>Title</th><th>Publisher</th><th>Year</th><th>Pages</th></tr></thead><tbody>';
        for (const c of this.results) {
            html += `<tr><td><a class="book-link" href="/books/${encodeURIComponent(c.id)}" data-route>${this.escapeHtml(c.series)}</a></td><td>${this.escapeHtml(c.number)}</td><td>${this.escapeHtml(c.title)}</td><td>${this.escapeHtml(c.publisher)}</td><td>${c.year || ''}</td><td>${c.page_count || ''}</td></tr>`;
        }
        html += '</tbody></table></div>';

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

        const showAllCheck = document.getElementById('browse-show-all');
        if (showAllCheck) showAllCheck.addEventListener('change', () => {
            this.showAll = showAllCheck.checked;
            this.search(true);
        });

        const queryInput = document.getElementById('browse-query');
        if (queryInput) queryInput.addEventListener('input', () => {
            this.query = queryInput.value;
            this.debouncedSearch();
        });
        const sortSel = document.getElementById('browse-sort');
        if (sortSel) sortSel.addEventListener('change', () => {
            this.sortField = sortSel.value;
            this.search(true);
        });
        const sortDesc = document.getElementById('browse-sort-desc');
        if (sortDesc) sortDesc.addEventListener('change', () => {
            this.sortDesc = sortDesc.checked;
            this.search(true);
        });

        document.querySelectorAll('.browse-facet-check').forEach(el => el.addEventListener('change', () =>
            this.setFacet(el.dataset.facet, el.dataset.value, el.checked)));
        document.querySelectorAll('.browse-chip-remove').forEach(el => el.addEventListener('click', () =>
            this.setFacet(el.dataset.facet, el.dataset.value, false)));
        const resetFacets = document.getElementById('browse-reset-facets');
        if (resetFacets) resetFacets.addEventListener('click', () => {
            this.facets = { publishers: [], formats: [], stages: [], flags: [] };
            this.search(true);
        });
        // Typing in a facet's filter box hides options without a re-render.
        document.querySelectorAll('.browse-facet-filter').forEach(el => el.addEventListener('input', () => {
            const q = el.value.trim().toLowerCase();
            el.parentElement.querySelectorAll('.browse-facet-option').forEach(opt => {
                opt.style.display = !q || opt.dataset.label.includes(q) ? '' : 'none';
            });
        }));
        const rail = document.querySelector('.browse-rail');
        if (rail) rail.addEventListener('toggle', () => { this.railOpen = rail.open; });
        document.querySelectorAll('.browse-view-btn').forEach(el => el.addEventListener('click', () => {
            this.view = el.dataset.view;
            try { localStorage.setItem('browse-view', this.view); } catch (e) { /* storage blocked */ }
            this.render();
            this.attachListeners();
        }));

        const addBtn = document.getElementById('browse-add-matcher-btn');
        if (addBtn) addBtn.addEventListener('click', () => this.addMatcher());

        const saveBtn = document.getElementById('browse-save-as-list-btn');
        if (saveBtn) saveBtn.addEventListener('click', () => this.saveAsSmartList());

        const loadMoreBtn = document.getElementById('browse-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.search(false));

        this.attachMatcherListeners();
        this.attachDMListeners();
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
        this.writeUrl();
        if (this.matchers.length === 0 && !this.showAll) {
            this.results = [];
            this.total = 0;
            this.facetCounts = null;
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
                body: JSON.stringify({ matcher_mode: this.matcherMode, matchers: this.matchers, show_all: this.showAll, query: this.query, sort: this.sortField, sort_desc: this.sortDesc, facets: this.facets }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to browse library'));
            }
            const result = JSON.parse(text);
            if (result.facets) this.facetCounts = result.facets;
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

    // saveAsSmartList persists the current in-progress filter
    // (this.matcherMode/this.matchers) as a real, saved smart list -
    // bridges Browse's throwaway exploration into the existing smart-list
    // CRUD (listDetail.js/listsBrowser.js) rather than duplicating it
    // (comic-server-hil). The matcher objects are already wire-compatible
    // with a saved list's (see this file's own header comment), so no
    // transformation is needed beyond wrapping them with a name.
    async saveAsSmartList() {
        if (this.matchers.length === 0) return;

        const name = await dialogs.prompt({ title: 'Save as Smart List', placeholder: 'e.g. Currently Reading' });
        if (!name) return;

        try {
            const response = await fetch('/api/library/lists', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: name,
                    type: 'ComicSmartListItem',
                    matcher_mode: this.matcherMode,
                    matchers: this.matchers,
                }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to save smart list'));
            }
            const created = JSON.parse(text);
            router.navigate(`/lists/${created.id}`);
        } catch (error) {
            console.error('Failed to save smart list:', error);
            dialogs.toast('Failed to save smart list: ' + error.message, 'error');
        }
    }

    // --- Data Manager section (comic-server-w7ig) ---
    // Ported from dataManagerPage.js's own whole-library preview/apply
    // job logic, pointed at the Browse-scoped endpoints instead - see
    // this file's header comment for why both still share one job slot.

    renderDataManagerPanel() {
        const scopeLabel = this.showAll ? 'the whole library' : `the ${this.matchers.length} matcher${this.matchers.length === 1 ? '' : 's'} above`;
        return `
            <div class="panel datamanager-section">
                <div class="datamanager-page-header">
                    <h2>Data Manager</h2>
                    <p class="empty-message">Runs every enabled Data Manager rule against ${this.matchers.length === 0 && !this.showAll ? 'nothing yet - add a matcher or check "Show all books" first' : scopeLabel}.</p>
                </div>
                <div class="datamanager-actions">
                    <button class="btn btn-primary" id="dm-preview-btn" ${this.dmJobKind || (this.matchers.length === 0 && !this.showAll) ? 'disabled' : ''}>
                        ${this.dmJobKind === 'preview' ? 'Previewing…' : 'Preview'}
                    </button>
                    <button class="btn btn-primary" id="dm-apply-selected-btn" ${this.dmApplyDisabled() ? 'disabled' : ''}>
                        ${this.dmJobKind === 'apply' ? 'Applying…' : `Apply Selected (${this.dmSelected.size})`}
                    </button>
                    <button class="btn btn-secondary" id="dm-select-all-btn" ${!this.dmBooks.length ? 'disabled' : ''}>Select All Shown</button>
                    <button class="btn btn-secondary" id="dm-select-none-btn" ${!this.dmBooks.length ? 'disabled' : ''}>Select None</button>
                </div>
                ${this.renderDMBody()}
            </div>
        `;
    }

    dmApplyDisabled() {
        return !!this.dmJobKind || this.dmSelected.size === 0;
    }

    dmFieldKey(bookId, field, custom) {
        return `${bookId} ${field} ${custom}`;
    }

    renderDMBody() {
        if (this.dmJobKind) {
            return this.renderDMProgress();
        }
        if (this.dmError) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.dmError)}</p>`;
        }
        if (!this.dmSummary) {
            return '';
        }

        let html = `<p class="datamanager-summary">${this.dmSummary.changed} of ${this.dmSummary.processed} books would change.</p>`;
        if (this.dmLastResult) {
            html += `<p>${this.dmLastResult.applied ? 'Applied' : 'Previewed'}: ${this.dmLastResult.changed} book${this.dmLastResult.changed === 1 ? '' : 's'} committed.</p>`;
            if (this.dmLastResult.errors && this.dmLastResult.errors.length > 0) {
                html += `<p class="datamanager-errors">Errors: ${this.escapeHtml(this.dmLastResult.errors.join('; '))}</p>`;
            }
        }

        if (this.dmBooks.length === 0) {
            return html;
        }

        html += '<table class="datamanager-diff-table"><thead><tr><th></th><th>Book</th><th>Field</th><th>Old</th><th>New</th></tr></thead><tbody>';
        for (const book of this.dmBooks) {
            const label = `${book.series}${book.number ? ' #' + book.number : ''}${book.title ? ' - ' + book.title : ''}`;
            book.changes.forEach((c, i) => {
                const key = this.dmFieldKey(book.book_id, c.field, c.custom);
                const checked = this.dmSelected.has(key);
                const fieldLabel = c.custom ? `${c.field} (custom)` : c.field;
                html += '<tr>';
                html += `<td><input type="checkbox" class="dm-field-check" data-book-id="${this.escapeAttr(book.book_id)}" data-field="${this.escapeAttr(c.field)}" data-custom="${c.custom}" ${checked ? 'checked' : ''}></td>`;
                if (i === 0) {
                    html += `<td rowspan="${book.changes.length}">${this.escapeHtml(label)}</td>`;
                }
                html += `<td>${this.escapeHtml(fieldLabel)}</td><td>${this.escapeHtml(c.old)}</td><td>${this.escapeHtml(c.new)}</td>`;
                html += '</tr>';
            });
        }
        html += '</tbody></table>';

        if (this.dmHasMore) {
            html += `<div class="load-more-container"><button id="dm-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
        }

        return html;
    }

    renderDMProgress() {
        const pct = this.dmJobTotal > 0 ? Math.round((this.dmJobProcessed / this.dmJobTotal) * 100) : 0;
        const verb = this.dmJobKind === 'apply' ? 'Applying' : 'Evaluating';
        return `
            <div class="datamanager-progress">
                <p>${verb} rules… ${this.dmJobProcessed} / ${this.dmJobTotal} books (${pct}%)</p>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${pct}%"></div>
                </div>
            </div>
        `;
    }

    attachDMListeners() {
        const previewBtn = document.getElementById('dm-preview-btn');
        if (previewBtn) previewBtn.addEventListener('click', () => this.dmPreview(true));

        const loadMoreBtn = document.getElementById('dm-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.dmPreview(false));

        const applyBtn = document.getElementById('dm-apply-selected-btn');
        if (applyBtn) applyBtn.addEventListener('click', () => this.dmApplySelected());

        const selectAllBtn = document.getElementById('dm-select-all-btn');
        if (selectAllBtn) {
            selectAllBtn.addEventListener('click', () => {
                this.dmForEachFieldRow((bookId, field, custom, key) => {
                    this.dmSelected.set(key, { book_id: bookId, field, custom });
                });
                this.render();
                this.attachListeners();
            });
        }
        const selectNoneBtn = document.getElementById('dm-select-none-btn');
        if (selectNoneBtn) {
            selectNoneBtn.addEventListener('click', () => {
                this.dmForEachFieldRow((bookId, field, custom, key) => {
                    this.dmSelected.delete(key);
                });
                this.render();
                this.attachListeners();
            });
        }

        document.querySelectorAll('.dm-field-check').forEach(el => {
            el.addEventListener('change', () => {
                const bookId = el.dataset.bookId;
                const field = el.dataset.field;
                const custom = el.dataset.custom === 'true';
                const key = this.dmFieldKey(bookId, field, custom);
                if (el.checked) {
                    this.dmSelected.set(key, { book_id: bookId, field, custom });
                } else {
                    this.dmSelected.delete(key);
                }
                const applyBtn = document.getElementById('dm-apply-selected-btn');
                if (applyBtn) {
                    applyBtn.textContent = `Apply Selected (${this.dmSelected.size})`;
                    applyBtn.disabled = this.dmApplyDisabled();
                }
            });
        });
    }

    dmForEachFieldRow(fn) {
        for (const book of this.dmBooks) {
            for (const c of book.changes) {
                const key = this.dmFieldKey(book.book_id, c.field, c.custom);
                fn(book.book_id, c.field, c.custom, key);
            }
        }
    }

    // dmScopeBody is the matchers/show_all/query/facets scope every
    // Browse-scoped Data Manager request sends, so the run always matches
    // exactly what the browse table above is currently showing.
    dmScopeBody() {
        return { matcher_mode: this.matcherMode, matchers: this.matchers, show_all: this.showAll, query: this.query, facets: this.facets };
    }

    async dmPreview(reset) {
        if (!reset) {
            await this.dmFetchPage(this.dmOffset);
            return;
        }
        if (this.matchers.length === 0 && !this.showAll) return;

        this.dmError = null;
        this.dmOffset = 0;
        this.dmBooks = [];
        this.dmSelected = new Map();
        this.dmLastResult = null;
        this.dmSummary = null;

        try {
            const response = await fetch('/api/library/browse/datamanager-preview', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this.dmScopeBody()),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to start Data Manager preview'));
            }
        } catch (error) {
            console.error('Failed to start Data Manager preview:', error);
            this.dmError = `Failed: ${error.message}`;
            this.render();
            this.attachListeners();
            return;
        }

        this.dmRunJob('preview');
    }

    async dmApplySelected() {
        const ok = await dialogs.confirm({
            title: 'Apply Data Manager Rules',
            message: `Apply the selected ${this.dmSelected.size} field change(s) now? This cannot be undone from this page.`,
            confirmLabel: 'Apply',
            danger: true,
        });
        if (!ok) return;

        this.dmError = null;
        try {
            const response = await fetch('/api/library/browse/datamanager-apply', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ...this.dmScopeBody(), fields: Array.from(this.dmSelected.values()) }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to start Data Manager apply'));
            }
        } catch (error) {
            console.error('Failed to start Data Manager apply:', error);
            this.dmError = `Failed: ${error.message}`;
            this.render();
            this.attachListeners();
            return;
        }

        this.dmRunJob('apply');
    }

    dmRunJob(kind) {
        this.dmJobKind = kind;
        this.dmJobTotal = 0;
        this.dmJobProcessed = 0;
        this.dmPollCtx = (typeof router !== 'undefined') ? router._navCtx : null;
        this.render();
        this.attachListeners();

        this.dmStopPolling();
        this.dmPollJob();
        this.dmPollTimer = setInterval(() => this.dmPollJob(), 1000);
    }

    dmStopPolling() {
        if (this.dmPollTimer) {
            clearInterval(this.dmPollTimer);
            this.dmPollTimer = null;
        }
    }

    async dmPollJob() {
        if (this.dmPollCtx && this.dmPollCtx.aborted) {
            this.dmStopPolling();
            return;
        }
        let status;
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.dmLimit}&offset=0`);
            if (this.dmPollCtx && this.dmPollCtx.aborted) {
                this.dmStopPolling();
                return;
            }
            if (!response.ok) return;
            status = await response.json();
        } catch (error) {
            console.error('Failed to poll Data Manager job status:', error);
            return;
        }
        this.dmHandleJobStatus(status);
    }

    // resumeDMJobIfAny picks a still-running (or just-finished) job back
    // up on page load/return, same as dataManagerPage.js's own version -
    // covers both a plain SPA navigation away-and-back and a hard
    // browser refresh mid-run.
    async resumeDMJobIfAny(ctx) {
        let status;
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.dmLimit}&offset=0`);
            if (ctx && ctx.aborted) return;
            if (!response.ok) return;
            status = await response.json();
        } catch (error) {
            console.error('Failed to check for an in-progress Data Manager job:', error);
            return;
        }
        if (status.status === 'running') {
            this.dmJobKind = status.apply ? 'apply' : 'preview';
            this.dmPollCtx = (typeof router !== 'undefined') ? router._navCtx : null;
            this.dmStopPolling();
            this.dmPollJob();
            this.dmPollTimer = setInterval(() => this.dmPollJob(), 1000);
        } else if (status.status === 'completed') {
            this.dmHandleJobStatus(status);
        }
    }

    dmHandleJobStatus(status) {
        if (status.status === 'none') {
            this.dmStopPolling();
            this.dmJobKind = null;
            this.render();
            this.attachListeners();
            return;
        }

        this.dmJobKind = status.apply ? 'apply' : 'preview';
        this.dmJobTotal = status.total || 0;
        this.dmJobProcessed = status.processed || 0;

        if (status.status !== 'completed') {
            this.render();
            this.attachListeners();
            return;
        }

        this.dmStopPolling();
        this.dmJobKind = null;

        if (status.job_id && status.job_id === this.dmLastHandledJobId) {
            this.render();
            this.attachListeners();
            return;
        }
        this.dmLastHandledJobId = status.job_id;

        if (status.apply) {
            this.dmLastResult = { applied: true, changed: status.changed, errors: status.errors };
            this.dmRemoveAppliedChanges(status.books || []);
            this.render();
            this.attachListeners();
            return;
        }

        this.dmSummary = { processed: status.total, changed: status.changed };
        this.dmHasMore = !!status.has_more;
        const page = status.books || [];
        this.dmBooks = page;
        this.dmOffset = this.dmLimit;
        for (const book of page) {
            for (const c of book.changes) {
                const key = this.dmFieldKey(book.book_id, c.field, c.custom);
                this.dmSelected.set(key, { book_id: book.book_id, field: c.field, custom: c.custom });
            }
        }

        this.render();
        this.attachListeners();
    }

    dmRemoveAppliedChanges(appliedBooks) {
        let resolvedCount = 0;
        for (const applied of appliedBooks) {
            const bookEntry = this.dmBooks.find(b => b.book_id === applied.book_id);
            if (!bookEntry) continue;
            const appliedKeys = new Set(applied.changes.map(c => `${c.field} ${c.custom}`));
            bookEntry.changes = bookEntry.changes.filter(c => !appliedKeys.has(`${c.field} ${c.custom}`));
            for (const c of applied.changes) {
                this.dmSelected.delete(this.dmFieldKey(applied.book_id, c.field, c.custom));
            }
            if (bookEntry.changes.length === 0) {
                resolvedCount++;
            }
        }
        this.dmBooks = this.dmBooks.filter(b => b.changes.length > 0);
        if (this.dmSummary) {
            this.dmSummary.changed = Math.max(0, this.dmSummary.changed - resolvedCount);
        }
    }

    async dmFetchPage(offset) {
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.dmLimit}&offset=${offset}`);
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to load more results'));
            }
            const status = JSON.parse(text);
            const page = status.books || [];
            this.dmBooks = this.dmBooks.concat(page);
            this.dmHasMore = !!status.has_more;
            this.dmOffset = offset + this.dmLimit;
            for (const book of page) {
                for (const c of book.changes) {
                    const key = this.dmFieldKey(book.book_id, c.field, c.custom);
                    this.dmSelected.set(key, { book_id: book.book_id, field: c.field, custom: c.custom });
                }
            }
        } catch (error) {
            console.error('Failed to load more Data Manager results:', error);
            this.dmError = `Failed: ${error.message}`;
        } finally {
            this.render();
            this.attachListeners();
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text === undefined || text === null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }
}

window.BrowsePage = BrowsePage;
