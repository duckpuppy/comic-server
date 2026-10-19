// ComicVine pacing chip (tracker #26 follow-up): a small pill in the corner of
// every page showing whether a ComicVine request can go out now, or a live
// countdown to the next one. It reads GET /api/comicvine/pacing every 15 s and
// counts down locally in between. Hidden when ComicVine is not configured.
(function () {
    const POLL_MS = 15000;
    let st = null;        // last answer
    let fetchedAt = 0;
    let el = null;
    let failures = 0;

    const clock = sec => `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
    const left = sec => Math.max(0, Math.ceil(sec - (Date.now() - fetchedAt) / 1000));

    function ensure() {
        if (el) return el;
        const css = document.createElement('style');
        css.textContent = '#cv-pacing-chip{position:fixed;left:12px;bottom:12px;z-index:50;padding:4px 10px;border-radius:999px;font-size:.78rem;' +
            'background:var(--panel-bg,#2b2f36);color:var(--text-color,#ddd);border:1px solid var(--border-color,#444);opacity:.9;cursor:default}' +
            '#cv-pacing-chip.cv-wait{border-color:#d9a441}#cv-pacing-chip.cv-paused{border-color:#c0392b}';
        document.head.appendChild(css);
        el = document.createElement('div');
        el.id = 'cv-pacing-chip';
        el.setAttribute('role', 'status');
        el.hidden = true;
        document.body.appendChild(el);
        return el;
    }

    function draw() {
        const chip = ensure();
        if (!st || !st.configured) { chip.hidden = true; return; }
        chip.hidden = false;
        const wait = left(st.next_request_seconds);
        const bg = left(st.background_wait_seconds);
        chip.className = '';
        if (st.circuit === 'open' && wait > 0) {
            chip.classList.add('cv-paused');
            chip.textContent = `ComicVine paused after an error: ${clock(wait)}`;
        } else if (wait > 0) {
            chip.classList.add('cv-wait');
            chip.textContent = `ComicVine: next request in ${clock(wait)}`;
        } else {
            chip.textContent = `ComicVine: ready · ${st.requests_last_hour} requests this hour`;
        }
        chip.title = `Burst ${st.burst_used}/${st.burst_size}, window ${st.window_used}/${st.window_size}. ` +
            `The background sync may use ${st.background_hourly_cap} an hour` +
            (bg > 0 ? ` and waits ${clock(bg)} (${st.background_reason || 'headroom'}).` : '.') +
            (wait > 0 && st.reason ? ` Your next request waits for the ${st.reason} limit.` : '');
    }

    async function refresh() {
        if (document.hidden) return;
        try {
            const res = await fetch('/api/comicvine/pacing');
            if (!res.ok) throw new Error(String(res.status));
            st = await res.json();
            fetchedAt = Date.now();
            failures = 0;
        } catch (e) {
            failures++;
            st = null; // login screen, server down: show nothing rather than stale numbers
        }
        draw();
    }

    window.cvPacing = { refresh };
    document.addEventListener('DOMContentLoaded', () => {
        refresh();
        setInterval(() => { if (failures < 5) refresh(); }, POLL_MS);
        setInterval(draw, 1000);
        document.addEventListener('visibilitychange', () => { if (!document.hidden) refresh(); });
    });
})();
