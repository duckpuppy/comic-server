// Collects suggestions (tracker #40). A collected book's description often
// says "Collects GHOST RIDER (2006) #33-35". The server reads that text and
// looks each title up among the ComicVine volumes already cached here. Nothing
// is saved until you accept: tick the entries that are right, then Accept, or
// Dismiss to stop being asked about the book.
class CollectsSuggestionsPage {
    constructor() {
        this.items = [];
        this.total = 0;
        this.offset = 0;
        this.limit = 25;
        this.q = '';
        this.onlyConfident = false;
        this.loading = true;
        this.error = '';
        this.busyId = null;
        this.ticked = {};   // book id -> Set of entry positions
        this.debounce = null;
    }

    async init() {
        this.loading = true;
        this.render();
        await this.load();
        this.render();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) { return this.escapeHtml(text).replace(/"/g, '&quot;'); }

    async load() {
        const params = new URLSearchParams({ limit: this.limit, offset: this.offset });
        if (this.q) params.set('q', this.q);
        if (this.onlyConfident) params.set('min', 'high');
        try {
            const res = await fetch(`/api/library/collects-suggestions?${params}`);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            this.items = data.suggestions || [];
            this.total = data.total || 0;
            this.error = '';
            // Default: tick the entries that matched a cached ComicVine volume.
            this.items.forEach(it => {
                if (!this.ticked[it.book_id]) {
                    this.ticked[it.book_id] = new Set(it.entries.map((e, i) => (e.confidence === 'low' ? -1 : i)).filter(i => i >= 0));
                }
            });
        } catch (e) {
            console.error('Failed to load collects suggestions:', e);
            this.error = 'Failed to load suggestions.';
            this.items = [];
        }
        this.loading = false;
    }

    range(e) { return e.to ? `#${e.from}–#${e.to}` : `#${e.from}`; }

    renderEntry(it, e, i) {
        const checked = this.ticked[it.book_id].has(i);
        const target = e.volume
            ? `<a href="https://comicvine.gamespot.com/volume/4050-${e.volume}/" target="_blank" rel="noopener noreferrer">${this.escapeHtml(e.volume_name || e.series)}</a>${e.volume_year ? ` (${this.escapeHtml(e.volume_year)})` : ''}`
            : `<em>${this.escapeHtml(e.series)}</em> <span class="empty-message">(series name only, no ComicVine volume)</span>`;
        const warn = (e.warnings || []).map(w => `<div class="cx-warning">Check: ${this.escapeHtml(w)}</div>`).join('');
        return `<tr>
            <td><input type="checkbox" class="cs-tick" data-book="${this.escapeAttr(it.book_id)}" data-i="${i}" ${checked ? 'checked' : ''} aria-label="Use this link"></td>
            <td>${this.escapeHtml(e.text)}</td>
            <td>${target}${warn}</td>
            <td><span class="cs-conf cs-conf-${this.escapeAttr(e.confidence)}">${this.escapeHtml(e.confidence)}</span> <span class="empty-message">${this.escapeHtml(e.reason)}</span></td>
        </tr>`;
    }

    renderItem(it) {
        const busy = this.busyId === it.book_id;
        const n = this.ticked[it.book_id].size;
        const label = `${it.series || it.title || it.book_id}${it.number ? ` #${it.number}` : ''}`;
        return `<section class="panel cs-item">
            <h2><a href="/books/${encodeURIComponent(it.book_id)}" data-route>${this.escapeHtml(label)}</a></h2>
            <p class="cs-excerpt">${this.escapeHtml(it.excerpt)}</p>
            ${(it.notes || []).map(x => `<p class="empty-message">Note: ${this.escapeHtml(x)}</p>`).join('')}
            <table class="cvm-table"><thead><tr><th></th><th>Text says</th><th>Link to</th><th>How sure</th></tr></thead>
                <tbody>${it.entries.map((e, i) => this.renderEntry(it, e, i)).join('')}</tbody></table>
            <div class="cx-actions">
                <button class="btn btn-primary cs-accept" data-book="${this.escapeAttr(it.book_id)}" ${n === 0 || busy ? 'disabled' : ''}>${busy ? 'Working…' : `Accept ${n} link${n === 1 ? '' : 's'}`}</button>
                <button class="btn btn-secondary cs-dismiss" data-book="${this.escapeAttr(it.book_id)}" ${busy ? 'disabled' : ''}>Dismiss</button>
            </div>
        </section>`;
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        const active = document.activeElement;
        const focusId = active && active.id && app.contains(active) ? active.id : null;
        const caret = focusId && typeof active.selectionStart === 'number' ? active.selectionStart : null;
        let body;
        if (this.loading) body = '<p class="empty-message">Loading…</p>';
        else if (this.error) body = `<p class="empty-message">${this.escapeHtml(this.error)}</p>`;
        else if (!this.items.length) body = '<p class="empty-message" id="cs-empty">Nothing to suggest. Books show here when their description says "Collects …" with issue numbers and they have no links yet.</p>';
        else body = `${this.items.map(it => this.renderItem(it)).join('')}
            <div class="missing-pager">
                <button class="btn btn-secondary" id="cs-prev" ${this.offset === 0 ? 'disabled' : ''}>&larr; Previous</button>
                <span class="empty-message">${this.offset + 1}–${this.offset + this.items.length} of ${this.total}</span>
                <button class="btn btn-secondary" id="cs-next" ${this.offset + this.items.length >= this.total ? 'disabled' : ''}>Next &rarr;</button>
            </div>`;
        app.innerHTML = `<div class="missing-page">
            <div class="missing-header">
                <h1>Collects Suggestions</h1>
                <p class="missing-subtitle">Links read from a book's description. They are guesses: check each one. Nothing is saved until you accept.</p>
            </div>
            <div class="missing-filters">
                <input type="text" id="cs-q" value="${this.escapeAttr(this.q)}" placeholder="Filter by series name">
                <label><input type="checkbox" id="cs-confident" ${this.onlyConfident ? 'checked' : ''}> Only books with a confident match</label>
            </div>
            ${body}
        </div>`;
        if (focusId) {
            const el = document.getElementById(focusId);
            if (el) { el.focus(); if (caret !== null && el.setSelectionRange) { try { el.setSelectionRange(caret, caret); } catch (e) { /* not text */ } } }
        }
        this.attach();
    }

    attach() {
        const q = document.getElementById('cs-q');
        if (q) q.addEventListener('input', () => {
            this.q = q.value;
            clearTimeout(this.debounce);
            this.debounce = setTimeout(() => this.reload(), 300);
        });
        const conf = document.getElementById('cs-confident');
        if (conf) conf.addEventListener('change', () => { this.onlyConfident = conf.checked; this.reload(); });
        const prev = document.getElementById('cs-prev');
        if (prev) prev.addEventListener('click', () => { this.offset = Math.max(0, this.offset - this.limit); this.reload(false); });
        const next = document.getElementById('cs-next');
        if (next) next.addEventListener('click', () => { this.offset += this.limit; this.reload(false); });
        document.querySelectorAll('.cs-tick').forEach(cb => cb.addEventListener('change', () => {
            const set = this.ticked[cb.dataset.book];
            const i = parseInt(cb.dataset.i, 10);
            if (cb.checked) set.add(i); else set.delete(i);
            this.render();
        }));
        document.querySelectorAll('.cs-accept').forEach(b => b.addEventListener('click', () =>
            this.act(b.dataset.book, 'accept', [...this.ticked[b.dataset.book]].sort((x, y) => x - y))));
        document.querySelectorAll('.cs-dismiss').forEach(b => b.addEventListener('click', () => this.act(b.dataset.book, 'dismiss', [])));
    }

    async reload(resetOffset = true) {
        if (resetOffset) this.offset = 0;
        await this.load();
        this.render();
    }

    async act(bookId, action, keep) {
        if (this.busyId) return;
        this.busyId = bookId;
        this.render();
        try {
            const res = await fetch(`/api/library/collects-suggestions/${encodeURIComponent(bookId)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action, keep }),
            });
            const text = await res.text();
            if (!res.ok) throw new Error(friendlyErrorText(res, text, `HTTP ${res.status}`));
            if (typeof dialogs !== 'undefined') dialogs.toast(action === 'accept' ? 'Links saved.' : 'Dismissed.', 'success');
            delete this.ticked[bookId];
        } catch (e) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
        }
        this.busyId = null;
        await this.load();
        this.render();
    }
}
