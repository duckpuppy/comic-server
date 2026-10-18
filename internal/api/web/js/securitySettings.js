// Settings > Security (tracker #21): set or change the login password, sign
// out, and manage API tokens (for scripts and Prometheus /metrics).
class SecuritySettings {
    constructor(host) {
        this.host = host;
        this.status = null;     // GET /api/auth/status
        this.tokens = [];       // GET /api/auth/tokens
        this.error = '';
        this.notice = '';
        this.newToken = '';     // shown once, right after creation
        this.oidc = null;       // GET /api/auth/oidc
        this.proxies = null;    // GET /api/auth/trusted-proxies
        this.busy = false;
        this.load();
    }

    esc(t) {
        const d = document.createElement('div');
        d.textContent = t == null ? '' : String(t);
        return d.innerHTML;
    }

    async load() {
        try {
            const res = await fetch('/api/auth/status');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            this.status = await res.json();
            if (this.status.enabled) {
                const t = await fetch('/api/auth/tokens');
                this.tokens = t.ok ? ((await t.json()).tokens || []) : [];
                const o = await fetch('/api/auth/oidc');
                this.oidc = o.ok ? await o.json() : null;
                const p = await fetch('/api/auth/trusted-proxies');
                this.proxies = p.ok ? await p.json() : null;
            } else {
                this.tokens = [];
                this.oidc = null;
                this.proxies = null;
            }
            this.error = '';
        } catch (e) {
            this.error = e.message;
        }
        this.render();
    }

    async post(url, body, method = 'POST') {
        const res = await fetch(url, {
            method,
            headers: { 'Content-Type': 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) throw new Error(friendlyErrorText(res, text));
        return text ? JSON.parse(text) : {};
    }

    fmtTime(t) {
        if (!t) return 'never';
        const d = new Date(t);
        return isNaN(d) ? '' : d.toLocaleString();
    }

    renderPassword() {
        const on = this.status.enabled;
        return `
            <h3>${on ? 'Change password' : 'Turn on login'}</h3>
            <p class="form-help">${on
                ? 'Changing the password signs out every other browser.'
                : 'Login is <strong>off</strong>: anyone who can reach this server can use it. Set a password to turn it on. Forgotten it later? Stop the server and run <code>comic-server auth set-password</code> (or <code>comic-server auth disable</code>).'}</p>
            ${on ? `<div class="form-group"><label for="sec-current">Current password</label>
                <input type="password" id="sec-current" class="form-control" autocomplete="current-password" style="max-width:20rem;"></div>` : ''}
            <div class="form-group"><label for="sec-new">New password (at least 12 characters)</label>
                <input type="password" id="sec-new" class="form-control" autocomplete="new-password" style="max-width:20rem;"></div>
            <div class="form-group"><label for="sec-repeat">Repeat new password</label>
                <input type="password" id="sec-repeat" class="form-control" autocomplete="new-password" style="max-width:20rem;"></div>
            <div class="scan-info-actions">
                <button class="btn btn-primary" id="sec-save" ${this.busy ? 'disabled' : ''}>${on ? 'Change password' : 'Set password and turn on login'}</button>
                ${on ? '<button class="btn btn-secondary" id="sec-logout">Sign out</button>' : ''}
            </div>`;
    }

    renderTokens() {
        if (!this.status.enabled) return '';
        const rows = this.tokens.map(t => `
            <tr><td>${this.esc(t.name)}</td><td>${this.esc(this.fmtTime(t.created_at))}</td><td>${this.esc(this.fmtTime(t.last_used_at))}</td>
            <td><button class="btn btn-secondary" data-revoke="${t.id}">Revoke</button></td></tr>`).join('');
        return `
            <h3>API tokens</h3>
            <p class="form-help">For scripts and Prometheus. Send <code>Authorization: Bearer &lt;token&gt;</code>. A token is shown once, when you create it.</p>
            ${this.newToken ? `<div class="panel" style="border-color:var(--warning-color);"><strong>Copy this token now. It cannot be shown again.</strong>
                <pre style="user-select:all;overflow-x:auto;">${this.esc(this.newToken)}</pre></div>` : ''}
            ${this.tokens.length ? `<table class="data-table"><thead><tr><th>Name</th><th>Created</th><th>Last used</th><th></th></tr></thead><tbody>${rows}</tbody></table>`
                : '<p class="form-help">No tokens yet.</p>'}
            <div class="form-group" style="display:flex;gap:.5rem;margin-top:.75rem;">
                <input type="text" id="sec-token-name" class="form-control" placeholder="Name, for example prometheus" maxlength="80" style="max-width:20rem;">
                <button class="btn btn-secondary" id="sec-token-create">Create token</button>
            </div>`;
    }

    // What the server makes of THIS request, with the one fix that matters:
    // a proxy header from an address that is not on the list.
    renderPeek() {
        const k = this.proxies.seen;
        const hasProxyHeaders = !!(k.forwarded_for || k.forwarded_proto || k.forwarded_host || k.cf_connecting_ip);
        let verdict;
        if (!k.peer_trusted && hasProxyHeaders) {
            verdict = `<p class="datamanager-errors">This page came in from <code>${this.esc(k.peer_ip)}</code> with proxy headers, but that address is not trusted, so the headers are ignored. If <code>${this.esc(k.peer_ip)}</code> is your reverse proxy, add it below.</p>`;
        } else if (k.peer_trusted) {
            verdict = `<p class="form-help" role="status"><code>${this.esc(k.peer_ip)}</code> is a trusted proxy. The server reads your address as <code>${this.esc(k.client_ip)}</code>.</p>`;
        } else {
            verdict = `<p class="form-help" role="status">A direct connection from <code>${this.esc(k.peer_ip)}</code>. No proxy is involved.</p>`;
        }
        return `${verdict}
            <table class="data-table"><tbody>
                <tr><td>Connection from</td><td><code>${this.esc(k.peer_ip)}</code> (${k.peer_trusted ? 'trusted' : 'not trusted'})</td></tr>
                <tr><td>Your address, as read</td><td><code>${this.esc(k.client_ip)}</code></td></tr>
                <tr><td>Counted as HTTPS</td><td>${k.secure ? 'yes' : 'no'}</td></tr>
                <tr><td>X-Forwarded-For</td><td>${this.esc(k.forwarded_for || 'not sent')}</td></tr>
                <tr><td>X-Forwarded-Proto</td><td>${this.esc(k.forwarded_proto || 'not sent')}</td></tr>
                <tr><td>X-Forwarded-Host</td><td>${this.esc(k.forwarded_host || 'not sent')}</td></tr>
                <tr><td>CF-Connecting-IP</td><td>${this.esc(k.cf_connecting_ip || 'not sent')}</td></tr>
            </tbody></table>`;
    }

    renderProxies() {
        if (!this.status.enabled || !this.proxies) return '';
        const p = this.proxies;
        const where = p.source === 'saved'
            ? 'Saved here. This overrides config.yaml and the environment variable.'
            : `Taken from config.yaml or <code>COMIC_SERVER_TRUSTED_PROXIES</code>${(p.config || []).length ? '' : ' (nothing set)'}. Saving a list here overrides it.`;
        return `
            <h3>Trusted proxies</h3>
            <p class="form-help">Addresses of reverse proxies in front of this server (Nginx Proxy Manager, a Cloudflare Tunnel connector). Only they are believed when they say who the real visitor is and whether the connection used HTTPS. With none listed the login rate limit sees the proxy for everybody, and the cookie is not marked Secure. Changes apply at once.</p>
            <p class="form-help">${where}</p>
            <div class="form-group"><label for="sec-proxies">One address or range per line (for example <code>192.168.0.56</code> or <code>172.18.0.0/16</code>)</label>
                <textarea id="sec-proxies" class="form-control" rows="4" style="max-width:28rem;">${this.esc((p.proxies || []).join('\n'))}</textarea></div>
            <div class="scan-info-actions">
                <button class="btn btn-primary" id="sec-proxies-save" ${this.busy ? 'disabled' : ''}>Save trusted proxies</button>
                ${p.source === 'saved' ? '<button class="btn btn-secondary" id="sec-proxies-clear">Use config.yaml / environment instead</button>' : ''}
            </div>
            <h4>What the server sees for this page</h4>
            ${this.renderPeek()}
            <p class="form-help">If a bad list ever makes saving here fail, stop the server and run <code>comic-server auth trusted-proxies --clear</code>.</p>`;
    }

    renderOIDC() {
        if (!this.status.enabled || !this.oidc) return '';
        const o = this.oidc;
        const check = o.check
            ? (o.check.ok ? '<p class="form-help" role="status">The provider answered. Try it: sign out, then use the single sign-on button.</p>'
                : `<p class="datamanager-errors">The settings are saved, but the provider could not be reached: ${this.esc(o.check.error)}</p>`)
            : '';
        return `
            <h3>Single sign-on (Authelia, or any OpenID Connect provider)</h3>
            <p class="form-help">Adds a sign-in button next to the password. The password keeps working, so a provider outage cannot lock you out.
                In the provider, register this server as a client with the redirect address shown below, the <code>authorization_code</code> flow with PKCE,
                and the scopes <code>openid profile email groups</code>.</p>
            ${check}
            <div class="form-group"><label><input type="checkbox" id="oidc-enabled" ${o.enabled ? 'checked' : ''}> Allow sign-in with single sign-on</label></div>
            <div class="form-group"><label for="oidc-issuer">Provider address (issuer)</label>
                <input type="text" id="oidc-issuer" class="form-control" value="${this.esc(o.issuer)}" placeholder="https://login.example.com" style="max-width:28rem;"></div>
            <div class="form-group"><label for="oidc-client">Client ID</label>
                <input type="text" id="oidc-client" class="form-control" value="${this.esc(o.client_id)}" autocomplete="off" style="max-width:20rem;"></div>
            <div class="form-group"><label for="oidc-secret">Client secret</label>
                <input type="password" id="oidc-secret" class="form-control" autocomplete="new-password" placeholder="${o.has_secret ? 'saved - leave empty to keep it' : ''}" style="max-width:20rem;">
                <div class="form-help">Stored in this server's database, so it is also in backups. It is never shown again.</div></div>
            <div class="form-group"><label for="oidc-public">This server's address, as browsers use it</label>
                <input type="text" id="oidc-public" class="form-control" value="${this.esc(o.public_url)}" placeholder="https://comic.example.com" style="max-width:28rem;">
                ${o.redirect_uri ? `<div class="form-help">Redirect address to register: <code>${this.esc(o.redirect_uri)}</code></div>` : ''}</div>
            <div class="form-group"><label for="oidc-groups">Allowed groups (comma separated)</label>
                <input type="text" id="oidc-groups" class="form-control" value="${this.esc((o.allowed_groups || []).join(', '))}" style="max-width:28rem;"></div>
            <div class="form-group"><label for="oidc-users">Allowed users (comma separated; a username, or a verified email)</label>
                <input type="text" id="oidc-users" class="form-control" value="${this.esc((o.allowed_users || []).join(', '))}" style="max-width:28rem;">
                <div class="form-help">At least one group or user is required. Nobody else in your single sign-on can get in.</div></div>
            <div class="scan-info-actions"><button class="btn btn-primary" id="oidc-save" ${this.busy ? 'disabled' : ''}>Save single sign-on</button></div>`;
    }

    render() {
        if (!this.host || !document.body.contains(this.host)) return;
        if (!this.status) {
            this.host.innerHTML = `<div class="panel scan-info-panel"><h2>Security</h2>${this.error
                ? `<p class="datamanager-errors">${this.esc(this.error)}</p>` : '<p>Loading…</p>'}</div>`;
            return;
        }
        this.host.innerHTML = `
            <div class="panel scan-info-panel">
                <div class="settings-section-header"><h2>Security</h2>
                    <p class="settings-section-description">Login for the web UI and API. Syncing with your devices does not use it and is not affected.</p></div>
                ${this.error ? `<p class="datamanager-errors">${this.esc(this.error)}</p>` : ''}
                ${this.notice ? `<p class="form-help" role="status">${this.esc(this.notice)}</p>` : ''}
                ${this.renderPassword()}
                ${this.renderTokens()}
                ${this.renderProxies()}
                ${this.renderOIDC()}
            </div>`;
        this.attach();
    }

    attach() {
        const $ = (id) => this.host.querySelector(id);
        $('#sec-save')?.addEventListener('click', () => this.savePassword());
        $('#sec-logout')?.addEventListener('click', () => window.AuthUI && window.AuthUI.logout());
        $('#sec-token-create')?.addEventListener('click', () => this.createToken());
        $('#oidc-save')?.addEventListener('click', () => this.saveOIDC());
        $('#sec-proxies-save')?.addEventListener('click', () => this.saveProxies());
        $('#sec-proxies-clear')?.addEventListener('click', () => this.clearProxies());
        this.host.querySelectorAll('[data-revoke]').forEach(b =>
            b.addEventListener('click', () => this.revoke(b.dataset.revoke)));
    }

    async savePassword() {
        const v = (id) => (this.host.querySelector(id) || {}).value || '';
        const next = v('#sec-new');
        if (next !== v('#sec-repeat')) {
            this.error = 'The two new passwords do not match.';
            this.notice = '';
            return this.render();
        }
        this.busy = true;
        try {
            await this.post('/api/auth/password', { current_password: v('#sec-current'), new_password: next });
            this.error = '';
            this.notice = this.status.enabled ? 'Password changed.' : 'Login is now on.';
        } catch (e) {
            this.error = e.message;
            this.notice = '';
        }
        this.busy = false;
        await this.load();
    }

    async saveProxies() {
        const lines = ((this.host.querySelector('#sec-proxies') || {}).value || '')
            .split(/[\n,]+/).map(x => x.trim()).filter(Boolean);
        this.busy = true;
        try {
            this.proxies = await this.post('/api/auth/trusted-proxies', { proxies: lines }, 'PUT');
            this.error = '';
            this.notice = 'Trusted proxies saved. They apply now.';
        } catch (e) {
            this.error = e.message;
            this.notice = '';
        }
        this.busy = false;
        this.render();
    }

    async clearProxies() {
        if (!confirm('Forget the list saved here and use config.yaml / the environment variable again?')) return;
        try {
            this.proxies = await this.post('/api/auth/trusted-proxies', undefined, 'DELETE');
            this.error = '';
            this.notice = 'Saved list cleared.';
        } catch (e) {
            this.error = e.message;
            this.notice = '';
        }
        this.render();
    }

    async saveOIDC() {
        const v = (id) => ((this.host.querySelector(id) || {}).value || '').trim();
        const list = (id) => v(id).split(',').map(x => x.trim()).filter(Boolean);
        this.busy = true;
        try {
            const out = await this.post('/api/auth/oidc', {
                enabled: this.host.querySelector('#oidc-enabled').checked,
                issuer: v('#oidc-issuer'), client_id: v('#oidc-client'), client_secret: (this.host.querySelector('#oidc-secret') || {}).value || '',
                public_url: v('#oidc-public'), allowed_groups: list('#oidc-groups'), allowed_users: list('#oidc-users'),
            }, 'PUT');
            this.error = '';
            this.notice = 'Single sign-on settings saved.';
            this.busy = false;
            this.oidc = out;
            return this.render();
        } catch (e) {
            this.error = e.message;
            this.notice = '';
        }
        this.busy = false;
        this.render();
    }

    async createToken() {
        const name = (this.host.querySelector('#sec-token-name') || {}).value || '';
        try {
            const out = await this.post('/api/auth/tokens', { name });
            this.newToken = out.token;
            this.error = '';
        } catch (e) {
            this.error = e.message;
        }
        await this.load();
    }

    async revoke(id) {
        if (!confirm('Revoke this token? Anything using it stops working.')) return;
        try {
            await this.post(`/api/auth/tokens/${encodeURIComponent(id)}`, undefined, 'DELETE');
            this.newToken = '';
            this.error = '';
        } catch (e) {
            this.error = e.message;
        }
        await this.load();
    }
}
window.SecuritySettings = SecuritySettings;
