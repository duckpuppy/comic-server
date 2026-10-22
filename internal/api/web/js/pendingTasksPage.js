// Pending tasks (tracker #29, first step): what is waiting for ComicVine right
// now, with live timers. It reads GET /api/comicvine/tasks every few seconds
// and counts down locally in between. There is no queue behind it: requests
// still run and wait as before, so a web request that must wait more than 20 s
// is refused at once and never shows here. A running scrape job can be
// cancelled (POST /api/scrape/cancel).
class PendingTasksPage {
    constructor() {
        this.data = null;
        this.fetchedAt = 0;
        this.error = '';
        this.cancelling = false;
        this.pollTimer = null;
        this.tickTimer = null;
        this.hooked = false;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    async init(ctx) {
        this.render();
        await this.load();
        if (ctx && ctx.aborted) return;
        this.hookEvents();
        this.startTimers();
    }

    // Stops by itself once the page is no longer on screen.
    onScreen() { return !!document.getElementById('pending-tasks-page'); }

    startTimers() {
        clearInterval(this.pollTimer);
        clearInterval(this.tickTimer);
        this.pollTimer = setInterval(() => {
            if (!this.onScreen()) { clearInterval(this.pollTimer); clearInterval(this.tickTimer); return; }
            if (!document.hidden) this.load();
        }, 4000);
        this.tickTimer = setInterval(() => {
            if (this.onScreen()) this.drawTimers();
        }, 1000);
    }

    hookEvents() {
        if (this.hooked || !window.wsClient) return;
        this.hooked = true;
        ['scrape_progress', 'scrape_completed'].forEach(ev =>
            window.wsClient.on(ev, () => { if (this.onScreen()) this.load(); }));
    }

    async load() {
        try {
            const res = await fetch('/api/comicvine/tasks');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            this.data = await res.json();
            this.fetchedAt = Date.now();
            this.error = '';
        } catch (e) {
            this.error = `Could not load the task list: ${e.message}`;
        }
        if (this.onScreen()) this.render();
    }

    left(sec) { return Math.max(0, Math.ceil(sec - (Date.now() - this.fetchedAt) / 1000)); }
    since(sec) { return Math.max(0, Math.floor(sec + (Date.now() - this.fetchedAt) / 1000)); }
    clock(sec) { return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`; }

    static REASONS = {
        burst: 'burst limit (20 requests, then a pause)',
        window: 'window limit (200 requests, then a long pause)',
        hourly: 'hourly budget kept free for people',
    };

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        app.innerHTML = `
            <div class="panel" id="pending-tasks-page">
                <h1>Pending tasks</h1>
                <p class="browse-compact-meta">ComicVine allows only a few requests, so work that needs it waits its turn. This shows what is waiting and when it should run. The times are estimates: the pacer lets callers go as soon as it can, and new work can change the order.</p>
                ${this.error ? `<p class="datamanager-errors">${this.escapeHtml(this.error)}</p>` : ''}
                ${this.data ? this.renderBody() : '<p>Loading…</p>'}
            </div>`;
        const cancel = document.getElementById('pt-cancel-btn');
        if (cancel) cancel.addEventListener('click', () => this.cancelScrape());
        const log = document.getElementById('pt-log-btn');
        if (log) log.addEventListener('click', () => ScrapeLog.open(log.dataset.job, { title: 'Scrape job' }));
    }

    renderBody() {
        const d = this.data;
        if (!d.pacing.configured && !d.scrape) {
            return '<p>ComicVine is not set up (no API key), so there is nothing to wait for.</p>';
        }
        return `${this.renderStatus()}${this.renderScrape()}${this.renderWaiters()}`;
    }

    renderStatus() {
        const p = this.data.pacing;
        if (!p.configured) return '';
        const circuit = p.circuit === 'open'
            ? `<strong>Paused after an error.</strong> <span id="pt-circuit" data-sec="${p.next_request_seconds}"></span>`
            : `Working normally${p.circuit === 'half-open' ? ' (testing after an error)' : ''}.`;
        return `<div class="bulk-preview" id="pt-status">
            <p><strong>ComicVine:</strong> ${circuit}</p>
            <p>Next request you start: <strong id="pt-next" data-sec="${p.next_request_seconds}"></strong>.
               Background sync: <strong id="pt-bg" data-sec="${p.background_wait_seconds}"></strong>.</p>
            <p class="browse-compact-meta">${p.requests_last_hour} requests in the last hour · burst ${p.burst_used}/${p.burst_size} · window ${p.window_used}/${p.window_size} · the sync may use ${p.background_hourly_cap} an hour</p>
        </div>`;
    }

    renderScrape() {
        const d = this.data;
        const j = d.scrape;
        if (!j) return '<h2>Scrape job</h2><p>No scrape job has run since the server started.</p>';
        const done = j.completed + j.skipped + j.failed + j.pending_review;
        const bits = [`${j.completed} matched`, `${j.pending_review} to review`, `${j.skipped} skipped`, `${j.failed} failed`];
        const state = d.scrape_running ? '<strong>Running</strong>' : this.escapeHtml(j.status);
        return `<h2>Scrape job</h2>
            <p>${state}: ${done} of ${j.total} books done (${bits.join(', ')}).</p>
            <div class="bulk-editor-buttons">
                <button class="btn btn-secondary" id="pt-log-btn" data-job="${this.escapeHtml(j.job_id)}">Open log</button>
                ${d.scrape_running ? `<button class="btn btn-danger" id="pt-cancel-btn" ${this.cancelling ? 'disabled' : ''}>${this.cancelling ? 'Cancelling…' : 'Cancel job'}</button>` : ''}
            </div>
            ${d.scrape_running ? '<p class="browse-compact-meta">Cancel stops the job at its next step. Books it already finished keep their results.</p>' : ''}`;
    }

    renderWaiters() {
        const w = this.data.waiters;
        if (!w.length) {
            return '<h2>Waiting for ComicVine</h2><p>Nothing is waiting right now. A request that has to wait appears here until it can go.</p>';
        }
        const rows = w.map((x, i) => `<tr>
            <td>${this.escapeHtml(x.label)}</td>
            <td>${x.background ? 'Background' : 'You started it'}</td>
            <td>${this.escapeHtml(PendingTasksPage.REASONS[x.reason] || x.reason || '')}</td>
            <td class="pt-since" data-sec="${x.waiting_for}"></td>
            <td><strong class="pt-runs" data-sec="${x.wait_seconds}"></strong> <span class="browse-compact-meta">(estimate)</span></td>
        </tr>`).join('');
        return `<h2>Waiting for ComicVine</h2>
            <table class="datamanager-diff-table"><thead><tr><th>Task</th><th>Kind</th><th>Waiting for</th><th>Waited</th><th>Runs in</th></tr></thead><tbody>${rows}</tbody></table>`;
    }

    // Fills the countdown cells; runs every second without redrawing the page.
    drawTimers() {
        const set = (el, text) => { if (el && el.textContent !== text) el.textContent = text; };
        const rdy = sec => { const s = this.left(sec); return s > 0 ? `in ${this.clock(s)}` : 'now'; };
        const next = document.getElementById('pt-next');
        if (next) set(next, rdy(+next.dataset.sec));
        const bg = document.getElementById('pt-bg');
        if (bg) set(bg, rdy(+bg.dataset.sec));
        const circuit = document.getElementById('pt-circuit');
        if (circuit) set(circuit, `Requests resume ${rdy(+circuit.dataset.sec)}.`);
        document.querySelectorAll('.pt-runs').forEach(el => {
            const s = this.left(+el.dataset.sec);
            set(el, s > 0 ? this.clock(s) : 'any moment');
        });
        document.querySelectorAll('.pt-since').forEach(el => set(el, this.clock(this.since(+el.dataset.sec))));
    }

    async cancelScrape() {
        const ok = await dialogs.confirm({ message: 'Stop the running scrape job? Books it already finished keep their results.', confirmLabel: 'Stop the job', cancelLabel: 'Keep running', danger: true });
        if (!ok) return;
        this.cancelling = true;
        this.render();
        this.drawTimers();
        try {
            const res = await fetch('/api/scrape/cancel', { method: 'POST' });
            if (!res.ok && res.status !== 409) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
        } catch (e) {
            this.error = `Cancel failed: ${e.message}`;
        }
        this.cancelling = false;
        await this.load();
        this.drawTimers();
    }
}
