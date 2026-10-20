// "Collects" editor (tracker #36, #39). Shows the issues a collected edition
// (TPB, hardcover, omnibus, event book) contains and lets the user link them:
// one row per run of issues, several rows for an event book. Paste a ComicVine
// volume link to fill the volume name and check the range. Preview first,
// nothing is saved until Confirm. Used on the book page and Scrape Review.
class CollectsEditor {
    // entries: current links, or undefined to load them from the book.
    // onSaved(entries) is called after a successful save.
    constructor(bookId, host, entries, onSaved) {
        this.bookId = bookId;
        this.host = host;
        this.onSaved = onSaved || (() => {});
        this.entries = entries;
        this.mode = 'view';   // view | edit | preview
        this.rows = [];
        this.warnings = [];
        this.busy = false;
        this.error = '';
        if (entries === undefined) this.load(); else this.render();
    }

    esc(t) {
        const d = document.createElement('div');
        d.textContent = t == null ? '' : String(t);
        // textContent does not escape quotes, and these values also go inside value="...".
        return d.innerHTML.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    base() { return `/api/library/books/${encodeURIComponent(this.bookId)}/collects`; }

    async post(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(friendlyErrorText(res, text, `HTTP ${res.status}`));
        return JSON.parse(text);
    }

    async load() {
        this.entries = [];
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}`);
            if (res.ok) this.entries = (await res.json()).collects || [];
        } catch (e) {
            this.error = 'Could not load the current links.';
        }
        this.render();
    }

    blankRow() { return { volume: 0, series: '', from: '', to: '', kind: 'main', partial: false, note: '', source: 'manual', ref: '', info: '', lookupError: '' }; }

    startEdit() {
        this.rows = (this.entries || []).map(e => ({
            ...this.blankRow(), ...e, kind: e.kind || 'main',
            ref: e.volume ? `4050-${e.volume}` : '',
        }));
        if (!this.rows.length) this.rows = [this.blankRow()];
        this.mode = 'edit';
        this.error = '';
        this.render();
    }

    // Entry sent to the server: only the fields it knows.
    entryOf(r) {
        return { volume: r.volume || 0, series: r.series.trim(), from: r.from.trim(), to: r.to.trim(),
            kind: r.kind, partial: !!r.partial, note: (r.note || '').trim(), source: r.source || 'manual' };
    }

    // The server skips nothing: an empty row (no volume, name or number) is
    // dropped here so a stray "Add another series" click does not block a save.
    filledRows() { return this.rows.filter(r => r.volume || r.series.trim() || r.from.trim() || r.to.trim()); }

    async lookup(i) {
        const r = this.rows[i];
        if (!r.ref.trim() || this.busy) return;
        this.busy = true; r.lookupError = ''; this.render();
        try {
            const v = await this.post(this.base() + '/lookup', { ref: r.ref.trim() });
            r.volume = v.volume_id;
            r.series = r.series || v.name;
            r.info = `${v.name} (${v.start_year || '?'}) · ${v.publisher || 'unknown publisher'} · ${v.issue_count || v.numbers.length} issues`;
        } catch (e) {
            r.lookupError = e.message;
        }
        this.busy = false; this.render();
    }

    async preview() {
        const rows = this.filledRows();
        if (!rows.length) { this.error = 'Add at least one row, or use Clear all.'; this.render(); return; }
        this.busy = true; this.error = ''; this.render();
        try {
            const out = await this.post(this.base(), { entries: rows.map(r => this.entryOf(r)), preview: true });
            this.rows = rows;
            this.warnings = out.warnings || [];
            this.mode = 'preview';
        } catch (e) {
            this.error = e.message;
        }
        this.busy = false; this.render();
    }

    async save(entries) {
        this.busy = true; this.error = ''; this.render();
        try {
            const out = await this.post(this.base(), { entries });
            this.entries = out.collects || [];
            this.mode = 'view';
            if (typeof dialogs !== 'undefined') dialogs.toast(entries.length ? 'Links saved.' : 'Links cleared.', 'success');
            this.busy = false; this.render();
            this.onSaved(this.entries);
            return;
        } catch (e) {
            this.error = e.message;
        }
        this.busy = false; this.render();
    }

    async clearAll() {
        const ok = await dialogs.confirm({ message: 'Remove every link from this book?' });
        if (ok) this.save([]);
    }

    rangeText(e) { return e.to ? `#${this.esc(e.from)}–#${this.esc(e.to)}` : `#${this.esc(e.from)}`; }

    notes(e) {
        return [e.kind === 'backup' ? 'backup story' : '', e.partial ? 'partial' : '', e.source === 'suggested' ? 'suggested' : '']
            .filter(Boolean).map(n => `<span class="bd-collects-note">${n}</span>`).join(' ');
    }

    renderList(entries, warnings) {
        return `<ul class="bd-collects">${entries.map((e, i) => {
            const label = e.series || `ComicVine volume ${e.volume}`;
            const name = e.volume
                ? `<a href="https://comicvine.gamespot.com/volume/4050-${e.volume}/" target="_blank" rel="noopener noreferrer">${this.esc(label)}</a>`
                : this.esc(label);
            const w = ((warnings || [])[i] || []).map(x => `<div class="cx-warning">Check: ${this.esc(x)}</div>`).join('');
            const story = e.note ? ` <span class="bd-collects-story">“${this.esc(e.note)}”</span>` : '';
            return `<li class="bd-collects-row">${name} ${this.rangeText(e)} ${this.notes(e)}${story}${w}</li>`;
        }).join('')}</ul>`;
    }

    renderRow(r, i) {
        const dis = this.busy ? 'disabled' : '';
        return `<div class="cx-row" data-i="${i}">
            <div class="cx-line">
                <input type="text" class="cx-ref" placeholder="ComicVine volume link or id (optional)" value="${this.esc(r.ref)}" aria-label="ComicVine volume link or id">
                <button class="btn btn-small btn-secondary cx-lookup" ${dis}>Look up</button>
            </div>
            ${r.info ? `<div class="cx-info">${this.esc(r.info)}</div>` : ''}
            ${r.lookupError ? `<div class="cvm-error">${this.esc(r.lookupError)}</div>` : ''}
            <div class="cx-line">
                <input type="text" class="cx-series" placeholder="Series name" value="${this.esc(r.series)}" aria-label="Series name">
                <input type="text" class="cx-from" placeholder="From #" size="6" value="${this.esc(r.from)}" aria-label="First issue number">
                <input type="text" class="cx-to" placeholder="To #" size="6" value="${this.esc(r.to)}" aria-label="Last issue number (empty for one issue)">
                <select class="cx-kind" aria-label="Kind">
                    <option value="main" ${r.kind !== 'backup' ? 'selected' : ''}>Story</option>
                    <option value="backup" ${r.kind === 'backup' ? 'selected' : ''}>Backup story</option>
                </select>
                <label><input type="checkbox" class="cx-partial" ${r.partial ? 'checked' : ''}> partial</label>
                <button class="btn btn-small btn-secondary cx-remove" ${dis}>Remove</button>
            </div>
            <div class="cx-line">
                <input type="text" class="cx-note" maxlength="200" placeholder="Which story? (optional)" value="${this.esc(r.note)}" aria-label="Which story">
            </div>
        </div>`;
    }

    render() {
        const err = this.error ? `<p class="cvm-error">${this.esc(this.error)}</p>` : '';
        const dis = this.busy ? 'disabled' : '';
        let html;
        if (this.mode === 'edit') {
            html = `${this.rows.map((r, i) => this.renderRow(r, i)).join('')}
                <div class="cx-actions">
                    <button class="btn btn-secondary cx-add" ${dis}>Add another series</button>
                    <button class="btn btn-primary cx-preview" ${dis}>Preview</button>
                    <button class="btn btn-secondary cx-cancel" ${dis}>Cancel</button>
                </div>${err}`;
        } else if (this.mode === 'preview') {
            const entries = this.rows.map(r => this.entryOf(r));
            const flagged = this.warnings.some(w => w && w.length);
            html = `<p>${flagged ? 'Check the notes below, then confirm or go back.' : 'This is what will be saved.'}</p>
                ${this.renderList(entries, this.warnings)}
                <div class="cx-actions">
                    <button class="btn btn-primary cx-confirm" ${dis}>Confirm</button>
                    <button class="btn btn-secondary cx-back" ${dis}>Back to edit</button>
                </div>${err}`;
        } else {
            const list = this.entries || [];
            html = `${list.length ? this.renderList(list) : '<p>Not linked yet.</p>'}
                <div class="cx-actions">
                    <button class="btn btn-secondary cx-edit" ${dis}>${list.length ? 'Edit links' : 'Link the issues it contains'}</button>
                    ${list.length ? `<button class="btn btn-secondary cx-clear" ${dis}>Clear all</button>` : ''}
                </div>${err}`;
        }
        this.host.innerHTML = `<div class="cx-box">${html}</div>`;
        this.bind();
    }

    bind() {
        const q = sel => this.host.querySelector(sel);
        const on = (sel, fn) => { const el = q(sel); if (el) el.addEventListener('click', fn); };
        on('.cx-edit', () => this.startEdit());
        on('.cx-clear', () => this.clearAll());
        on('.cx-cancel', () => { this.mode = 'view'; this.error = ''; this.render(); });
        on('.cx-back', () => { this.mode = 'edit'; this.error = ''; this.render(); });
        on('.cx-add', () => { this.rows.push(this.blankRow()); this.render(); });
        on('.cx-preview', () => this.preview());
        on('.cx-confirm', () => this.save(this.rows.map(r => this.entryOf(r))));
        this.host.querySelectorAll('.cx-row').forEach(el => {
            const i = parseInt(el.dataset.i, 10);
            const r = this.rows[i];
            const bind = (sel, key, isCheck) => {
                const input = el.querySelector(sel);
                input.addEventListener(isCheck ? 'change' : 'input', () => { r[key] = isCheck ? input.checked : input.value; });
            };
            bind('.cx-ref', 'ref'); bind('.cx-series', 'series'); bind('.cx-from', 'from');
            bind('.cx-to', 'to'); bind('.cx-kind', 'kind'); bind('.cx-partial', 'partial', true); bind('.cx-note', 'note');
            el.querySelector('.cx-ref').addEventListener('keydown', e => { if (e.key === 'Enter') this.lookup(i); });
            el.querySelector('.cx-lookup').addEventListener('click', () => this.lookup(i));
            el.querySelector('.cx-remove').addEventListener('click', () => {
                this.rows.splice(i, 1);
                if (!this.rows.length) this.rows.push(this.blankRow());
                this.render();
            });
        });
    }
}
