// Scan Info Settings - first UI/API surface for Server.ScanInfo
// (Scanners/Blacklist/Prefix/Unknown), previously config.yaml-hand-edit-
// only (comic-server-4ms). Edits the Scanners/Blacklist lists locally and
// saves the whole config at once via PUT /api/settings/scan-info, matching
// how the server stores it (configdb.UpsertScanInfo replaces wholesale,
// no per-entry endpoints).
class ScanInfoSettings {
    constructor() {
        this.config = null; // { enabled, scanners, blacklist, prefix, unknown }
        this.error = null;
        this.saving = false;
        this.dirty = false;
        // Appearance (comic-server-8qk) - loaded/saved independently of
        // the Scan Info config above (separate endpoint, separate
        // concern); this is just the only page /settings has today.
        this.theme = null; // 'light' | 'dark' | 'system'
        this.themeSaving = false;
        // Trash settings (comic-server-4hsz) - loaded/saved independently
        // of the Scan Info config above (separate endpoint, separate
        // concern), same pattern Appearance already established. First
        // section of the broader "give Settings a real config UI" push -
        // network/library path, ComicVine key, CBZ Convert, ignore-devices,
        // log level, and Komga still remain config.yaml-only.
        this.trash = null; // { path, retention_days }
        this.trashSaving = false;
        // Library import (comic-server-szvk) - one-time/occasional
        // ComicRack -> comic-server migration, replacing the old always-on
        // file-watcher. selectedFile is the browser File object chosen but
        // not yet uploaded; importJob is the last completed/failed job
        // returned by the server, shown until the next attempt.
        this.selectedFile = null;
        this.importing = false;
        this.importJob = null;
        this.importError = null;

        // Server misc settings (comic-server-wp8k, second slice of the
        // Settings UI push): CBZ Convert enabled + ignore-devices, both
        // live-effect (no restart needed) unlike the still-config.yaml-only
        // library path/ports/ComicVine key/Komga connection settings
        // (comic-server-yvbh).
        this.serverMisc = null; // { cbz_convert_enabled, ignore_devices }
        this.serverMiscSaving = false;

        // Restart-required settings (comic-server-yvbh): library path,
        // network ports, bind address, ComicVine key, Komga connection -
        // every one baked into an object or listener socket once at
        // startup, so unlike Server above these can never take effect
        // until the process restarts. restartRequired holds the raw
        // {saved, active, restart_required} response; restartRequiredForm
        // is the editable copy the inputs are bound to - its two API key
        // fields always start blank (GET never returns the actual key,
        // only whether one is set) and PUT treats "left blank" as "leave
        // the existing key untouched", not "clear it".
        this.restartRequired = null;
        this.restartRequiredForm = null;
        this.restartRequiredSaving = false;

        // Self-restart (comic-server-9klu): true from the moment the user
        // confirms "Restart now" until the server has answered
        // /api/health again and the page reloads. Whether this platform
        // supports it at all comes from restartRequired.restart_supported.
        this.restarting = false;

        // Watch Folders (comic-server-obe): "dump" directories comic files
        // land in before being promoted into real library book records
        // (comic-server-chh's Workflow "New Files" stage). Read fresh at
        // the point of use server-side (see effectiveWatchFolders in
        // internal/api/watch_folders_settings.go), so - unlike the
        // restart-required section above - a folder added here is scanned
        // on the very next request, no restart needed.
        this.watchFolders = null; // string[]
        this.watchFoldersSaving = false;
    }

    async loadWatchFolders() {
        try {
            const response = await fetch('/api/settings/watch-folders');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.watchFolders = data.folders || [];
        } catch (error) {
            console.error('Failed to load watch folders:', error);
            this.watchFolders = [];
        }
    }

    async saveWatchFolders() {
        this.watchFoldersSaving = true;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/settings/watch-folders', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ folders: this.watchFolders }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text));
            const data = JSON.parse(text);
            this.watchFolders = data.folders || [];
            dialogs.toast('Watch folders saved.', 'success');
        } catch (error) {
            console.error('Failed to save watch folders:', error);
            dialogs.toast('Failed to save watch folders: ' + error.message, 'error');
        } finally {
            this.watchFoldersSaving = false;
            this.render();
            this.attachListeners();
        }
    }

    async addWatchFolder() {
        const picked = await dialogs.browseServerDirectory({ title: 'Add Watch Folder' });
        if (!picked) return;
        if (this.watchFolders.includes(picked)) {
            dialogs.toast('That folder is already in the list.', 'info');
            return;
        }
        this.watchFolders.push(picked);
        await this.saveWatchFolders();
    }

    removeWatchFolder(index) {
        this.watchFolders.splice(index, 1);
        this.saveWatchFolders();
    }

    async loadRestartRequired() {
        try {
            const response = await fetch('/api/settings/restart-required');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.restartRequired = data;
            this.restartRequiredForm = {
                library_path: data.saved.library_path || '',
                database_path: data.saved.database_path || '',
                cover_cache_dir: data.saved.cover_cache_dir || '',
                server_port: data.saved.server_port || 0,
                discovery_port: data.saved.discovery_port || 0,
                bind_address: data.saved.bind_address || '',
                comicvine_api_key: '',
                komga_enabled: !!data.saved.komga_enabled,
                komga_base_url: data.saved.komga_base_url || '',
                komga_api_key: '',
                komga_sync_interval_sec: data.saved.komga_sync_interval_sec || 0,
                max_concurrent_connections: data.saved.max_concurrent_connections || 0,
                max_connections_per_ip: data.saved.max_connections_per_ip || 0,
                max_requests_per_device: data.saved.max_requests_per_device || 0,
                rate_limit_window_seconds: data.saved.rate_limit_window_seconds || 0,
                library_cache_flush_interval_sec: data.saved.library_cache_flush_interval_sec || 0,
            };
        } catch (error) {
            console.error('Failed to load restart-required settings:', error);
        }
    }

    async saveRestartRequired() {
        this.restartRequiredSaving = true;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/settings/restart-required', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this.restartRequiredForm),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text));
            this.restartRequired = JSON.parse(text);
            this.restartRequiredForm.comicvine_api_key = '';
            this.restartRequiredForm.komga_api_key = '';
            dialogs.toast(
                this.restartRequired.restart_required
                    ? 'Saved. Restart comic-server to apply these changes.'
                    : 'Saved.',
                'success'
            );
        } catch (error) {
            console.error('Failed to save restart-required settings:', error);
            dialogs.toast('Failed to save: ' + error.message, 'error');
        } finally {
            this.restartRequiredSaving = false;
            this.render();
            this.attachListeners();
        }
    }

    // restartSupported is true when the server can re-exec itself in place
    // (POST /api/system/restart) - false on Windows or if unwired, in which
    // case the UI keeps its manual-restart wording and shows no button.
    restartSupported() {
        return !!(this.restartRequired && this.restartRequired.restart_supported);
    }

    renderRestartNowButton() {
        if (!this.restartSupported()) return '';
        return ` <button class="btn btn-primary restart-now-btn" ${this.restarting ? 'disabled' : ''}>` +
            `${this.restarting ? 'Restarting…' : 'Restart now'}</button>`;
    }

    // parseUptimeSeconds converts /api/health's Go Duration string
    // ("1h2m3.5s", "850ms", ...) to seconds, or NaN if unrecognised.
    parseUptimeSeconds(str) {
        if (typeof str !== 'string' || !str) return NaN;
        const unit = { h: 3600, m: 60, s: 1, ms: 0.001, 'µs': 1e-6, us: 1e-6, ns: 1e-9 };
        const re = /(\d+(?:\.\d+)?)(ms|µs|us|ns|h|m|s)/g;
        let total = 0, matched = false, m;
        while ((m = re.exec(str)) !== null) {
            total += parseFloat(m[1]) * unit[m[2]];
            matched = true;
        }
        return matched ? total : NaN;
    }

    async fetchUptimeSeconds() {
        const response = await fetch('/api/health', { cache: 'no-store' });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        return this.parseUptimeSeconds(data.uptime);
    }

    // restartNow confirms, asks the server to restart itself, then polls
    // /api/health until the NEW process answers (the old one can still
    // answer for a moment while it shuts down, so "answers" alone isn't
    // enough: we need to have seen it go down, or its uptime reset) and
    // reloads the page.
    async restartNow() {
        if (this.restarting) return;
        const ok = await dialogs.confirm({
            title: 'Restart comic-server',
            message: 'Restart comic-server now? Any sync in progress will be interrupted, and the web UI will be unavailable for a few seconds.',
            confirmLabel: 'Restart now',
        });
        if (!ok) return;

        let prevUptime = NaN;
        try { prevUptime = await this.fetchUptimeSeconds(); } catch (e) { /* best effort */ }

        this.restarting = true;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/system/restart', { method: 'POST' });
            if (!response.ok) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text));
            }
        } catch (error) {
            console.error('Failed to request restart:', error);
            dialogs.toast('Failed to restart: ' + error.message, 'error');
            this.restarting = false;
            this.render();
            this.attachListeners();
            return;
        }

        const sleep = (ms) => new Promise(r => setTimeout(r, ms));
        const deadline = Date.now() + 120000;
        let sawDown = false;
        await sleep(1000);
        while (Date.now() < deadline) {
            try {
                const uptime = await this.fetchUptimeSeconds();
                if (sawDown || (!Number.isNaN(prevUptime) && uptime < prevUptime)) {
                    window.location.reload();
                    return;
                }
            } catch (e) {
                sawDown = true;
            }
            await sleep(1000);
        }
        dialogs.toast('comic-server did not come back within 2 minutes - check the server logs.', 'error');
        this.restarting = false;
        this.render();
        this.attachListeners();
    }

    async loadServerMisc() {
        try {
            const response = await fetch('/api/settings/server-misc');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.serverMisc = { cbz_convert_enabled: !!data.cbz_convert_enabled, ignore_devices: data.ignore_devices || [], auto_sync: !!data.auto_sync };
        } catch (error) {
            console.error('Failed to load server settings:', error);
            this.serverMisc = { cbz_convert_enabled: false, ignore_devices: [], auto_sync: false };
        }
    }

    async init(ctx) {
        // Render once BEFORE the fetch so renderBody()'s existing
        // !this.config check shows "Loading..." immediately instead of
        // leaving the page blank until the fetch resolves
        // (comic-server-4te).
        this.render();
        await Promise.all([this.load(), this.loadTheme(), this.loadTrash(), this.loadImportStatus(), this.loadServerMisc(), this.loadRestartRequired(), this.loadWatchFolders()]);
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async loadImportStatus() {
        try {
            const response = await fetch('/api/settings/library-import');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.importJob = data.job || null; // null: no import has ever run
        } catch (error) {
            console.error('Failed to load library import status:', error);
        }
    }

    async load() {
        try {
            const response = await fetch('/api/settings/scan-info');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            const data = await response.json();
            this.config = {
                enabled: data.enabled || false,
                scanners: data.scanners || [],
                blacklist: data.blacklist || [],
                prefix: data.prefix || '',
                unknown: data.unknown || ''
            };
        } catch (error) {
            console.error('Failed to load scan info settings:', error);
            this.error = 'Failed to load scan info settings. Please try again.';
        }
    }

    async loadTheme() {
        try {
            const response = await fetch('/api/settings/theme');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.theme = data.theme || 'system';
        } catch (error) {
            console.error('Failed to load theme setting:', error);
            this.theme = 'system';
        }
    }

    async saveTheme(theme) {
        this.themeSaving = true;
        this.theme = theme;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/settings/theme', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ theme }),
            });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            // The default just changed - apply it to THIS window too,
            // unless this window already has its own toggle override
            // (sessionStorage), same rule as any other window.
            if (typeof themeManager !== 'undefined' && sessionStorage.getItem('comic-server-theme-override') === null) {
                themeManager.applyOverride(theme);
                themeManager.updateToggleButton();
            }
            dialogs.toast('Default theme saved.', 'success');
        } catch (error) {
            console.error('Failed to save theme setting:', error);
            dialogs.toast('Failed to save theme setting: ' + error.message, 'error');
        } finally {
            this.themeSaving = false;
            this.render();
            this.attachListeners();
        }
    }

    async loadTrash() {
        try {
            const response = await fetch('/api/settings/trash');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.trash = { path: data.path || '', retention_days: data.retention_days ?? 30 };
        } catch (error) {
            console.error('Failed to load trash settings:', error);
            this.trash = { path: '', retention_days: 30 };
        }
    }

    async saveTrash() {
        this.trashSaving = true;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/settings/trash', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this.trash),
            });
            if (!response.ok) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text));
            }
            dialogs.toast('Trash settings saved.', 'success');
        } catch (error) {
            console.error('Failed to save trash settings:', error);
            dialogs.toast('Failed to save trash settings: ' + error.message, 'error');
        } finally {
            this.trashSaving = false;
            this.render();
            this.attachListeners();
        }
    }

    addIgnoreDevice() {
        const input = document.getElementById('server-misc-ignore-input');
        if (!input) return;
        const value = input.value.trim();
        if (!value) return;
        this.serverMisc.ignore_devices.push(value);
        this.saveServerMisc();
    }

    async saveServerMisc() {
        this.serverMiscSaving = true;
        this.render();
        this.attachListeners();
        try {
            const response = await fetch('/api/settings/server-misc', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this.serverMisc),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text));
            this.serverMisc = JSON.parse(text);
            dialogs.toast('Server settings saved.', 'success');
        } catch (error) {
            console.error('Failed to save server settings:', error);
            dialogs.toast('Failed to save server settings: ' + error.message, 'error');
        } finally {
            this.serverMiscSaving = false;
            this.render();
            this.attachListeners();
        }
    }

    async runLibraryImport() {
        if (!this.selectedFile) return;
        this.importing = true;
        this.importError = null;
        this.render();
        this.attachListeners();

        try {
            const formData = new FormData();
            formData.append('file', this.selectedFile);
            const response = await fetch('/api/settings/library-import', { method: 'POST', body: formData });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text));
            this.importJob = JSON.parse(text);
            if (this.importJob.status === 'failed') {
                dialogs.toast('Import failed: ' + (this.importJob.error || 'unknown error'), 'error');
            } else {
                dialogs.toast('Library import complete.', 'success');
            }
        } catch (error) {
            console.error('Failed to import library:', error);
            this.importError = `Failed: ${error.message}`;
        } finally {
            this.importing = false;
            this.selectedFile = null;
            this.render();
            this.attachListeners();
        }
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="scan-info-settings-page">
                <div class="scan-info-settings-header">
                    <h1>Settings</h1>
                </div>
                ${this.renderAppearance()}
                ${this.renderLibraryImport()}
                ${this.renderServerMisc()}
                ${this.renderWatchFolders()}
                ${this.renderRestartRequired()}
                ${this.renderTrash()}
                ${this.renderBody()}
            </div>
        `;
    }

    renderLibraryImport() {
        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Library Import</h2>
                    <p class="settings-section-description">Import a ComicDb.xml exported from ComicRack. A one-time migration, or an occasional refresh - not an ongoing sync.</p>
                </div>

                <div class="form-group">
                    <input type="file" id="library-import-file" accept=".xml">
                </div>

                <div class="scan-info-actions">
                    <button class="btn btn-primary" id="library-import-btn" ${this.importing || !this.selectedFile ? 'disabled' : ''}>
                        ${this.importing ? 'Uploading and importing…' : 'Import'}
                    </button>
                </div>

                ${this.renderImportResult()}
            </div>
        `;
    }

    renderImportResult() {
        if (this.importError) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.importError)}</p>`;
        }
        const job = this.importJob;
        if (!job) return '';

        if (job.status === 'failed') {
            return `<p class="datamanager-errors">Import failed: ${this.escapeHtml(job.error || 'unknown error')}</p>`;
        }
        if (job.status !== 'completed' || !job.stats) return '';

        const s = job.stats;
        let html = `<p class="datamanager-summary">Imported "${this.escapeHtml(job.filename)}" in ${s.duration_sec.toFixed(1)}s: ` +
            `${s.books_added} added, ${s.books_updated} updated, ${s.books_deleted} deleted, ${s.books_unchanged} unchanged.</p>`;
        if (job.restart_required) {
            html += `<p class="datamanager-errors">This provisioned a new database - restart comic-server to start serving from it.${this.renderRestartNowButton()}</p>`;
        }
        return html;
    }

    renderServerMisc() {
        const m = this.serverMisc || { cbz_convert_enabled: false, ignore_devices: [], auto_sync: false };
        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Server</h2>
                    <p class="settings-section-description">All take effect immediately - no restart needed.</p>
                </div>

                <div class="form-group">
                    <label class="toggle-switch">
                        <input type="checkbox" id="server-misc-cbz-convert" ${m.cbz_convert_enabled ? 'checked' : ''}>
                        <span class="toggle-slider"></span>
                    </label>
                    <span class="form-label-inline">Convert to CBZ enabled</span>
                    <div class="form-help">Requires Trash to be configured above - repacks a comic archive as CBZ and embeds ComicInfo.xml, retiring the original into quarantine.</div>
                </div>

                <div class="form-group">
                    <label class="toggle-switch">
                        <input type="checkbox" id="server-misc-auto-sync" ${m.auto_sync ? 'checked' : ''}>
                        <span class="toggle-slider"></span>
                    </label>
                    <span class="form-label-inline">Auto-sync on connect</span>
                    <div class="form-help">Automatically start a sync whenever a device connects and requests one, instead of waiting for a manual sync.</div>
                </div>

                <div class="form-group scan-info-list-group">
                    <label>Ignore Devices</label>
                    <div class="form-help">Devices to never discover or sync with - by IP address, device ID, or device name.</div>
                    ${m.ignore_devices.length === 0 ? `
                        <p class="empty-hint">No entries yet.</p>
                    ` : `
                        <ul class="scan-info-list" data-field="ignore_devices">
                            ${m.ignore_devices.map((value, i) => `
                                <li class="scan-info-list-item">
                                    <span class="scan-info-list-value">${this.escapeHtml(value)}</span>
                                    <button type="button" class="btn btn-small btn-danger server-misc-ignore-remove" data-index="${i}">✕</button>
                                </li>
                            `).join('')}
                        </ul>
                    `}
                    <div class="scan-info-list-add form-row">
                        <input type="text" class="form-control" id="server-misc-ignore-input" placeholder="e.g. 192.168.0.24">
                        <button type="button" class="btn btn-secondary" id="server-misc-ignore-add-btn">+ Add</button>
                    </div>
                </div>
            </div>
        `;
    }

    // renderWatchFolders shows "dump" directories comic files land in
    // before being promoted into real library book records
    // (comic-server-chh, comic-server-obe). Each add/remove saves
    // immediately (no separate Save button) - matches the Server panel's
    // own "both take effect immediately" pattern above, since this is
    // also live-effect, not restart-required.
    renderWatchFolders() {
        const folders = this.watchFolders || [];
        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Watch Folders</h2>
                    <p class="settings-section-description">Directories scanned for comic files not yet in the library - see the Workflow dashboard's "New Files" stage to promote them.</p>
                </div>

                ${folders.length === 0 ? `
                    <p class="empty-hint">No watch folders configured.</p>
                ` : `
                    <ul class="scan-info-list" data-field="watch_folders">
                        ${folders.map((path, i) => `
                            <li class="scan-info-list-item">
                                <span class="scan-info-list-value">${this.escapeHtml(path)}</span>
                                <button type="button" class="btn btn-small btn-danger watch-folder-remove-btn" data-index="${i}" ${this.watchFoldersSaving ? 'disabled' : ''}>✕</button>
                            </li>
                        `).join('')}
                    </ul>
                `}

                <div class="scan-info-actions">
                    <button class="btn btn-secondary" id="watch-folder-add-btn" ${this.watchFoldersSaving ? 'disabled' : ''}>+ Add Watch Folder</button>
                </div>
            </div>
        `;
    }

    // renderRestartRequired shows library path/network/ComicVine/Komga
    // connection settings - every one baked into an object or listener
    // socket once at startup (comic-server-yvbh), unlike Server above.
    // The two API key inputs are password fields that start blank; a
    // "configured" hint next to each shows whether a key is already
    // saved without ever sending it back to the browser.
    renderRestartRequired() {
        const f = this.restartRequiredForm;
        if (!f) {
            return `<div class="panel scan-info-panel"><h2>Server (requires restart)</h2><p class="empty-hint">Loading...</p></div>`;
        }
        const saved = this.restartRequired.saved;
        const banner = this.restartRequired.restart_required
            ? `<p class="datamanager-errors">⚠ Saved changes below differ from what's currently running - restart comic-server to apply them.${this.renderRestartNowButton()}</p>`
            : '';

        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Server (requires restart)</h2>
                    <p class="settings-section-description">Library path, network ports, bind address, ComicVine key, and Komga connection - none of these take effect until comic-server restarts.</p>
                </div>
                ${banner}

                <div class="form-group">
                    <label for="rr-library-path">Library path</label>
                    <input type="text" id="rr-library-path" class="form-control" value="${this.escapeAttr(f.library_path)}" placeholder="/data/ComicDb.xml">
                    <div class="form-help">The XML library. Leave blank if using the experimental SQLite database path below instead.</div>
                </div>

                <div class="form-group">
                    <label for="rr-database-path">Database path</label>
                    <input type="text" id="rr-database-path" class="form-control" value="${this.escapeAttr(f.database_path)}" placeholder="(blank = use Library path above)">
                    <div class="form-help">Experimental SQLite storage - an alternative to Library path, not used alongside it.</div>
                </div>

                <div class="form-group">
                    <label for="rr-cover-cache-dir">Cover cache directory</label>
                    <input type="text" id="rr-cover-cache-dir" class="form-control" value="${this.escapeAttr(f.cover_cache_dir)}" placeholder="(blank = XDG cache dir)">
                </div>

                <div class="form-group">
                    <label for="rr-server-port">Server port</label>
                    <input type="number" id="rr-server-port" class="form-control" min="1" max="65535" value="${f.server_port}" style="max-width:8rem;">
                </div>

                <div class="form-group">
                    <label for="rr-discovery-port">Discovery port</label>
                    <input type="number" id="rr-discovery-port" class="form-control" min="1" max="65535" value="${f.discovery_port}" style="max-width:8rem;">
                </div>

                <div class="form-group">
                    <label for="rr-bind-address">Bind address</label>
                    <input type="text" id="rr-bind-address" class="form-control" value="${this.escapeAttr(f.bind_address)}" placeholder="(blank = all interfaces)">
                </div>

                <div class="form-group">
                    <label for="rr-comicvine-key">ComicVine API key</label>
                    <input type="password" id="rr-comicvine-key" class="form-control" value="${this.escapeAttr(f.comicvine_api_key)}" placeholder="${saved.comicvine_api_key_set ? 'Configured - leave blank to keep' : 'Not set'}" autocomplete="off">
                </div>

                <div class="form-group">
                    <label class="toggle-switch">
                        <input type="checkbox" id="rr-komga-enabled" ${f.komga_enabled ? 'checked' : ''}>
                        <span class="toggle-slider"></span>
                    </label>
                    <span class="form-label-inline">Komga sync enabled</span>
                </div>

                <div class="form-group">
                    <label for="rr-komga-base-url">Komga URL</label>
                    <input type="text" id="rr-komga-base-url" class="form-control" value="${this.escapeAttr(f.komga_base_url)}" placeholder="https://komga.example.com">
                </div>

                <div class="form-group">
                    <label for="rr-komga-key">Komga API key</label>
                    <input type="password" id="rr-komga-key" class="form-control" value="${this.escapeAttr(f.komga_api_key)}" placeholder="${saved.komga_api_key_set ? 'Configured - leave blank to keep' : 'Not set'}" autocomplete="off">
                </div>

                <div class="form-group">
                    <label for="rr-komga-interval">Komga sync interval (seconds)</label>
                    <input type="number" id="rr-komga-interval" class="form-control" min="1" value="${f.komga_sync_interval_sec}" style="max-width:8rem;">
                </div>

                <div class="settings-section-header">
                    <h3>Advanced</h3>
                    <p class="settings-section-description">Connection limits and rate limiting - safe to leave at their defaults.</p>
                </div>

                <div class="form-group">
                    <label for="rr-max-concurrent-connections">Max concurrent connections</label>
                    <input type="number" id="rr-max-concurrent-connections" class="form-control" min="0" value="${f.max_concurrent_connections}" style="max-width:8rem;">
                    <div class="form-help">0 = unlimited.</div>
                </div>

                <div class="form-group">
                    <label for="rr-max-connections-per-ip">Max connections per IP</label>
                    <input type="number" id="rr-max-connections-per-ip" class="form-control" min="0" value="${f.max_connections_per_ip}" style="max-width:8rem;">
                    <div class="form-help">Per rate-limit window below. 0 = unlimited.</div>
                </div>

                <div class="form-group">
                    <label for="rr-max-requests-per-device">Max requests per device</label>
                    <input type="number" id="rr-max-requests-per-device" class="form-control" min="0" value="${f.max_requests_per_device}" style="max-width:8rem;">
                    <div class="form-help">Per rate-limit window below. 0 = unlimited.</div>
                </div>

                <div class="form-group">
                    <label for="rr-rate-limit-window">Rate limit window (seconds)</label>
                    <input type="number" id="rr-rate-limit-window" class="form-control" min="1" value="${f.rate_limit_window_seconds}" style="max-width:8rem;">
                </div>

                <div class="form-group">
                    <label for="rr-cache-flush-interval">Library cache flush interval (seconds)</label>
                    <input type="number" id="rr-cache-flush-interval" class="form-control" min="0" value="${f.library_cache_flush_interval_sec}" style="max-width:8rem;">
                    <div class="form-help">0 = flush on every change.</div>
                </div>

                <div class="scan-info-actions">
                    <button class="btn btn-primary" id="rr-save" ${this.restartRequiredSaving ? 'disabled' : ''}>
                        ${this.restartRequiredSaving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>
        `;
    }

    renderTrash() {
        const t = this.trash || { path: '', retention_days: 30 };
        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Trash</h2>
                    <p class="settings-section-description">Quarantine directory for features that replace or delete a comic file (Convert to CBZ, Library Organizer).</p>
                </div>

                <div class="form-group">
                    <label for="trash-path">Quarantine directory</label>
                    <input type="text" id="trash-path" class="form-control" value="${this.escapeAttr(t.path)}" placeholder="/data/trash (leave blank to disable)">
                    <div class="form-help">Empty disables Convert to CBZ and Library Organizer's apply step - neither can run without somewhere to quarantine the files they replace.</div>
                </div>

                <div class="form-group">
                    <label for="trash-retention">Retention (days)</label>
                    <input type="number" id="trash-retention" class="form-control" min="0" value="${t.retention_days}" style="max-width:8rem;">
                    <div class="form-help">A quarantined file is permanently deleted this many days after being replaced.</div>
                </div>

                <div class="scan-info-actions">
                    <button class="btn btn-primary" id="trash-save" ${this.trashSaving ? 'disabled' : ''}>
                        ${this.trashSaving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>
        `;
    }

    renderAppearance() {
        const theme = this.theme || 'system';
        const options = [
            { value: 'light', label: 'Light' },
            { value: 'dark', label: 'Dark' },
            { value: 'system', label: 'Match system' },
        ];
        return `
            <div class="panel scan-info-panel">
                <h2>Appearance</h2>
                <p class="empty-message" style="text-align:left;padding:0 0 0.75rem 0;">
                    Default theme for new windows/tabs. Use the 🌙/☀️ button in the header to override just the current window - that never affects this default.
                </p>
                <div class="datamanager-actions">
                    ${options.map(o => `
                        <label style="display:inline-flex;align-items:center;gap:0.4rem;margin-right:1rem;">
                            <input type="radio" name="theme-default" value="${o.value}" ${theme === o.value ? 'checked' : ''} ${this.themeSaving ? 'disabled' : ''}>
                            ${o.label}
                        </label>
                    `).join('')}
                </div>
            </div>
        `;
    }

    renderBody() {
        if (this.error) {
            return `<div class="empty-state"><p>${this.escapeHtml(this.error)}</p></div>`;
        }
        if (!this.config) {
            return `<div class="loading-spinner">Loading...</div>`;
        }

        const c = this.config;

        return `
            <div class="panel scan-info-panel">
                <div class="settings-section-header">
                    <h2>Scan Information Detection</h2>
                    <p class="settings-section-description">ComicRack's ScanInformationFromFilename</p>
                </div>

                <div class="form-group">
                    <label class="toggle-switch">
                        <input type="checkbox" id="scan-info-enabled" ${c.enabled ? 'checked' : ''}>
                        <span class="toggle-slider"></span>
                    </label>
                    <span class="form-label-inline">Enabled</span>
                    <div class="form-help">When enabled, "Run Scan Info" on a smart list detects each book's scan group from its filename and writes it to ScanInformation.</div>
                </div>

                <div class="form-group">
                    <label for="scan-info-prefix">Tag prefix</label>
                    <input type="text" id="scan-info-prefix" class="form-control" value="${this.escapeAttr(c.prefix)}" placeholder="Scanner:">
                </div>

                <div class="form-group">
                    <label for="scan-info-unknown">Unknown tag</label>
                    <input type="text" id="scan-info-unknown" class="form-control" value="${this.escapeAttr(c.unknown)}" placeholder="Unknown">
                    <div class="form-help">Used when detection fails. Leave blank to skip the book instead of tagging it.</div>
                </div>

                ${this.renderStringList('scanners', 'Scanners', 'Known scan-group/release-team names, matched literally against the filename.', 'e.g. DCP')}
                ${this.renderStringList('blacklist', 'Blacklist', 'Regex fragments (not plain words) describing generic filename noise to ignore when extracting a bracketed tag.', 'e.g. digital')}

                <div class="scan-info-actions">
                    <button class="btn btn-primary" id="scan-info-save" ${this.saving ? 'disabled' : ''}>
                        ${this.saving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>
        `;
    }

    renderStringList(field, label, help, placeholder) {
        const items = this.config[field];
        return `
            <div class="form-group scan-info-list-group">
                <label>${label}</label>
                <div class="form-help">${help}</div>
                ${items.length === 0 ? `
                    <p class="empty-hint">No entries yet.</p>
                ` : `
                    <ul class="scan-info-list" data-field="${field}">
                        ${items.map((value, i) => `
                            <li class="scan-info-list-item">
                                <span class="scan-info-list-value">${this.escapeHtml(value)}</span>
                                <button type="button" class="btn btn-small btn-danger scan-info-list-remove" data-field="${field}" data-index="${i}">✕</button>
                            </li>
                        `).join('')}
                    </ul>
                `}
                <div class="scan-info-list-add form-row">
                    <input type="text" class="form-control scan-info-list-input" data-field="${field}" placeholder="${placeholder}">
                    <button type="button" class="btn btn-secondary scan-info-list-add-btn" data-field="${field}">+ Add</button>
                </div>
            </div>
        `;
    }

    attachListeners() {
        const cbzConvertToggle = document.getElementById('server-misc-cbz-convert');
        if (cbzConvertToggle) {
            cbzConvertToggle.addEventListener('change', (e) => {
                this.serverMisc.cbz_convert_enabled = e.target.checked;
                this.saveServerMisc();
            });
        }
        const autoSyncToggle = document.getElementById('server-misc-auto-sync');
        if (autoSyncToggle) {
            autoSyncToggle.addEventListener('change', (e) => {
                this.serverMisc.auto_sync = e.target.checked;
                this.saveServerMisc();
            });
        }
        const ignoreAddBtn = document.getElementById('server-misc-ignore-add-btn');
        if (ignoreAddBtn) ignoreAddBtn.addEventListener('click', () => this.addIgnoreDevice());
        const ignoreInput = document.getElementById('server-misc-ignore-input');
        if (ignoreInput) {
            ignoreInput.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    this.addIgnoreDevice();
                }
            });
        }
        document.querySelectorAll('.server-misc-ignore-remove').forEach(btn => {
            btn.addEventListener('click', () => {
                this.serverMisc.ignore_devices.splice(parseInt(btn.dataset.index, 10), 1);
                this.saveServerMisc();
            });
        });

        const importFileInput = document.getElementById('library-import-file');
        if (importFileInput) {
            importFileInput.addEventListener('change', (e) => {
                this.selectedFile = e.target.files[0] || null;
                const btn = document.getElementById('library-import-btn');
                if (btn) btn.disabled = this.importing || !this.selectedFile;
            });
        }
        const importBtn = document.getElementById('library-import-btn');
        if (importBtn) {
            importBtn.addEventListener('click', () => this.runLibraryImport());
        }

        document.querySelectorAll('input[name="theme-default"]').forEach(el => {
            el.addEventListener('change', () => {
                if (el.checked) this.saveTheme(el.value);
            });
        });

        const trashPathInput = document.getElementById('trash-path');
        if (trashPathInput) {
            trashPathInput.addEventListener('input', (e) => {
                this.trash.path = e.target.value;
            });
        }
        const trashRetentionInput = document.getElementById('trash-retention');
        if (trashRetentionInput) {
            trashRetentionInput.addEventListener('input', (e) => {
                this.trash.retention_days = parseInt(e.target.value, 10) || 0;
            });
        }
        const trashSaveBtn = document.getElementById('trash-save');
        if (trashSaveBtn) {
            trashSaveBtn.addEventListener('click', () => this.saveTrash());
        }

        const addWatchFolderBtn = document.getElementById('watch-folder-add-btn');
        if (addWatchFolderBtn) addWatchFolderBtn.addEventListener('click', () => this.addWatchFolder());
        document.querySelectorAll('.watch-folder-remove-btn').forEach(btn => {
            btn.addEventListener('click', () => this.removeWatchFolder(parseInt(btn.dataset.index, 10)));
        });

        document.querySelectorAll('.restart-now-btn').forEach(btn => {
            btn.addEventListener('click', () => this.restartNow());
        });

        if (this.restartRequiredForm) {
            const bindField = (id, key, transform) => {
                const el = document.getElementById(id);
                if (el) el.addEventListener('input', (e) => {
                    this.restartRequiredForm[key] = transform ? transform(e.target.value) : e.target.value;
                });
            };
            bindField('rr-library-path', 'library_path');
            bindField('rr-database-path', 'database_path');
            bindField('rr-cover-cache-dir', 'cover_cache_dir');
            bindField('rr-server-port', 'server_port', v => parseInt(v, 10) || 0);
            bindField('rr-discovery-port', 'discovery_port', v => parseInt(v, 10) || 0);
            bindField('rr-bind-address', 'bind_address');
            bindField('rr-comicvine-key', 'comicvine_api_key');
            bindField('rr-komga-base-url', 'komga_base_url');
            bindField('rr-komga-key', 'komga_api_key');
            bindField('rr-komga-interval', 'komga_sync_interval_sec', v => parseInt(v, 10) || 0);
            bindField('rr-max-concurrent-connections', 'max_concurrent_connections', v => parseInt(v, 10) || 0);
            bindField('rr-max-connections-per-ip', 'max_connections_per_ip', v => parseInt(v, 10) || 0);
            bindField('rr-max-requests-per-device', 'max_requests_per_device', v => parseInt(v, 10) || 0);
            bindField('rr-rate-limit-window', 'rate_limit_window_seconds', v => parseInt(v, 10) || 0);
            bindField('rr-cache-flush-interval', 'library_cache_flush_interval_sec', v => parseInt(v, 10) || 0);

            const komgaEnabledToggle = document.getElementById('rr-komga-enabled');
            if (komgaEnabledToggle) {
                komgaEnabledToggle.addEventListener('change', (e) => {
                    this.restartRequiredForm.komga_enabled = e.target.checked;
                });
            }

            const rrSaveBtn = document.getElementById('rr-save');
            if (rrSaveBtn) rrSaveBtn.addEventListener('click', () => this.saveRestartRequired());
        }

        if (!this.config) return;

        const enabledInput = document.getElementById('scan-info-enabled');
        if (enabledInput) {
            enabledInput.addEventListener('change', (e) => {
                this.config.enabled = e.target.checked;
                this.dirty = true;
            });
        }

        const prefixInput = document.getElementById('scan-info-prefix');
        if (prefixInput) {
            prefixInput.addEventListener('input', (e) => {
                this.config.prefix = e.target.value;
                this.dirty = true;
            });
        }

        const unknownInput = document.getElementById('scan-info-unknown');
        if (unknownInput) {
            unknownInput.addEventListener('input', (e) => {
                this.config.unknown = e.target.value;
                this.dirty = true;
            });
        }

        document.querySelectorAll('.scan-info-list-add-btn').forEach(btn => {
            btn.addEventListener('click', () => this.addListItem(btn.dataset.field));
        });
        document.querySelectorAll('.scan-info-list-input').forEach(input => {
            input.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    this.addListItem(input.dataset.field);
                }
            });
        });
        document.querySelectorAll('.scan-info-list-remove').forEach(btn => {
            btn.addEventListener('click', () => {
                const field = btn.dataset.field;
                const index = parseInt(btn.dataset.index, 10);
                this.config[field].splice(index, 1);
                this.dirty = true;
                this.render();
                this.attachListeners();
            });
        });

        const saveBtn = document.getElementById('scan-info-save');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => this.save());
        }
    }

    addListItem(field) {
        const input = document.querySelector(`.scan-info-list-input[data-field="${field}"]`);
        if (!input) return;
        const value = input.value.trim();
        if (!value) return;
        this.config[field].push(value);
        this.dirty = true;
        this.render();
        this.attachListeners();
        // Refocus the (freshly re-rendered) input for rapid multi-entry.
        const newInput = document.querySelector(`.scan-info-list-input[data-field="${field}"]`);
        if (newInput) newInput.focus();
    }

    async save() {
        this.saving = true;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch('/api/settings/scan-info', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    enabled: this.config.enabled,
                    scanners: this.config.scanners,
                    blacklist: this.config.blacklist,
                    prefix: this.config.prefix,
                    unknown: this.config.unknown
                })
            });

            if (!response.ok) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text));
            }

            this.dirty = false;
            dialogs.toast('Scan info settings saved.', 'success');
        } catch (error) {
            console.error('Failed to save scan info settings:', error);
            dialogs.toast(`Failed to save: ${error.message}`, 'error');
        } finally {
            this.saving = false;
            this.render();
            this.attachListeners();
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text || '';
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }
}

window.ScanInfoSettings = ScanInfoSettings;
