// Scrape Review page - books whose ComicVine match was ambiguous and need a
// human pick (comic-server-0nhp). Native replacement for ComicRack's
// "05 Proposed Values" review step.
class ScrapeReviewPage {
    constructor() {
        this.items = [];
        this.loading = true;
        this.notConfigured = false;
        this.busy = new Set(); // book ids with an accept in flight
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
            body = this.items.map(i => this.renderItem(i)).join('');
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

    renderItem(item) {
        const cands = item.candidates || [];
        const busy = this.busy.has(item.book_id);
        const rows = cands.length === 0
            ? '<tr><td colspan="6" class="empty-message">No candidates recorded.</td></tr>'
            : cands.map(c => `
                <tr>
                    <td>${this.escapeHtml(c.name)}</td>
                    <td>${this.escapeHtml(c.start_year)}</td>
                    <td>${this.escapeHtml(c.publisher)}</td>
                    <td>${(c.score ?? 0).toFixed(2)}</td>
                    <td><span class="scrape-review-confidence conf-${this.escapeHtml(c.confidence)}">${this.escapeHtml(c.confidence)}</span></td>
                    <td><button class="btn btn-small btn-primary scrape-review-accept"
                                data-book="${this.escapeHtml(item.book_id)}"
                                data-volume="${c.volume_id}" ${busy ? 'disabled' : ''}>Accept</button></td>
                </tr>`).join('');
        return `
            <section class="scrape-review-item" data-book="${this.escapeHtml(item.book_id)}">
                <h2>${this.escapeHtml(item.series || item.filename)}</h2>
                <p class="scrape-review-file" title="${this.escapeHtml(item.filename)}">${this.escapeHtml(item.filename)}</p>
                <table class="scrape-review-table">
                    <thead><tr><th>Volume</th><th>Start year</th><th>Publisher</th><th>Score</th><th>Confidence</th><th></th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </section>`;
    }

    attachListeners() {
        document.querySelectorAll('.scrape-review-accept').forEach(btn => {
            btn.addEventListener('click', () =>
                this.accept(btn.dataset.book, parseInt(btn.dataset.volume, 10)));
        });
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
