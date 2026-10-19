// Manual lists (tracker #9): ComicRack's plain "List", an ordered set of
// books picked by hand. Shared by Browse (bulk bar), the book page and the
// list page. Membership endpoints are under /api/library/lists/:id/books.
class ManualLists {
    static async call(url, options = {}) {
        const res = await fetch(url, options);
        const text = await res.text();
        if (!res.ok) throw new Error(friendlyErrorText(res, text, `HTTP ${res.status}`));
        return text ? JSON.parse(text) : {};
    }

    static post(url, body) {
        return ManualLists.call(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
    }

    // Every manual list, sorted by name; with bookId, `contains` says
    // whether that book is already in it.
    static async lists(bookId) {
        const q = bookId ? `?book_id=${encodeURIComponent(bookId)}` : '';
        const data = await ManualLists.call(`/api/library/manual-lists${q}`);
        return data.lists || [];
    }

    static create(name) {
        return ManualLists.post('/api/library/lists', { name, type: 'ComicIdListItem' });
    }

    // scope is {book_ids: [...]} or {browse: {...}}.
    static add(listId, scope, index) {
        const body = { ...scope };
        if (index !== undefined) body.index = index;
        return ManualLists.post(`/api/library/lists/${listId}/books`, body);
    }

    static remove(listId, bookIds) {
        return ManualLists.post(`/api/library/lists/${listId}/books/remove`, { book_ids: bookIds });
    }

    static move(listId, bookId, index) {
        return ManualLists.post(`/api/library/lists/${listId}/books/move`, { book_id: bookId, index });
    }

    // Opens a dialog to pick (or name a new) manual list and adds the scope
    // to it. count is how many books the scope holds, for the wording.
    // Resolves to the add result, or null when cancelled.
    static async pickAndAdd({ scope, count, bookId }) {
        let lists;
        try {
            lists = await ManualLists.lists(bookId);
        } catch (e) {
            dialogs.toast(`Could not load lists: ${e.message}`, 'error');
            return null;
        }
        const esc = t => dialogs.escapeHtml(t);
        const options = lists.map(l => `<option value="${esc(l.id)}" ${l.contains ? 'disabled' : ''}>${esc(l.name)} (${l.book_count})${l.contains ? ' - already has this book' : ''}</option>`).join('');
        const first = lists.find(l => !l.contains);
        const what = count === 1 ? '1 book' : `${count.toLocaleString()} books`;

        return new Promise(resolve => {
            let settled = false;
            const finish = v => { if (!settled) { settled = true; resolve(v); } };
            const close = dialogs.custom({
                title: 'Add to list',
                bodyHTML: `
                    <p>Add ${what} to:</p>
                    <label class="manual-list-choice">
                        <input type="radio" name="manual-list-mode" value="existing" ${first ? 'checked' : 'disabled'}>
                        An existing list
                        <select id="manual-list-select" ${first ? '' : 'disabled'} aria-label="List">${options || '<option value="">No lists yet</option>'}</select>
                    </label>
                    <label class="manual-list-choice">
                        <input type="radio" name="manual-list-mode" value="new" ${first ? '' : 'checked'}>
                        A new list named
                        <input type="text" id="manual-list-name" class="dialog-input" placeholder="e.g. Read next" aria-label="New list name">
                    </label>
                    <p class="datamanager-errors" id="manual-list-error" hidden></p>`,
                footerHTML: `
                    <button class="btn btn-secondary" data-action="cancel">Cancel</button>
                    <button class="btn btn-primary" id="manual-list-add-btn">Add</button>`,
                onClose: () => finish(null),
                onOpen: () => {
                    if (first) document.getElementById('manual-list-select').value = first.id;
                    const name = document.getElementById('manual-list-name');
                    name.addEventListener('focus', () => {
                        document.querySelector('input[name="manual-list-mode"][value="new"]').checked = true;
                    });
                    if (!first) name.focus();
                },
            });

            const btn = document.getElementById('manual-list-add-btn');
            const err = document.getElementById('manual-list-error');
            btn.addEventListener('click', async () => {
                const mode = document.querySelector('input[name="manual-list-mode"]:checked')?.value;
                const name = document.getElementById('manual-list-name').value.trim();
                let listId = document.getElementById('manual-list-select').value;
                if (mode === 'new' && !name) { err.textContent = 'Name the new list.'; err.hidden = false; return; }
                if (mode !== 'new' && !listId) { err.textContent = 'Pick a list.'; err.hidden = false; return; }
                btn.disabled = true;
                btn.textContent = 'Adding…';
                try {
                    if (mode === 'new') listId = (await ManualLists.create(name)).id;
                    const res = await ManualLists.add(listId, scope);
                    ManualLists.toastResult(res);
                    finish(res);
                    close();
                } catch (e) {
                    err.textContent = e.message;
                    err.hidden = false;
                    btn.disabled = false;
                    btn.textContent = 'Add';
                }
            });
        });
    }

    static toastResult(res) {
        const parts = [`Added ${res.added || 0} to "${res.name}"`];
        if (res.already) parts.push(`${res.already} already there`);
        if (res.unknown) parts.push(`${res.unknown} not found`);
        dialogs.toast(parts.join(', ') + '.', 'success');
    }
}
