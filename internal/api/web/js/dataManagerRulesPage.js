// Data Manager Rule Editor (comic-server-tj6o) - first slice: create,
// edit, and delete rulesets (each a Field/Modifier/Value condition list
// plus a Field/Modifier/Value action list) entirely through the web UI,
// with no dataman.dat import required. Nested group folders and drag
// reordering are deliberately NOT part of this slice - every ruleset
// created here is top-level (see comic-server-vkpq for that follow-up).
//
// Rulesets created/edited here are read by the exact same
// datamanager.LoadRulesets path the whole-library Data Manager page's
// Preview/Apply already use, so a rule added here takes effect on the
// very next preview/apply with no separate "publish" step.
class DataManagerRulesPage {
    constructor() {
        this.schema = null;
        this.rulesets = [];
        this.expandedId = null; // which ruleset's rule/action lists are shown
        this.loading = true;
        this.error = null;
    }

    async init(ctx) {
        this.render();
        await Promise.all([this.loadSchema(), this.loadRulesets()]);
        if (ctx && ctx.aborted) return;
        this.loading = false;
        this.render();
        this.attachListeners();
    }

    async loadSchema() {
        try {
            const response = await fetch('/api/datamanager/schema');
            this.schema = await response.json();
        } catch (error) {
            console.error('Failed to load Data Manager schema:', error);
            this.schema = { fields: [], ruleModifiers: {}, actionModifiers: [], customFieldRuleModifiers: [] };
        }
    }

    async loadRulesets() {
        try {
            const response = await fetch('/api/datamanager/rulesets');
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to load rulesets'));
            const data = JSON.parse(text);
            this.rulesets = data.rulesets || [];
        } catch (error) {
            console.error('Failed to load rulesets:', error);
            this.error = `Failed: ${error.message}`;
        }
    }

    fieldInfo(name) {
        return (this.schema.fields || []).find(f => f.name === name);
    }

    fieldKind(name) {
        const f = this.fieldInfo(name);
        return f ? f.kind : null;
    }

    ruleModifiersFor(field) {
        const kind = this.fieldKind(field);
        if (kind) return this.schema.ruleModifiers[kind] || [];
        return this.schema.customFieldRuleModifiers || [];
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Data Manager Rules</h1>
                    <p class="empty-message">Create and edit the rulesets Data Manager's Preview/Apply run against the whole library. Each ruleset is a list of conditions (AND/OR) plus a list of actions applied when they match.</p>
                </div>
                <div class="panel">
                    <div class="datamanager-actions">
                        <button class="btn btn-primary" id="dmr-new-ruleset-btn">+ New Ruleset</button>
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
        if (this.rulesets.length === 0) {
            return '<p class="empty-message">No rulesets yet. Click "New Ruleset" to create one, or import from a ComicRack dataman.dat file via the CLI.</p>';
        }
        return `<ul class="dmr-ruleset-list">${this.rulesets.map(rs => this.renderRuleset(rs)).join('')}</ul>`;
    }

    renderRuleset(rs) {
        const expanded = this.expandedId === rs.id;
        return `
            <li class="dmr-ruleset-row${rs.disabled ? ' dmr-disabled' : ''}" data-id="${this.escapeAttr(rs.id)}">
                <div class="dmr-ruleset-header">
                    <button class="btn btn-small dmr-toggle-btn" data-id="${this.escapeAttr(rs.id)}">${expanded ? '▾' : '▸'}</button>
                    <span class="dmr-ruleset-name">${this.escapeHtml(rs.name)}</span>
                    <span class="dmr-ruleset-summary">${rs.rules.length} condition${rs.rules.length === 1 ? '' : 's'} (${rs.mode}), ${rs.actions.length} action${rs.actions.length === 1 ? '' : 's'}</span>
                    <label class="dmr-disabled-toggle">
                        <input type="checkbox" class="dmr-ruleset-disabled-check" data-id="${this.escapeAttr(rs.id)}" ${rs.disabled ? 'checked' : ''}> Disabled
                    </label>
                    <button class="btn btn-small dmr-rename-btn" data-id="${this.escapeAttr(rs.id)}">Rename</button>
                    <select class="dmr-mode-select" data-id="${this.escapeAttr(rs.id)}">
                        <option value="And" ${rs.mode === 'And' ? 'selected' : ''}>Match ALL (AND)</option>
                        <option value="Or" ${rs.mode === 'Or' ? 'selected' : ''}>Match ANY (OR)</option>
                    </select>
                    <button class="btn btn-small btn-danger dmr-delete-ruleset-btn" data-id="${this.escapeAttr(rs.id)}">Delete</button>
                </div>
                ${expanded ? this.renderRulesetDetail(rs) : ''}
            </li>
        `;
    }

    renderRulesetDetail(rs) {
        return `
            <div class="dmr-ruleset-detail">
                <div class="dmr-column">
                    <h3>Conditions</h3>
                    <ul class="dmr-row-list">
                        ${rs.rules.map(r => this.renderConditionRow(rs.id, r)).join('')}
                    </ul>
                    ${this.renderNewRuleForm(rs.id)}
                </div>
                <div class="dmr-column">
                    <h3>Actions</h3>
                    <ul class="dmr-row-list">
                        ${rs.actions.map(a => this.renderActionRow(rs.id, a)).join('')}
                    </ul>
                    ${this.renderNewActionForm(rs.id)}
                </div>
            </div>
        `;
    }

    fieldOptions(selected) {
        const fields = (this.schema.fields || []).slice().sort((a, b) => a.name.localeCompare(b.name));
        return fields.map(f =>
            `<option value="${this.escapeAttr(f.name)}" ${f.name === selected ? 'selected' : ''}>${this.escapeHtml(f.name)}</option>`
        ).join('');
    }

    modifierOptions(modifiers, selected) {
        return modifiers.map(m =>
            `<option value="${this.escapeAttr(m.value)}" ${m.value === selected ? 'selected' : ''}>${this.escapeHtml(m.label)}</option>`
        ).join('');
    }

    renderConditionRow(rulesetId, rule) {
        const modifiers = this.ruleModifiersFor(rule.field);
        const modInfo = modifiers.find(m => m.value === rule.modifier);
        const params = modInfo ? modInfo.params : 1;
        const parts = params === 2 ? rule.value.split('||') : [rule.value];
        return `
            <li class="dmr-editor-row" data-ruleset="${this.escapeAttr(rulesetId)}" data-rule-id="${rule.id}">
                <select class="dmr-field-select" data-kind="rule" data-id="${rule.id}" data-ruleset="${this.escapeAttr(rulesetId)}">
                    <optgroup label="Fields">${this.fieldOptions(rule.field)}</optgroup>
                </select>
                <select class="dmr-modifier-select" data-kind="rule" data-id="${rule.id}" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${this.modifierOptions(modifiers, rule.modifier)}
                </select>
                <input type="text" class="dmr-value-input" data-kind="rule" data-part="0" data-id="${rule.id}" data-ruleset="${this.escapeAttr(rulesetId)}"
                       value="${this.escapeAttr(parts[0] || '')}" placeholder="${modInfo && modInfo.valueHint ? this.escapeAttr(modInfo.valueHint) : 'value'}">
                ${params === 2 ? `<span class="dmr-and">and</span><input type="text" class="dmr-value-input" data-kind="rule" data-part="1" data-id="${rule.id}" data-ruleset="${this.escapeAttr(rulesetId)}" value="${this.escapeAttr(parts[1] || '')}" placeholder="value 2">` : ''}
                <button class="btn btn-small btn-danger dmr-delete-row-btn" data-kind="rule" data-id="${rule.id}">✕</button>
            </li>
        `;
    }

    renderActionRow(rulesetId, action) {
        const modifiers = this.schema.actionModifiers || [];
        const modInfo = modifiers.find(m => m.value === action.modifier);
        const params = modInfo ? modInfo.params : 1;
        const parts = params === 2 ? action.value.split('||') : [action.value];
        return `
            <li class="dmr-editor-row" data-ruleset="${this.escapeAttr(rulesetId)}" data-action-id="${action.id}">
                <select class="dmr-field-select" data-kind="action" data-id="${action.id}" data-ruleset="${this.escapeAttr(rulesetId)}">
                    <optgroup label="Fields">${this.fieldOptions(action.field)}</optgroup>
                </select>
                <select class="dmr-modifier-select" data-kind="action" data-id="${action.id}" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${this.modifierOptions(modifiers, action.modifier)}
                </select>
                <input type="text" class="dmr-value-input" data-kind="action" data-part="0" data-id="${action.id}" data-ruleset="${this.escapeAttr(rulesetId)}"
                       value="${this.escapeAttr(parts[0] || '')}" placeholder="value">
                ${params === 2 ? `<span class="dmr-and">and</span><input type="text" class="dmr-value-input" data-kind="action" data-part="1" data-id="${action.id}" data-ruleset="${this.escapeAttr(rulesetId)}" value="${this.escapeAttr(parts[1] || '')}" placeholder="value 2">` : ''}
                <button class="btn btn-small btn-danger dmr-delete-row-btn" data-kind="action" data-id="${action.id}">✕</button>
            </li>
        `;
    }

    renderNewRuleForm(rulesetId) {
        const fields = this.schema.fields || [];
        const firstField = fields[0] ? fields[0].name : '';
        const modifiers = this.ruleModifiersFor(firstField);
        return `
            <div class="dmr-new-row" data-ruleset="${this.escapeAttr(rulesetId)}">
                <select class="dmr-new-field-select" data-kind="rule" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${this.fieldOptions(firstField)}
                </select>
                <select class="dmr-new-modifier-select" data-kind="rule" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${this.modifierOptions(modifiers, modifiers[0] && modifiers[0].value)}
                </select>
                <input type="text" class="dmr-new-value-input" data-kind="rule" data-ruleset="${this.escapeAttr(rulesetId)}" placeholder="value">
                <button class="btn btn-small dmr-add-row-btn" data-kind="rule" data-ruleset="${this.escapeAttr(rulesetId)}">+ Add Condition</button>
            </div>
        `;
    }

    renderNewActionForm(rulesetId) {
        const fields = (this.schema.fields || []).filter(f => f.writable);
        const firstField = fields[0] ? fields[0].name : '';
        const modifiers = this.schema.actionModifiers || [];
        return `
            <div class="dmr-new-row" data-ruleset="${this.escapeAttr(rulesetId)}">
                <select class="dmr-new-field-select" data-kind="action" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${fields.map(f => `<option value="${this.escapeAttr(f.name)}" ${f.name === firstField ? 'selected' : ''}>${this.escapeHtml(f.name)}</option>`).join('')}
                </select>
                <select class="dmr-new-modifier-select" data-kind="action" data-ruleset="${this.escapeAttr(rulesetId)}">
                    ${this.modifierOptions(modifiers, modifiers[0] && modifiers[0].value)}
                </select>
                <input type="text" class="dmr-new-value-input" data-kind="action" data-ruleset="${this.escapeAttr(rulesetId)}" placeholder="value">
                <button class="btn btn-small dmr-add-row-btn" data-kind="action" data-ruleset="${this.escapeAttr(rulesetId)}">+ Add Action</button>
            </div>
        `;
    }

    attachListeners() {
        const newBtn = document.getElementById('dmr-new-ruleset-btn');
        if (newBtn) newBtn.addEventListener('click', () => this.createRuleset());

        document.querySelectorAll('.dmr-toggle-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const id = btn.dataset.id;
                this.expandedId = this.expandedId === id ? null : id;
                this.renderAndReattach();
            });
        });

        document.querySelectorAll('.dmr-rename-btn').forEach(btn => {
            btn.addEventListener('click', () => this.renameRuleset(btn.dataset.id));
        });

        document.querySelectorAll('.dmr-mode-select').forEach(el => {
            el.addEventListener('change', () => this.updateRulesetMeta(el.dataset.id, { mode: el.value }));
        });

        document.querySelectorAll('.dmr-ruleset-disabled-check').forEach(el => {
            el.addEventListener('change', () => this.updateRulesetMeta(el.dataset.id, { disabled: el.checked }));
        });

        document.querySelectorAll('.dmr-delete-ruleset-btn').forEach(btn => {
            btn.addEventListener('click', () => this.deleteRuleset(btn.dataset.id));
        });

        document.querySelectorAll('.dmr-field-select').forEach(el => {
            el.addEventListener('change', () => this.updateRow(el.dataset.kind, el.dataset.ruleset, el.dataset.id, { field: el.value, resetModifier: true }));
        });
        document.querySelectorAll('.dmr-modifier-select').forEach(el => {
            el.addEventListener('change', () => this.updateRow(el.dataset.kind, el.dataset.ruleset, el.dataset.id, { modifier: el.value }));
        });
        document.querySelectorAll('.dmr-value-input').forEach(el => {
            el.addEventListener('change', () => this.updateRow(el.dataset.kind, el.dataset.ruleset, el.dataset.id, { part: parseInt(el.dataset.part, 10), value: el.value }));
        });
        document.querySelectorAll('.dmr-delete-row-btn').forEach(btn => {
            btn.addEventListener('click', () => this.deleteRow(btn.dataset.kind, btn.dataset.id));
        });

        document.querySelectorAll('.dmr-new-field-select').forEach(el => {
            el.addEventListener('change', () => this.refreshNewRowModifiers(el));
        });
        document.querySelectorAll('.dmr-add-row-btn').forEach(btn => {
            btn.addEventListener('click', () => this.addRow(btn.dataset.kind, btn.dataset.ruleset));
        });
    }

    renderAndReattach() {
        this.render();
        this.attachListeners();
    }

    refreshNewRowModifiers(fieldSelect) {
        const container = fieldSelect.closest('.dmr-new-row');
        const modSelect = container.querySelector('.dmr-new-modifier-select');
        const kind = fieldSelect.dataset.kind;
        const modifiers = kind === 'action' ? (this.schema.actionModifiers || []) : this.ruleModifiersFor(fieldSelect.value);
        modSelect.innerHTML = this.modifierOptions(modifiers, modifiers[0] && modifiers[0].value);
    }

    findRuleset(id) {
        return this.rulesets.find(rs => rs.id === id);
    }

    async createRuleset() {
        const name = await dialogs.prompt({ title: 'New Ruleset', placeholder: 'e.g. Batman Family' });
        if (!name) return;
        try {
            const response = await fetch('/api/datamanager/rulesets', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name, mode: 'And' }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to create ruleset'));
            const created = JSON.parse(text);
            this.rulesets.push(created);
            this.expandedId = created.id;
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to create ruleset:', error);
            dialogs.toast('Failed to create ruleset: ' + error.message, 'error');
        }
    }

    async renameRuleset(id) {
        const rs = this.findRuleset(id);
        if (!rs) return;
        const name = await dialogs.prompt({ title: 'Rename Ruleset', defaultValue: rs.name });
        if (!name) return;
        await this.updateRulesetMeta(id, { name });
    }

    async updateRulesetMeta(id, patch) {
        const rs = this.findRuleset(id);
        if (!rs) return;
        const body = { name: rs.name, mode: rs.mode, disabled: rs.disabled, ...patch };
        try {
            const response = await fetch(`/api/datamanager/rulesets/${encodeURIComponent(id)}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to update ruleset'));
            Object.assign(rs, JSON.parse(text));
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to update ruleset:', error);
            dialogs.toast('Failed to update ruleset: ' + error.message, 'error');
            await this.loadRulesets();
            this.renderAndReattach();
        }
    }

    async deleteRuleset(id) {
        const rs = this.findRuleset(id);
        const ok = await dialogs.confirm({
            title: 'Delete Ruleset',
            message: `Delete "${rs ? rs.name : id}" and all its conditions/actions? This cannot be undone.`,
            confirmLabel: 'Delete',
            danger: true,
        });
        if (!ok) return;
        try {
            const response = await fetch(`/api/datamanager/rulesets/${encodeURIComponent(id)}`, { method: 'DELETE' });
            if (!response.ok && response.status !== 204) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text, 'Failed to delete ruleset'));
            }
            this.rulesets = this.rulesets.filter(r => r.id !== id);
            if (this.expandedId === id) this.expandedId = null;
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to delete ruleset:', error);
            dialogs.toast('Failed to delete ruleset: ' + error.message, 'error');
        }
    }

    // updateRow patches one condition/action row in place, then PUTs the
    // whole row (field+modifier+value) - the server stores rule and
    // action rows identically (field/modifier/value/sort_order), so one
    // method serves both kinds.
    async updateRow(kind, rulesetId, idStr, patch) {
        const rs = this.findRuleset(rulesetId);
        if (!rs) return;
        const list = kind === 'action' ? rs.actions : rs.rules;
        const row = list.find(r => String(r.id) === String(idStr));
        if (!row) return;

        if (patch.field !== undefined) {
            row.field = patch.field;
            if (patch.resetModifier) {
                const modifiers = kind === 'action' ? (this.schema.actionModifiers || []) : this.ruleModifiersFor(row.field);
                row.modifier = modifiers[0] ? modifiers[0].value : '';
                row.value = '';
            }
        }
        if (patch.modifier !== undefined) {
            row.modifier = patch.modifier;
        }
        if (patch.part !== undefined) {
            const parts = row.value.split('||');
            parts[patch.part] = patch.value;
            row.value = parts.join('||');
        }

        const url = kind === 'action' ? `/api/datamanager/actions/${row.id}` : `/api/datamanager/rules/${row.id}`;
        try {
            const response = await fetch(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ field: row.field, modifier: row.modifier, value: row.value }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to save'));
            Object.assign(row, JSON.parse(text));
        } catch (error) {
            console.error('Failed to update row:', error);
            dialogs.toast('Failed to save change: ' + error.message, 'error');
        }
        this.renderAndReattach();
    }

    async deleteRow(kind, idStr) {
        const url = kind === 'action' ? `/api/datamanager/actions/${idStr}` : `/api/datamanager/rules/${idStr}`;
        try {
            const response = await fetch(url, { method: 'DELETE' });
            if (!response.ok && response.status !== 204) {
                const text = await response.text();
                throw new Error(friendlyErrorText(response, text, 'Failed to delete'));
            }
            for (const rs of this.rulesets) {
                const list = kind === 'action' ? rs.actions : rs.rules;
                const idx = list.findIndex(r => String(r.id) === String(idStr));
                if (idx !== -1) {
                    list.splice(idx, 1);
                    break;
                }
            }
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to delete row:', error);
            dialogs.toast('Failed to delete: ' + error.message, 'error');
        }
    }

    async addRow(kind, rulesetId) {
        const container = document.querySelector(`.dmr-new-row[data-ruleset="${CSS.escape(rulesetId)}"] .dmr-new-field-select[data-kind="${kind}"]`).closest('.dmr-new-row');
        const field = container.querySelector('.dmr-new-field-select').value;
        const modifier = container.querySelector('.dmr-new-modifier-select').value;
        const value = container.querySelector('.dmr-new-value-input').value;

        const url = kind === 'action' ? `/api/datamanager/rulesets/${encodeURIComponent(rulesetId)}/actions` : `/api/datamanager/rulesets/${encodeURIComponent(rulesetId)}/rules`;
        try {
            const response = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ field, modifier, value }),
            });
            const text = await response.text();
            if (!response.ok) throw new Error(friendlyErrorText(response, text, 'Failed to add'));
            const created = JSON.parse(text);
            const rs = this.findRuleset(rulesetId);
            if (rs) {
                if (kind === 'action') rs.actions.push(created);
                else rs.rules.push(created);
            }
            this.renderAndReattach();
        } catch (error) {
            console.error('Failed to add row:', error);
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

window.DataManagerRulesPage = DataManagerRulesPage;
