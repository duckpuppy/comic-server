// Duplicate review (comic-server-as8l): groups of suspected duplicate
// books side by side, with Keep / Keep and trash the rest / Keep all.
// "Trash" quarantines through internal/trash - recoverable from /trash.
class DuplicatesPage {
    constructor() {
        this.groups = [];
        this.total = 0;
        this.covers = false;
        this.loading = true;
        this.error = null;
    }

    async init(ctx) {
        this.loading = true;
        this.render();
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
    }

    async load() {
        this.error = null;
        try {
            const res = await fetch(`/api/library/duplicates?limit=100${this.covers ? '&covers=1' : ''}`);
            if (!res.ok) throw new Error(await res.text());
            const data = await res.json();
            this.groups = data.groups || [];
            this.total = data.total || 0;
        } catch (e) {
            console.error('Failed to load duplicates:', e);
            this.error = String(e.message || e);
            this.groups = [];
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

    formatSize(bytes) {
        if (!bytes) return '-';
        if (bytes < 1024) return `${bytes} B`;
        const units = ['KB', 'MB', 'GB'];
        let v = bytes / 1024, u = 0;
        while (v >= 1024 && u < units.length - 1) { v /= 1024; u++; }
        return `${v.toFixed(1)} ${units[u]}`;
    }

    reasonLabel(r) {
        return ({
            comicvine_issue: 'Same ComicVine issue',
            series_number: 'Same series + number',
            cover: 'Matching cover',
            filename_copy: 'Copy-style filename',
        })[r] || r;
    }

    // Suggested keeper: most pages, then largest file.
    suggestedKeeper(group) {
        return group.books.reduce((best, b) => {
            if (!best) return b;
            if (b.page_count !== best.page_count) return b.page_count > best.page_count ? b : best;
            return b.file_size > best.file_size ? b : best;
        }, null).id;
    }

    render() {
        const app = document.getElementById('app');
        const body = this.loading
            ? '<p class="empty-message">Loading…</p>'
            : this.error
                ? `<p class="empty-message">Failed to load duplicates: ${this.escapeHtml(this.error)}</p>`
                : this.groups.length === 0
                    ? '<p class="empty-message">No suspected duplicates.</p>'
                    : this.groups.map(g => this.renderGroup(g)).join('');
        app.innerHTML = `
            <div class="duplicates-page">
                <div class="duplicates-header">
                    <h1>Duplicate Review</h1>
                    <p class="duplicates-subtitle">
                        Books that look like copies of each other. Trashed files are quarantined
                        (see Trash), never deleted outright. Decisions are remembered.
                    </p>
                    <label class="duplicates-covers-toggle">
                        <input type="checkbox" id="dup-covers" ${this.covers ? 'checked' : ''}>
                        Also compare cover images within a series (slower)
                    </label>
                    <span class="duplicates-count">${this.total} group${this.total === 1 ? '' : 's'}</span>
                </div>
                ${body}
            </div>
        `;
        this.attachListeners();
    }

    renderGroup(g) {
        const keeper = this.suggestedKeeper(g);
        const reasons = g.reasons.map(r => `<span class="dup-reason">${this.escapeHtml(this.reasonLabel(r))}</span>`).join('');
        const books = g.books.map(b => `
            <label class="dup-book">
                <input type="radio" name="keep-${this.escapeHtml(g.key)}" value="${this.escapeHtml(b.id)}" ${b.id === keeper ? 'checked' : ''}>
                <img class="dup-cover" loading="lazy" src="${this.escapeHtml(b.cover_url)}" alt="">
                <div class="dup-meta">
                    <div class="dup-title">${this.escapeHtml(b.series)} #${this.escapeHtml(b.number)}${b.volume ? ` (vol ${b.volume})` : ''}</div>
                    <div>${this.escapeHtml(b.title)}</div>
                    <div class="dup-sub">${b.page_count} pages &middot; ${this.formatSize(b.file_size)}${b.year ? ` &middot; ${b.year}` : ''}</div>
                    <div class="dup-path" title="${this.escapeHtml(b.file_path)}">${this.escapeHtml(b.file_path)}</div>
                </div>
            </label>
        `).join('');
        return `
            <div class="dup-group" data-key="${this.escapeHtml(g.key)}">
                <div class="dup-group-header">${reasons}</div>
                <div class="dup-books">${books}</div>
                <div class="dup-actions">
                    <button class="btn btn-secondary" data-act="keep">Keep selected</button>
                    <button class="btn btn-danger" data-act="keep_trash">Keep selected and trash the rest</button>
                    <button class="btn btn-secondary" data-act="keep_all">Keep all (not duplicates)</button>
                </div>
            </div>
        `;
    }

    attachListeners() {
        const toggle = document.getElementById('dup-covers');
        if (toggle) {
            toggle.addEventListener('change', async () => {
                this.covers = toggle.checked;
                this.loading = true;
                this.render();
                await this.load();
                this.render();
            });
        }
        document.querySelectorAll('.dup-group [data-act]').forEach(btn => {
            btn.addEventListener('click', () => {
                const groupEl = btn.closest('.dup-group');
                this.resolve(groupEl, btn.dataset.act);
            });
        });
    }

    async resolve(groupEl, action) {
        const key = groupEl.dataset.key;
        const checked = groupEl.querySelector('input[type=radio]:checked');
        const keepId = checked ? checked.value : '';
        if (action === 'keep_trash') {
            const ok = await dialogs.confirm({
                title: 'Trash duplicates',
                message: 'Quarantine every other file in this group? They can be restored from Trash until the retention period ends.',
                confirmLabel: 'Keep selected, trash the rest',
                danger: true,
            });
            if (!ok) return;
        }
        try {
            const res = await fetch('/api/library/duplicates/resolve', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ group_key: key, action, keep_id: keepId, covers: this.covers }),
            });
            const text = await res.text();
            if (!res.ok) throw new Error(text || res.statusText);
            const result = JSON.parse(text);
            if (result.errors && result.errors.length > 0) {
                dialogs.toast(`Some files could not be trashed:\n${result.errors.join('\n')}`, 'error', 8000);
            } else if (action === 'keep_trash') {
                dialogs.toast(`Trashed ${(result.trashed || []).length} file(s)`, 'success');
            }
        } catch (e) {
            dialogs.toast(`Failed to resolve: ${e.message || e}`, 'error', 6000);
        }
        await this.load();
        this.render();
    }
}
