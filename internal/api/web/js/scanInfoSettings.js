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
    }

    async init(ctx) {
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
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

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="scan-info-settings-page">
                <div class="scan-info-settings-header">
                    <h1>Settings</h1>
                    <p class="scan-info-settings-subtitle">Scan information detection (ComicRack's ScanInformationFromFilename)</p>
                </div>
                ${this.renderBody()}
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
                throw new Error(text || `HTTP ${response.status}`);
            }

            this.dirty = false;
            alert('Scan info settings saved.');
        } catch (error) {
            console.error('Failed to save scan info settings:', error);
            alert(`Failed to save: ${error.message}`);
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
