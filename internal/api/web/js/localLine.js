// "Link this whole line to a ComicVine volume" (tracker #44). A book with a
// local identity has siblings that share its volume key (Vol 1, Vol 2 ...).
// When ComicVine finally lists the line, paste its volume link: each book is
// matched to the volume's issue with the same number, you confirm, and the
// real ComicVine match is applied book by book through the same endpoint as
// "Match by ComicVine link or ID". Collects links stay; the local identity is
// retired by each match. A paced 429 stops the run; press Continue later.
class LocalLinePanel {
    constructor(bookId, host, onDone) {
        this.bookId = bookId;
        this.host = host;
        this.onDone = onDone || (() => {});
        this.line = null;     // books sharing the volume key
        this.volume = null;   // volume preview from ComicVine
        this.rows = [];       // {id, number, title, issue, checked, state}
        this.ref = '';
        this.busy = false;
        this.error = '';
        this.render();
        this.load();
    }

    esc(t) {
        const d = document.createElement('div');
        d.textContent = t == null ? '' : String(t);
        return d.innerHTML;
    }

    async post(path, body) {
        const res = await fetch(`/api/library/books/${encodeURIComponent(path.id)}/${path.op}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(friendlyErrorText(res, text, `HTTP ${res.status}`));
        return JSON.parse(text);
    }

    norm(n) {
        const t = String(n == null ? '' : n).trim().toLowerCase();
        const f = Number(t);
        return t !== '' && !Number.isNaN(f) ? String(f) : t;
    }

    async load() {
        try {
            const r = await this.post({ id: this.bookId, op: 'local-identity' }, { line: true });
            this.line = (r.line || []).slice().sort((a, b) => (parseFloat(a.number) || 0) - (parseFloat(b.number) || 0));
        } catch (e) {
            this.error = e.message;
        }
        this.render();
    }

    async lookup() {
        if (!this.ref.trim() || this.busy) return;
        this.busy = true; this.error = ''; this.render();
        try {
            const r = await this.post({ id: this.bookId, op: 'cv-match' }, { ref: this.ref.trim() });
            if (r.kind !== 'volume') throw new Error('Paste a ComicVine volume link (it looks like 4050-12345), not an issue.');
            this.volume = r.volume;
            const byNumber = new Map();
            (r.volume.issues || []).forEach(i => { if (!byNumber.has(this.norm(i.number))) byNumber.set(this.norm(i.number), i); });
            this.rows = this.line.map(b => {
                const issue = b.number ? byNumber.get(this.norm(b.number)) : null;
                return { id: b.id, number: b.number, title: b.title, issue: issue || null, checked: !!issue, state: '' };
            });
        } catch (e) {
            this.error = e.message;
            this.volume = null;
        }
        this.busy = false; this.render();
    }

    // Own book last, so this panel stays until the run is over.
    async run() {
        if (this.busy) return;
        this.busy = true; this.error = ''; this.render();
        const todo = this.rows.filter(r => r.checked && r.issue && r.state !== 'done');
        todo.sort((a, b) => (a.id === this.bookId) - (b.id === this.bookId));
        for (const r of todo) {
            try {
                await this.post({ id: r.id, op: 'cv-match' }, { issue_id: r.issue.id, apply: true });
                r.state = 'done';
            } catch (e) {
                r.state = 'failed';
                this.error = `Stopped at #${r.number || '?'}: ${e.message}`;
                break;
            }
            this.render();
        }
        this.busy = false; this.render();
        if (!this.error && todo.length) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Linked ${todo.length} book${todo.length === 1 ? '' : 's'} to ComicVine.`, 'success');
            this.onDone();
        }
    }

    render() {
        const dis = this.busy ? 'disabled' : '';
        let body = '';
        if (!this.line) {
            body = this.error ? '' : '<p>Loading…</p>';
        } else if (!this.volume) {
            body = `<p>${this.line.length} book${this.line.length === 1 ? '' : 's'} share this local line. Paste the ComicVine volume that lists them.</p>
                <div class="cvm-row">
                    <input type="text" class="cvm-input ll-ref" placeholder="ComicVine volume link or id (4050-…)" value="${this.esc(this.ref)}" aria-label="ComicVine volume link or id" ${dis}>
                    <button class="btn btn-secondary ll-go" ${dis}>${this.busy ? 'Working…' : 'Look up'}</button>
                </div>`;
        } else {
            const v = this.volume;
            const rows = this.rows.map((r, i) => `
                <tr><td><input type="checkbox" class="ll-check" data-i="${i}" ${r.checked ? 'checked' : ''} ${r.issue && !this.busy && r.state !== 'done' ? '' : 'disabled'}></td>
                    <td>#${this.esc(r.number || '?')} ${this.esc(r.title || '')}</td>
                    <td>${r.issue ? `ComicVine #${this.esc(r.issue.number)} ${this.esc(r.issue.name || '')}` : '<em>no issue with this number in the volume</em>'}</td>
                    <td>${r.state === 'done' ? 'linked' : (r.state === 'failed' ? 'failed' : '')}</td></tr>`).join('');
            const pending = this.rows.filter(r => r.checked && r.issue && r.state !== 'done').length;
            body = `<p><strong>${this.esc(v.name)}</strong> (${this.esc(v.start_year)}) · ${this.esc(v.publisher)} · ${v.issue_count || (v.issues || []).length} issues</p>
                <table class="cvm-table"><thead><tr><th></th><th>This library</th><th>ComicVine issue</th><th></th></tr></thead><tbody>${rows}</tbody></table>
                <div class="cx-actions">
                    <button class="btn btn-primary ll-run" ${dis || pending === 0 ? 'disabled' : ''}>${this.rows.some(r => r.state) ? 'Continue' : 'Link'} ${pending} book${pending === 1 ? '' : 's'}</button>
                    <button class="btn btn-secondary ll-back" ${dis}>Choose another volume</button>
                </div>`;
        }
        this.host.innerHTML = `<div class="cvm-box">${body}${this.error ? `<p class="cvm-error">${this.esc(this.error)}</p>` : ''}</div>`;
        const q = s => this.host.querySelector(s);
        const ref = q('.ll-ref');
        if (ref) {
            ref.addEventListener('input', () => { this.ref = ref.value; });
            ref.addEventListener('keydown', e => { if (e.key === 'Enter') this.lookup(); });
        }
        const go = q('.ll-go'); if (go) go.addEventListener('click', () => this.lookup());
        const run = q('.ll-run'); if (run) run.addEventListener('click', () => this.run());
        const back = q('.ll-back'); if (back) back.addEventListener('click', () => { this.volume = null; this.rows = []; this.error = ''; this.render(); });
        this.host.querySelectorAll('.ll-check').forEach(cb => cb.addEventListener('change', () => {
            this.rows[parseInt(cb.dataset.i, 10)].checked = cb.checked;
            this.render();
        }));
    }
}
