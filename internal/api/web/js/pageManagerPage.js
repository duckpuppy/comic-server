// Page Manager (comic-server-j1cv): reorder, delete and add pages inside a
// book's CBZ. Everything is staged in the browser; Save sends one request and
// the server rewrites the archive atomically (the original goes to the trash).
class PageManagerPage {
    constructor() {
        this.bookId = null;
        this.info = null;     // response of GET /api/library/books/:id/pages
        this.items = [];      // {kind:'page'|'upload', index, name, size, type, bookmark, file, url, deleted}
        this.dragFrom = null;
        this.loading = false;
        this.saving = false;
        this.message = '';
        this.error = '';
        this.canWriteFile = false; // set after labels are saved
    }

    async init() {
        this.bookId = new URLSearchParams(window.location.search).get('book');
        this.message = '';
        this.error = '';
        this.render();
        if (!this.bookId) {
            this.error = 'No book selected. Open a book and choose "Manage pages".';
            this.render();
            return;
        }
        await this.load();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    async load() {
        this.loading = true;
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/pages`);
            if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
            this.info = await res.json();
            this.resetItems();
        } catch (e) {
            this.error = `Could not load pages: ${e.message}`;
            this.info = null;
        }
        this.loading = false;
        this.render();
    }

    resetItems() {
        this.items.forEach(it => it.url && it.kind === 'upload' && URL.revokeObjectURL(it.url));
        const v = encodeURIComponent(this.info.version || '');
        this.items = (this.info.pages || []).map(p => ({
            kind: 'page', index: p.index, name: p.name, size: p.size,
            type: p.type || '', bookmark: p.bookmark || '', origType: p.type || '', origBookmark: p.bookmark || '', deleted: false,
            url: `/api/library/books/${encodeURIComponent(this.bookId)}/pages/${p.index}?w=240&v=${v}`,
        }));
    }

    isDirty() {
        if (!this.info) return false;
        if (this.items.length !== (this.info.pages || []).length) return true;
        return this.items.some((it, i) => it.kind !== 'page' || it.deleted || it.index !== i);
    }

    // labelEdits are the pages whose type or bookmark differs from the saved
    // record (tracker #12). Only existing pages have a record to edit.
    labelEdits() {
        return this.items.filter(it => it.kind === 'page' && !it.deleted && (it.type !== it.origType || it.bookmark !== it.origBookmark));
    }

    kept() { return this.items.filter(it => !it.deleted); }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        const info = this.info;
        const title = info ? `${this.escapeHtml(info.series)} ${info.number ? '#' + this.escapeHtml(info.number) : ''}` : '';
        const canEdit = info && info.can_edit && !this.saving;
        const kept = this.kept().length;
        const removed = this.items.filter(it => it.deleted && it.kind === 'page').length;
        const added = this.items.filter(it => it.kind === 'upload' && !it.deleted).length;
        const labels = this.labelEdits().length;
        const structural = this.isDirty();

        app.innerHTML = `
            <div class="page-manager-page">
                <div class="page-manager-header">
                    <h1>Manage Pages</h1>
                    <p class="page-manager-subtitle">${title}
                        ${this.bookId ? `<a href="/books/${encodeURIComponent(this.bookId)}" data-route>Back to book</a>` : ''}</p>
                    <p class="page-manager-subtitle">Drag pages to reorder, or use the arrows. Delete removes a page from the file when you save. The original file goes to the trash folder.</p>
                </div>
                ${this.loading ? '<div class="panel"><p class="empty-message">Loading pages…</p></div>' : ''}
                ${info && info.reason ? `<div class="panel"><p class="empty-message" id="pm-reason">${this.escapeHtml(info.reason)}</p></div>` : ''}
                ${this.message ? `<div class="panel"><p id="pm-message">${this.escapeHtml(this.message)}</p></div>` : ''}
                ${this.error ? `<div class="panel"><p class="datamanager-errors" id="pm-error">${this.escapeHtml(this.error)}</p></div>` : ''}
                ${info ? `
                <div class="panel page-manager-toolbar">
                    <span id="pm-summary">${kept} page${kept === 1 ? '' : 's'} after save${removed ? `, ${removed} removed` : ''}${added ? `, ${added} added` : ''}</span>
                    <label class="btn btn-secondary ${canEdit ? '' : 'disabled'}" for="pm-add-input">Add pages…</label>
                    <input type="file" id="pm-add-input" accept="image/jpeg,image/png,image/gif,image/webp" multiple hidden ${canEdit ? '' : 'disabled'}>
                    <button class="btn btn-secondary" id="pm-reset-btn" ${canEdit && this.isDirty() ? '' : 'disabled'}>Reset</button>
                    <button class="btn btn-secondary" id="pm-labels-btn" ${labels && !structural ? '' : 'disabled'}
                        title="${structural ? 'Save the page order changes first' : 'Save page types and bookmarks to the library record'}">Save page labels${labels ? ` (${labels})` : ''}</button>
                    ${this.canWriteFile ? '<button class="btn btn-secondary" id="pm-writefile-btn">Write labels to the file now</button>' : ''}
                    <button class="btn btn-primary" id="pm-save-btn" ${canEdit && this.isDirty() && kept > 0 ? '' : 'disabled'}>${this.saving ? 'Saving…' : 'Save changes'}</button>
                </div>
                <div class="page-manager-grid" id="pm-grid">
                    ${this.items.map((it, i) => this.renderCard(it, i, canEdit)).join('')}
                </div>` : ''}
            </div>`;
        this.attach();
    }

    renderCard(it, i, canEdit) {
        const badges = [];
        if (it.kind === 'upload') badges.push('<span class="pm-badge pm-badge-new">New</span>');
        if (it.type) badges.push(`<span class="pm-badge">${this.escapeHtml(it.type)}</span>`);
        if (it.bookmark) badges.push('<span class="pm-badge">Bookmark</span>');
        const dis = canEdit ? '' : 'disabled';
        const types = (this.info && this.info.page_types) || [''];
        const opts = types.includes(it.type) ? types : types.concat([it.type]); // keep a value the list does not know
        const labelEditor = it.kind === 'page' && !it.deleted ? `
                <div class="pm-labels">
                    <select class="pm-type" data-pos="${i}" aria-label="Type of page ${i + 1}">
                        ${opts.map(t => `<option value="${this.escapeHtml(t)}" ${t === it.type ? 'selected' : ''}>${t ? this.escapeHtml(t) : '(no type)'}</option>`).join('')}
                    </select>
                    <input type="text" class="pm-bookmark" data-pos="${i}" maxlength="200" placeholder="Bookmark" value="${this.escapeHtml(it.bookmark)}" aria-label="Bookmark of page ${i + 1}">
                </div>` : '';
        return `
            <div class="pm-card ${it.deleted ? 'pm-card-deleted' : ''}" data-pos="${i}" draggable="${canEdit ? 'true' : 'false'}">
                <div class="pm-thumb"><img src="${this.escapeHtml(it.url)}" alt="Page ${i + 1}" loading="lazy" draggable="false"></div>
                <div class="pm-meta">
                    <strong>${i + 1}</strong>
                    <span class="pm-name" title="${this.escapeHtml(it.name)}">${this.escapeHtml(it.name)}</span>
                    ${badges.join('')}
                </div>
                ${labelEditor}
                <div class="pm-actions">
                    <button class="btn btn-secondary pm-left" data-pos="${i}" ${dis} ${i === 0 ? 'disabled' : ''} aria-label="Move page ${i + 1} earlier">←</button>
                    <button class="btn btn-secondary pm-right" data-pos="${i}" ${dis} ${i === this.items.length - 1 ? 'disabled' : ''} aria-label="Move page ${i + 1} later">→</button>
                    <button class="btn btn-secondary pm-delete" data-pos="${i}" ${dis}>${it.deleted ? 'Restore' : 'Delete'}</button>
                </div>
            </div>`;
    }

    attach() {
        const on = (id, ev, fn) => {
            const el = document.getElementById(id);
            if (el) el.addEventListener(ev, fn);
        };
        on('pm-save-btn', 'click', () => this.save());
        on('pm-labels-btn', 'click', () => this.saveLabels());
        on('pm-writefile-btn', 'click', () => this.writeFile());
        // Typing in a label only updates the staged value and the buttons;
        // a full re-render would drop the caret.
        document.querySelectorAll('.pm-type').forEach(el => el.addEventListener('change', () => {
            this.items[+el.dataset.pos].type = el.value;
            this.message = '';
            this.render();
        }));
        document.querySelectorAll('.pm-bookmark').forEach(el => el.addEventListener('input', () => {
            this.items[+el.dataset.pos].bookmark = el.value;
            const btn = document.getElementById('pm-labels-btn');
            const n = this.labelEdits().length;
            if (btn) {
                btn.disabled = !(n && !this.isDirty());
                btn.textContent = `Save page labels${n ? ` (${n})` : ''}`;
            }
        }));
        on('pm-reset-btn', 'click', () => { this.resetItems(); this.message = ''; this.error = ''; this.render(); });
        on('pm-add-input', 'change', (e) => this.addFiles(e.target.files));

        document.querySelectorAll('.pm-left').forEach(b => b.addEventListener('click', () => this.move(+b.dataset.pos, -1)));
        document.querySelectorAll('.pm-right').forEach(b => b.addEventListener('click', () => this.move(+b.dataset.pos, 1)));
        document.querySelectorAll('.pm-delete').forEach(b => b.addEventListener('click', () => this.toggleDelete(+b.dataset.pos)));

        document.querySelectorAll('.pm-card').forEach(card => {
            card.addEventListener('dragstart', (e) => {
                this.dragFrom = +card.dataset.pos;
                e.dataTransfer.effectAllowed = 'move';
                try { e.dataTransfer.setData('text/plain', String(this.dragFrom)); } catch (err) { /* ignore */ }
            });
            card.addEventListener('dragover', (e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; });
            card.addEventListener('drop', (e) => {
                e.preventDefault();
                const to = +card.dataset.pos;
                if (this.dragFrom !== null && this.dragFrom !== to) this.moveTo(this.dragFrom, to);
                this.dragFrom = null;
            });
            card.addEventListener('dragend', () => { this.dragFrom = null; });
        });
    }

    move(pos, delta) { this.moveTo(pos, pos + delta); }

    moveTo(from, to) {
        if (to < 0 || to >= this.items.length) return;
        const [item] = this.items.splice(from, 1);
        this.items.splice(to, 0, item);
        this.message = '';
        this.render();
    }

    toggleDelete(pos) {
        const it = this.items[pos];
        if (!it) return;
        if (it.kind === 'upload') {
            // An upload that was never saved is simply dropped.
            URL.revokeObjectURL(it.url);
            this.items.splice(pos, 1);
        } else {
            it.deleted = !it.deleted;
        }
        this.message = '';
        this.render();
    }

    addFiles(fileList) {
        for (const file of Array.from(fileList || [])) {
            this.items.push({
                kind: 'upload', index: -1, name: file.name, size: file.size,
                type: '', bookmark: '', file, url: URL.createObjectURL(file), deleted: false,
            });
        }
        this.message = '';
        this.render();
    }

    // saveLabels sends the changed types and bookmarks. It changes the library
    // record only and marks the file out of date; the file itself is rewritten
    // by the Write metadata action (offered here once labels are saved).
    async saveLabels() {
        const edits = this.labelEdits();
        if (this.saving || !edits.length || this.isDirty()) return;
        this.saving = true;
        this.error = '';
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/pages/meta`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ version: this.info.version, pages: edits.map(it => ({ index: it.index, type: it.type, bookmark: it.bookmark.trim() })) }),
            });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            const out = await res.json();
            this.canWriteFile = !!(this.info && this.info.can_edit);
            this.message = `Saved ${out.changed} page label${out.changed === 1 ? '' : 's'} to the library.` +
                (this.canWriteFile ? ' The file still has the old labels until you write them.' : ' This file cannot be rewritten here (only CBZ files can).');
            this.saving = false;
            await this.load();
        } catch (e) {
            this.saving = false;
            this.error = `Could not save labels: ${e.message}`;
            this.render();
        }
    }

    async writeFile() {
        if (this.saving) return;
        if (!window.confirm('Rewrite this file with the new labels? The original goes to the trash folder.')) return;
        this.saving = true;
        this.render();
        try {
            const res = await fetch('/api/library/workflow/write-metadata', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_ids: [this.bookId] }),
            });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            const out = await res.json();
            if (out.written) { this.message = 'The file now has the new labels.'; this.canWriteFile = false; this.error = ''; }
            else this.error = `Could not write the file: ${(out.errors || []).join('; ') || 'unknown error'}`;
        } catch (e) {
            this.error = `Could not write the file: ${e.message}`;
        }
        this.saving = false;
        await this.load();
    }

    async save() {
        if (this.saving || !this.isDirty()) return;
        const kept = this.kept();
        const removed = this.items.filter(it => it.deleted && it.kind === 'page').length;
        const warn = removed
            ? `Remove ${removed} page${removed === 1 ? '' : 's'} from the file? The original goes to the trash folder.`
            : 'Rewrite this file with the new page order? The original goes to the trash folder.';
        if (!window.confirm(warn)) return;

        const uploads = kept.filter(it => it.kind === 'upload');
        const order = kept.map(it => it.kind === 'page'
            ? { page: it.index }
            : { upload: uploads.indexOf(it) });
        const form = new FormData();
        form.append('plan', JSON.stringify({ version: this.info.version, order }));
        uploads.forEach(it => form.append('upload', it.file, it.name));

        this.saving = true;
        this.error = '';
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/pages`, { method: 'POST', body: form });
            if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
            const out = await res.json();
            this.message = `Saved. The file now has ${out.page_count} pages (${out.deleted} removed, ${out.added} added).` +
                (out.warning ? ` ${out.warning}` : '');
            this.saving = false;
            await this.load();
        } catch (e) {
            this.saving = false;
            this.error = `Save failed: ${e.message}`;
            this.render();
        }
    }
}
