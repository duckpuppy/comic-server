// "Match by ComicVine link or ID" box (comic-server-5wer, phase 2). Used on the
// book page and the Scrape Review page. Paste an issue link/id: preview, then
// Confirm. Paste a volume link: pick one of its issues, preview, Confirm.
// Nothing is written until Confirm.
class CVMatchBox {
    // host: element to render into. onApplied: called after a successful apply.
    constructor(bookId, host, onApplied) {
        this.bookId = bookId;
        this.host = host;
        this.onApplied = onApplied || (() => {});
        this.state = null; // last preview response
        this.busy = false;
        this.error = '';
        this.render();
    }

    esc(t) {
        const d = document.createElement('div');
        d.textContent = t == null ? '' : String(t);
        return d.innerHTML;
    }

    async call(body) {
        const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/cv-match`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(friendlyErrorText(res, text, `HTTP ${res.status}`));
        return JSON.parse(text);
    }

    async run(body) {
        if (this.busy) return;
        this.busy = true;
        this.error = '';
        this.render();
        try {
            const r = await this.call(body);
            if (r.applied) {
                if (typeof dialogs !== 'undefined') dialogs.toast('Matched to ComicVine.', 'success');
                this.state = null;
                this.busy = false;
                this.render();
                this.onApplied(r);
                return;
            }
            this.state = r;
        } catch (e) {
            this.error = e.message;
            this.state = null;
        }
        this.busy = false;
        this.render();
    }

    renderPreview() {
        const r = this.state;
        if (!r) return '';
        const b = r.book || {};
        const mine = [b.number ? `#${b.number}` : '', b.year ? `Year ${b.year}` : '', b.page_count ? `${b.page_count} pages` : '', b.format || '']
            .filter(Boolean).join(' · ');
        const mineHtml = `<p class="cvm-mine">This book: ${this.esc(mine || 'no details')}</p>`;
        if (r.kind === 'volume') {
            const v = r.volume;
            const rows = (v.issues || []).map(i => `
                <tr><td>${this.esc(i.number)}</td><td>${this.esc(i.name)}</td><td>${this.esc(i.cover_date)}</td>
                <td><button class="btn btn-small btn-secondary cvm-pick" data-issue="${i.id}">Preview</button></td></tr>`).join('');
            return `<div class="cvm-preview">
                <p><strong>${this.esc(v.name)}</strong> (${this.esc(v.start_year)}) · ${this.esc(v.publisher)} · ${v.issue_count || (v.issues || []).length} issues
                ${v.volume_url ? ` · <a href="${this.esc(v.volume_url)}" target="_blank" rel="noopener noreferrer">ComicVine</a>` : ''}</p>
                ${mineHtml}
                <table class="cvm-table"><thead><tr><th>#</th><th>Name</th><th>Cover date</th><th></th></tr></thead><tbody>${rows}</tbody></table>
            </div>`;
        }
        const i = r.issue;
        return `<div class="cvm-preview cvm-issue">
            ${i.cover_url ? `<img class="cvm-cover" src="${this.esc(i.cover_url)}" alt="" referrerpolicy="no-referrer">` : ''}
            <div>
                <p><strong>${this.esc(i.volume_name)}</strong> #${this.esc(i.number)}${i.name ? ` - ${this.esc(i.name)}` : ''}</p>
                <p>${[i.cover_date, i.start_year ? `volume from ${i.start_year}` : '', i.publisher, i.issue_count ? `${i.issue_count} issues in volume` : ''].filter(Boolean).map(x => this.esc(x)).join(' · ')}
                ${i.issue_url ? ` · <a href="${this.esc(i.issue_url)}" target="_blank" rel="noopener noreferrer">ComicVine</a>` : ''}</p>
                ${mineHtml}
                <button class="btn btn-primary cvm-confirm" data-issue="${i.issue_id}">Use this issue</button>
            </div>
        </div>`;
    }

    render() {
        this.host.innerHTML = `
            <div class="cvm-box">
                <div class="cvm-row">
                    <input type="text" class="cvm-input" placeholder="ComicVine issue or volume link, or issue id" aria-label="ComicVine link or id" ${this.busy ? 'disabled' : ''}>
                    <button class="btn btn-secondary cvm-go" ${this.busy ? 'disabled' : ''}>${this.busy ? 'Working…' : 'Look up'}</button>
                </div>
                ${this.error ? `<p class="cvm-error">${this.esc(this.error)}</p>` : ''}
                ${this.renderPreview()}
            </div>`;
        const input = this.host.querySelector('.cvm-input');
        if (this.ref) input.value = this.ref;
        const go = () => { this.ref = input.value.trim(); this.run({ ref: this.ref }); };
        this.host.querySelector('.cvm-go').addEventListener('click', go);
        input.addEventListener('keydown', e => { if (e.key === 'Enter') go(); });
        this.host.querySelectorAll('.cvm-pick').forEach(btn =>
            btn.addEventListener('click', () => this.run({ issue_id: parseInt(btn.dataset.issue, 10) })));
        this.host.querySelectorAll('.cvm-confirm').forEach(btn =>
            btn.addEventListener('click', () => this.run({ issue_id: parseInt(btn.dataset.issue, 10), apply: true })));
    }
}
