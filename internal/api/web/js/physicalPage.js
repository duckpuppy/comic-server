// Paper copies (comic-server-vffq): add one by hand, or import a CSV with a
// preview first. A paper copy is a book with no file whose Collection Status
// is "Physical"; it counts as owned in Missing Issues and is not Wanted.
class PhysicalPage {
    constructor() {
        this.csv = '';
        this.preview = null;
        this.busy = false;
    }

    async init() {
        this.preview = null;
        this.render();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    // Badge for a book with no file: ComicPreview.kind / BookDetail.kind.
    static kindBadge(kind) {
        if (kind === 'physical') return '<span class="book-kind-badge book-kind-physical" title="Paper copy">Physical</span>';
        if (kind === 'wanted') return '<span class="book-kind-badge book-kind-wanted" title="No file yet">Wanted</span>';
        return '';
    }

    static browseHref() {
        const m = [{ Type: 'ComicBookBookCollectionStatusMatcher', Not: false, MatchOperator: '0', MatchValue: 'Physical', MatchValue2: '' }];
        return '/browse?m=' + encodeURIComponent(JSON.stringify(m));
    }

    render() {
        const field = (name, label, attrs = '') =>
            `<label class="physical-field"><span>${label}</span><input name="${name}" ${attrs}></label>`;
        document.getElementById('app').innerHTML = `
            <div class="physical-page">
                <h1>Paper Copies</h1>
                <p class="physical-subtitle">
                    Comics you own on paper. They are saved as books with no file and
                    Collection Status "Physical", count as owned in Missing Issues, and never
                    show as Wanted. <a href="${PhysicalPage.browseHref()}" data-route>Show paper copies in Browse</a>
                </p>

                <section class="physical-card">
                    <h2>Add one</h2>
                    <form id="physical-form" class="physical-form">
                        ${field('series', 'Series *', 'required')}
                        ${field('number', 'Number')}
                        ${field('volume', 'Volume', 'type="number" min="0" placeholder="e.g. 2012"')}
                        ${field('year', 'Year', 'type="number" min="0"')}
                        ${field('publisher', 'Publisher')}
                        ${field('title', 'Title')}
                        ${field('format', 'Format')}
                        ${field('location', 'Location', 'placeholder="e.g. Longbox 12"')}
                        ${field('condition', 'Condition', 'placeholder="e.g. NM"')}
                        ${field('comicvine_issue', 'ComicVine issue ID', 'type="number" min="0"')}
                        ${field('comicvine_volume', 'ComicVine volume ID', 'type="number" min="0"')}
                        <label class="physical-field physical-notes"><span>Notes</span><textarea name="notes" rows="2"></textarea></label>
                        <div class="physical-actions"><button type="submit" class="btn btn-primary">Add paper copy</button></div>
                    </form>
                </section>

                <section class="physical-card">
                    <h2>Import a CSV</h2>
                    <p class="physical-hint">
                        The first row names the columns, in any case: Series (required), Number / Issue,
                        Volume, Year, Publisher, Title, Format, Location / Storage / Box, Condition / Grade,
                        Notes, ComicVine Issue, ComicVine Volume. Other columns are ignored. A row that is
                        already a paper copy (same series, volume and number) is skipped, so importing the
                        same file twice adds nothing.
                    </p>
                    <input type="file" id="physical-file" accept=".csv,text/csv">
                    <textarea id="physical-csv" rows="6" placeholder="Series,Number,Volume,Location&#10;Saga,1,2012,Longbox 12">${this.escapeHtml(this.csv)}</textarea>
                    <div class="physical-actions">
                        <button id="physical-preview" class="btn btn-secondary" ${this.busy ? 'disabled' : ''}>Preview</button>
                        <button id="physical-apply" class="btn btn-primary" ${this.busy || !this.preview || this.preview.applied || !this.preview.new ? 'disabled' : ''}>
                            Import ${this.preview ? this.preview.new : ''} new
                        </button>
                    </div>
                    <div id="physical-preview-out">${this.renderPreview()}</div>
                </section>
            </div>`;
        this.attachListeners();
    }

    renderPreview() {
        const p = this.preview;
        if (!p) return '';
        const cols = Object.entries(p.columns || {}).map(([h, f]) => `${this.escapeHtml(h)} → ${f}`).join(', ');
        const label = { new: 'New', exists: 'Already there', error: 'Error', added: 'Added' };
        const rows = (p.rows || []).slice(0, 500).map(r => `
            <tr class="physical-row-${r.status}">
                <td>${r.line}</td>
                <td>${label[r.status] || r.status}${r.error ? ': ' + this.escapeHtml(r.error) : ''}</td>
                <td>${this.escapeHtml(r.book.series)}</td>
                <td>${this.escapeHtml(r.book.number)}</td>
                <td>${r.book.volume || ''}</td>
                <td>${this.escapeHtml(r.book.location)}</td>
            </tr>`).join('');
        return `
            <p class="physical-summary">
                ${p.applied ? `<strong>Added ${p.added}.</strong> ` : ''}
                ${p.new} new, ${p.exists} already there, ${p.errors} with errors.
                ${p.truncated ? 'Only the first 10,000 rows were read.' : ''}
            </p>
            <p class="physical-hint">Columns: ${cols || 'none'}${(p.ignored || []).length ? ` · Ignored: ${p.ignored.map(h => this.escapeHtml(h)).join(', ')}` : ''}</p>
            <div class="physical-table-wrap">
                <table class="physical-table">
                    <thead><tr><th>Line</th><th>Result</th><th>Series</th><th>#</th><th>Vol</th><th>Location</th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>
            ${(p.rows || []).length > 500 ? '<p class="physical-hint">First 500 rows shown.</p>' : ''}`;
    }

    attachListeners() {
        document.getElementById('physical-form').addEventListener('submit', e => {
            e.preventDefault();
            this.addOne(e.target);
        });
        const ta = document.getElementById('physical-csv');
        ta.addEventListener('input', () => {
            this.csv = ta.value;
            if (this.preview) { this.preview = null; this.render(); document.getElementById('physical-csv').focus(); }
        });
        document.getElementById('physical-file').addEventListener('change', async e => {
            const f = e.target.files[0];
            if (!f) return;
            this.csv = await f.text();
            this.preview = null;
            this.render();
        });
        document.getElementById('physical-preview').addEventListener('click', () => this.runImport(false));
        document.getElementById('physical-apply').addEventListener('click', () => {
            if (!this.preview) return;
            if (!confirm(`Add ${this.preview.new} paper cop${this.preview.new === 1 ? 'y' : 'ies'} to the library?`)) return;
            this.runImport(true);
        });
    }

    async addOne(form) {
        const data = Object.fromEntries(new FormData(form).entries());
        for (const k of ['volume', 'year', 'comicvine_issue', 'comicvine_volume']) {
            data[k] = parseInt(data[k], 10) || 0;
        }
        try {
            const res = await fetch('/api/library/physical', {
                method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data),
            });
            if (!res.ok) throw new Error(await res.text());
            dialogs.toast(`Added ${data.series} ${data.number ? '#' + data.number : ''}`, 'success');
            form.reset();
            form.querySelector('[name=series]').value = data.series; // next issue of the same run
            form.querySelector('[name=volume]').value = data.volume || '';
            form.querySelector('[name=publisher]').value = data.publisher;
            form.querySelector('[name=location]').value = data.location;
            form.querySelector('[name=number]').focus();
        } catch (e) {
            dialogs.toast(`Could not add: ${e.message || e}`, 'error', 6000);
        }
    }

    async runImport(apply) {
        this.csv = document.getElementById('physical-csv').value;
        if (!this.csv.trim()) {
            dialogs.toast('Paste a CSV or pick a file first', 'error');
            return;
        }
        this.busy = true;
        this.render();
        try {
            const res = await fetch('/api/library/physical/import', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ csv: this.csv, apply }),
            });
            if (!res.ok) throw new Error(await res.text());
            this.preview = await res.json();
            if (apply) dialogs.toast(`Added ${this.preview.added} paper cop${this.preview.added === 1 ? 'y' : 'ies'}`, 'success');
        } catch (e) {
            this.preview = null;
            dialogs.toast(`Import failed: ${e.message || e}`, 'error', 8000);
        } finally {
            this.busy = false;
            this.render();
        }
    }
}
