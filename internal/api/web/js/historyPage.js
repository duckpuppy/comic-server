// History page - the audit log of what each pipeline action changed on each
// book, with stage rollback (comic-server-vqsj).
class HistoryPage {
    constructor() {
        this.entries = [];
        this.total = 0;
        this.offset = 0;
        this.limit = 50;
        this.bookId = '';
        this.action = '';
        this.loading = true;
        this.error = '';
        this.message = '';
        this.undo = null; // {entry, items, force} while the undo preview is open
    }

    async init(ctx) {
        // ?book_id= (from the book detail page's "All history" link).
        const fromUrl = new URLSearchParams(window.location.search).get('book_id');
        if (fromUrl) {
            this.bookId = fromUrl;
            this.offset = 0;
        }
        this.loading = true;
        this.render();
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
    }

    async load() {
        const q = new URLSearchParams({ limit: this.limit, offset: this.offset });
        if (this.bookId) q.set('book_id', this.bookId);
        if (this.action) q.set('action', this.action);
        try {
            const res = await fetch(`/api/library/audit?${q}`);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.entries = data.entries || [];
            this.total = data.total || 0;
            this.error = '';
        } catch (e) {
            console.error('Failed to load history:', e);
            this.error = 'Failed to load history.';
            this.entries = [];
            this.total = 0;
        } finally {
            this.loading = false;
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }

    renderChanges(changes) {
        if (!changes || changes.length === 0) return '<span class="empty-message">stage only</span>';
        const shown = changes.slice(0, 6).map(c => `
            <li><span class="history-field">${this.escapeHtml(c.field)}</span>
                <span class="history-old">${this.escapeHtml(c.old) || '∅'}</span> →
                <span class="history-new">${this.escapeHtml(c.new) || '∅'}</span></li>`).join('');
        const more = changes.length > 6 ? `<li class="empty-message">+${changes.length - 6} more</li>` : '';
        return `<ul class="history-changes">${shown}${more}</ul>`;
    }

    renderRow(e) {
        const when = new Date(e.at);
        const stage = e.stage_before !== e.stage_after
            ? `${this.escapeHtml(e.stage_before || '—')} → ${this.escapeHtml(e.stage_after || '—')}`
            : this.escapeHtml(e.stage_after || '—');
        const canRollBack = e.stage_before && e.stage_before !== e.stage_after && e.action !== 'rollback';
        return `
            <tr>
                <td class="history-when" title="${this.escapeAttr(e.at)}">${this.escapeHtml(when.toLocaleString())}</td>
                <td><span class="history-action">${this.escapeHtml(e.action)}</span></td>
                <td><a href="#" class="history-book-link" data-book="${this.escapeAttr(e.book_id)}">${this.escapeHtml(e.series || e.book_id)} ${this.escapeHtml(e.number ? '#' + e.number : '')}</a> <a class="book-link" title="Book details" href="/books/${encodeURIComponent(e.book_id)}" data-route>&#8599;</a></td>
                <td>${stage}</td>
                <td>${this.renderChanges(e.changes)}</td>
                <td>${(e.changes && e.changes.length && e.action !== 'rollback' && e.action !== 'undo') ? `<button class="btn btn-secondary history-undo-btn" data-entry="${e.id}">Undo values…</button> ` : ''}${canRollBack ? `<button class="btn btn-secondary history-rollback-btn" data-book="${this.escapeAttr(e.book_id)}" data-stage="${this.escapeAttr(e.stage_before)}">Roll back to ${this.escapeHtml(e.stage_before)}</button>` : ''}</td>
            </tr>`;
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        let body;
        if (this.loading) {
            body = '<p class="empty-message">Loading…</p>';
        } else if (this.error) {
            body = `<p class="empty-message">${this.escapeHtml(this.error)}</p>`;
        } else if (this.entries.length === 0) {
            body = '<p class="empty-message" id="history-empty">No history recorded yet.</p>';
        } else {
            body = `
                <div class="history-table-wrap">
                <table class="history-table">
                    <thead><tr><th>When</th><th>Action</th><th>Book</th><th>Stage</th><th>Changes</th><th></th></tr></thead>
                    <tbody>${this.entries.map(e => this.renderRow(e)).join('')}</tbody>
                </table>
                </div>
                <div class="history-pager">
                    <button class="btn btn-secondary" id="history-prev" ${this.offset === 0 ? 'disabled' : ''}>&larr; Newer</button>
                    <span class="empty-message">${this.offset + 1}–${this.offset + this.entries.length} of ${this.total}</span>
                    <button class="btn btn-secondary" id="history-next" ${this.offset + this.entries.length >= this.total ? 'disabled' : ''}>Older &rarr;</button>
                </div>`;
        }
        const actions = ['convert_cbz', 'write_metadata', 'scan_info', 'data_manager', 'organize', 'scrape', 'link_wanted', 'rollback', 'undo', 'edit'];
        app.innerHTML = `
            <div class="history-page">
                <div class="history-header">
                    <h1>History</h1>
                    <p class="history-subtitle">What each pipeline action changed. Rolling back resets a book's stage so the pipeline runs it again; "Undo values…" restores the old values an entry recorded in full (after a preview); file moves are never undone.</p>
                </div>
                <div class="history-filters">
                    <select id="history-action">
                        <option value="">All actions</option>
                        ${actions.map(a => `<option value="${a}" ${this.action === a ? 'selected' : ''}>${a}</option>`).join('')}
                    </select>
                    ${this.bookId ? `<span class="history-filter-chip">Book ${this.escapeHtml(this.bookId)} <button class="btn btn-secondary" id="history-clear-book">Clear</button></span>` : ''}
                </div>
                ${this.renderUndo()}
                ${this.message ? `<div class="panel"><p>${this.escapeHtml(this.message)}</p></div>` : ''}
                ${body}
            </div>`;
        this.attach();
    }

    attach() {
        const actionSel = document.getElementById('history-action');
        if (actionSel) actionSel.addEventListener('change', () => {
            this.action = actionSel.value;
            this.reload(true);
        });
        const clear = document.getElementById('history-clear-book');
        if (clear) clear.addEventListener('click', () => { this.bookId = ''; this.reload(true); });
        const prev = document.getElementById('history-prev');
        if (prev) prev.addEventListener('click', () => { this.offset = Math.max(0, this.offset - this.limit); this.reload(false); });
        const next = document.getElementById('history-next');
        if (next) next.addEventListener('click', () => { this.offset += this.limit; this.reload(false); });
        document.querySelectorAll('.history-book-link').forEach(a => a.addEventListener('click', ev => {
            ev.preventDefault();
            this.bookId = a.dataset.book;
            this.reload(true);
        }));
        document.querySelectorAll('.history-undo-btn').forEach(btn => btn.addEventListener('click', () => this.openUndo(Number(btn.dataset.entry))));
        const uForce = document.getElementById('history-undo-force');
        if (uForce) uForce.addEventListener('change', () => { this.undo.force = uForce.checked; this.render(); });
        const uApply = document.getElementById('history-undo-apply');
        if (uApply) uApply.addEventListener('click', () => this.applyUndo());
        const uCancel = document.getElementById('history-undo-cancel');
        if (uCancel) uCancel.addEventListener('click', () => { this.undo = null; this.render(); });
        document.querySelectorAll('.history-rollback-btn').forEach(btn => btn.addEventListener('click', () => {
            if (confirm(`Reset this book to the "${btn.dataset.stage}" stage? Field values and files are not changed.`)) {
                this.rollback(btn.dataset.book, btn.dataset.stage);
            }
        }));
    }

    undoable(it) {
        return it.status === 'ok' || (it.status === 'conflict' && this.undo.force);
    }

    renderUndo() {
        if (!this.undo) return '';
        const u = this.undo;
        const rows = u.items.map(it => `
            <tr class="history-undo-${this.escapeAttr(it.status)}">
                <td>${this.escapeHtml(it.field)}</td>
                <td>${this.escapeHtml(it.current) || '∅'}</td>
                <td>${this.escapeHtml(it.old) || '∅'}</td>
                <td>${it.status === 'ok' ? 'will restore' : it.status === 'conflict' ? 'changed since this entry' : it.status === 'unchanged' ? 'already the old value' : 'cannot undo: ' + this.escapeHtml(it.reason)}</td>
            </tr>`).join('');
        const n = u.items.filter(it => this.undoable(it)).length;
        const hasConflict = u.items.some(it => it.status === 'conflict');
        return `
            <div class="panel" id="history-undo-panel">
                <h2>Undo values: ${this.escapeHtml(u.entry.action)} on ${this.escapeHtml(u.entry.series || u.entry.book_id)} ${this.escapeHtml(u.entry.number ? '#' + u.entry.number : '')}</h2>
                <table class="history-table"><thead><tr><th>Field</th><th>Now</th><th>Restore to</th><th></th></tr></thead><tbody>${rows}</tbody></table>
                ${hasConflict ? `<label><input type="checkbox" id="history-undo-force" ${u.force ? 'checked' : ''}> Also overwrite values that changed since this entry</label>` : ''}
                <p>
                    <button class="btn" id="history-undo-apply" ${n ? '' : 'disabled'}>Restore ${n} field${n === 1 ? '' : 's'}</button>
                    <button class="btn btn-secondary" id="history-undo-cancel">Cancel</button>
                </p>
            </div>`;
    }

    async openUndo(entryId) {
        const entry = this.entries.find(e => e.id === entryId);
        try {
            const res = await fetch('/api/library/audit/undo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ entry_id: entryId, apply: false }),
            });
            if (!res.ok) throw new Error((await res.text()).trim() || res.status);
            const data = await res.json();
            this.undo = { entry, items: data.items || [], force: false };
            this.message = '';
        } catch (e) {
            this.message = `Could not preview undo: ${e.message || e}`;
        }
        this.render();
    }

    async applyUndo() {
        const u = this.undo;
        const fields = u.items.filter(it => this.undoable(it)).map(it => it.field);
        try {
            const res = await fetch('/api/library/audit/undo', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ entry_id: u.entry.id, fields, apply: true, force: u.force }),
            });
            this.message = res.ok
                ? `Restored ${fields.length} field${fields.length === 1 ? '' : 's'}.`
                : `Could not undo: ${(await res.text()).trim() || res.status}`;
        } catch (e) {
            this.message = 'Could not undo (network error).';
        }
        this.undo = null;
        await this.reload(true);
    }

    async reload(resetOffset) {
        if (resetOffset) this.offset = 0;
        this.loading = true;
        this.render();
        await this.load();
        this.render();
    }

    async rollback(bookId, stage) {
        try {
            const res = await fetch('/api/library/audit/rollback', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_id: bookId, stage }),
            });
            this.message = res.ok
                ? `Book reset to "${stage}".`
                : `Could not roll back: ${(await res.text()).trim() || res.status}`;
        } catch (e) {
            console.error('Rollback failed:', e);
            this.message = 'Could not roll back (network error).';
        }
        await this.reload(true);
    }
}
