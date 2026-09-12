// Library Organizer Profile Editor (comic-server-7ecr) - create, edit,
// and delete profiles entirely through the web UI, with no
// losettingsx.dat import required. First slice covers only the fields
// Plan/Apply actually read (BaseFolder/FolderTemplate/FileTemplate/
// CopyMode/ReplaceMultipleSpaces/EmptyFolder/FilelessFormat + exclude
// rules) - the rest of LOProfile's fields are either dead weight
// (comic-server-b2al) or deferred lookup tables (comic-server-kt4w) and
// keep whatever value they already have (see the API's mergeLOProfileWire).
//
// Profiles created/edited here are read by the exact same
// configdb.GetLOProfile path organizePage.js's own Preview/Apply already
// use, so a profile created here is immediately selectable there.
class LibraryOrganizerProfilesPage {
    constructor() {
        this.schema = null; // { excludeFields, excludeOperators }
        this.profiles = [];
        this.expandedId = null;
        this.loading = true;
        this.error = null;
    }

    async init(ctx) {
        this.render();
        await Promise.all([this.loadSchema(), this.loadProfiles()]);
        if (ctx && ctx.aborted) return;
        this.loading = false;
        this.render();
        this.attachListeners();
    }

    async loadSchema() {
        try {
            const response = await fetch('/api/library/organize-profiles/schema');
            this.schema = await response.json();
        } catch (error) {
            console.error('Failed to load Library Organizer schema:', error);
            this.schema = { excludeFields: [], excludeOperators: [] };
        }
    }

    async loadProfiles() {
        try {
            const response = await fetch('/api/library/organize-profiles');
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to load profiles'));
            // The list endpoint only returns summaries (id/name/copy_mode) -
            // fetch each profile's full detail up front since this page
            // (unlike Data Manager's tree) is small and always shows the
            // whole set, so there's no lazy-load benefit to deferring it.
            const data = JSON.parse(text);
            const summaries = data.profiles || [];
            this.profiles = await Promise.all(summaries.map(async (s) => {
                const r = await fetch(`/api/library/organize-profiles/${encodeURIComponent(s.id)}`);
                return r.json();
            }));
        } catch (error) {
            console.error('Failed to load profiles:', error);
            this.error = `Failed: ${error.message}`;
        }
    }

    async refreshProfiles() {
        await this.loadProfiles();
        this.renderAndReattach();
    }

    findProfile(id) {
        return this.profiles.find(p => p.id === id);
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Library Organizer Profiles</h1>
                    <p class="empty-message">Create and edit the profiles Library Organizer's Preview/Apply move books under. Templates use ComicRack's own <code>{&lt;field&gt;}</code> syntax, e.g. <code>{&lt;publisher&gt;}\\{&lt;series&gt;}</code>.</p>
                </div>
                <div class="panel">
                    <div class="datamanager-actions">
                        <button class="btn btn-primary" id="lop-new-btn">+ New Profile</button>
                    </div>
                    ${this.loading ? '<p class="empty-message">Loading…</p>' : this.renderBody()}
                </div>
            </div>
        `;
    }

    renderBody() {
        if (this.error) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.error)}</p>`;
        }
        if (this.profiles.length === 0) {
            return '<p class="empty-message">No profiles yet. Click "New Profile" to create one, or import from ComicRack via the CLI.</p>';
        }
        return `<ul class="dmr-tree">${this.profiles.map(p => this.renderProfile(p)).join('')}</ul>`;
    }

    renderProfile(p) {
        const expanded = this.expandedId === p.id;
        return `
            <li class="dmr-tree-node dmr-ruleset-node" data-id="${this.escapeAttr(p.id)}">
                <div class="dmr-node-row">
                    <button class="btn btn-small lop-toggle-btn" data-id="${this.escapeAttr(p.id)}">${expanded ? '▾' : '▸'}</button>
                    <span class="dmr-node-name">${this.escapeHtml(p.name)}</span>
                    <span class="dmr-ruleset-summary">${p.copy_mode ? 'Copy' : 'Move'} · ${p.exclude_rules.length} exclude rule${p.exclude_rules.length === 1 ? '' : 's'}</span>
                    <button class="btn btn-small lop-delete-btn" data-id="${this.escapeAttr(p.id)}">Delete</button>
                </div>
                ${expanded ? this.renderProfileDetail(p) : ''}
            </li>
        `;
    }

    renderProfileDetail(p) {
        return `
            <div class="dmr-ruleset-detail">
                <div class="dmr-column" style="flex-basis:100%;">
                    <div class="form-group">
                        <label>Name</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="name" value="${this.escapeAttr(p.name)}">
                    </div>
                    <div class="form-group">
                        <label>Base folder</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="base_folder" value="${this.escapeAttr(p.base_folder)}" placeholder="/comics">
                    </div>
                    <div class="form-group">
                        <label>Folder template</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="folder_template" value="${this.escapeAttr(p.folder_template)}" placeholder="{&lt;publisher&gt;}\\{&lt;series&gt;}">
                    </div>
                    <div class="form-group">
                        <label>File template</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="file_template" value="${this.escapeAttr(p.file_template)}" placeholder="{&lt;series&gt;} #{&lt;number2&gt;}">
                    </div>
                    <div class="form-group">
                        <label><input type="checkbox" class="lop-field" data-id="${p.id}" data-field="copy_mode" ${p.copy_mode ? 'checked' : ''}> Copy instead of move</label>
                    </div>
                    <div class="form-group">
                        <label><input type="checkbox" class="lop-field" data-id="${p.id}" data-field="replace_multiple_spaces" ${p.replace_multiple_spaces ? 'checked' : ''}> Collapse multiple spaces</label>
                    </div>
                    <div class="form-group">
                        <label>Empty-folder fallback text</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="empty_folder" value="${this.escapeAttr(p.empty_folder)}" placeholder="Unknown">
                    </div>
                    <div class="form-group">
                        <label>Fileless extension</label>
                        <input type="text" class="form-control lop-field" data-id="${p.id}" data-field="fileless_format" value="${this.escapeAttr(p.fileless_format)}" placeholder=".jpg" style="max-width:8rem;">
                    </div>

                    <h3>Exclude rules</h3>
                    <div class="form-group">
                        <label>Mode:
                            <select class="lop-field" data-id="${p.id}" data-field="exclude_mode">
                                <option value="Do not" ${p.exclude_mode !== 'Only' ? 'selected' : ''}>Do not move books that qualify</option>
                                <option value="Only" ${p.exclude_mode === 'Only' ? 'selected' : ''}>Only move books that qualify</option>
                            </select>
                        </label>
                        <label>Qualify when:
                            <select class="lop-field" data-id="${p.id}" data-field="exclude_operator">
                                <option value="Any" ${p.exclude_operator !== 'All' ? 'selected' : ''}>ANY rule matches</option>
                                <option value="All" ${p.exclude_operator === 'All' ? 'selected' : ''}>ALL rules match</option>
                            </select>
                        </label>
                    </div>
                    <ul class="dmr-row-list">
                        ${p.exclude_rules.map(r => this.renderRuleRow(p.id, r)).join('')}
                    </ul>
                    ${this.renderNewRuleForm(p.id)}
                </div>
            </div>
        `;
    }

    fieldOptions(selected) {
        return (this.schema.excludeFields || []).map(f =>
            `<option value="${this.escapeAttr(f)}" ${f === selected ? 'selected' : ''}>${this.escapeHtml(f)}</option>`
        ).join('');
    }

    operatorOptions(selected) {
        return (this.schema.excludeOperators || []).map(o =>
            `<option value="${this.escapeAttr(o)}" ${o === selected ? 'selected' : ''}>${this.escapeHtml(o)}</option>`
        ).join('');
    }

    renderRuleRow(profileId, rule) {
        return `
            <li class="dmr-editor-row" data-profile="${this.escapeAttr(profileId)}" data-rule-id="${rule.id}">
                <select class="lop-rule-field" data-id="${rule.id}" data-profile="${this.escapeAttr(profileId)}">${this.fieldOptions(rule.field)}</select>
                <select class="lop-rule-operator" data-id="${rule.id}" data-profile="${this.escapeAttr(profileId)}">${this.operatorOptions(rule.operator)}</select>
                <input type="text" class="dmr-value-input lop-rule-value" data-id="${rule.id}" data-profile="${this.escapeAttr(profileId)}" value="${this.escapeAttr(rule.value)}" placeholder="value">
                <button class="btn btn-small btn-danger lop-delete-rule-btn" data-id="${rule.id}">✕</button>
            </li>
        `;
    }

    renderNewRuleForm(profileId) {
        const fields = this.schema.excludeFields || [];
        const operators = this.schema.excludeOperators || [];
        return `
            <div class="dmr-new-row" data-profile="${this.escapeAttr(profileId)}">
                <select class="lop-new-rule-field" data-profile="${this.escapeAttr(profileId)}">${this.fieldOptions(fields[0])}</select>
                <select class="lop-new-rule-operator" data-profile="${this.escapeAttr(profileId)}">${this.operatorOptions(operators[0])}</select>
                <input type="text" class="lop-new-rule-value" data-profile="${this.escapeAttr(profileId)}" placeholder="value">
                <button class="btn btn-small lop-add-rule-btn" data-profile="${this.escapeAttr(profileId)}">+ Add Rule</button>
            </div>
        `;
    }

    attachListeners() {
        const newBtn = document.getElementById('lop-new-btn');
        if (newBtn) newBtn.addEventListener('click', () => this.createProfile());

        document.querySelectorAll('.lop-toggle-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const id = btn.dataset.id;
                this.expandedId = this.expandedId === id ? null : id;
                this.renderAndReattach();
            });
        });
        document.querySelectorAll('.lop-delete-btn').forEach(btn => {
            btn.addEventListener('click', () => this.deleteProfile(btn.dataset.id));
        });
        document.querySelectorAll('.lop-field').forEach(el => {
            const commit = () => this.updateProfileField(el.dataset.id, el.dataset.field, el.type === 'checkbox' ? el.checked : el.value);
            el.addEventListener(el.tagName === 'SELECT' || el.type === 'checkbox' ? 'change' : 'change', commit);
        });
        document.querySelectorAll('.lop-rule-field, .lop-rule-operator, .lop-rule-value').forEach(el => {
            el.addEventListener('change', () => this.updateRule(el.dataset.profile, el.dataset.id));
        });
        document.querySelectorAll('.lop-delete-rule-btn').forEach(btn => {
            btn.addEventListener('click', () => this.deleteRule(btn.dataset.id));
        });
        document.querySelectorAll('.lop-add-rule-btn').forEach(btn => {
            btn.addEventListener('click', () => this.addRule(btn.dataset.profile));
        });
    }

    renderAndReattach() {
        this.render();
        this.attachListeners();
    }

    async createProfile() {
        const name = await dialogs.prompt({ title: 'New Profile', placeholder: 'e.g. Default' });
        if (!name) return;
        try {
            const response = await fetch('/api/library/organize-profiles', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name, exclude_mode: 'Do not', exclude_operator: 'Any' }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to create profile'));
            const created = JSON.parse(text);
            this.expandedId = created.id;
            await this.refreshProfiles();
        } catch (error) {
            console.error('Failed to create profile:', error);
            dialogs.toast('Failed to create profile: ' + error.message, 'error');
        }
    }

    async deleteProfile(id) {
        const p = this.findProfile(id);
        const ok = await dialogs.confirm({
            title: 'Delete Profile',
            message: `Delete "${p ? p.name : id}"? This cannot be undone.`,
            confirmLabel: 'Delete',
            danger: true,
        });
        if (!ok) return;
        try {
            const response = await fetch(`/api/library/organize-profiles/${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (!response.ok && response.status !== 204) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text, 'Failed to delete profile'));
            }
            if (this.expandedId === id) this.expandedId = null;
            await this.refreshProfiles();
        } catch (error) {
            console.error('Failed to delete profile:', error);
            dialogs.toast('Failed to delete profile: ' + error.message, 'error');
        }
    }

    async updateProfileField(id, field, value) {
        const p = this.findProfile(id);
        if (!p) return;
        p[field] = value;
        try {
            const response = await fetch(`/api/library/organize-profiles/${encodeURIComponent(id)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(p),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to save'));
            Object.assign(p, JSON.parse(text));
        } catch (error) {
            console.error('Failed to update profile:', error);
            dialogs.toast('Failed to save change: ' + error.message, 'error');
        }
        this.renderAndReattach();
    }

    async updateRule(profileId, ruleIdStr) {
        const p = this.findProfile(profileId);
        if (!p) return;
        const rule = p.exclude_rules.find(r => String(r.id) === String(ruleIdStr));
        if (!rule) return;
        const fieldSel = document.querySelector(`.lop-rule-field[data-id="${ruleIdStr}"]`);
        const opSel = document.querySelector(`.lop-rule-operator[data-id="${ruleIdStr}"]`);
        const valInput = document.querySelector(`.lop-rule-value[data-id="${ruleIdStr}"]`);
        rule.field = fieldSel.value;
        rule.operator = opSel.value;
        rule.value = valInput.value;
        try {
            const response = await fetch(`/api/library/organize-rules/${ruleIdStr}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(rule),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to save'));
            Object.assign(rule, JSON.parse(text));
        } catch (error) {
            console.error('Failed to update rule:', error);
            dialogs.toast('Failed to save change: ' + error.message, 'error');
        }
        this.renderAndReattach();
    }

    async deleteRule(idStr) {
        try {
            const response = await fetch(`/api/library/organize-rules/${idStr}`, { method: 'DELETE' });
            if (!response.ok && response.status !== 204) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text, 'Failed to delete'));
            }
            for (const p of this.profiles) {
                const idx = p.exclude_rules.findIndex(r => String(r.id) === String(idStr));
                if (idx !== -1) {
                    p.exclude_rules.splice(idx, 1);
                    break;
                }
            }
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to delete rule:', error);
            dialogs.toast('Failed to delete: ' + error.message, 'error');
        }
    }

    async addRule(profileId) {
        const container = document.querySelector(`.dmr-new-row[data-profile="${CSS.escape(profileId)}"]`);
        const field = container.querySelector('.lop-new-rule-field').value;
        const operator = container.querySelector('.lop-new-rule-operator').value;
        const value = container.querySelector('.lop-new-rule-value').value;
        try {
            const response = await fetch(`/api/library/organize-profiles/${encodeURIComponent(profileId)}/rules`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ field, operator, value }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to add'));
            const created = JSON.parse(text);
            const p = this.findProfile(profileId);
            if (p) p.exclude_rules.push(created);
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to add rule:', error);
            dialogs.toast('Failed to add: ' + error.message, 'error');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text === undefined || text === null ? '' : String(text);
        return div.innerHTML;
    }

    escapeAttr(text) {
        return this.escapeHtml(text).replace(/"/g, '&quot;');
    }
}

window.LibraryOrganizerProfilesPage = LibraryOrganizerProfilesPage;
