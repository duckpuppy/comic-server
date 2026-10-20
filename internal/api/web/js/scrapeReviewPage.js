// Scrape Review page - books whose ComicVine match was ambiguous and need a
// human pick (comic-server-0nhp). Native replacement for ComicRack's
// "05 Proposed Values" review step.
class ScrapeReviewPage {
    constructor() {
        this.items = [];
        this.loading = true;
        this.notConfigured = false;
        this.busy = new Set(); // book ids with an accept in flight
        this.selected = new Set(); // book ids ticked for a bulk "not in ComicVine"
        this.bulkBusy = false;
        this.error = '';
    }

    async init(ctx) {
        ScrapeReviewPage.active = this;
        this.loading = true;
        this.render();
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
    }

    async load() {
        try {
            const res = await fetch('/api/scrape/review');
            if (res.status === 503) {
                this.notConfigured = true;
                this.items = [];
            } else if (!res.ok) {
                throw new Error(`HTTP ${res.status}`);
            } else {
                this.notConfigured = false;
                this.items = (await res.json()) || [];
                const ids = new Set(this.items.map(i => i.book_id));
                this.selected = new Set([...this.selected].filter(id => ids.has(id)));
            }
            this.error = '';
        } catch (e) {
            console.error('Failed to load scrape review queue:', e);
            this.error = 'Failed to load the review queue.';
            this.items = [];
        } finally {
            this.loading = false;
            if (typeof navigation !== 'undefined') {
                navigation.updateBadge('scrape-review', this.items.length);
            }
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        let body;
        if (this.loading) {
            body = '<p class="empty-message">Loading…</p>';
        } else if (this.notConfigured) {
            body = '<p class="empty-message">The ComicVine scraper is not configured (set server.comicvine_api_key).</p>';
        } else if (this.error) {
            body = `<p class="empty-message">${this.escapeHtml(this.error)}</p>`;
        } else if (this.items.length === 0) {
            body = '<p class="empty-message" id="scrape-review-empty">Nothing is waiting for review.</p>';
        } else {
            body = this.renderBulkBar() + this.items.map(i => this.renderItem(i)).join('');
        }
        app.innerHTML = `
            <div class="scrape-review-page">
                <div class="scrape-review-header">
                    <h1>Scrape Review <span class="scrape-review-count" id="scrape-review-total">${this.loading ? '' : this.items.length}</span></h1>
                    <p class="scrape-review-subtitle">These books matched more than one ComicVine volume. Pick the right one.</p>
                </div>
                <div id="scrape-review-list">${body}</div>
            </div>
        `;
        this.attachListeners();
    }

    // Bulk "not in ComicVine" for the ticked items (comic-server-5wer phase 5).
    renderBulkBar() {
        const n = this.selected.size;
        const all = n > 0 && n === this.items.filter(i => !i.local_recheck).length;
        return `
            <div class="scrape-review-bulk">
                <label><input type="checkbox" id="scrape-review-select-all" ${all ? 'checked' : ''}> Select all</label>
                <span class="scrape-review-bulk-count">${n} selected</span>
                <select id="scrape-review-bulk-reason" aria-label="Why they are not in ComicVine">
                    ${ScrapeReviewPage.NO_MATCH_REASONS.map(r => `<option value="${r[0]}" ${r[0] === this.bulkReason ? 'selected' : ''}>${r[1]}</option>`).join('')}
                </select>
                <button class="btn btn-small btn-secondary" id="scrape-review-bulk-nomatch" ${n === 0 || this.bulkBusy ? 'disabled' : ''}>Mark selected as not in ComicVine</button>
            </div>`;
    }

    renderItem(item) {
        const cands = item.candidates || [];
        const busy = this.busy.has(item.book_id);
        const rows = cands.length === 0
            ? '<tr><td colspan="8" class="empty-message">No candidates recorded.</td></tr>'
            : cands.map(c => `
                <tr>
                    <td>${c.cover_url ? `<img class="scrape-review-cover" src="${this.escapeHtml(c.cover_url)}" alt="" loading="lazy" referrerpolicy="no-referrer">` : ''}</td>
                    <td>${c.site_url ? `<a href="${this.escapeHtml(c.site_url)}" target="_blank" rel="noopener noreferrer">${this.escapeHtml(c.name)}</a>` : this.escapeHtml(c.name)}${c.deck ? `<div class="scrape-review-deck">${this.escapeHtml(c.deck)}</div>` : ''}</td>
                    <td>${this.escapeHtml(c.start_year)}</td>
                    <td>${c.issue_count || ''}</td>
                    <td>${this.escapeHtml(c.publisher)}</td>
                    <td>${(c.score ?? 0).toFixed(2)}</td>
                    <td><span class="scrape-review-confidence conf-${this.escapeHtml(c.confidence)}">${this.escapeHtml(c.confidence)}</span></td>
                    <td>${item.needs_issue && item.only_issues && item.only_issues[c.volume_id]
                        ? `<button class="btn btn-small btn-primary scrape-review-only"
                                data-book="${this.escapeHtml(item.book_id)}"
                                data-issue="${item.only_issues[c.volume_id].issue_id}" ${busy ? 'disabled' : ''}
                                title="This volume has one issue. Preview it, then confirm.">Use #${this.escapeHtml(item.only_issues[c.volume_id].number || '?')}…</button>`
                        : item.needs_issue
                        ? `<button class="btn btn-small btn-primary scrape-review-choose"
                                data-book="${this.escapeHtml(item.book_id)}"
                                data-volume="${c.volume_id}" ${busy ? 'disabled' : ''}>Choose issue…</button>`
                        : `<button class="btn btn-small btn-primary scrape-review-accept"
                                data-book="${this.escapeHtml(item.book_id)}"
                                data-volume="${c.volume_id}" ${busy ? 'disabled' : ''}>Accept</button>
                           <button class="btn btn-small btn-secondary scrape-review-choose"
                                data-book="${this.escapeHtml(item.book_id)}"
                                data-volume="${c.volume_id}" ${busy ? 'disabled' : ''}>Issues…</button>`}</td>
                </tr>`).join('');
        const b = item.book;
        const facts = b ? [
            b.number ? `#${b.number}` : '',
            b.year ? `Year ${b.year}` : '',
            b.page_count ? `${b.page_count} pages` : '',
            b.format || '',
            b.publisher || '',
        ].filter(Boolean).join(' · ') : '';
        return `
            <section class="scrape-review-item" data-book="${this.escapeHtml(item.book_id)}">
                <div class="scrape-review-book">
                    ${item.local_recheck ? '' : `<input type="checkbox" class="scrape-review-select" data-book="${this.escapeHtml(item.book_id)}" ${this.selected.has(item.book_id) ? 'checked' : ''} aria-label="Select for bulk action">`}
                    <img class="scrape-review-cover" src="/api/library/books/${encodeURIComponent(item.book_id)}/cover" alt="" loading="lazy">
                    <div>
                        <h2><a href="/books/${encodeURIComponent(item.book_id)}" data-route>${this.escapeHtml(item.series || item.filename)}</a></h2>
                        ${facts ? `<p class="scrape-review-facts">${this.escapeHtml(facts)}</p>` : ''}
                        <p class="scrape-review-file" title="${this.escapeHtml(item.filename)}">${this.escapeHtml(item.filename)}</p>
                    </div>
                </div>
                ${item.needs_issue ? '<p class="scrape-review-hint">This book has no issue number, so it is never matched on its own. Choose the volume, then the issue.</p>' : ''}
                ${item.local_recheck ? `
                <p class="scrape-review-hint scrape-review-recheck">Suggestion from the background re-check: this book has a local identity, and ComicVine may now list it. Accepting replaces the local identity with the ComicVine tags; nothing changes until you accept.
                    <button class="btn btn-small btn-secondary scrape-review-recheck-dismiss" data-book="${this.escapeHtml(item.book_id)}" ${busy ? 'disabled' : ''} title="Keep the local identity and do not suggest this book again">Dismiss - keep local identity</button></p>` : `
                <div class="scrape-review-nomatch">
                    <select class="scrape-review-reason" aria-label="Why it is not in ComicVine">
                        ${ScrapeReviewPage.NO_MATCH_REASONS.map(r => `<option value="${r[0]}">${r[1]}</option>`).join('')}
                    </select>
                    <button class="btn btn-small btn-secondary scrape-review-nomatch-btn" data-book="${this.escapeHtml(item.book_id)}" ${busy ? 'disabled' : ''}>None of these - not in ComicVine</button>
                    <button class="btn btn-small btn-secondary scrape-review-rescrape" data-book="${this.escapeHtml(item.book_id)}" ${busy ? 'disabled' : ''} title="Search ComicVine again and replace these candidates">Re-scrape</button>
                </div>`}
                <details class="scrape-review-cvmatch" ${this.matchBoxes && this.matchBoxes[item.book_id] && this.matchBoxes[item.book_id].state ? 'open' : ''}><summary>Match by ComicVine link or ID</summary><div class="scrape-review-cvmatch-host" data-book="${this.escapeHtml(item.book_id)}"></div></details>
                <details class="scrape-review-collects"><summary>This is a collected edition: link the issues it contains</summary><div class="scrape-review-collects-host" data-book="${this.escapeHtml(item.book_id)}"></div></details>
                <table class="scrape-review-table">
                    <thead><tr><th>Cover</th><th>Volume</th><th>Start year</th><th>Issues</th><th>Publisher</th><th>Score</th><th>Confidence</th><th></th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </section>`;
    }

    // openMatch opens a book's match box on ref and runs its preview.
    openMatch(bookId, ref) {
        const box = this.matchBoxes && this.matchBoxes[bookId];
        if (!box) return;
        const details = box.host.closest('details');
        if (details) details.open = true;
        box.ref = ref;
        box.run({ ref });
    }

    attachListeners() {
        document.querySelectorAll('.scrape-review-choose').forEach(btn => {
            btn.addEventListener('click', () => this.openMatch(btn.dataset.book, `4050-${btn.dataset.volume}`));
        });
        document.querySelectorAll('.scrape-review-only').forEach(btn => {
            btn.addEventListener('click', () => this.openMatch(btn.dataset.book, `4000-${btn.dataset.issue}`));
        });
        document.querySelectorAll('.scrape-review-select').forEach(cb => {
            cb.addEventListener('change', () => {
                if (cb.checked) this.selected.add(cb.dataset.book); else this.selected.delete(cb.dataset.book);
                this.render();
            });
        });
        const all = document.getElementById('scrape-review-select-all');
        if (all) all.addEventListener('change', () => {
            this.selected = all.checked ? new Set(this.items.filter(i => !i.local_recheck).map(i => i.book_id)) : new Set();
            this.render();
        });
        const reason = document.getElementById('scrape-review-bulk-reason');
        if (reason) reason.addEventListener('change', () => { this.bulkReason = reason.value; });
        const bulkBtn = document.getElementById('scrape-review-bulk-nomatch');
        if (bulkBtn) bulkBtn.addEventListener('click', () => this.bulkNoMatch(reason ? reason.value : 'other'));
        document.querySelectorAll('.scrape-review-cvmatch-host').forEach(host => {
            const bookId = host.dataset.book;
            const prior = this.matchBoxes && this.matchBoxes[bookId];
            if (typeof CVMatchBox === 'undefined') return;
            const box = new CVMatchBox(bookId, host, () => {
                this.items = this.items.filter(i => i.book_id !== bookId);
                if (typeof navigation !== 'undefined') navigation.updateBadge('scrape-review', this.items.length);
                this.render();
            });
            if (prior) { box.ref = prior.ref; box.state = prior.state; box.render(); }
            (this.matchBoxes = this.matchBoxes || {})[bookId] = box;
        });
        document.querySelectorAll('.scrape-review-collects-host').forEach(host => {
            if (typeof CollectsEditor === 'undefined') return;
            // Load the current links only when the section is first opened.
            const details = host.closest('details');
            const start = () => { if (!host.dataset.started) { host.dataset.started = '1'; new CollectsEditor(host.dataset.book, host); } };
            if (details.open) start(); else details.addEventListener('toggle', start, { once: true });
        });
        document.querySelectorAll('.scrape-review-nomatch-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const sel = btn.parentElement.querySelector('.scrape-review-reason');
                this.markNoMatch(btn.dataset.book, sel ? sel.value : 'other');
            });
        });
        document.querySelectorAll('.scrape-review-recheck-dismiss').forEach(btn => {
            btn.addEventListener('click', () => this.dismissRecheck(btn.dataset.book));
        });
        document.querySelectorAll('.scrape-review-rescrape').forEach(btn => {
            btn.addEventListener('click', () => this.rescrape(btn.dataset.book));
        });
        document.querySelectorAll('.scrape-review-accept').forEach(btn => {
            btn.addEventListener('click', () =>
                this.accept(btn.dataset.book, parseInt(btn.dataset.volume, 10)));
        });
    }

    // Dismiss a background re-check suggestion: the local identity stays and
    // the book is not suggested again (tracker #47).
    async dismissRecheck(bookId) {
        if (this.busy.has(bookId)) return;
        this.busy.add(bookId);
        this.render();
        try {
            const res = await fetch(`/api/scrape/recheck/dismiss/${encodeURIComponent(bookId)}`, { method: 'POST' });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            this.items = this.items.filter(i => i.book_id !== bookId);
            this.selected.delete(bookId);
            if (typeof dialogs !== 'undefined') dialogs.toast('Dismissed. The local identity stays.', 'success');
            if (typeof navigation !== 'undefined') navigation.updateBadge('scrape-review', this.items.length);
        } catch (e) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Could not dismiss: ${e.message}`, 'error', 8000);
        } finally {
            this.busy.delete(bookId);
            this.render();
        }
    }

    // "None of these": record the decision (comic-server-5wer) and drop the item.
    async markNoMatch(bookId, reason) {
        if (this.busy.has(bookId)) return;
        this.busy.add(bookId);
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(bookId)}/cv-nomatch`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ set: true, reason }),
            });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            this.items = this.items.filter(i => i.book_id !== bookId);
            if (typeof dialogs !== 'undefined') dialogs.toast('Marked as not in ComicVine.', 'success');
            if (typeof navigation !== 'undefined') navigation.updateBadge('scrape-review', this.items.length);
        } catch (e) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
        } finally {
            this.busy.delete(bookId);
            this.render();
        }
    }

    // Mark every ticked item as not in ComicVine in one request.
    async bulkNoMatch(reason) {
        const ids = [...this.selected];
        if (ids.length === 0 || this.bulkBusy) return;
        if (!confirm(`Mark ${ids.length} book${ids.length === 1 ? '' : 's'} as not in ComicVine? Whole-library scrapes will skip them. You can undo this from the workflow page.`)) return;
        this.bulkBusy = true;
        this.render();
        try {
            const res = await fetch('/api/library/workflow/cv-nomatch', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_ids: ids, set: true, reason }),
            });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            const out = await res.json();
            const skipped = new Set((out.skipped || []).map(x => x.book_id));
            this.items = this.items.filter(i => !this.selected.has(i.book_id) || skipped.has(i.book_id));
            this.selected = new Set([...skipped].filter(id => this.selected.has(id)));
            const n = out.changed + out.unchanged;
            let msg = `Marked ${n} book${n === 1 ? '' : 's'} as not in ComicVine.`;
            if (skipped.size) msg += ` ${skipped.size} could not be marked (${out.skipped[0].reason}).`;
            if (typeof dialogs !== 'undefined') dialogs.toast(msg, skipped.size ? 'error' : 'success', skipped.size ? 8000 : 4000);
            if (typeof navigation !== 'undefined') navigation.updateBadge('scrape-review', this.items.length);
        } catch (e) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
        } finally {
            this.bulkBusy = false;
            this.render();
        }
    }

    // Re-scrape one book: a fresh search replaces its stored candidates
    // (comic-server-5wer phase 4). A live log shows the search as it runs;
    // the page refreshes on the scrape websocket events.
    async rescrape(bookId) {
        if (this.busy.has(bookId)) return;
        this.busy.add(bookId);
        this.render();
        try {
            const res = await fetch('/api/scrape', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_ids: [bookId] }),
            });
            if (res.status === 409) throw new Error('a scrape is already running. Try again when it finishes.');
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            const started = await res.json();
            ScrapeLog.open(started.job_id, { title: 'Re-scrape' });
        } catch (e) {
            if (typeof dialogs !== 'undefined') dialogs.toast(`Could not re-scrape: ${e.message}`, 'error', 8000);
        } finally {
            this.busy.delete(bookId);
            this.render();
        }
    }

    async accept(bookId, volumeId) {
        if (this.busy.has(bookId)) return;
        this.busy.add(bookId);
        this.render();
        try {
            const res = await fetch(`/api/scrape/review/${encodeURIComponent(bookId)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ volume_cv_id: volumeId }),
            });
            if (!res.ok) {
                const msg = (await res.text()).trim();
                this.busy.delete(bookId);
                await this.load();
                this.render();
                alert(`Could not apply that match: ${msg || res.status}`);
                return;
            }
        } catch (e) {
            console.error('Accept failed:', e);
            this.busy.delete(bookId);
            this.render();
            alert('Could not apply that match (network error).');
            return;
        }
        this.busy.delete(bookId);
        await this.load();
        this.render();
    }
}
ScrapeReviewPage.active = null;

// Live updates: keep the nav badge fresh on every route and reload the page
// when it is the one on screen.
(function registerScrapeReviewLiveUpdates() {
    const refresh = async () => {
        const page = ScrapeReviewPage.active;
        const onPage = page && document.querySelector('.scrape-review-page');
        if (onPage) {
            await page.load();
            page.render();
        } else if (typeof navigation !== 'undefined') {
            try {
                const res = await fetch('/api/scrape/review');
                const items = res.ok ? ((await res.json()) || []) : [];
                navigation.updateBadge('scrape-review', items.length);
            } catch (e) { /* ignore */ }
        }
    };
    const hook = () => {
        if (!window.wsClient) return false;
        ['scrape_review_needed', 'scrape_progress', 'scrape_completed']
            .forEach(ev => window.wsClient.on(ev, refresh));
        return true;
    };
    if (!hook()) document.addEventListener('DOMContentLoaded', hook);
})();

ScrapeReviewPage.NO_MATCH_REASONS = [['collected','Collected edition'],['not_listed','Not listed'],['web_comic','Web comic'],['other','Other']];
