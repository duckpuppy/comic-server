// Variant Covers page (comic-server-0u48): pick a book, preview the extra
// images ComicVine has for its issue, and add the ones you choose to the CBZ
// as Front Cover pages. ComicVine does not label which images are variant
// covers, so nothing is added without this manual pick.
class VariantCoversPage {
    constructor() {
        this.query = '';
        this.results = [];
        this.searching = false;
        this.selectedBook = null; // preview object from /browse
        this.info = null;         // response of GET /api/library/variant-covers/:id
        this.chosen = new Set();
        this.loadingInfo = false;
        this.applying = false;
        this.message = '';
        this.error = '';
        this.debounce = null;
    }

    async init() {
        this.render();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        const active = document.activeElement;
        const focusId = active && active.id && app.contains(active) ? active.id : null;
        const caret = focusId && typeof active.selectionStart === 'number' ? active.selectionStart : null;

        app.innerHTML = `
            <div class="variant-covers-page">
                <div class="variant-covers-header">
                    <h1>Variant Covers</h1>
                    <p class="variant-covers-subtitle">Add extra cover images from ComicVine to a book's CBZ as Front Cover pages. They are appended after the existing pages. The original file goes to the trash folder.</p>
                </div>
                <div class="panel">
                    <input type="text" id="variant-query" class="variant-query-input" value="${this.escapeAttr(this.query)}"
                           placeholder="Find a book, e.g. Batman V2017 #15">
                    ${this.renderResults()}
                </div>
                ${this.message ? `<div class="panel"><p>${this.escapeHtml(this.message)}</p></div>` : ''}
                ${this.error ? `<div class="panel"><p class="datamanager-errors">${this.escapeHtml(this.error)}</p></div>` : ''}
                ${this.renderCandidates()}
            </div>`;

        if (focusId) {
            const el = document.getElementById(focusId);
            if (el) {
                el.focus();
                if (caret !== null && el.setSelectionRange) {
                    try { el.setSelectionRange(caret, caret); } catch (e) { /* ignore */ }
                }
            }
        }
        this.attach();
    }

    renderResults() {
        if (this.searching) return '<p class="empty-message">Searching…</p>';
        if (!this.query.trim()) return '';
        if (this.results.length === 0) return '<p class="empty-message" id="variant-no-results">No books match.</p>';
        const rows = this.results.map(b => `
            <li class="variant-book-row">
                <span>${this.escapeHtml(b.series)} ${this.escapeHtml(b.number ? '#' + b.number : '')}
                    <span class="empty-message">${this.escapeHtml(b.title || '')} ${b.year ? '(' + b.year + ')' : ''}</span></span>
                <button class="btn btn-secondary variant-pick-btn" data-id="${this.escapeAttr(b.id)}">Find covers</button>
            </li>`).join('');
        return `<ul class="variant-book-list">${rows}</ul>`;
    }

    renderCandidates() {
        if (this.loadingInfo) return '<div class="panel"><p class="empty-message">Asking ComicVine…</p></div>';
        if (!this.info) return '';
        const info = this.info;
        const title = `${this.escapeHtml(info.series)} ${this.escapeHtml(info.number ? '#' + info.number : '')}`;
        if (info.candidates.length === 0) {
            return `<div class="panel"><h2>${title}</h2><p class="empty-message">ComicVine has no extra images for this issue.</p></div>`;
        }
        const cards = info.candidates.map(c => `
            <label class="variant-card ${c.added ? 'variant-card-added' : ''}">
                <img src="${this.escapeAttr(c.url)}" alt="ComicVine image ${c.id}" loading="lazy" referrerpolicy="no-referrer">
                <span class="variant-card-meta">
                    <input type="checkbox" class="variant-check" data-id="${c.id}"
                           ${c.added ? 'checked disabled' : ''} ${this.chosen.has(c.id) ? 'checked' : ''}>
                    ${c.added ? 'Already added' : 'Add'}
                    ${c.caption ? `<span class="empty-message">${this.escapeHtml(c.caption)}</span>` : ''}
                </span>
            </label>`).join('');
        const blocked = !info.can_apply;
        const selectable = this.chosen.size;
        return `
            <div class="panel">
                <h2>${title}</h2>
                <p class="empty-message">ComicVine does not say which of these are variant covers. Tick only the ones that are.</p>
                ${blocked ? `<p class="datamanager-errors" id="variant-blocked">${this.escapeHtml(info.reason || 'Cannot write to this book.')}</p>` : ''}
                <div class="variant-grid">${cards}</div>
                <div class="datamanager-actions">
                    <button class="btn btn-primary" id="variant-apply-btn" ${blocked || selectable === 0 || this.applying ? 'disabled' : ''}>
                        ${this.applying ? 'Adding…' : `Add ${selectable} cover${selectable === 1 ? '' : 's'} to the CBZ`}
                    </button>
                </div>
            </div>`;
    }

    attach() {
        const q = document.getElementById('variant-query');
        if (q) q.addEventListener('input', () => {
            this.query = q.value;
            clearTimeout(this.debounce);
            this.debounce = setTimeout(() => this.search(), 350);
        });
        document.querySelectorAll('.variant-pick-btn').forEach(btn => btn.addEventListener('click', () => {
            this.pick(btn.dataset.id);
        }));
        document.querySelectorAll('.variant-check').forEach(cb => cb.addEventListener('change', () => {
            const id = parseInt(cb.dataset.id, 10);
            if (cb.checked) this.chosen.add(id); else this.chosen.delete(id);
            this.render();
        }));
        const apply = document.getElementById('variant-apply-btn');
        if (apply) apply.addEventListener('click', () => this.apply());
    }

    async search() {
        if (!this.query.trim()) {
            this.results = [];
            this.render();
            return;
        }
        this.searching = true;
        this.render();
        try {
            const res = await fetch('/api/library/browse?limit=15&offset=0', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ show_all: true, query: this.query, sort: 'series' }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            this.results = (await res.json()).comics || [];
        } catch (e) {
            console.error('Variant covers search failed:', e);
            this.error = 'Search failed.';
            this.results = [];
        }
        this.searching = false;
        this.render();
    }

    async pick(bookId, keepMessage = false) {
        this.selectedBook = this.results.find(b => b.id === bookId) || null;
        this.info = null;
        this.chosen = new Set();
        if (!keepMessage) {
            this.message = '';
            this.error = '';
        }
        this.loadingInfo = true;
        this.render();
        try {
            const res = await fetch(`/api/library/variant-covers/${encodeURIComponent(bookId)}`);
            const text = await res.text();
            if (!res.ok) {
                this.error = text.trim() || `HTTP ${res.status}`;
            } else {
                this.info = JSON.parse(text);
            }
        } catch (e) {
            console.error('Variant cover lookup failed:', e);
            this.error = 'Could not reach the server.';
        }
        this.loadingInfo = false;
        this.render();
    }

    async apply() {
        if (!this.info || this.chosen.size === 0 || this.applying) return;
        this.applying = true;
        this.message = '';
        this.error = '';
        this.render();
        const ids = Array.from(this.chosen);
        try {
            const res = await fetch(`/api/library/variant-covers/${encodeURIComponent(this.info.book_id)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ image_ids: ids }),
            });
            const text = await res.text();
            if (!res.ok) {
                this.error = text.trim() || `HTTP ${res.status}`;
            } else {
                const r = JSON.parse(text);
                this.message = `Added ${r.added} cover${r.added === 1 ? '' : 's'}` +
                    (r.skipped ? `, ${r.skipped} already there` : '') +
                    (r.errors && r.errors.length ? `. ${r.errors.length} failed: ${r.errors.join('; ')}` : '.');
            }
        } catch (e) {
            console.error('Applying variant covers failed:', e);
            this.error = 'Could not reach the server.';
        }
        this.applying = false;
        if (this.info) await this.pick(this.info.book_id, true);
    }
}
