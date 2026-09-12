// Data Manager Page - whole-library "Apply All" + selective apply
// (comic-server-dpq, refined to field-level in comic-server-z93).
// Complements the per-list Data Manager tab on the list detail page
// (comic-server-764.6): that one scopes a run to one smart list's matched
// books; this one scopes to the ENTIRE library, since the real
// dataman.dat's rules (series-grouping, tagging) are meant to apply
// globally, not just to whatever smart list happens to exist.
//
// Summary-first design: Preview shows a total count immediately (cheap to
// read even for a 66K-book library) and a paginated diff table underneath
// for drill-in, rather than trying to load every changed book's diff at
// once. Selective apply is per FIELD CHANGE, not per book - each row in
// the diff table (one book's one changed field) has its own checkbox, so
// "keep the SeriesGroup change but skip the Tags change on the same book"
// is possible, not just whole-book cherry-picking.
//
// Preview/Apply both run as a server-side background job, not a blocking
// request: evaluating every rule against every book in a real ~66K-book
// library takes long enough to trip a reverse proxy's own gateway timeout
// (a real user hit exactly this - a raw 504 HTML page landing in the
// error banner) well before the server itself would ever answer. This
// page starts the job, then polls GET /api/library/datamanager-job every
// second for a live processed/total count until it completes.
class DataManagerPage {
    constructor() {
        this.summary = null; // { processed, changed } from the last completed job
        this.books = [];     // current page of DMBookChange
        this.limit = 20;
        this.offset = 0;
        this.hasMore = false;
        // selected keys are `${book_id} ${field} ${custom}` -
        // one entry per checked FIELD ROW, not per book, so a book with
        // multiple changed fields can have some checked and some not.
        this.selected = new Map(); // key -> { book_id, field, custom }
        this.lastResult = null; // last apply result, shown until the next preview
        this.error = null;

        // Job/progress state - jobKind is 'preview' or 'apply' while a
        // job is running, null otherwise; jobTotal/jobProcessed drive the
        // progress bar.
        this.jobKind = null;
        this.jobTotal = 0;
        this.jobProcessed = 0;
        this.pollTimer = null;
        this.pollCtx = null; // captured router._navCtx, see stopPolling
        this.lastHandledJobId = null; // see handleJobStatus's completed branch
    }

    async init(ctx) {
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
        await this.resumeJobIfAny(ctx);
    }

    // resumeJobIfAny checks the server for whatever the current whole-
    // library job actually is and picks progress display back up if it's
    // still running (or shows the result immediately if it finished while
    // the user was away) - covers both a plain SPA navigation away and
    // back (this page instance survives that, but its polling interval
    // was stopped the moment the router context it captured got marked
    // aborted) and a hard browser refresh (a brand new instance with no
    // memory of ever having started a job at all). Without this, the job
    // keeps running server-side exactly as before, but the tab has
    // nothing telling it that, or to keep checking.
    async resumeJobIfAny(ctx) {
        let status;
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.limit}&offset=0`);
            if (ctx && ctx.aborted) return;
            if (!response.ok) return;
            status = await response.json();
        } catch (error) {
            console.error('Failed to check Data Manager job status:', error);
            return;
        }
        this.pollCtx = ctx;
        const wasRunning = status.status === 'running';
        this.handleJobStatus(status);
        if (wasRunning) {
            this.stopPolling();
            this.pollTimer = setInterval(() => this.pollJob(), 1000);
        }
    }

    fieldKey(bookId, field, custom) {
        return `${bookId} ${field} ${custom}`;
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Data Manager</h1>
                    <p class="empty-message">Runs every enabled Data Manager rule against the WHOLE library, not just one smart list. Manage rules directly with the button below, or import from a ComicRack <code>dataman.dat</code> file via <code>comic-server datamanager import</code>.</p>
                    <button class="btn btn-secondary" id="dm-manage-rules-btn">Manage Rules</button>
                </div>
                <div class="panel">
                    <div class="datamanager-actions">
                        <button class="btn btn-primary" id="dm-preview-btn" ${this.jobKind ? 'disabled' : ''}>
                            ${this.jobKind === 'preview' ? 'Previewing…' : 'Preview All'}
                        </button>
                        <button class="btn btn-primary" id="dm-apply-selected-btn" ${this.applyDisabled() ? 'disabled' : ''}>
                            ${this.jobKind === 'apply' ? 'Applying…' : `Apply Selected (${this.selected.size})`}
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
        return !!this.jobKind || this.selected.size === 0;
    }

    renderBody() {
        if (this.jobKind) {
            return this.renderProgress();
        }
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
            book.changes.forEach((c, i) => {
                const key = this.fieldKey(book.book_id, c.field, c.custom);
                const checked = this.selected.has(key);
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

        if (this.hasMore) {
            html += `<div class="load-more-container"><button id="dm-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
        }

        return html;
    }

    // renderProgress is shown in place of the diff table while a job is
    // running - a plain "xxx / xxx books" counter plus the shared
    // .progress-bar/.progress-fill classes the sync page already uses, so
    // this looks consistent with the rest of the app rather than
    // inventing new progress-bar styling.
    renderProgress() {
        const pct = this.jobTotal > 0 ? Math.round((this.jobProcessed / this.jobTotal) * 100) : 0;
        const verb = this.jobKind === 'apply' ? 'Applying' : 'Evaluating';
        return `
            <div class="datamanager-progress">
                <p>${verb} rules… ${this.jobProcessed} / ${this.jobTotal} books (${pct}%)</p>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${pct}%"></div>
                </div>
            </div>
        `;
    }

    attachListeners() {
        const manageRulesBtn = document.getElementById('dm-manage-rules-btn');
        if (manageRulesBtn) manageRulesBtn.addEventListener('click', () => router.navigate('/datamanager/rules'));

        const previewBtn = document.getElementById('dm-preview-btn');
        if (previewBtn) previewBtn.addEventListener('click', () => this.preview(true));

        const loadMoreBtn = document.getElementById('dm-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.preview(false));

        const applyBtn = document.getElementById('dm-apply-selected-btn');
        if (applyBtn) applyBtn.addEventListener('click', () => this.applySelected());

        const selectAllBtn = document.getElementById('dm-select-all-btn');
        if (selectAllBtn) {
            selectAllBtn.addEventListener('click', () => {
                this.forEachFieldRow((bookId, field, custom, key) => {
                    this.selected.set(key, { book_id: bookId, field, custom });
                });
                this.render();
                this.attachListeners();
            });
        }
        const selectNoneBtn = document.getElementById('dm-select-none-btn');
        if (selectNoneBtn) {
            selectNoneBtn.addEventListener('click', () => {
                this.forEachFieldRow((bookId, field, custom, key) => {
                    this.selected.delete(key);
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
                const key = this.fieldKey(bookId, field, custom);
                if (el.checked) {
                    this.selected.set(key, { book_id: bookId, field, custom });
                } else {
                    this.selected.delete(key);
                }
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

    forEachFieldRow(fn) {
        for (const book of this.books) {
            for (const c of book.changes) {
                const key = this.fieldKey(book.book_id, c.field, c.custom);
                fn(book.book_id, c.field, c.custom, key);
            }
        }
    }

    // reset=true starts a FRESH background preview job (used by the
    // Preview button); reset=false just fetches the next page of an
    // already-completed job's stored results (Load More) - no
    // re-evaluation needed, the server keeps the full result until the
    // next job starts.
    async preview(reset) {
        if (!reset) {
            await this.fetchPage(this.offset);
            return;
        }

        this.error = null;
        this.offset = 0;
        this.books = [];
        this.selected = new Map();
        this.lastResult = null;
        this.summary = null;

        try {
            const response = await fetch('/api/library/datamanager-preview', { method: 'POST' });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to start Data Manager preview'));
            }
        } catch (error) {
            console.error('Failed to start Data Manager preview:', error);
            this.error = `Failed: ${error.message}`;
            this.render();
            this.attachListeners();
            return;
        }

        this.runJob('preview');
    }

    async applySelected() {
        const ok = await dialogs.confirm({
            title: 'Apply Data Manager Rules',
            message: `Apply the selected ${this.selected.size} field change(s) now? This cannot be undone from this page.`,
            confirmLabel: 'Apply',
            danger: true,
        });
        if (!ok) return;

        this.error = null;
        try {
            const response = await fetch('/api/library/datamanager-apply', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ fields: Array.from(this.selected.values()) }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to start Data Manager apply'));
            }
        } catch (error) {
            console.error('Failed to start Data Manager apply:', error);
            this.error = `Failed: ${error.message}`;
            this.render();
            this.attachListeners();
            return;
        }

        this.runJob('apply');
    }

    // runJob begins polling for the job that was just started (kind is
    // 'preview' or 'apply', purely for this page's own display - both
    // poll the exact same status endpoint, since only one job runs at a
    // time on the server). pollCtx is captured from the router's current
    // navigation context so a stray tick after the user has navigated
    // away never overwrites whatever page they're actually looking at -
    // same pattern deviceDetail.js's sync-status polling uses, adapted
    // for a page instance that (unlike DeviceDetail) is a session-long
    // singleton reused across navigations rather than recreated per visit.
    runJob(kind) {
        this.jobKind = kind;
        this.jobTotal = 0;
        this.jobProcessed = 0;
        this.pollCtx = (typeof router !== 'undefined') ? router._navCtx : null;
        this.render();
        this.attachListeners();

        this.stopPolling();
        this.pollJob();
        this.pollTimer = setInterval(() => this.pollJob(), 1000);
    }

    stopPolling() {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = null;
        }
    }

    async pollJob() {
        if (this.pollCtx && this.pollCtx.aborted) {
            this.stopPolling();
            return;
        }

        let status;
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.limit}&offset=0`);
            if (this.pollCtx && this.pollCtx.aborted) {
                this.stopPolling();
                return;
            }
            if (!response.ok) return; // transient - next tick will retry
            status = await response.json();
        } catch (error) {
            console.error('Failed to poll Data Manager job status:', error);
            return; // transient - next tick will retry
        }

        this.handleJobStatus(status);
    }

    // handleJobStatus applies one DMJobStatus reading, shared by the
    // regular poll loop and resumeJobIfAny's one-shot check on page
    // load/return - status.apply (the server's own record of what kind of
    // run this is) drives the post-completion branch rather than this
    // page's own this.jobKind, since a resumed job's this.jobKind may
    // still be whatever it was left at (a stale SPA-navigation-away
    // instance) or simply null (a fresh instance after a hard refresh).
    handleJobStatus(status) {
        if (status.status === 'none') {
            this.stopPolling();
            this.jobKind = null;
            this.render();
            this.attachListeners();
            return;
        }

        this.jobKind = status.apply ? 'apply' : 'preview';
        this.jobTotal = status.total || 0;
        this.jobProcessed = status.processed || 0;

        if (status.status !== 'completed') {
            this.render();
            this.attachListeners();
            return;
        }

        this.stopPolling();
        this.jobKind = null;

        // A completed job's status stays "completed" server-side until
        // the next one starts, so a plain page revisit (or
        // resumeJobIfAny catching up on load) would otherwise see the
        // same job every time and re-apply the effects below repeatedly.
        // Only react to a given job_id once.
        if (status.job_id && status.job_id === this.lastHandledJobId) {
            this.render();
            this.attachListeners();
            return;
        }
        this.lastHandledJobId = status.job_id;

        if (status.apply) {
            // An apply job's own Books IS exactly what got committed
            // (comic-server-c9v) - no need to re-run a whole fresh
            // preview just to find out the diff table should now be
            // smaller by exactly that much. That used to mean every
            // apply - even a 32-book selective one - triggered a THIRD
            // full-library evaluation pass just to refresh the table.
            this.lastResult = { applied: true, changed: status.changed, errors: status.errors };
            this.removeAppliedChanges(status.books || []);
            this.render();
            this.attachListeners();
            return;
        }

        this.summary = { processed: status.total, changed: status.changed };
        this.hasMore = !!status.has_more;
        const page = status.books || [];
        this.books = page;
        this.offset = this.limit;
        for (const book of page) {
            for (const c of book.changes) {
                const key = this.fieldKey(book.book_id, c.field, c.custom);
                this.selected.set(key, { book_id: book.book_id, field: c.field, custom: c.custom });
            }
        }

        this.render();
        this.attachListeners();
    }

    // removeAppliedChanges drops exactly the field-changes an apply job
    // just committed from the currently displayed diff table (and their
    // selection state) - a book with no changes left after that is
    // dropped entirely - and corrects the running "N would change" total
    // by the same amount, all without touching the server again.
    removeAppliedChanges(appliedBooks) {
        // "N books would change" only drops for a book that's now FULLY
        // resolved - a book selectively applied on just one of its two
        // changed fields still has a pending change and must stay
        // counted, not silently vanish from the total.
        let resolvedCount = 0;
        for (const applied of appliedBooks) {
            const bookEntry = this.books.find(b => b.book_id === applied.book_id);
            if (!bookEntry) continue;
            const appliedKeys = new Set(applied.changes.map(c => `${c.field} ${c.custom}`));
            bookEntry.changes = bookEntry.changes.filter(c => !appliedKeys.has(`${c.field} ${c.custom}`));
            for (const c of applied.changes) {
                this.selected.delete(this.fieldKey(applied.book_id, c.field, c.custom));
            }
            if (bookEntry.changes.length === 0) {
                resolvedCount++;
            }
        }
        this.books = this.books.filter(b => b.changes.length > 0);
        if (this.summary) {
            this.summary.changed = Math.max(0, this.summary.changed - resolvedCount);
        }
    }

    // fetchPage re-reads a later page of an already-completed job's
    // stored results (Load More) - cheap, no re-evaluation.
    async fetchPage(offset) {
        try {
            const response = await fetch(`/api/library/datamanager-job?limit=${this.limit}&offset=${offset}`);
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to load more results'));
            }
            const status = JSON.parse(text);
            const page = status.books || [];
            this.books = this.books.concat(page);
            this.hasMore = !!status.has_more;
            this.offset = offset + this.limit;
            for (const book of page) {
                for (const c of book.changes) {
                    const key = this.fieldKey(book.book_id, c.field, c.custom);
                    this.selected.set(key, { book_id: book.book_id, field: c.field, custom: c.custom });
                }
            }
        } catch (error) {
            console.error('Failed to load more Data Manager results:', error);
            this.error = `Failed: ${error.message}`;
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

window.DataManagerPage = DataManagerPage;
