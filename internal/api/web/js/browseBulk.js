// Bulk edit for Browse (comic-server-k1bt.2): tick books (or "select all N
// matching"), then change fields on all of them, or mark them read/unread.
// ComicRack's multi-book Info dialog. Every change is previewed first and
// applied only after a confirm. POST /api/library/browse/bulk-edit.
class BrowseBulk {
    constructor(page) {
        this.page = page;
        this.reset();
    }

    reset() {
        this.ids = new Set();      // ticked rows
        this.allMatching = false;  // "select all N matching": the filter is the scope
        this.editing = false;
        this.rows = [];            // [{field, mode, value}] fields to change
        this.read = '';            // '' | 'read' | 'unread'
        this.preview = null;
        this.busy = false;
        this.error = '';
    }

    esc(t) { return this.page.escapeHtml(t); }

    count() { return this.allMatching ? this.page.total : this.ids.size; }
    any() { return this.count() > 0; }

    // Field specs from the book edit form, minus Number (one number for many books makes no sense).
    static fields() {
        const groups = (typeof BookDetailPage !== 'undefined' && BookDetailPage.EDIT_GROUPS) || [];
        const out = [];
        for (const [, fields] of groups) {
            for (const [name, label, type] of fields) {
                if (name !== 'Number') out.push({ name, label, type });
            }
        }
        return out;
    }

    static LIST_FIELDS = new Set(['Genre', 'Writer', 'Penciller', 'Inker', 'Colorist', 'Letterer',
        'CoverArtist', 'Editor', 'Translator', 'Characters', 'Teams', 'Locations', 'Tags']);

    // Checkbox for one result row.
    checkbox(id) {
        const on = this.allMatching || this.ids.has(id);
        return `<input type="checkbox" class="bulk-row-check" data-id="${this.esc(id)}" ${on ? 'checked' : ''} ${this.allMatching ? 'disabled' : ''} aria-label="Select this book">`;
    }

    headerCheckbox() {
        const rs = this.page.results;
        const all = rs.length > 0 && (this.allMatching || rs.every(c => this.ids.has(c.id)));
        return `<input type="checkbox" class="bulk-page-check" ${all ? 'checked' : ''} ${this.allMatching ? 'disabled' : ''} aria-label="Select all loaded books">`;
    }

    // The bar above the results: selection count, "select all N", actions.
    render() {
        if (!this.any()) return '';
        const p = this.page;
        const loadedAll = p.results.length > 0 && p.results.every(c => this.ids.has(c.id));
        let banner = '';
        if (this.allMatching) {
            banner = `All ${p.total} matching books are selected. <button class="btn-link bulk-clear-all">Clear selection</button>`;
        } else if (loadedAll && p.total > this.ids.size) {
            banner = `${this.ids.size} loaded books selected. <button class="btn-link bulk-select-all">Select all ${p.total} matching</button>`;
        }
        return `<div class="bulk-bar panel" id="bulk-bar">
            <div class="bulk-bar-row">
                <strong>${this.count()} selected</strong>
                <button class="btn btn-small btn-primary" id="bulk-edit-btn" ${this.busy ? 'disabled' : ''}>Edit…</button>
                <button class="btn btn-small btn-secondary" id="bulk-read-btn" ${this.busy ? 'disabled' : ''}>Mark read</button>
                <button class="btn btn-small btn-secondary" id="bulk-unread-btn" ${this.busy ? 'disabled' : ''}>Mark unread</button>
                <button class="btn btn-small btn-secondary" id="bulk-clear-btn">Clear</button>
            </div>
            ${banner ? `<p class="bulk-banner">${banner}</p>` : ''}
            ${this.editing ? this.renderEditor() : ''}
        </div>`;
    }

    renderEditor() {
        const specs = BrowseBulk.fields();
        const used = new Set(this.rows.map(r => r.field));
        const picker = `<select id="bulk-add-field" aria-label="Add a field to change">
            <option value="">+ Add a field to change…</option>
            ${specs.filter(f => !used.has(f.name)).map(f => `<option value="${this.esc(f.name)}">${this.esc(f.label)}</option>`).join('')}
        </select>`;
        const rows = this.rows.map((r, i) => this.renderRow(r, i, specs.find(f => f.name === r.field))).join('');
        return `<div class="bulk-editor">
            ${rows || '<p class="empty-message">Pick a field to change. Only the fields you add are written.</p>'}
            <div class="bulk-editor-add">${picker}
                <label>Read state: <select id="bulk-read-state">
                    <option value="" ${this.read === '' ? 'selected' : ''}>Leave as is</option>
                    <option value="read" ${this.read === 'read' ? 'selected' : ''}>Mark read</option>
                    <option value="unread" ${this.read === 'unread' ? 'selected' : ''}>Mark unread</option>
                </select></label>
            </div>
            ${this.error ? `<p class="datamanager-errors">${this.esc(this.error)}</p>` : ''}
            ${this.renderPreview()}
            <div class="bulk-editor-buttons">
                <button class="btn btn-secondary" id="bulk-preview-btn" ${this.busy ? 'disabled' : ''}>${this.busy ? 'Working…' : 'Preview'}</button>
                <button class="btn btn-primary" id="bulk-apply-btn" ${this.busy || !this.preview || this.preview.changed === 0 ? 'disabled' : ''}>Apply to ${this.preview ? this.preview.changed : 0} book${this.preview && this.preview.changed === 1 ? '' : 's'}</button>
                <button class="btn btn-secondary" id="bulk-close-btn">Close</button>
            </div>
        </div>`;
    }

    renderRow(r, i, spec) {
        if (!spec) return '';
        const isList = BrowseBulk.LIST_FIELDS.has(r.field);
        const mode = isList ? `<select class="bulk-row-mode" data-i="${i}" aria-label="How to change ${this.esc(spec.label)}">
            ${[['replace', 'Replace with'], ['add', 'Add'], ['remove', 'Remove']].map(([v, l]) => `<option value="${v}" ${r.mode === v ? 'selected' : ''}>${l}</option>`).join('')}
        </select>` : '<span class="bulk-row-mode-fixed">Set to</span>';
        let input;
        const attrs = `class="bulk-row-value" data-i="${i}" aria-label="${this.esc(spec.label)}"`;
        if (spec.type === 'textarea') {
            input = `<textarea ${attrs} rows="3">${this.esc(r.value)}</textarea>`;
        } else if (spec.type === 'bool') {
            input = `<select ${attrs}><option value="Yes" ${r.value === 'Yes' ? 'selected' : ''}>Yes</option><option value="No" ${r.value !== 'Yes' ? 'selected' : ''}>No</option></select>`;
        } else if (typeof BookDetailPage !== 'undefined' && BookDetailPage.EDIT_CHOICES[spec.type]) {
            input = `<select ${attrs}>${BookDetailPage.EDIT_CHOICES[spec.type].map(([v, t]) =>
                `<option value="${this.esc(v)}" ${v === r.value ? 'selected' : ''}>${this.esc(t)}</option>`).join('')}</select>`;
        } else {
            const extra = spec.type === 'int' ? 'type="number" step="1" min="0"'
                : spec.type === 'num' ? `type="number" min="0" ${spec.name.endsWith('Rating') ? 'max="5" step="0.5"' : 'step="any"'}`
                : 'type="text"';
            input = `<input ${extra} ${attrs} value="${this.esc(r.value)}" placeholder="${isList ? 'comma-separated' : 'empty clears the field'}">`;
        }
        return `<div class="bulk-row"><span class="bulk-row-label">${this.esc(spec.label)}</span>${mode}${input}
            <button class="btn btn-small btn-secondary bulk-row-remove" data-i="${i}" aria-label="Remove this field">×</button></div>`;
    }

    renderPreview() {
        const pv = this.preview;
        if (!pv) return '';
        const fields = Object.entries(pv.fields || {}).map(([f, n]) => `${this.esc(f)}: ${n}`).join(', ');
        const sample = (pv.sample || []).map(s =>
            `<li>${this.esc(s.series)}${s.number ? ' #' + this.esc(s.number) : ''} <span class="browse-compact-meta">${this.esc((s.changed || []).join(', '))}</span></li>`).join('');
        return `<div class="bulk-preview">
            <p><strong>${pv.changed}</strong> of ${pv.total} books would change; ${pv.unchanged} already match.${fields ? ` (${fields})` : ''}</p>
            ${sample ? `<ul class="bulk-sample">${sample}</ul>${pv.changed > pv.sample.length ? `<p class="browse-compact-meta">…and ${pv.changed - pv.sample.length} more.</p>` : ''}` : ''}
        </div>`;
    }

    // Build the edit body from the rows. Empty values clear a field on Replace.
    editBody() {
        const fields = {}, list_add = {}, list_remove = {};
        for (const r of this.rows) {
            if (BrowseBulk.LIST_FIELDS.has(r.field) && r.mode === 'add') list_add[r.field] = r.value;
            else if (BrowseBulk.LIST_FIELDS.has(r.field) && r.mode === 'remove') list_remove[r.field] = r.value;
            else fields[r.field] = r.value;
        }
        const edit = {};
        if (Object.keys(fields).length) edit.fields = fields;
        if (Object.keys(list_add).length) edit.list_add = list_add;
        if (Object.keys(list_remove).length) edit.list_remove = list_remove;
        if (this.read) edit.read = this.read;
        return edit;
    }

    scopeBody() {
        const p = this.page;
        if (this.allMatching) {
            return { browse: { matcher_mode: p.matcherMode, matchers: p.matchers, show_all: p.showAll, query: p.query, facets: p.facets } };
        }
        return { book_ids: Array.from(this.ids) };
    }

    async call(edit, apply) {
        const res = await fetch('/api/library/browse/bulk-edit', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ...this.scopeBody(), expected_count: this.count(), edit, apply }),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(text.trim() || `HTTP ${res.status}`);
        return JSON.parse(text);
    }

    async runPreview() {
        const edit = this.editBody();
        if (!Object.keys(edit).length) { this.error = 'Add a field or a read state first.'; this.redraw(); return; }
        this.busy = true; this.error = ''; this.redraw();
        try {
            this.preview = await this.call(edit, false);
        } catch (e) {
            this.preview = null;
            this.error = e.message;
        }
        this.busy = false;
        this.redraw();
    }

    async apply(edit, label) {
        const n = this.count();
        const ok = await dialogs.confirm({
            title: 'Apply to selection',
            message: `${label} for ${n} book${n === 1 ? '' : 's'}${this.allMatching ? ' (everything the filter matches)' : ''}? There is no undo; the history page keeps the old values.`,
            confirmLabel: 'Apply',
        });
        if (!ok) return;
        this.busy = true; this.error = ''; this.redraw();
        try {
            const r = await this.call(edit, true);
            dialogs.toast(`Changed ${r.changed} book${r.changed === 1 ? '' : 's'}.`, 'success');
            this.reset();
            await this.page.search(true);
            return;
        } catch (e) {
            this.error = e.message;
            dialogs.toast(`Bulk edit failed: ${e.message}`, 'error', 8000);
        }
        this.busy = false;
        this.redraw();
    }

    redraw() {
        const host = document.getElementById('bulk-host');
        if (!host) return;
        host.innerHTML = this.render();
        this.attachBar();
        // Row checkboxes follow the selection.
        document.querySelectorAll('.bulk-row-check').forEach(el => {
            el.checked = this.allMatching || this.ids.has(el.dataset.id);
            el.disabled = this.allMatching;
        });
        const head = document.querySelector('.bulk-page-check');
        if (head) { head.outerHTML = this.headerCheckbox(); this.attachHeader(); }
    }

    attachHeader() {
        const head = document.querySelector('.bulk-page-check');
        if (head) head.addEventListener('change', () => {
            for (const c of this.page.results) head.checked ? this.ids.add(c.id) : this.ids.delete(c.id);
            this.afterSelectionChange();
        });
    }

    invalidatePreview() {
        if (!this.preview) return;
        this.preview = null;
        const pv = document.querySelector('.bulk-preview');
        if (pv) pv.remove();
        const btn = document.getElementById('bulk-apply-btn');
        if (btn) { btn.disabled = true; btn.textContent = 'Apply to 0 books'; }
    }

    afterSelectionChange() {
        this.preview = null;
        if (!this.any()) this.editing = false;
        this.redraw();
    }

    attach() {
        document.querySelectorAll('.bulk-row-check').forEach(el => el.addEventListener('change', () => {
            el.checked ? this.ids.add(el.dataset.id) : this.ids.delete(el.dataset.id);
            this.afterSelectionChange();
        }));
        this.attachHeader();
        this.attachBar();
    }

    attachBar() {
        const $ = id => document.getElementById(id);
        const on = (id, fn) => { const el = $(id); if (el) el.addEventListener('click', fn); };
        on('bulk-edit-btn', () => { this.editing = !this.editing; this.redraw(); });
        on('bulk-read-btn', () => this.apply({ read: 'read' }, 'Mark read'));
        on('bulk-unread-btn', () => this.apply({ read: 'unread' }, 'Mark unread'));
        on('bulk-clear-btn', () => { this.reset(); this.redraw(); });
        on('bulk-close-btn', () => { this.editing = false; this.redraw(); });
        on('bulk-preview-btn', () => this.runPreview());
        on('bulk-apply-btn', () => this.apply(this.editBody(), `Apply ${this.preview ? this.preview.changed : 0} changes`));
        document.querySelectorAll('.bulk-select-all').forEach(el => el.addEventListener('click', () => {
            this.allMatching = true; this.afterSelectionChange();
        }));
        document.querySelectorAll('.bulk-clear-all').forEach(el => el.addEventListener('click', () => {
            this.reset(); this.redraw();
        }));
        const add = $('bulk-add-field');
        if (add) add.addEventListener('change', () => {
            if (!add.value) return;
            this.rows.push({ field: add.value, mode: 'replace', value: '' });
            this.preview = null;
            this.redraw();
        });
        const readState = $('bulk-read-state');
        if (readState) readState.addEventListener('change', () => { this.read = readState.value; this.preview = null; this.redraw(); });
        // Typing must not re-render (it would drop focus and fire blur mid-removal),
        // so a value edit only drops the now-stale preview in place.
        document.querySelectorAll('.bulk-row-value').forEach(el => {
            const set = () => { this.rows[+el.dataset.i].value = el.value; this.invalidatePreview(); };
            el.addEventListener('input', set);
            el.addEventListener('change', set);
        });
        document.querySelectorAll('.bulk-row-mode').forEach(el => el.addEventListener('change', () => {
            this.rows[+el.dataset.i].mode = el.value; this.preview = null; this.redraw();
        }));
        document.querySelectorAll('.bulk-row-remove').forEach(el => el.addEventListener('click', () => {
            this.rows.splice(+el.dataset.i, 1); this.preview = null; this.redraw();
        }));
    }
}
