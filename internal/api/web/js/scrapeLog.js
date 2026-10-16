// Live log of one scrape job (comic-server-5wer): what the scraper is doing
// for a Re-scrape, step by step - the ComicVine search, the best match, any
// rate-limit wait - instead of a silent wait. Lines arrive as scrape_log
// WebSocket events; GET /api/scrape/log supplies what was logged before the
// dialog opened. Closing the dialog does not stop the scrape.
class ScrapeLog {
    // open(jobId, {title, onDone}) - onDone(stats) runs once when the job ends.
    static open(jobId, { title = 'Re-scrape', onDone = null } = {}) {
        const lines = new Map(); // seq -> line
        let finished = false;

        const close = dialogs.custom({
            title,
            contentClass: 'scrape-log-dialog',
            bodyHTML: `
                <p class="scrape-log-status" aria-live="polite">Running…</p>
                <pre class="scrape-log" tabindex="0"></pre>
                <p class="empty-message scrape-log-note">Closing this does not stop the scrape.</p>`,
            footerHTML: `<button class="btn btn-secondary" data-action="cancel">Close</button>`,
            onClose: () => {
                if (window.wsClient) {
                    window.wsClient.off('scrape_log', onLine);
                    window.wsClient.off('scrape_completed', onCompleted);
                }
            },
        });

        const root = () => document.querySelector('#dialogs-modal.modal-overlay');
        const render = () => {
            const el = root() && root().querySelector('.scrape-log');
            if (!el) return;
            const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
            el.innerHTML = [...lines.values()].sort((a, b) => a.seq - b.seq).map(l =>
                `<span class="scrape-log-line scrape-log-${dialogs.escapeHtml(l.level)}"><span class="scrape-log-time">${ScrapeLog.clock(l.time)}</span> ${dialogs.escapeHtml(l.msg)}</span>`
            ).join('\n');
            if (atBottom) el.scrollTop = el.scrollHeight;
        };
        const setStatus = (text, cls) => {
            const el = root() && root().querySelector('.scrape-log-status');
            if (!el) return;
            el.textContent = text;
            el.className = `scrape-log-status${cls ? ' ' + cls : ''}`;
        };
        const finish = (stats) => {
            if (finished) return;
            finished = true;
            setStatus(stats ? ScrapeLog.summary(stats) : 'Finished.', stats && stats.failed ? 'scrape-log-failed' : 'scrape-log-done');
            if (onDone) onDone(stats);
        };
        const add = (l) => { if (l && !lines.has(l.seq)) { lines.set(l.seq, l); return true; } return false; };

        function onLine(data) {
            if (!data || data.job_id !== jobId) return;
            if (add(data)) render();
        }
        function onCompleted(data) {
            if (data && data.job_id === jobId) finish(data.stats);
        }
        if (window.wsClient) {
            window.wsClient.on('scrape_log', onLine);
            window.wsClient.on('scrape_completed', onCompleted);
        }

        // Backlog: lines logged before this dialog subscribed, and whether
        // the job already ended (its completion event would be missed).
        fetch(`/api/scrape/log?job_id=${encodeURIComponent(jobId)}`)
            .then(res => res.ok ? res.json() : null)
            .then(body => {
                if (!body) return;
                (body.lines || []).forEach(add);
                render();
                if (!body.running) finish(null);
            })
            .catch(() => { /* live lines still arrive */ });

        return close;
    }

    static clock(t) {
        const d = new Date(t);
        return isNaN(d) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    static summary(s) {
        const parts = [];
        if (s.completed) parts.push(`${s.completed} matched`);
        if (s.pending_review) parts.push(`${s.pending_review} sent to Scrape Review`);
        if (s.skipped) parts.push(`${s.skipped} skipped`);
        if (s.failed) parts.push(`${s.failed} failed`);
        return `Finished${parts.length ? ': ' + parts.join(', ') : ''}.`;
    }
}
