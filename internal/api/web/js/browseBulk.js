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
        this.replacing = false;    // the find-and-replace panel (tracker #31)
        this.fr = { field: '', find: '', replace: '', ignoreCase: false, wholeWord: false };
        this.numbering = false;    // the autonumber panel (tracker #30)
        this.an = { order: 'current', start: 1, step: 1, pad: 0 };
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

    // Text fields a find-and-replace may touch: the server refuses numbers,
    // dates and fixed-choice fields (findReplaceField in bookedit.go).
    static REPLACE_SKIP = new Set(['Number', 'Count', 'Volume', 'AlternateCount', 'Year', 'Month', 'Day',
        'Rating', 'CommunityRating', 'BookPrice', 'BlackAndWhite', 'Manga', 'SeriesComplete', 'Checked',
        'AgeRating', 'Format', 'LanguageISO', 'BookCollectionStatus']);

    static replaceFields() {
        return BrowseBulk.fields().filter(f => !BrowseBulk.REPLACE_SKIP.has(f.name));
    }

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
                <button class="btn btn-small btn-secondary" id="bulk-replace-btn" ${this.busy ? 'disabled' : ''}>Find &amp; replace…</button>
                <button class="btn btn-small btn-secondary" id="bulk-number-btn" ${this.busy ? 'disabled' : ''}>Autonumber…</button>
                <button class="btn btn-small btn-secondary" id="bulk-read-btn" ${this.busy ? 'disabled' : ''}>Mark read</button>
                <button class="btn btn-small btn-secondary" id="bulk-unread-btn" ${this.busy ? 'disabled' : ''}>Mark unread</button>
                <button class="btn btn-small btn-secondary" id="bulk-list-btn" ${this.busy ? 'disabled' : ''}>Add to list…</button>
                <button class="btn btn-small btn-secondary" id="bulk-clear-btn">Clear</button>
            </div>
            ${banner ? `<p class="bulk-banner">${banner}</p>` : ''}
            ${this.editing ? this.renderEditor() : ''}
            ${this.replacing ? this.renderReplace() : ''}
            ${this.numbering ? this.renderNumbering() : ''}
        </div>`;
    }

    // Autonumber: write Number 1, 2, 3... in a chosen order. Previewed per
    // book, then applied through the same bulk-edit endpoint.
    renderNumbering() {
        const an = this.an;
        const orders = [['current', 'As selected'], ['date', 'Date (year, month, day)'], ['title', 'Title'], ['file', 'File name']];
        const num = (id, label, val, min, max) => `<label>${label} <input type="number" id="${id}" value="${this.esc(val)}" min="${min}" ${max ? `max="${max}"` : ''} step="1" size="4"></label>`;
        return `<div class="bulk-editor bulk-number">
            <div class="bulk-replace-grid">
                <label>Order <select id="bulk-an-order">${orders.map(([v, l]) => `<option value="${v}" ${an.order === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
                ${num('bulk-an-start', 'Start at', an.start, 0)}
                ${num('bulk-an-step', 'Step', an.step, 1)}
                ${num('bulk-an-pad', 'Zero padding (width)', an.pad, 0, 10)}
            </div>
            <div class="bulk-editor-add">
                <span class="browse-compact-meta">Sets the Number of the ${this.count()} selected books. Books with a ComicVine issue keep their number. Books with no date go last in Date order.</span>
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

    // Find and replace in one field: plain text, optional case-insensitive
    // and whole-word. Previewed per book, then applied through the same
    // bulk-edit endpoint.
    renderReplace() {
        const fr = this.fr;
        const opts = BrowseBulk.replaceFields().map(f =>
            `<option value="${this.esc(f.name)}" ${f.name === fr.field ? 'selected' : ''}>${this.esc(f.label)}</option>`).join('');
        const ready = fr.field && fr.find;
        return `<div class="bulk-editor bulk-replace">
            <div class="bulk-replace-grid">
                <label>Field <select id="bulk-fr-field"><option value="">Pick a field…</option>${opts}</select></label>
                <label>Find <input type="text" id="bulk-fr-find" value="${this.esc(fr.find)}" placeholder="text to find"></label>
                <label>Replace with <input type="text" id="bulk-fr-replace" value="${this.esc(fr.replace)}" placeholder="empty removes the text"></label>
            </div>
            <div class="bulk-editor-add">
                <label><input type="checkbox" id="bulk-fr-ic" ${fr.ignoreCase ? 'checked' : ''}> Ignore case</label>
                <label><input type="checkbox" id="bulk-fr-ww" ${fr.wholeWord ? 'checked' : ''}> Whole word</label>
                <span class="browse-compact-meta">Plain text, not a pattern. A list field is changed as its whole comma-separated text.</span>
            </div>
            ${this.error ? `<p class="datamanager-errors">${this.esc(this.error)}</p>` : ''}
            ${this.renderPreview()}
            <div class="bulk-editor-buttons">
                <button class="btn btn-secondary" id="bulk-preview-btn" ${this.busy || !ready ? 'disabled' : ''}>${this.busy ? 'Working…' : 'Preview'}</button>
                <button class="btn btn-primary" id="bulk-apply-btn" ${this.busy || !this.preview || this.preview.changed === 0 ? 'disabled' : ''}>Apply to ${this.preview ? this.preview.changed : 0} book${this.preview && this.preview.changed === 1 ? '' : 's'}</button>
                <button class="btn btn-secondary" id="bulk-close-btn">Close</button>
            </div>
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
            `<li>${this.esc(s.series)}${s.number ? ' #' + this.esc(s.number) : ''} <span class="browse-compact-meta">${this.esc((s.changed || []).join(', '))}</span>${(s.diffs || []).map(d =>
                `<div class="bulk-diff"><span class="bulk-diff-field">${this.esc(d.field)}</span> <del>${this.esc(d.before) || '(empty)'}</del> &rarr; <ins>${this.esc(d.after) || '(empty)'}</ins></div>`).join('')}</li>`).join('');
        return `<div class="bulk-preview">
            <p><strong>${pv.changed}</strong> of ${pv.total} books would change; ${pv.unchanged} already match.${fields ? ` (${fields})` : ''}</p>
            ${pv.skipped ? `<p class="browse-compact-meta">${pv.skipped} skipped, they keep their number: ${(pv.skipped_books || []).map(b => `${this.esc(b.series)}${b.number ? ' #' + this.esc(b.number) : ''} (${this.esc(b.reason)})`).join(', ')}${pv.skipped > (pv.skipped_books || []).length ? ', …' : ''}</p>` : ''}
            ${sample ? `<ul class="bulk-sample">${sample}</ul>${pv.changed > pv.sample.length ? `<p class="browse-compact-meta">…and ${pv.changed - pv.sample.length} more.</p>` : ''}` : ''}
        </div>`;
    }

    // Build the edit body from the rows. Empty values clear a field on Replace.
    editBody() {
        if (this.numbering) {
            const an = this.an;
            return { autonumber: { order: an.order, start: an.start, step: an.step, pad: an.pad } };
        }
        if (this.replacing) {
            const fr = this.fr;
            if (!fr.field || !fr.find) return {};
            return { find_replace: { field: fr.field, find: fr.find, replace: fr.replace, ignore_case: fr.ignoreCase, whole_word: fr.wholeWord } };
        }
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
            return { browse: { matcher_mode: p.matcherMode, matchers: p.matchers, show_all: p.showAll, query: p.query, facets: p.facets, ...p.groupScope() } };
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
        if (!Object.keys(edit).length) { this.error = this.replacing ? 'Pick a field and enter the text to find.' : 'Add a field or a read state first.'; this.redraw(); return; }
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

    // Preview needs a field and some text to find.
    syncReplaceButtons() {
        const btn = document.getElementById('bulk-preview-btn');
        if (btn && this.replacing) btn.disabled = this.busy || !(this.fr.field && this.fr.find);
    }

    afterSelectionChange() {
        this.preview = null;
        if (!this.any()) { this.editing = false; this.replacing = false; this.numbering = false; }
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
        on('bulk-edit-btn', () => { this.editing = !this.editing; this.replacing = false; this.numbering = false; this.preview = null; this.redraw(); });
        on('bulk-replace-btn', () => { this.replacing = !this.replacing; this.editing = false; this.numbering = false; this.preview = null; this.error = ''; this.redraw(); });
        on('bulk-number-btn', () => { this.numbering = !this.numbering; this.editing = false; this.replacing = false; this.preview = null; this.error = ''; this.redraw(); });
        on('bulk-read-btn', () => this.apply({ read: 'read' }, 'Mark read'));
        on('bulk-unread-btn', () => this.apply({ read: 'unread' }, 'Mark unread'));
        on('bulk-list-btn', () => ManualLists.pickAndAdd({ scope: this.scopeBody(), count: this.count() }));
        on('bulk-clear-btn', () => { this.reset(); this.redraw(); });
        on('bulk-close-btn', () => { this.editing = false; this.replacing = false; this.numbering = false; this.redraw(); });
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
        // Find-and-replace inputs: like the row values, typing only drops the stale preview.
        const frInput = (id, key, prop = 'value') => {
            const el = $(id);
            if (!el) return;
            const set = () => { this.fr[key] = el[prop]; this.invalidatePreview(); this.syncReplaceButtons(); };
            el.addEventListener('input', set);
            el.addEventListener('change', set);
        };
        frInput('bulk-fr-field', 'field');
        frInput('bulk-fr-find', 'find');
        frInput('bulk-fr-replace', 'replace');
        frInput('bulk-fr-ic', 'ignoreCase', 'checked');
        frInput('bulk-fr-ww', 'wholeWord', 'checked');
        // Autonumber inputs: typing only drops the stale preview.
        const anInput = (id, key, num) => {
            const el = $(id);
            if (!el) return;
            const set = () => { this.an[key] = num ? (parseInt(el.value, 10) || 0) : el.value; this.invalidatePreview(); };
            el.addEventListener('input', set);
            el.addEventListener('change', set);
        };
        anInput('bulk-an-order', 'order');
        anInput('bulk-an-start', 'start', true);
        anInput('bulk-an-step', 'step', true);
        anInput('bulk-an-pad', 'pad', true);
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
