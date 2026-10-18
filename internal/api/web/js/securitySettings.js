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
            } else {
                this.tokens = [];
                this.oidc = null;
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
