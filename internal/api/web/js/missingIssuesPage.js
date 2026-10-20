// Missing Issues page (comic-server-buhs): per ComicVine volume, which issues
// the library owns and which it does not, with a way to put missing ones on
// the Wanted list.
class MissingIssuesPage {
    constructor() {
        this.volumes = [];
        this.total = 0;
        this.offset = 0;
        this.limit = 50;
        this.q = '';
        this.onlyMissing = true;
        this.loading = true;
        this.error = '';
        this.notConfigured = false;
        this.detail = null;       // series-completeness detail object
        this.detailLoading = false;
        this.chosen = new Set();  // missing issue ids ticked in the detail view
        this.adding = false;
        this.message = '';
        this.debounce = null;
    }

    async init() {
        this.loading = true;
        this.render();
        await this.loadVolumes();
        this.render();
        // /missing-issues?volume=<id> opens that volume (book page link).
        const want = parseInt(new URLSearchParams(window.location.search).get('volume'), 10);
        if (want > 0) await this.open(want);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }

    async loadVolumes() {
        const params = new URLSearchParams({
            limit: this.limit, offset: this.offset, only_missing: this.onlyMissing ? '1' : '0',
        });
        if (this.q) params.set('q', this.q);
        try {
            const res = await fetch(`/api/library/series-completeness?${params}`);
            if (res.status === 503) {
                this.notConfigured = true;
                this.volumes = [];
            } else if (!res.ok) {
                throw new Error(`HTTP ${res.status}`);
            } else {
                const data = await res.json();
                this.notConfigured = false;
                this.volumes = data.volumes || [];
                this.total = data.total || 0;
            }
            this.error = '';
        } catch (e) {
            console.error('Failed to load series completeness:', e);
            this.error = 'Failed to load series.';
            this.volumes = [];
        }
        this.loading = false;
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        const active = document.activeElement;
        const focusId = active && active.id && app.contains(active) ? active.id : null;
        const caret = focusId && typeof active.selectionStart === 'number' ? active.selectionStart : null;

        app.innerHTML = this.detail || this.detailLoading ? this.renderDetail() : this.renderList();

        if (focusId) {
            const el = document.getElementById(focusId);
            if (el) {
                el.focus();
                if (caret !== null && el.setSelectionRange) {
                    try { el.setSelectionRange(caret, caret); } catch (e) { /* not a text input */ }
                }
            }
        }
        this.attach();
    }

    bar(owned, total) {
        const pct = total > 0 ? Math.round(owned * 100 / total) : 0;
        return `<span class="missing-bar" title="${owned} of ${total}"><span class="missing-bar-fill" style="width:${pct}%"></span></span>`;
    }

    renderList() {
        let body;
        if (this.loading) {
            body = '<p class="empty-message">Loading…</p>';
        } else if (this.notConfigured) {
            body = '<p class="empty-message">ComicVine enrichment is not configured (set server.comicvine_api_key).</p>';
        } else if (this.error) {
            body = `<p class="empty-message">${this.escapeHtml(this.error)}</p>`;
        } else if (this.volumes.length === 0) {
            body = '<p class="empty-message" id="missing-empty">No series to show. Volumes appear here once ComicVine has synced their issue lists.</p>';
        } else {
            const rows = this.volumes.map(v => `
                <tr class="missing-row" data-volume="${v.volume_id}">
                    <td><a href="#" class="missing-open" data-volume="${v.volume_id}">${this.escapeHtml(v.name)}</a>
                        <span class="empty-message">${this.escapeHtml(v.start_year || '')}</span></td>
                    <td>${this.escapeHtml(v.publisher || '')}</td>
                    <td>${this.bar(v.owned, v.total)}</td>
                    <td class="missing-num">${v.owned} / ${v.total}${v.covered ? ` <span class="empty-message" title="Inside a collected edition you own">+${v.covered} covered</span>` : ''}</td>
                    <td class="missing-num"><strong>${v.missing}</strong> missing</td>
                </tr>`).join('');
            body = `
                <table class="datamanager-diff-table missing-table">
                    <thead><tr><th>Series</th><th>Publisher</th><th></th><th>Owned</th><th></th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
                <div class="missing-pager">
                    <button class="btn btn-secondary" id="missing-prev" ${this.offset === 0 ? 'disabled' : ''}>&larr; Previous</button>
                    <span class="empty-message">${this.offset + 1}–${this.offset + this.volumes.length} of ${this.total}</span>
                    <button class="btn btn-secondary" id="missing-next" ${this.offset + this.volumes.length >= this.total ? 'disabled' : ''}>Next &rarr;</button>
                </div>`;
        }
        return `
            <div class="missing-page">
                <div class="missing-header">
                    <h1>Missing Issues</h1>
                    <p class="missing-subtitle">Series you own part of, from ComicVine's issue lists. Most missing first.</p>
                </div>
                <div class="missing-filters">
                    <input type="text" id="missing-q" value="${this.escapeAttr(this.q)}" placeholder="Filter by series name">
                    <label><input type="checkbox" id="missing-only" ${this.onlyMissing ? 'checked' : ''}> Only series with missing issues</label>
                </div>
                ${this.message ? `<div class="panel"><p>${this.escapeHtml(this.message)}</p></div>` : ''}
                <div class="panel">${body}</div>
            </div>`;
    }

    renderDetail() {
        if (this.detailLoading || !this.detail) {
            return `<div class="missing-page"><div class="panel"><p class="empty-message">Loading…</p></div></div>`;
        }
        const d = this.detail;
        const cells = d.issues.map(i => {
            const state = i.owned ? 'owned' : (i.covered ? 'covered' : (i.wanted ? 'wanted' : 'missing'));
            const selectable = state === 'missing';
            const label = i.number || '?';
            return `
                <label class="missing-issue missing-issue-${state}" title="${this.escapeAttr(i.name || '')}${i.cover_date ? ' · ' + this.escapeAttr(i.cover_date) : ''}">
                    ${selectable ? `<input type="checkbox" class="missing-check" data-issue="${i.issue_id}" ${this.chosen.has(i.issue_id) ? 'checked' : ''}>` : ''}
                    <span class="missing-issue-num">#${this.escapeHtml(label)}</span>
                    <span class="missing-issue-state">${state === 'covered'
                        ? `<a href="/books/${encodeURIComponent(i.covered_by)}" data-route title="Inside a collected edition you own">covered</a>`
                        : (state === 'wanted' ? 'wanted' : (state === 'owned' ? 'owned' : ''))}</span>
                </label>`;
        }).join('');
        const missingIds = d.issues.filter(i => !i.owned && !i.covered && !i.wanted).map(i => i.issue_id);
        return `
            <div class="missing-page">
                <div class="missing-header">
                    <button class="btn btn-secondary" id="missing-back">&larr; Back</button>
                    <h1>${this.escapeHtml(d.name)} <span class="empty-message">${this.escapeHtml(d.start_year || '')} · ${this.escapeHtml(d.publisher || '')}</span></h1>
                </div>
                <div class="panel">
                    <p class="missing-summary">${this.bar(d.owned, d.total)}
                        <strong>${d.owned}</strong> of ${d.total} owned (${d.percent}%) ·
                        ${d.covered ? `<strong>${d.covered}</strong> covered by a collected edition · ` : ''}<strong>${d.missing}</strong> missing${d.wanted ? ` (${d.wanted} already wanted)` : ''}
                        ${d.unmatched_owned ? ` · ${d.unmatched_owned} owned book${d.unmatched_owned === 1 ? '' : 's'} not matched to an issue` : ''}</p>
                    ${this.message ? `<p>${this.escapeHtml(this.message)}</p>` : ''}
                    <div class="datamanager-actions">
                        <button class="btn btn-secondary" id="missing-select-all" ${missingIds.length === 0 ? 'disabled' : ''}>Select all missing</button>
                        <button class="btn btn-primary" id="missing-add-wanted" ${this.chosen.size === 0 || this.adding ? 'disabled' : ''}>
                            ${this.adding ? 'Adding…' : `Add ${this.chosen.size} to Wanted`}
                        </button>
                    </div>
                    <div class="missing-grid">${cells}</div>
                </div>
            </div>`;
    }

    attach() {
        const q = document.getElementById('missing-q');
        if (q) q.addEventListener('input', () => {
            this.q = q.value;
            clearTimeout(this.debounce);
            this.debounce = setTimeout(() => this.reload(), 300);
        });
        const only = document.getElementById('missing-only');
        if (only) only.addEventListener('change', () => { this.onlyMissing = only.checked; this.reload(); });
        const prev = document.getElementById('missing-prev');
        if (prev) prev.addEventListener('click', () => { this.offset = Math.max(0, this.offset - this.limit); this.reload(false); });
        const next = document.getElementById('missing-next');
        if (next) next.addEventListener('click', () => { this.offset += this.limit; this.reload(false); });
        document.querySelectorAll('.missing-open').forEach(a => a.addEventListener('click', ev => {
            ev.preventDefault();
            this.open(parseInt(a.dataset.volume, 10));
        }));
        const back = document.getElementById('missing-back');
        if (back) back.addEventListener('click', async () => {
            this.detail = null;
            this.chosen = new Set();
            this.message = '';
            this.loading = true;
            this.render();
            await this.loadVolumes();
            this.render();
        });
        document.querySelectorAll('.missing-check').forEach(cb => cb.addEventListener('change', () => {
            const id = parseInt(cb.dataset.issue, 10);
            if (cb.checked) this.chosen.add(id); else this.chosen.delete(id);
            this.render();
        }));
        const all = document.getElementById('missing-select-all');
        if (all) all.addEventListener('click', () => {
            this.detail.issues.filter(i => !i.owned && !i.covered && !i.wanted).forEach(i => this.chosen.add(i.issue_id));
            this.render();
        });
        const add = document.getElementById('missing-add-wanted');
        if (add) add.addEventListener('click', () => this.addWanted());
    }

    async reload(resetOffset = true) {
        if (resetOffset) this.offset = 0;
        await this.loadVolumes();
        this.render();
    }

    async open(volumeId) {
        this.detail = null;
        this.detailLoading = true;
        this.chosen = new Set();
        this.message = '';
        this.render();
        await this.loadDetail(volumeId);
    }

    async loadDetail(volumeId) {
        try {
            const res = await fetch(`/api/library/series-completeness/${volumeId}`);
            const text = await res.text();
            if (!res.ok) {
                this.message = text.trim() || `HTTP ${res.status}`;
                this.detail = null;
            } else {
                this.detail = JSON.parse(text);
            }
        } catch (e) {
            console.error('Failed to load series detail:', e);
            this.message = 'Could not reach the server.';
        }
        this.detailLoading = false;
        // Fall back to the list when the detail could not be loaded.
        this.render();
    }

    async addWanted() {
        if (!this.detail || this.chosen.size === 0 || this.adding) return;
        this.adding = true;
        this.render();
        const volumeId = this.detail.volume_id;
        try {
            const res = await fetch(`/api/library/series-completeness/${volumeId}/wanted`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ issue_ids: Array.from(this.chosen) }),
            });
            const text = await res.text();
            if (!res.ok) {
                this.message = text.trim() || `HTTP ${res.status}`;
            } else {
                const r = JSON.parse(text);
                this.message = `Added ${r.added} to Wanted` + (r.skipped ? `, ${r.skipped} skipped` : '') +
                    (r.errors && r.errors.length ? `. ${r.errors.length} failed.` : '.');
                this.chosen = new Set();
            }
        } catch (e) {
            console.error('Add to wanted failed:', e);
            this.message = 'Could not reach the server.';
        }
        this.adding = false;
        await this.loadDetail(volumeId);
    }
}
