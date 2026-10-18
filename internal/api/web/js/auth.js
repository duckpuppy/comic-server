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

    const LOGIN_ERRORS = {
        not_allowed: 'Your account signed in, but it is not allowed to use this server.',
        sso_off: 'Single sign-on is not switched on.',
        sso_unreachable: 'Could not reach the sign-in provider. Use your password instead.',
        sso_refused: 'The sign-in provider refused the request.',
        sso_failed: 'Single sign-on failed. Try again, or use your password.',
        rate_limited: 'Too many attempts. Wait a few minutes.',
    };

    // The callback sends failures back as ?login_error=<code>.
    function takeLoginError() {
        try {
            const url = new URL(location.href);
            const code = url.searchParams.get('login_error');
            if (!code) return '';
            url.searchParams.delete('login_error');
            history.replaceState(null, '', url.pathname + url.search + url.hash);
            return LOGIN_ERRORS[code] || 'Sign-in failed.';
        } catch (e) {
            return '';
        }
    }
    let pendingError = takeLoginError();

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
        if (pendingError) {
            err.textContent = pendingError;
            pendingError = '';
        }
        pw.focus();
        // Offer single sign-on when the server has it switched on.
        realFetch('/api/auth/status').then(r => r.ok ? r.json() : null).then(st => {
            if (!st || !st.oidc || !overlay) return;
            const a = document.createElement('a');
            a.href = '/auth/oidc/start';
            a.className = 'btn btn-secondary';
            a.textContent = 'Sign in with single sign-on';
            a.style.cssText = 'display:block;width:100%;box-sizing:border-box;text-align:center;margin-top:.5rem;';
            form.appendChild(a);
        }).catch(() => {});
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
