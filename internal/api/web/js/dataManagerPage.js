// Data Manager Page - whole-library "Apply All" + selective apply
// (comic-server-dpq). Complements the per-list Data Manager tab on the
// list detail page (comic-server-764.6): that one scopes a run to one
// smart list's matched books; this one scopes to the ENTIRE library,
// since the real dataman.dat's rules (series-grouping, tagging) are meant
// to apply globally, not just to whatever smart list happens to exist.
//
// Summary-first design: Preview shows a total count immediately (cheap to
// read even for a 66K-book library) and a paginated diff table underneath
// for drill-in, rather than trying to load every changed book's diff at
// once. Selective apply is per-book only for now (comic-server-z93 tracks
// per-field-change as a future refinement) - each book in the diff table
// has a checkbox, checked by default, and only checked books get
// committed.
class DataManagerPage {
    constructor() {
        this.summary = null; // { processed, changed } from the last preview
        this.books = [];     // current page of DMBookChange
        this.limit = 20;
        this.offset = 0;
        this.hasMore = false;
        this.selected = new Set(); // book_ids currently checked
        this.previewing = false;
        this.applying = false;
        this.lastResult = null; // last apply result, shown until the next preview
        this.error = null;
    }

    async init(ctx) {
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Data Manager</h1>
                    <p class="empty-message">Runs every enabled Data Manager rule (comic-server-764) against the WHOLE library, not just one smart list. Import rules first via <code>comic-server datamanager import</code> if none are configured yet.</p>
                </div>
                <div class="panel">
                    <div class="datamanager-actions">
                        <button class="btn btn-primary" id="dm-preview-btn" ${this.previewing ? 'disabled' : ''}>
                            ${this.previewing ? 'Previewing…' : 'Preview All'}
                        </button>
                        <button class="btn btn-primary" id="dm-apply-selected-btn" ${this.applyDisabled() ? 'disabled' : ''}>
                            Apply Selected (${this.selected.size})
                        </button>
                        <button class="btn btn-secondary" id="dm-select-all-btn" ${!this.books.length ? 'disabled' : ''}>Select All Shown</button>
                        <button class="btn btn-secondary" id="dm-select-none-btn" ${!this.books.length ? 'disabled' : ''}>Select None</button>
                    </div>
                    ${this.renderBody()}
                </div>
            </div>
        `;
    }

    applyDisabled() {
        return this.applying || this.selected.size === 0;
    }

    renderBody() {
        if (this.error) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.error)}</p>`;
        }
        if (!this.summary) {
            return '';
        }

        let html = `<p class="datamanager-summary">${this.summary.changed} of ${this.summary.processed} books would change.</p>`;
        if (this.lastResult) {
            html += `<p>${this.lastResult.applied ? 'Applied' : 'Previewed'}: ${this.lastResult.changed} book${this.lastResult.changed === 1 ? '' : 's'} committed.</p>`;
            if (this.lastResult.errors && this.lastResult.errors.length > 0) {
                html += `<p class="datamanager-errors">Errors: ${this.escapeHtml(this.lastResult.errors.join('; '))}</p>`;
            }
        }

        if (this.books.length === 0) {
            return html;
        }

        html += '<table class="datamanager-diff-table"><thead><tr><th></th><th>Book</th><th>Field</th><th>Old</th><th>New</th></tr></thead><tbody>';
        for (const book of this.books) {
            const label = `${book.series}${book.number ? ' #' + book.number : ''}${book.title ? ' - ' + book.title : ''}`;
            const checked = this.selected.has(book.book_id);
            book.changes.forEach((c, i) => {
                const fieldLabel = c.custom ? `${c.field} (custom)` : c.field;
                html += '<tr>';
                if (i === 0) {
                    html += `<td rowspan="${book.changes.length}"><input type="checkbox" class="dm-book-check" data-book-id="${book.book_id}" ${checked ? 'checked' : ''}></td>`;
                    html += `<td rowspan="${book.changes.length}">${this.escapeHtml(label)}</td>`;
                }
                html += `<td>${this.escapeHtml(fieldLabel)}</td><td>${this.escapeHtml(c.old)}</td><td>${this.escapeHtml(c.new)}</td>`;
                html += '</tr>';
            });
        }
        html += '</tbody></table>';

        if (this.hasMore) {
            html += `<div class="load-more-container"><button id="dm-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
        }

        return html;
    }

    attachListeners() {
        const previewBtn = document.getElementById('dm-preview-btn');
        if (previewBtn) previewBtn.addEventListener('click', () => this.preview(true));

        const loadMoreBtn = document.getElementById('dm-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.preview(false));

        const applyBtn = document.getElementById('dm-apply-selected-btn');
        if (applyBtn) applyBtn.addEventListener('click', () => this.applySelected());

        const selectAllBtn = document.getElementById('dm-select-all-btn');
        if (selectAllBtn) {
            selectAllBtn.addEventListener('click', () => {
                this.books.forEach(b => this.selected.add(b.book_id));
                this.render();
                this.attachListeners();
            });
        }
        const selectNoneBtn = document.getElementById('dm-select-none-btn');
        if (selectNoneBtn) {
            selectNoneBtn.addEventListener('click', () => {
                this.books.forEach(b => this.selected.delete(b.book_id));
                this.render();
                this.attachListeners();
            });
        }

        document.querySelectorAll('.dm-book-check').forEach(el => {
            el.addEventListener('change', () => {
                const id = el.dataset.bookId;
                if (el.checked) this.selected.add(id);
                else this.selected.delete(id);
                // Only the Apply button's label/disabled state needs to
                // change here, not a full re-render - avoids losing
                // scroll position while checking boxes in a long table.
                const applyBtn = document.getElementById('dm-apply-selected-btn');
                if (applyBtn) {
                    applyBtn.textContent = `Apply Selected (${this.selected.size})`;
                    applyBtn.disabled = this.applyDisabled();
                }
            });
        });
    }

    // reset=true starts a fresh preview from offset 0 (used by the
    // Preview button); reset=false appends the next page (Load More).
    async preview(reset) {
        this.previewing = true;
        this.error = null;
        if (reset) {
            this.offset = 0;
            this.books = [];
            this.selected = new Set();
            this.lastResult = null;
        }
        this.render();
        this.attachListeners();

        try {
            const url = `/api/library/datamanager-preview?limit=${this.limit}&offset=${this.offset}`;
            const response = await fetch(url, { method: 'POST' });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(text || 'Failed to preview Data Manager rules');
            }
            const result = JSON.parse(text);
            this.summary = { processed: result.processed, changed: result.changed };
            this.hasMore = !!result.has_more;
            const page = result.books || [];
            this.books = reset ? page : this.books.concat(page);
            // New books default to selected, matching "Apply Selected"
            // being the primary action for a freshly loaded page.
            page.forEach(b => this.selected.add(b.book_id));
            this.offset += this.limit;
        } catch (error) {
            console.error('Failed to preview Data Manager rules:', error);
            this.error = `Failed: ${error.message}`;
        } finally {
            this.previewing = false;
            this.render();
            this.attachListeners();
        }
    }

    async applySelected() {
        const ok = await dialogs.confirm({
            title: 'Apply Data Manager Rules',
            message: `Apply the selected changes to ${this.selected.size} book(s) now? This cannot be undone from this page.`,
            confirmLabel: 'Apply',
            danger: true,
        });
        if (!ok) return;

        this.applying = true;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch('/api/library/datamanager-apply', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_ids: Array.from(this.selected) }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(text || 'Failed to apply Data Manager rules');
            }
            this.lastResult = JSON.parse(text);
            // Refresh from offset 0 so the table reflects post-apply
            // state (committed books should no longer show a diff).
            await this.preview(true);
            return;
        } catch (error) {
            console.error('Failed to apply Data Manager rules:', error);
            this.error = `Failed: ${error.message}`;
        } finally {
            this.applying = false;
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

window.DataManagerPage = DataManagerPage;
