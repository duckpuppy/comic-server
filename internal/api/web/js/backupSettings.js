// Settings > Backups (comic-server-yzdt): scheduled and manual backups of
// comic-server's own state (config.db, library.db, the ComicVine cache,
// config.yaml). Renders into a host element inside the Settings page.
// Restore is not built yet; the page says so.
class BackupSettings {
    constructor(host) {
        this.host = host;
        this.data = null;     // GET /api/settings/backups
        this.list = [];       // GET /api/backups
        this.error = '';
        this.saving = false;
        this.pollTimer = null;
        this.load();
    }

    esc(t) {
        const d = document.createElement('div');
        d.textContent = t == null ? '' : String(t);
        return d.innerHTML;
    }

    async load() {
        try {
            const res = await fetch('/api/settings/backups');
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            this.data = await res.json();
            this.error = '';
            const l = await fetch('/api/backups');
            this.list = l.ok ? ((await l.json()).backups || []) : [];
        } catch (e) {
            this.error = e.message;
        }
        this.render();
        this.schedulePoll();
    }

    // While a backup runs, refresh every 2 seconds so the card shows the result.
    schedulePoll() {
        clearTimeout(this.pollTimer);
        if (this.data && this.data.status.running && document.body.contains(this.host)) {
            this.pollTimer = setTimeout(() => this.load(), 2000);
        }
    }

    fmtSize(n) {
        if (!n) return '0 B';
        const u = ['B', 'KB', 'MB', 'GB'];
        let i = 0;
        while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
        return `${n.toFixed(i ? 1 : 0)} ${u[i]}`;
    }

    fmtTime(t) {
        if (!t) return '';
        const d = new Date(t);
        return isNaN(d) ? '' : d.toLocaleString();
    }

    renderStatus() {
        const st = this.data.status;
        const bits = [];
        if (st.running) bits.push('<strong>A backup is running…</strong>');
        const last = st.last_run;
        if (last) {
            bits.push(last.ok
                ? `Last backup: ${this.esc(this.fmtTime(last.started_at))}, ${this.esc(this.fmtSize(last.size))} (${this.esc(last.trigger)}).`
                : `<span class="backup-failed">Last backup FAILED ${this.esc(this.fmtTime(last.started_at))}: ${this.esc(last.error)}</span>`);
            if (!last.ok && st.last_ok) bits.push(`Last good backup: ${this.esc(this.fmtTime(st.last_ok.started_at))}.`);
        } else {
            bits.push('No backup has run yet.');
        }
        if (this.data.settings.enabled && st.next_run) bits.push(`Next backup: ${this.esc(this.fmtTime(st.next_run))}.`);
        if (!this.data.settings.enabled) bits.push('Scheduled backups are off.');
        return `<p class="backup-status">${bits.join(' ')}</p>`;
    }

    renderList() {
        if (!this.list.length) return '<p class="form-help">No backups in the folder yet.</p>';
        const rows = this.list.map(b => `<tr><td>${this.esc(b.name)}</td><td>${this.esc(this.fmtTime(b.created_at))}</td>
            <td>${this.esc(this.fmtSize(b.size))}</td><td>${this.esc(b.app_version)}</td><td>${b.book_count || ''}</td></tr>`).join('');
        return `<table class="datamanager-diff-table"><thead><tr><th>Backup</th><th>Taken</th><th>Size</th><th>Version</th><th>Books</th></tr></thead><tbody>${rows}</tbody></table>`;
    }

    render() {
        if (!this.data) {
            this.host.innerHTML = `<div class="panel scan-info-panel"><div class="settings-section-header"><h2>Backups</h2></div>
                <p class="empty-message">${this.error ? this.esc(this.error) : 'Loading…'}</p></div>`;
            return;
        }
        const s = this.data.settings;
        const warn = (this.data.warnings || []).map(w => `<p class="datamanager-errors">${this.esc(w)}</p>`).join('');
        this.host.innerHTML = `
            <div class="panel scan-info-panel" id="backup-panel">
                <div class="settings-section-header">
                    <h2>Backups</h2>
                    <p class="settings-section-description">Safe copies of comic-server's own data: edits, Wanted and paper copies, device and list settings, the history log and the ComicVine cache. Comic files are not included. Restore is not built yet; a backup is a folder you can copy back by hand with the server stopped.</p>
                </div>
                ${this.renderStatus()}
                ${warn}
                ${this.error ? `<p class="datamanager-errors">${this.esc(this.error)}</p>` : ''}
                <div class="form-group">
                    <label><input type="checkbox" id="backup-enabled" ${s.enabled ? 'checked' : ''}> Back up on a schedule</label>
                </div>
                <div class="form-group">
                    <label for="backup-dir">Backup folder (on the server)</label>
                    <div style="display:flex;gap:.5rem;">
                        <input type="text" id="backup-dir" class="form-control" value="${this.esc(s.dir)}" placeholder="/backups/comic-server">
                        <button class="btn btn-secondary" id="backup-browse" type="button">Browse…</button>
                    </div>
                    <div class="form-help">Use a different disk from the databases, and a mounted volume under Docker. The folder is created private (owner only) because a backup holds your ComicVine key.</div>
                </div>
                <div class="form-group">
                    <label for="backup-mode">Schedule</label>
                    <select id="backup-mode" class="form-control" style="max-width:14rem;">
                        <option value="daily" ${s.mode === 'daily' ? 'selected' : ''}>Every day at</option>
                        <option value="interval" ${s.mode === 'interval' ? 'selected' : ''}>Every N hours</option>
                    </select>
                    <input type="time" id="backup-at" class="form-control" value="${this.esc(s.at)}" style="max-width:8rem;display:${s.mode === 'daily' ? 'inline-block' : 'none'};">
                    <input type="number" id="backup-every" class="form-control" min="1" max="720" value="${s.every_hours}" style="max-width:8rem;display:${s.mode === 'interval' ? 'inline-block' : 'none'};">
                    <div class="form-help">A scheduled backup waits while a device is syncing, and a failed one is not retried for an hour. Turning backups on takes the first one right away.</div>
                </div>
                <div class="form-group">
                    <label for="backup-keep">Keep the newest</label>
                    <input type="number" id="backup-keep" class="form-control" min="1" max="365" value="${s.keep}" style="max-width:8rem;">
                    <div class="form-help">Older backups are deleted after each good one.</div>
                </div>
                <div class="form-group">
                    <label><input type="checkbox" id="backup-cv" ${s.include_cv_cache ? 'checked' : ''}> Include the ComicVine cache (weeks of API calls to rebuild)</label><br>
                    <label><input type="checkbox" id="backup-yaml" ${s.include_config_yaml ? 'checked' : ''}> Include config.yaml (holds the ComicVine key)</label>
                    <div class="form-help">Each backup will hold: ${this.esc((this.data.includes || []).join(', ') || 'nothing')}.</div>
                </div>
                <div class="scan-info-actions">
                    <button class="btn btn-primary" id="backup-save" ${this.saving ? 'disabled' : ''}>${this.saving ? 'Saving…' : 'Save'}</button>
                    <button class="btn btn-secondary" id="backup-run" ${this.data.status.running ? 'disabled' : ''}>Back up now</button>
                </div>
                <h3>Existing backups</h3>
                ${this.renderList()}
            </div>`;
        this.attach();
    }

    form() {
        const v = id => document.getElementById(id);
        return {
            enabled: v('backup-enabled').checked,
            dir: v('backup-dir').value.trim(),
            mode: v('backup-mode').value,
            at: v('backup-at').value || '03:30',
            every_hours: parseInt(v('backup-every').value, 10) || 24,
            keep: parseInt(v('backup-keep').value, 10) || 0,
            include_cv_cache: v('backup-cv').checked,
            include_config_yaml: v('backup-yaml').checked,
        };
    }

    attach() {
        const v = id => document.getElementById(id);
        v('backup-mode').addEventListener('change', () => {
            const daily = v('backup-mode').value === 'daily';
            v('backup-at').style.display = daily ? 'inline-block' : 'none';
            v('backup-every').style.display = daily ? 'none' : 'inline-block';
        });
        v('backup-browse').addEventListener('click', async () => {
            const picked = await dialogs.browseServerDirectory({ title: 'Choose the backup folder', startPath: v('backup-dir').value || '/' });
            if (picked) v('backup-dir').value = picked;
        });
        v('backup-save').addEventListener('click', () => this.save());
        v('backup-run').addEventListener('click', () => this.runNow());
    }

    async save() {
        this.saving = true;
        const body = this.form();
        try {
            const res = await fetch('/api/settings/backups', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            this.data = await res.json();
            this.error = '';
            dialogs.toast('Backup settings saved.', 'success');
        } catch (e) {
            this.error = e.message;
            dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
        }
        this.saving = false;
        this.render();
    }

    async runNow() {
        // Save what is on screen first, so "Back up now" uses the folder shown.
        await this.save();
        if (this.error) return;
        try {
            const res = await fetch('/api/backups/run', { method: 'POST' });
            if (!res.ok) throw new Error((await res.text()).trim() || `HTTP ${res.status}`);
            dialogs.toast('Backup started.', 'success');
        } catch (e) {
            dialogs.toast(`Could not start the backup: ${e.message}`, 'error', 8000);
        }
        await this.load();
        // The run is asynchronous: poll until it ends, then show the outcome.
        const wait = async () => {
            if (!document.body.contains(this.host)) return;
            const st = await (await fetch('/api/settings/backups')).json();
            if (st.status.running) { setTimeout(wait, 2000); return; }
            await this.load();
            const last = this.data.status.last_run;
            if (last && !last.ok) dialogs.toast(`Backup failed: ${last.error}`, 'error', 10000);
            else if (last) dialogs.toast('Backup finished.', 'success');
        };
        setTimeout(wait, 1500);
    }
}
