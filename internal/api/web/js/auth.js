// Built-in login (tracker #21). Loaded before every other script.
//
// While no password is set, nothing here shows. Once one is set, any API
// answer of 401 raises a full-page login form; a good login reloads the page.
// The page files themselves stay open (they hold no data); only /api/*, /ws
// and /metrics need the session.
(function () {
    const realFetch = window.fetch.bind(window);
    let overlay = null;

    function isOwnAPI(input) {
        const url = typeof input === 'string' ? input : (input && input.url) || '';
        if (url.startsWith('/')) return url.startsWith('/api/');
        try {
            const u = new URL(url, location.href);
            return u.origin === location.origin && u.pathname.startsWith('/api/');
        } catch (e) {
            return false;
        }
    }

    function isLoginCall(input) {
        const url = typeof input === 'string' ? input : (input && input.url) || '';
        return /\/api\/auth\/(login|password|status|logout)/.test(url);
    }

    window.fetch = async function (input, init) {
        const res = await realFetch(input, init);
        if (res.status === 401 && isOwnAPI(input) && !isLoginCall(input)) {
            showLogin();
        }
        return res;
    };

    function showLogin() {
        if (overlay) return;
        overlay = document.createElement('div');
        overlay.setAttribute('role', 'dialog');
        overlay.setAttribute('aria-modal', 'true');
        overlay.style.cssText = 'position:fixed;inset:0;z-index:100000;display:flex;align-items:center;justify-content:center;' +
            'background:var(--bg-primary,#f9fafb);';
        overlay.innerHTML = `
            <form id="auth-login-form" style="width:min(22rem,90vw);padding:1.5rem;border:1px solid var(--border-color,#e5e7eb);
                border-radius:8px;background:var(--bg-secondary,#fff);color:var(--text-primary,#111827);box-shadow:var(--shadow-lg);">
                <h2 style="margin:0 0 1rem;">Comic Server</h2>
                <label for="auth-password" style="display:block;margin-bottom:.25rem;">Password</label>
                <input type="password" id="auth-password" class="form-control" autocomplete="current-password" required
                    style="width:100%;box-sizing:border-box;margin-bottom:.75rem;">
                <p id="auth-error" role="alert" style="color:var(--danger-color,#ef4444);min-height:1.25rem;margin:0 0 .75rem;"></p>
                <button type="submit" class="btn btn-primary" style="width:100%;">Sign in</button>
            </form>`;
        document.body.appendChild(overlay);
        const form = overlay.querySelector('#auth-login-form');
        const pw = overlay.querySelector('#auth-password');
        const err = overlay.querySelector('#auth-error');
        pw.focus();
        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            err.textContent = '';
            try {
                const res = await realFetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password: pw.value }),
                });
                if (res.ok) {
                    location.reload();
                    return;
                }
                const text = (await res.text()).trim();
                err.textContent = res.status === 401 ? 'Wrong password.' : (text || `Sign-in failed (HTTP ${res.status}).`);
                pw.select();
            } catch (ex) {
                err.textContent = 'Could not reach the server.';
            }
        });
    }

    async function logout() {
        try {
            await realFetch('/api/auth/logout', { method: 'POST' });
        } finally {
            location.reload();
        }
    }

    async function check() {
        try {
            const res = await realFetch('/api/auth/status');
            if (!res.ok) return;
            const st = await res.json();
            if (st.enabled && !st.authenticated) showLogin();
        } catch (e) { /* the server may be starting; the next 401 will raise the form */ }
    }

    window.AuthUI = { showLogin, logout };
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', check);
    } else {
        check();
    }
})();
