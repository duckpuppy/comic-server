// Library Organizer page (comic-server-3bz.6) - the UI for
// comic-server-3bz.5's preview/apply engine: comic-server's first feature
// that moves or renames the user's own existing comic files. Reached from
// the Workflow dashboard's "To Move" stage card (comic-server-1iv.3),
// mirroring the Data Manager whole-library page's pattern
// (dataManagerPage.js) - Preview first, then an explicit Apply with a
// danger-styled confirm, never a silent write.
//
// Selection is per BOOK, not per field like Data Manager - a Library
// Organizer move is a single all-or-nothing file operation, there's no
// finer-grained "keep this part, skip that part" within one book's move.
// Skipped/Failed/Collision rows (computed by the server's own Plan, from
// the profile's exclude rules or a path conflict) are shown for
// visibility but never selectable - nothing the user can do from this
// page makes a bad plan row safe to apply.
class OrganizePage {
    constructor() {
        this.profiles = [];
        this.profileId = '';
        this.moves = [];      // last preview's LOPlannedMove list
        this.selected = new Set(); // book_id set, only for eligible rows
        this.previewing = false;
        this.applying = false;
        this.lastResult = null;
        this.error = null;
    }

    async init(ctx) {
        await this.loadProfiles();
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async loadProfiles() {
        try {
            const response = await fetch('/api/library/organize-profiles');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            this.profiles = data.profiles || [];
            if (this.profiles.length > 0 && !this.profileId) {
                this.profileId = this.profiles[0].id;
            }
        } catch (error) {
            console.error('Failed to load Library Organizer profiles:', error);
            this.error = 'Failed to load profiles. Please try again.';
        }
    }

    eligible(m) {
        return !m.skipped && !m.failed && !m.collision;
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="datamanager-page">
                <div class="datamanager-page-header">
                    <h1>Library Organizer</h1>
                    <p class="empty-message">Moves or copies every book currently in the "To Move" workflow stage to its planned location under the chosen profile. Preview first - nothing is written until you apply.</p>
                </div>
                <div class="panel">
                    ${this.renderControls()}
                    ${this.renderBody()}
                </div>
            </div>
        `;
    }

    renderControls() {
        if (this.profiles.length === 0) {
            return `<p class="empty-message">No Library Organizer profiles configured yet. Import one via <code>comic-server library-organizer import --dat &lt;path&gt;</code>.</p>`;
        }
        const options = this.profiles.map(p =>
            `<option value="${this.escapeAttr(p.id)}" ${p.id === this.profileId ? 'selected' : ''}>${this.escapeHtml(p.name)}${p.copy_mode ? ' (Copy)' : ' (Move)'}</option>`
        ).join('');
        return `
            <div class="datamanager-actions">
                <select id="organize-profile-select" class="btn btn-secondary">${options}</select>
                <button class="btn btn-primary" id="organize-preview-btn" ${this.previewing ? 'disabled' : ''}>
                    ${this.previewing ? 'Previewing…' : 'Preview'}
                </button>
                <button class="btn btn-primary" id="organize-apply-btn" ${this.applyDisabled() ? 'disabled' : ''}>
                    Apply Selected (${this.selected.size})
                </button>
                <button class="btn btn-secondary" id="organize-select-all-btn" ${!this.moves.length ? 'disabled' : ''}>Select All Eligible</button>
                <button class="btn btn-secondary" id="organize-select-none-btn" ${!this.moves.length ? 'disabled' : ''}>Select None</button>
            </div>
        `;
    }

    applyDisabled() {
        return this.applying || this.selected.size === 0;
    }

    renderBody() {
        if (this.error) {
            return `<p class="datamanager-errors">${this.escapeHtml(this.error)}</p>`;
        }
        if (this.lastResult) {
            var html = `<p>Applied ${this.lastResult.applied} book(s), ${this.lastResult.no_op} already in place, ${this.lastResult.skipped} skipped, ${this.lastResult.failed} failed.</p>`;
            if (this.lastResult.errors && this.lastResult.errors.length > 0) {
                html += `<p class="datamanager-errors">Errors: ${this.escapeHtml(this.lastResult.errors.join('; '))}</p>`;
            }
        } else {
            var html = '';
        }

        if (this.moves.length === 0) {
            return html + (this.previewing ? '' : '<p class="empty-message">No preview yet - pick a profile and click Preview.</p>');
        }

        html += '<table class="datamanager-diff-table"><thead><tr><th></th><th>Book</th><th>Old Path</th><th>New Path</th><th>Status</th></tr></thead><tbody>';
        for (const m of this.moves) {
            const label = `${m.series}${m.number ? ' #' + m.number : ''}${m.title ? ' - ' + m.title : ''}`;
            const eligible = this.eligible(m);
            const checked = this.selected.has(m.book_id);
            let status = 'OK';
            if (m.skipped) status = 'Excluded by profile rules';
            else if (m.failed) status = `Failed: ${m.fail_reason}`;
            else if (m.collision) status = `Collision: ${m.collision_reason}`;
            html += '<tr>';
            html += `<td>${eligible ? `<input type="checkbox" class="organize-check" data-book-id="${this.escapeAttr(m.book_id)}" ${checked ? 'checked' : ''}>` : ''}</td>`;
            html += `<td>${this.escapeHtml(label)}</td>`;
            html += `<td>${this.escapeHtml(m.old_path)}</td>`;
            html += `<td>${this.escapeHtml(m.new_path)}</td>`;
            html += `<td>${this.escapeHtml(status)}</td>`;
            html += '</tr>';
        }
        html += '</tbody></table>';

        return html;
    }

    attachListeners() {
        const select = document.getElementById('organize-profile-select');
        if (select) select.addEventListener('change', () => { this.profileId = select.value; });

        const previewBtn = document.getElementById('organize-preview-btn');
        if (previewBtn) previewBtn.addEventListener('click', () => this.preview());

        const applyBtn = document.getElementById('organize-apply-btn');
        if (applyBtn) applyBtn.addEventListener('click', () => this.apply());

        const selectAllBtn = document.getElementById('organize-select-all-btn');
        if (selectAllBtn) {
            selectAllBtn.addEventListener('click', () => {
                for (const m of this.moves) {
                    if (this.eligible(m)) this.selected.add(m.book_id);
                }
                this.render();
                this.attachListeners();
            });
        }
        const selectNoneBtn = document.getElementById('organize-select-none-btn');
        if (selectNoneBtn) {
            selectNoneBtn.addEventListener('click', () => {
                this.selected.clear();
                this.render();
                this.attachListeners();
            });
        }

        document.querySelectorAll('.organize-check').forEach(el => {
            el.addEventListener('change', () => {
                if (el.checked) this.selected.add(el.dataset.bookId);
                else this.selected.delete(el.dataset.bookId);
                const applyBtn = document.getElementById('organize-apply-btn');
                if (applyBtn) {
                    applyBtn.textContent = `Apply Selected (${this.selected.size})`;
                    applyBtn.disabled = this.applyDisabled();
                }
            });
        });
    }

    async preview() {
        if (!this.profileId) return;
        this.previewing = true;
        this.error = null;
        this.lastResult = null;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch(`/api/library/workflow/organize-preview?profile=${encodeURIComponent(this.profileId)}`);
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to preview Library Organizer plan'));
            }
            const result = JSON.parse(text);
            this.moves = result.moves || [];
            this.selected = new Set();
            for (const m of this.moves) {
                if (this.eligible(m)) this.selected.add(m.book_id);
            }
        } catch (error) {
            console.error('Failed to preview Library Organizer plan:', error);
            this.error = `Failed: ${error.message}`;
        } finally {
            this.previewing = false;
            this.render();
            this.attachListeners();
        }
    }

    async apply() {
        const ok = await dialogs.confirm({
            title: 'Apply Library Organizer',
            message: `Move or copy the selected ${this.selected.size} book(s) now? This writes to your comic files on disk. Replaced/relocated originals go to the trash quarantine, not permanent deletion, but this cannot be undone from this page.`,
            confirmLabel: 'Apply',
            danger: true,
        });
        if (!ok) return;

        this.applying = true;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch(`/api/library/workflow/organize-apply?profile=${encodeURIComponent(this.profileId)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ book_ids: Array.from(this.selected) }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to apply Library Organizer plan'));
            }
            this.lastResult = JSON.parse(text);
            await this.preview();
            return;
        } catch (error) {
            console.error('Failed to apply Library Organizer plan:', error);
            this.error = `Failed: ${error.message}`;
        } finally {
            this.applying = false;
            this.render();
            this.attachListeners();
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

window.OrganizePage = OrganizePage;
