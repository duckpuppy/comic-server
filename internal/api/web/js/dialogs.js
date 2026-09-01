// Shared modal-based replacements for native confirm()/prompt()/alert() -
// see comic-server-orl. Native browser dialogs can't be styled, block the
// whole page, and don't match the app's theme, so every page should go
// through this instead of calling confirm()/prompt()/alert() directly.
//
// confirm/prompt/pickFolder all return Promises so a caller can `await`
// them the same way it would have used the synchronous native calls:
//   const ok = await dialogs.confirm({ message: 'Delete this?' });
// toast() is fire-and-forget (no Promise) since it never blocks for input.
//
// Only one confirm/prompt/pickFolder modal is shown at a time, matching
// how the native versions block - opening a second one while one is open
// replaces it rather than stacking.
class Dialogs {
    constructor() {
        this._toastContainer = null;
        this._onAction = null;
        this._onCancel = null;
        this._escHandler = null;
    }

    // confirm({title, message, confirmLabel, cancelLabel, danger}) -> Promise<boolean>
    confirm({ title = 'Confirm', message = '', confirmLabel = 'Confirm', cancelLabel = 'Cancel', danger = false } = {}) {
        return new Promise((resolve) => {
            this._openModal({
                title,
                bodyHTML: `<p>${this.escapeHtml(message)}</p>`,
                footerHTML: `
                    <button class="btn btn-secondary" data-action="cancel">${this.escapeHtml(cancelLabel)}</button>
                    <button class="btn ${danger ? 'btn-danger' : 'btn-primary'}" data-action="confirm">${this.escapeHtml(confirmLabel)}</button>
                `,
                onAction: () => this._settle(resolve, true),
                onCancel: () => this._settle(resolve, false),
            });
        });
    }

    // prompt({title, message, defaultValue, placeholder, confirmLabel}) -> Promise<string|null>
    // Resolves the trimmed input value, or null if cancelled or left blank.
    prompt({ title = 'Enter a value', message = '', defaultValue = '', placeholder = '', confirmLabel = 'OK', cancelLabel = 'Cancel' } = {}) {
        return new Promise((resolve) => {
            this._openModal({
                title,
                bodyHTML: `
                    ${message ? `<p>${this.escapeHtml(message)}</p>` : ''}
                    <input type="text" class="dialog-input" id="dialog-prompt-input"
                           value="${this.escapeHtml(defaultValue)}" placeholder="${this.escapeHtml(placeholder)}">
                `,
                footerHTML: `
                    <button class="btn btn-secondary" data-action="cancel">${this.escapeHtml(cancelLabel)}</button>
                    <button class="btn btn-primary" data-action="confirm">${this.escapeHtml(confirmLabel)}</button>
                `,
                onAction: () => {
                    const input = document.getElementById('dialog-prompt-input');
                    const value = input ? input.value.trim() : '';
                    this._settle(resolve, value || null);
                },
                onCancel: () => this._settle(resolve, null),
                onOpen: () => {
                    const input = document.getElementById('dialog-prompt-input');
                    if (input) {
                        input.focus();
                        input.select();
                    }
                },
            });
        });
    }

    // pickFolder({title, tree, excludeId}) -> Promise<{id, name} | null>
    // tree is the same list-tree shape listPicker.js consumes (folder
    // nodes have is_folder=true and a children array); only folders are
    // shown/selectable, plus a "(Root)" option. excludeId (and its
    // descendants) are left out so a folder can't be moved into itself.
    pickFolder({ title = 'Move to Folder', tree = [], excludeId = null } = {}) {
        return new Promise((resolve) => {
            const folders = this._collectFolders(tree, excludeId);
            const rows = [{ id: '', name: '(Root)', depth: 0 }, ...folders];
            const bodyHTML = `
                <div class="dialog-folder-tree">
                    ${rows.map(f => `
                        <div class="dialog-folder-row" data-folder-id="${this.escapeHtml(f.id)}" data-folder-name="${this.escapeHtml(f.name)}"
                             style="padding-left: ${f.depth * 1.25 + 0.75}rem">
                            <span class="dialog-folder-icon">${f.depth === 0 && f.id === '' ? '\u{1F3E0}' : '\u{1F4C1}'}</span>
                            <span class="dialog-folder-name">${this.escapeHtml(f.name)}</span>
                        </div>
                    `).join('')}
                </div>
            `;
            this._openModal({
                title,
                bodyHTML,
                footerHTML: `<button class="btn btn-secondary" data-action="cancel">Cancel</button>`,
                onCancel: () => this._settle(resolve, null),
                onOpen: () => {
                    document.querySelectorAll('.dialog-folder-row').forEach(el => {
                        el.addEventListener('click', () => {
                            this._settle(resolve, { id: el.dataset.folderId, name: el.dataset.folderName });
                        });
                    });
                },
            });
        });
    }

    // toast(message, type) - non-blocking, auto-dismissing notification.
    // type: 'success' | 'error' | 'info' (default). Replaces the ~44
    // alert() call sites that were just transient success/error feedback,
    // not something needing an OK click.
    toast(message, type = 'info', durationMs = 4000) {
        if (!this._toastContainer) {
            this._toastContainer = document.createElement('div');
            this._toastContainer.className = 'toast-container';
            document.body.appendChild(this._toastContainer);
        }
        const el = document.createElement('div');
        el.className = `toast toast-${type}`;
        el.textContent = message;
        this._toastContainer.appendChild(el);
        // Force a reflow so the entrance transition actually plays instead
        // of the element appearing already in its "visible" state.
        void el.offsetWidth;
        el.classList.add('toast-visible');
        setTimeout(() => {
            el.classList.remove('toast-visible');
            el.addEventListener('transitionend', () => el.remove(), { once: true });
        }, durationMs);
    }

    _settle(resolve, value) {
        this._closeModal();
        resolve(value);
    }

    // --- internal modal chrome, shared by confirm/prompt/pickFolder ---

    _openModal({ title, bodyHTML, footerHTML, onAction, onCancel, onOpen }) {
        this._closeModal(); // only one dialog at a time

        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        overlay.id = 'dialogs-modal';
        overlay.innerHTML = `
            <div class="modal-content dialog-content">
                <div class="modal-header">
                    <h2>${this.escapeHtml(title)}</h2>
                    <button class="modal-close" data-action="cancel">×</button>
                </div>
                <div class="modal-body">${bodyHTML}</div>
                <div class="modal-footer">${footerHTML}</div>
            </div>
        `;
        document.body.appendChild(overlay);

        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) this._triggerCancel();
        });
        overlay.querySelectorAll('[data-action="cancel"]').forEach(el => {
            el.addEventListener('click', () => this._triggerCancel());
        });
        overlay.querySelectorAll('[data-action="confirm"]').forEach(el => {
            el.addEventListener('click', () => { if (this._onAction) this._onAction(); });
        });

        this._onAction = onAction || null;
        this._onCancel = onCancel || null;

        this._escHandler = (e) => {
            if (e.key === 'Escape') { this._triggerCancel(); return; }
            // Skip when focus is already on a button - its own native
            // Enter/Space activation would otherwise double-fire this.
            if (e.key === 'Enter' && this._onAction && !e.target.closest('[data-action]')) {
                e.preventDefault();
                this._onAction();
            }
        };
        document.addEventListener('keydown', this._escHandler);

        if (onOpen) onOpen();
    }

    _triggerCancel() {
        if (this._onCancel) this._onCancel();
    }

    _closeModal() {
        const el = document.getElementById('dialogs-modal');
        if (el) el.remove();
        if (this._escHandler) {
            document.removeEventListener('keydown', this._escHandler);
            this._escHandler = null;
        }
        this._onAction = null;
        this._onCancel = null;
    }

    // Builds a flat, indented folder list from a list-tree (see
    // listPicker.js for the same tree shape), excluding excludeId and its
    // descendants so a folder can't be moved into itself.
    _collectFolders(nodes, excludeId, depth = 0) {
        const result = [];
        for (const node of nodes) {
            if (!node.is_folder || node.id === excludeId) continue;
            result.push({ id: node.id, name: node.name, depth: depth + 1 });
            result.push(...this._collectFolders(node.children || [], excludeId, depth + 1));
        }
        return result;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }
}

// Global shared instance, following the same pattern as listPicker.js.
const dialogs = new Dialogs();
