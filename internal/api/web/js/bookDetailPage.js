// Book detail page (comic-server-br14): one book's record, file, workflow
// stage, attention flags, ComicVine links, history and the file's own
// ComicInfo.xml compared with the library. Actions (comic-server-2s4f) and
// an edit form for the book's metadata (POST .../edit).
class BookDetailPage {
    constructor() {
        this.bookId = null;
        this.detail = null;
        this.history = null;
        this.comicInfo = null; // null = not loaded, 'loading', or the response
        this.error = null;
        this.loading = true;
        this.openSections = {};
        this.completeness = null; // series-completeness of the book's CV volume
        this.lists = null; // manual lists, each with `contains` for this book
    }

    async init(ctx, bookId) {
        this.bookId = bookId;
        this.detail = null;
        this.history = null;
        this.comicInfo = null;
        this.error = null;
        this.loading = true;
        this.openSections = {};
        this.completeness = null;
        this.lists = null;
        this.busy = false;
        this.editing = false;
        this.render();
        await Promise.all([this.loadDetail(), this.loadHistory(), this.loadLists()]);
        if (ctx && ctx.aborted) return;
        this.loading = false;
        this.render();
        this.loadCompleteness(ctx); // slower and optional: fills in when it arrives
    }

    // How much of the book's ComicVine volume the library owns
    // (comic-server-n2yb). Quiet when ComicVine is off, the volume is not
    // synced yet, or the book is untagged: the bar just does not appear.
    async loadCompleteness(ctx) {
        const volumeId = this.detail && this.detail.comicvine && this.detail.comicvine.volume_id;
        if (!volumeId) return;
        const bookId = this.bookId;
        try {
            const res = await fetch(`/api/library/series-completeness/${volumeId}`);
            if (!res.ok) return;
            const c = await res.json();
            if ((ctx && ctx.aborted) || this.bookId !== bookId) return;
            this.completeness = c;
            this.render();
        } catch (e) {
            console.error('Failed to load series completeness:', e);
        }
    }

    // A Browse link that lists books with this character/team (list
    // membership, exact name) or this main character.
    browseLink(type, operator, value) {
        const m = [{ Type: type, Not: false, MatchOperator: String(operator), MatchValue: value, MatchValue2: '' }];
        return `/browse?m=${encodeURIComponent(JSON.stringify(m))}`;
    }

    chips(value, type, operator) {
        const names = String(value || '').split(/[,;]/).map(v => v.trim()).filter(Boolean);
        return names.map(n =>
            `<a class="bd-chip" href="${this.escapeHtml(this.browseLink(type, operator, n))}" data-route title="Books with ${this.escapeHtml(n)}">${this.escapeHtml(n)}</a>`
        ).join(' ');
    }

    renderCompleteness() {
        const c = this.completeness;
        if (!c || !c.total) return '';
        const pct = Math.max(0, Math.min(100, c.percent || 0));
        const more = c.missing > 0
            ? ` &middot; <a href="/missing-issues?volume=${c.volume_id}" data-route>${c.missing} missing</a>` : ' &middot; complete';
        return `
            <div class="bd-completeness">
                <div class="bd-completeness-bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${pct}"><span style="width:${pct}%"></span></div>
                <p class="empty-message">Own ${c.owned} of ${c.total} issues of ${this.escapeHtml(c.name)}${c.start_year ? ` (${this.escapeHtml(c.start_year)})` : ''}${more}</p>
            </div>`;
    }

    async loadDetail() {
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}`);
            if (!res.ok) throw new Error(res.status === 404 ? 'Book not found.' : await res.text());
            this.detail = await res.json();
        } catch (e) {
            console.error('Failed to load book:', e);
            this.error = String(e.message || e);
        }
    }

    // Manual lists (tracker #9): which ones hold this book.
    async loadLists() {
        try {
            this.lists = await ManualLists.lists(this.bookId);
        } catch (e) {
            console.error('Failed to load lists:', e);
            this.lists = null;
        }
    }

    // Issues this collected edition contains (tracker #38). Read-only here;
    // the linking screen comes with tracker #39.
    renderCollects(entries) {
        const rows = entries.map(e => {
            const label = e.series || `ComicVine volume ${e.volume}`;
            const name = e.volume
                ? `<a href="https://comicvine.gamespot.com/volume/4050-${e.volume}/" target="_blank" rel="noopener noreferrer">${this.escapeHtml(label)}</a>`
                : this.escapeHtml(label);
            const range = e.to ? `#${this.escapeHtml(e.from)}\u2013#${this.escapeHtml(e.to)}` : `#${this.escapeHtml(e.from)}`;
            const notes = [e.kind === 'backup' ? 'backup story' : '', e.partial ? 'partial' : '', e.source === 'suggested' ? 'suggested' : '']
                .filter(Boolean).map(n => `<span class="bd-collects-note">${n}</span>`).join(' ');
            return `<li class="bd-collects-row">${name} ${range} ${notes}</li>`;
        }).join('');
        return `<ul class="bd-collects">${rows}</ul>`;
    }

    renderLists() {
        if (!this.lists) return '';
        const mine = this.lists.filter(l => l.contains);
        const rows = mine.map(l => `<li class="bd-list-row">
                <a href="/lists/${encodeURIComponent(l.id)}" data-route>${this.escapeHtml(l.name)}</a>
                <button class="btn btn-small btn-secondary bd-list-remove" data-list-id="${this.escapeHtml(l.id)}" data-list-name="${this.escapeHtml(l.name)}" ${this.busy ? 'disabled' : ''}>Remove</button>
            </li>`).join('');
        return `${mine.length ? `<ul class="bd-lists">${rows}</ul>` : '<p class="empty-message">This book is not in any list.</p>'}
            <button class="btn btn-secondary" id="bd-add-list-btn" ${this.busy ? 'disabled' : ''}>Add to list…</button>`;
    }

    async addToList() {
        const res = await ManualLists.pickAndAdd({ scope: { book_ids: [this.bookId] }, count: 1, bookId: this.bookId });
        if (res) await this.runAction(async () => {});
    }

    async removeFromList(listId, name) {
        await this.runAction(async () => {
            try {
                await ManualLists.remove(listId, [this.bookId]);
                dialogs.toast(`Removed from "${name}".`, 'success');
            } catch (e) {
                dialogs.toast(`Could not remove from list: ${e.message}`, 'error');
            }
        });
    }

    async loadHistory() {
        try {
            const res = await fetch(`/api/library/audit?book_id=${encodeURIComponent(this.bookId)}&limit=20`);
            if (!res.ok) return; // 503 without config.db: just no history section
            const data = await res.json();
            this.history = { entries: data.entries || [], total: data.total || 0 };
        } catch (e) {
            console.error('Failed to load book history:', e);
        }
    }

    async loadComicInfo() {
        this.comicInfo = 'loading';
        this.render();
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/comicinfo`);
            if (!res.ok) throw new Error(await res.text());
            this.comicInfo = await res.json();
        } catch (e) {
            this.comicInfo = { error: String(e.message || e), diff: [] };
        }
        this.render();
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    formatSize(bytes) {
        if (!bytes) return '';
        const units = ['B', 'KB', 'MB', 'GB'];
        let v = bytes, u = 0;
        while (v >= 1024 && u < units.length - 1) { v /= 1024; u++; }
        return `${u === 0 ? v : v.toFixed(1)} ${units[u]}`;
    }

    // ComicTime marshals as an RFC3339 string; year 1 means "never set".
    formatTime(t) {
        if (!t || t.startsWith('0001-')) return '';
        const d = new Date(t);
        return isNaN(d) ? '' : d.toLocaleString();
    }

    // Rows with an empty value are left out, so short records stay short.
    renderGrid(rows) {
        const shown = rows.filter(([, v]) => v !== '' && v != null && v !== 0);
        if (!shown.length) return '';
        return `<dl class="bd-grid">${shown.map(([k, v, raw]) =>
            `<dt>${this.escapeHtml(k)}</dt><dd>${raw ? v : this.escapeHtml(v)}</dd>`).join('')}</dl>`;
    }

    // Open/closed state survives re-renders (e.g. after Compare with file).
    section(title, body, defaultOpen = true) {
        if (!body) return '';
        const open = this.openSections[title] ?? defaultOpen;
        return `<details class="panel bd-section" data-title="${this.escapeHtml(title)}" ${open ? 'open' : ''}><summary><h2>${this.escapeHtml(title)}</h2></summary>${body}</details>`;
    }

    render() {
        const app = document.getElementById('app');
        if (this.loading) {
            app.innerHTML = '<div class="book-detail-page"><p class="empty-message">Loading…</p></div>';
            return;
        }
        if (this.error || !this.detail) {
            app.innerHTML = `<div class="book-detail-page">
                <button class="btn btn-secondary" id="bd-back">&larr; Back</button>
                <p class="empty-message">${this.escapeHtml(this.error || 'Book not found.')}</p></div>`;
            this.attachListeners();
            return;
        }
        const d = this.detail;
        const b = d.book;
        const heading = `${b.Series || '(no series)'}${b.Number ? ' #' + b.Number : ''}${b.Volume ? ` (${b.Volume})` : ''}`;
        if (this.editing) {
            app.innerHTML = `
                <div class="book-detail-page">
                    <div class="bd-header">
                        <div>
                            <h1>Edit ${this.escapeHtml(heading)}</h1>
                            ${b.Title ? `<p class="bd-subtitle">${this.escapeHtml(b.Title)}</p>` : ''}
                        </div>
                    </div>
                    ${this.renderEditForm(b)}
                </div>`;
            this.attachEditListeners();
            return;
        }
        app.innerHTML = `
            <div class="book-detail-page">
                <div class="bd-header">
                    <button class="btn btn-secondary" id="bd-back">&larr; Back</button>
                    <div>
                        <h1>${this.escapeHtml(heading)}${PhysicalPage.kindBadge(d.kind)}</h1>
                        ${b.Title ? `<p class="bd-subtitle">${this.escapeHtml(b.Title)}</p>` : ''}
                        ${d.kind === 'physical' && (b.BookLocation || b.BookCondition) ? `<p class="bd-subtitle">${[b.BookLocation && 'Location: ' + this.escapeHtml(b.BookLocation), b.BookCondition && 'Condition: ' + this.escapeHtml(b.BookCondition)].filter(Boolean).join(' · ')}</p>` : ''}
                    </div>
                </div>
                <div class="bd-hero">
                    <div class="bd-cover">
                        ${d.file ? `<img src="${this.escapeHtml(d.cover_url)}" alt="" onerror="this.replaceWith(Object.assign(document.createElement('div'),{className:'bd-cover-none',textContent:'No cover'}))">`
                                 : '<div class="bd-cover-none">No file</div>'}
                    </div>
                    <div class="bd-hero-info">
                        ${this.renderBadges(d)}
                        ${this.renderGrid([
                            ['Publisher', [b.Publisher, b.Imprint].filter(Boolean).join(' / ')],
                            ['Released', [b.Year, b.Month, b.Day].filter(Boolean).join('-')],
                            ['Format', b.Format],
                            ['Issue count', b.Count],
                            ['Pages', b.PageCount],
                            ['Series group', b.SeriesGroup],
                            ['Story arc', b.StoryArc],
                            ['Alternate', b.AlternateSeries ? `${b.AlternateSeries}${b.AlternateNumber ? ' #' + b.AlternateNumber : ''}` : ''],
                            ['Genre', b.Genre],
                            ['Language', b.LanguageISO],
                            ['Age rating', b.AgeRating],
                            ['Added', this.formatTime(b.AddedTime)],
                        ])}
                        ${this.renderComicVine(d.comicvine)}
                    </div>
                </div>
                ${this.renderActions(d)}
                ${this.renderFlags(d.flags)}
                ${this.section('File', this.renderFile(d))}
                ${this.section('Credits', this.renderGrid([
                    ['Writer', b.Writer], ['Penciller', b.Penciller], ['Inker', b.Inker],
                    ['Colorist', b.Colorist], ['Letterer', b.Letterer], ['Cover artist', b.CoverArtist],
                    ['Editor', b.Editor], ['Translator', b.Translator],
                ]))}
                ${this.section('Story', this.renderStory(b))}
                ${this.section('Reading', this.renderGrid([
                    ['Status', b.CurrentPage > 0 || b.LastPageRead > 0 ? `Page ${(b.LastPageRead || b.CurrentPage) + 1} of ${b.PageCount || '?'}` : 'Unread'],
                    ['Opened', b.OpenCount ? `${b.OpenCount} time${b.OpenCount === 1 ? '' : 's'}` : ''],
                    ['Last opened', this.formatTime(b.OpenedTime)],
                    ['Rating', b.Rating],
                ]))}
                ${(d.collects || []).length ? this.section('Collects', this.renderCollects(d.collects)) : ''}
                ${this.section('Lists', this.renderLists())}
                ${this.section('Custom values', this.renderCustomValues(d.custom_values), false)}
                ${this.section('History', this.renderHistory())}
                ${d.file ? this.section('ComicInfo.xml in the file', this.renderComicInfo(), false) : ''}
            </div>
        `;
        this.attachListeners();
    }

    renderBadges(d) {
        const badges = [];
        if (d.stage_label) badges.push(`<span class="bd-badge">Stage: ${this.escapeHtml(d.stage_label)}</span>`);
        if (!d.file) badges.push('<span class="bd-badge">No file (wanted or physical)</span>');
        else if (!d.file.exists) badges.push('<span class="bd-badge bd-badge-error">File missing</span>');
        if (d.cv_nomatch) badges.push(`<span class="bd-badge">Not in ComicVine (${this.escapeHtml(this.noMatchLabel(d.cv_nomatch.reason))}, ${this.escapeHtml(d.cv_nomatch.date)})</span>`);
        if (d.book.ComicInfoIsDirty) badges.push('<span class="bd-badge">ComicInfo not yet written</span>');
        return badges.length ? `<div class="bd-badges">${badges.join('')}</div>` : '';
    }

    noMatchLabel(reason) {
        const found = BookDetailPage.NO_MATCH_REASONS.find(r => r[0] === reason);
        if (reason === 'legacy_cvdbskip') return 'Old CVDBSKIP tag';
        return found ? found[1] : reason;
    }

    // Mark / undo "not in ComicVine" (comic-server-5wer).
    renderNoMatch(d) {
        if (!d.file) return '';
        if (d.cv_nomatch) {
            return `<button class="btn btn-secondary" id="bd-nomatch-undo-btn" ${this.busy ? 'disabled' : ''}>Undo "not in ComicVine"</button>`;
        }
        if (d.comicvine) return '';
        return `<span class="bd-rollback">
            <select id="bd-nomatch-reason" aria-label="Why it is not in ComicVine">
                ${BookDetailPage.NO_MATCH_REASONS.map(r => `<option value="${r[0]}">${r[1]}</option>`).join('')}
            </select>
            <button class="btn btn-secondary" id="bd-nomatch-btn" ${this.busy ? 'disabled' : ''}>Mark as not in ComicVine</button>
        </span>`;
    }

    async setNoMatch(set) {
        const sel = document.getElementById('bd-nomatch-reason');
        await this.runAction(async () => {
            try {
                await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/cv-nomatch`,
                    { set, reason: set && sel ? sel.value : '' });
                dialogs.toast(set ? 'Marked as not in ComicVine. The book moves on in the workflow.' : 'Mark removed. The book can be scraped again.', 'success');
            } catch (e) {
                dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
            }
        });
    }

    renderComicVine(cv) {
        if (!cv) return '<p class="empty-message">Not tagged with ComicVine.</p>';
        const links = [];
        if (cv.volume_url) links.push(`<a href="${this.escapeHtml(cv.volume_url)}" target="_blank" rel="noopener">ComicVine volume ${cv.volume_id}</a>`);
        if (cv.issue_url) links.push(`<a href="${this.escapeHtml(cv.issue_url)}" target="_blank" rel="noopener">ComicVine issue ${cv.issue_id}</a>`);
        return `<p class="bd-links">${links.join(' &middot; ')}</p>${this.renderCompleteness()}`;
    }

    // Actions (comic-server-2s4f). A blocked action stays visible but
    // disabled, with the reason, so the user knows what to set up.
    renderActions(d) {
        const a = d.actions || {};
        const btn = (id, label, blocker, cls = 'btn-secondary') =>
            `<button class="btn ${cls}" id="${id}" ${blocker || this.busy ? 'disabled' : ''} title="${this.escapeHtml(blocker || '')}">${label}</button>`;
        const notes = [];
        if (a.scrape_blocker) notes.push(`Re-scrape: ${a.scrape_blocker}.`);
        if (a.write_metadata_blocker) notes.push(`Write metadata: ${a.write_metadata_blocker}.`);
        if (a.variant_covers_blocker) notes.push(`Variant covers: ${a.variant_covers_blocker}.`);
        if (a.reader_blocker) notes.push(`Read: ${a.reader_blocker}.`);
        if (a.page_manager_blocker) notes.push(`Manage pages: ${a.page_manager_blocker}.`);
        const stages = a.rollback_stages || [];
        const rollback = stages.length ? `
            <span class="bd-rollback">
                <select id="bd-rollback-stage" aria-label="Stage to roll back to">
                    ${stages.map(s => `<option value="${this.escapeHtml(s.value)}">${this.escapeHtml(s.label)}</option>`).join('')}
                </select>
                ${btn('bd-rollback-btn', 'Roll back stage', '')}
            </span>` : '';
        return `
            <div class="panel bd-actions-panel">
                <div class="bd-actions">
                    ${btn('bd-scrape-btn', 'Re-scrape', a.scrape_blocker)}
                    ${btn('bd-write-btn', a.stale ? 'Write metadata to file (out of date)' : 'Write metadata to file', a.write_metadata_blocker)}
                    ${btn('bd-variants-btn', 'Variant covers…', a.variant_covers_blocker)}
                    ${btn('bd-read-btn', 'Read', a.reader_blocker, 'btn-primary')}
                    ${btn('bd-edit-btn', 'Edit', '')}
                    ${btn('bd-mark-read-btn', d.book.OpenCount > 0 && !this.isUnread(d.book) ? 'Mark as unread' : 'Mark as read', '')}
                    ${btn('bd-pages-btn', 'Manage pages…', a.page_manager_blocker)}
                    ${d.file && !d.file.exists ? btn('bd-relink-btn', 'Relink file…', '') + btn('bd-remove-record-btn', 'Remove record…', '') : ''}
                    ${this.renderNoMatch(d)}
                    ${rollback}
                </div>
                ${d.file ? `<details class="bd-cvmatch"><summary>Match by ComicVine link or ID</summary><div id="bd-cvmatch-host"></div></details>` : ''}
                ${notes.length ? `<p class="empty-message bd-action-notes">${notes.map(n => this.escapeHtml(n)).join('<br>')}</p>` : ''}
            </div>`;
    }

    async runAction(fn) {
        if (this.busy) return;
        this.busy = true;
        this.render();
        try {
            await fn();
        } finally {
            this.busy = false;
            await Promise.all([this.loadDetail(), this.loadHistory(), this.loadLists()]);
            if (this.comicInfo && this.comicInfo !== 'loading') this.comicInfo = null; // file may have changed
            this.render();
        }
    }

    async postJSON(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        const text = await res.text();
        if (!res.ok) {
            const err = new Error(text.trim() || res.statusText);
            err.status = res.status;
            throw err;
        }
        return text ? JSON.parse(text) : {};
    }

    async rescrape() {
        const ok = await dialogs.confirm({
            title: 'Re-scrape this book',
            message: 'Search ComicVine again for this book. A confident match replaces its ComicVine tags and metadata; an unsure match goes to Scrape Review.',
            confirmLabel: 'Re-scrape',
        });
        if (!ok) return;
        await this.runAction(async () => {
            try {
                const started = await this.postJSON('/api/scrape', { book_ids: [this.bookId] });
                ScrapeLog.open(started.job_id, {
                    title: 'Re-scrape',
                    onDone: async () => {
                        await Promise.all([this.loadDetail(), this.loadHistory()]);
                        this.render();
                    },
                });
            } catch (e) {
                dialogs.toast(e.status === 409 ? 'A scrape is already running. Try again when it finishes.' : `Re-scrape failed: ${e.message}`, 'error', 8000);
            }
        });
    }

    async writeMetadata() {
        const ok = await dialogs.confirm({
            title: 'Write metadata to file',
            message: 'Replace the ComicInfo.xml inside this CBZ with the library\'s metadata. Fields that are only in the file (not in the library) are removed - use "Compare with file" below to check first. The old file goes to Trash.',
            confirmLabel: 'Write to file',
            danger: true,
        });
        if (!ok) return;
        await this.runAction(async () => {
            try {
                const res = await this.postJSON('/api/library/workflow/write-metadata', { book_ids: [this.bookId] });
                if (res.errors && res.errors.length) dialogs.toast(`Write failed: ${res.errors.join('; ')}`, 'error', 8000);
                else dialogs.toast('Metadata written to the file.', 'success');
            } catch (e) {
                dialogs.toast(`Write failed: ${e.message}`, 'error', 8000);
            }
        });
    }

    // Look for the missing file in its old folder (comic-server-heox). When
    // that finds nothing, offer to pick the file by hand (comic-server-679r).
    async relinkFile() {
        await this.runAction(async () => {
            try {
                const url = `/api/library/books/${encodeURIComponent(this.bookId)}/relink`;
                let p = await this.postJSON(url, { apply: false });
                let picked = '';
                if (p.status === 'not_missing' || p.status === 'no_file') {
                    dialogs.toast(p.status === 'no_file' ? 'This book has no file.' : 'The file is on disk; nothing to relink.', 'info', 9000);
                    return;
                }
                if (p.status !== 'found' && p.taken_by) {
                    this.showTakenDialog(p);
                    return;
                }
                if (p.status !== 'found') {
                    const why = {
                        not_found: 'No file in the old folder fits this book.',
                        ambiguous: `More than one file fits, so none was picked: ${(p.candidates || []).join(', ')}.`,
                        folder_missing: 'The old folder is gone as well.',
                    }[p.status] || `Not relinked (${p.status}).`;
                    const pick = await dialogs.confirm({
                        title: 'Relink file',
                        message: `${why}\n\nPick the file by hand?`,
                        confirmLabel: 'Pick file…',
                    });
                    if (!pick) return;
                    picked = await dialogs.browseServerFile({ title: 'Pick the file for this book', startPath: p.start_dir || '/' });
                    if (!picked) return;
                    p = await this.postJSON(url, { apply: false, file_path: picked });
                    if (p.status !== 'found' && p.taken_by) {
                        this.showTakenDialog(p);
                        return;
                    }
                    if (p.status !== 'found') {
                        dialogs.toast(p.reason || `Not relinked (${p.status}).`, 'error', 9000);
                        return;
                    }
                }
                const ok = await dialogs.confirm({
                    title: 'Relink file',
                    message: `Point this book at:\n${p.new_path}\n\nMatched by: ${p.reason}. Only the library record changes; no file is moved.`,
                    confirmLabel: 'Relink',
                });
                if (!ok) return;
                await this.postJSON(url, picked ? { apply: true, file_path: picked } : { apply: true });
                dialogs.toast('Relinked.', 'success');
            } catch (e) {
                dialogs.toast(`Relink failed: ${e.message}`, 'error', 8000);
            }
        });
    }

    // The file a relink wanted already belongs to another record
    // (comic-server-2rz8). Name that record, link to it, and offer to remove
    // this one (the one with the missing file - often a stale leftover).
    showTakenDialog(p) {
        const t = p.taken_by;
        const esc = (x) => this.escapeHtml(String(x == null ? '' : x));
        const name = [t.series, t.number ? `#${t.number}` : '', t.title && !t.title.startsWith(t.series || '') ? `- ${t.title}` : ''].filter(Boolean).join(' ') || t.book_id;
        const close = dialogs.custom({
            title: 'File already used by another book',
            bodyHTML: `
                <p>${esc(p.reason || 'Another book already uses that file.')}</p>
                <p>Used by: <a id="bd-taken-link" href="/books/${encodeURIComponent(t.book_id)}" data-route>${esc(name)}</a></p>
                ${t.path ? `<p class="empty-message">${esc(t.path)}</p>` : ''}
                <p class="empty-message">If that record is the real one and this one is a leftover, remove this record. Nothing is changed until you choose.</p>`,
            footerHTML: `
                <button class="btn btn-secondary" data-action="cancel">Close</button>
                <button class="btn btn-danger" id="bd-taken-remove">Remove this record…</button>`,
            onOpen: () => {
                const link = document.getElementById('bd-taken-link');
                if (link) link.addEventListener('click', () => close());
                const rm = document.getElementById('bd-taken-remove');
                if (rm) rm.addEventListener('click', () => { close(); this.removeRecord(); });
            },
        });
    }

    // Delete only the library record of a book whose file is gone.
    async removeRecord() {
        const ok = await dialogs.confirm({
            title: 'Remove this record',
            message: 'Delete this book from the library. Its file is already missing; no file is touched. Its metadata and reading progress are lost.',
            confirmLabel: 'Remove record',
            danger: true,
        });
        if (!ok) return;
        try {
            await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/remove-record`, {});
            dialogs.toast('Record removed.', 'success');
            router.navigate('/workflow');
        } catch (e) {
            dialogs.toast(`Could not remove: ${e.message}`, 'error', 8000);
        }
    }

    async rollback() {
        const sel = document.getElementById('bd-rollback-stage');
        if (!sel) return;
        const label = sel.options[sel.selectedIndex].text;
        const ok = await dialogs.confirm({
            title: 'Roll back stage',
            message: `Move this book back to "${label}" so the pipeline runs it again from there. Field values and files are not changed.`,
            confirmLabel: 'Roll back',
        });
        if (!ok) return;
        await this.runAction(async () => {
            try {
                await this.postJSON('/api/library/audit/rollback', { book_id: this.bookId, stage: sel.value });
                dialogs.toast(`Moved back to ${label}.`, 'success');
            } catch (e) {
                dialogs.toast(`Rollback failed: ${e.message}`, 'error', 8000);
            }
        });
    }

    renderFlags(flags) {
        const accepted = (this.detail && this.detail.accepted_flags) || [];
        if (!flags.length && !accepted.length) return '';
        const rows = flags.map(f => `
            <li class="attention-row attention-${this.escapeHtml(f.severity)}">
                <span class="attention-chip">${this.escapeHtml(f.severity)}</span>
                <span class="attention-label">${this.escapeHtml(f.label)}</span>
                <span class="attention-hint empty-message">${this.escapeHtml(f.hint)}</span>
                <button class="btn btn-secondary bd-accept-btn" data-flag="${this.escapeHtml(f.flag)}" data-accept="1"
                        title="Hide this flag on this book. It stays hidden until you show it again.">Accept</button>
            </li>`).join('');
        const hidden = accepted.length ? `
            <p class="empty-message bd-accepted">Accepted (hidden) on this book:
                ${accepted.map(f => `<span class="bd-badge">${this.escapeHtml(f.label)}
                    <button class="bd-unaccept-btn bd-accept-btn" data-flag="${this.escapeHtml(f.flag)}" data-accept="0" title="Show this flag again">&times;</button></span>`).join(' ')}
            </p>` : '';
        return `<div class="panel attention-panel"><h2>Needs attention</h2>${rows ? `<ul class="attention-list">${rows}</ul>` : ''}${hidden}</div>`;
    }

    // setFieldLock saves a lock at once, without re-rendering, so unsaved
    // edits in the form are kept (tracker #27).
    async setFieldLock(field, locked, box) {
        try {
            const data = await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/lock-field`, { field, locked });
            if (this.detail) this.detail.locked_fields = data.locked_fields || [];
        } catch (e) {
            box.checked = !locked;
            dialogs.toast(`Could not ${locked ? 'lock' : 'unlock'} ${field}: ${e.message}`, 'error', 6000);
        }
    }

    async setAccepted(flag, accepted) {
        await this.runAction(async () => {
            try {
                await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/accept-flag`, { flag, accepted });
            } catch (e) {
                dialogs.toast(`Could not update the flag: ${e.message}`, 'error', 6000);
            }
        });
    }

    renderFile(d) {
        if (!d.file) return '<p class="empty-message">This book has no file. It is a Wanted placeholder or a physical copy.</p>';
        const f = d.file;
        return this.renderGrid([
            ['Path', `<code>${this.escapeHtml(f.path)}</code>`, true],
            ['Opened from', f.resolved_path !== f.path ? `<code>${this.escapeHtml(f.resolved_path)}</code>` : '', true],
            ['On disk', f.exists ? 'Yes' : 'No - not found at that path'],
            ['Size', this.formatSize(f.size || d.book.FileSize)],
            ['Type', f.ext],
            ['Scan info', d.book.ScanInformation],
        ]);
    }

    renderStory(b) {
        // Characters, teams and the main character link to Browse
        // (comic-server-n2yb). 6 = "is a member of the comma list", 0 = equals.
        const grid = this.renderGrid([
            ['Characters', this.chips(b.Characters, 'Characters', 6), true],
            ['Main character', this.chips(b.MainCharacterOrTeam, 'MainCharacterOrTeam', 0), true],
            ['Teams', this.chips(b.Teams, 'Teams', 6), true],
            ['Locations', b.Locations], ['Tags', b.Tags],
        ]);
        const text = (label, v) => v ? `<h3>${label}</h3><p class="bd-text">${this.escapeHtml(v)}</p>` : '';
        const body = `${text('Summary', b.Summary)}${grid}${text('Notes', b.Notes)}${text('Review', b.Review)}`;
        return body.trim() ? body : '';
    }

    renderCustomValues(cv) {
        const keys = Object.keys(cv || {}).sort();
        if (!keys.length) return '';
        return `<table class="datamanager-diff-table"><thead><tr><th>Key</th><th>Value</th></tr></thead><tbody>${
            keys.map(k => `<tr><td><code>${this.escapeHtml(k)}</code></td><td>${this.escapeHtml(cv[k])}</td></tr>`).join('')
        }</tbody></table>`;
    }

    renderHistory() {
        if (!this.history) return '';
        if (!this.history.entries.length) return '<p class="empty-message">No pipeline actions recorded for this book.</p>';
        const rows = this.history.entries.map(e => {
            const changes = (e.changes || []).map(c =>
                `<div><code>${this.escapeHtml(c.field)}</code>: ${this.escapeHtml(c.old)} &rarr; ${this.escapeHtml(c.new)}</div>`).join('');
            const stage = e.stage_before !== e.stage_after && e.stage_after ? `${this.escapeHtml(e.stage_before || '-')} &rarr; ${this.escapeHtml(e.stage_after)}` : '';
            return `<tr><td>${this.escapeHtml(new Date(e.at).toLocaleString())}</td><td>${this.escapeHtml(e.action)}</td><td>${stage}</td><td>${changes}</td></tr>`;
        }).join('');
        const more = this.history.total > this.history.entries.length
            ? `<p class="empty-message">Showing the latest ${this.history.entries.length} of ${this.history.total}. <a href="/history?book_id=${encodeURIComponent(this.bookId)}" data-route>All history</a></p>` : '';
        return `<table class="datamanager-diff-table bd-history"><thead><tr><th>When</th><th>Action</th><th>Stage</th><th>Changes</th></tr></thead><tbody>${rows}</tbody></table>${more}`;
    }

    renderComicInfo() {
        const ci = this.comicInfo;
        if (ci === null) {
            return `<p class="empty-message">Opens the archive to compare its ComicInfo.xml with the library.</p>
                <button class="btn btn-secondary" id="bd-comicinfo-btn">Compare with file</button>`;
        }
        if (ci === 'loading') return '<p class="empty-message">Reading the file…</p>';
        let html = '';
        if (ci.error) html += `<p class="empty-message">${this.escapeHtml(ci.error)}</p>`;
        if (!ci.present && !ci.error) {
            html += '<p class="empty-message">The file has no ComicInfo.xml. Write Metadata adds one.</p>';
        } else if (ci.present) {
            html += ci.diff.length === 0
                ? `<p class="empty-message">The file matches the library (${ci.same} fields).</p>`
                : `<p class="empty-message">${ci.diff.length} field(s) differ, ${ci.same} match. "Library" is what Write Metadata would put in the file.</p>
                   <table class="datamanager-diff-table"><thead><tr><th>Field</th><th>In the file</th><th>Library</th></tr></thead><tbody>${
                    ci.diff.map(f => `<tr><td><code>${this.escapeHtml(f.field)}</code></td><td>${this.escapeHtml(f.file)}</td><td>${this.escapeHtml(f.library)}</td></tr>`).join('')
                   }</tbody></table>`;
            html += `<details class="bd-raw"><summary>Raw XML${ci.truncated ? ' (cut at 256 KB)' : ''}</summary><pre>${this.escapeHtml(ci.raw)}</pre></details>`;
        }
        return html;
    }

    // Same rule as library.ComicBook.IsUnread.
    isUnread(b) {
        if (!b.OpenCount) return true;
        return b.PageCount > 0 ? b.LastPageRead < b.PageCount - 1 : false;
    }

    async toggleRead() {
        const read = !(this.detail.book.OpenCount > 0 && !this.isUnread(this.detail.book));
        await this.runAction(async () => {
            try {
                await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/edit`, { read: read ? 'read' : 'unread' });
                dialogs.toast(read ? 'Marked as read.' : 'Marked as unread.', 'success');
            } catch (e) {
                dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
            }
        });
    }

    // The value a field shows in the form, as a string. A 0 number is
    // shown empty, the way the library treats it as unset.
    editValue(b, name, type) {
        const v = b[name];
        if (type === 'bool') return v ? 'Yes' : 'No';
        if (type === 'int' || type === 'num') return v ? String(v) : '';
        if (BookDetailPage.EDIT_CHOICES[type] && v === 'Unknown') return '';
        return v == null ? '' : String(v);
    }

    renderEditField(b, [name, label, type]) {
        const id = `bd-edit-${name}`;
        const v = this.editValue(b, name, type);
        const attrs = `id="${id}" data-field="${name}" data-type="${type}"`;
        let input;
        if (type === 'textarea') {
            input = `<textarea ${attrs} rows="4">${this.escapeHtml(v)}</textarea>`;
        } else if (type === 'bool') {
            return `<label class="bd-edit-check"><input type="checkbox" ${attrs} ${b[name] ? 'checked' : ''}> ${this.escapeHtml(label)}${this.lockToggle(name)}</label>`;
        } else if (BookDetailPage.EDIT_CHOICES[type]) {
            const opts = BookDetailPage.EDIT_CHOICES[type].slice();
            if (v && !opts.some(o => o[0] === v)) opts.push([v, v]); // keep a value the list does not know
            input = `<select ${attrs}>${opts.map(([val, text]) =>
                `<option value="${this.escapeHtml(val)}" ${val === v ? 'selected' : ''}>${this.escapeHtml(text)}</option>`).join('')}</select>`;
        } else {
            const extra = type === 'int' ? 'type="number" step="1" min="0"'
                : type === 'num' ? `type="number" min="0" ${name.endsWith('Rating') ? 'max="5" step="0.5"' : 'step="any"'}`
                : 'type="text"';
            input = `<input ${extra} ${attrs} value="${this.escapeHtml(v)}">`;
        }
        return `<label class="bd-edit-field${type === 'textarea' ? ' bd-edit-wide' : ''}" for="${id}"><span>${this.escapeHtml(label)}${this.lockToggle(name)}</span>${input}</label>`;
    }

    // lockToggle is the small "lock against undo" checkbox beside a field
    // label (tracker #27). It saves at once, apart from the form's Save.
    lockToggle(name) {
        const locked = ((this.detail && this.detail.locked_fields) || []).includes(name);
        return ` <span class="bd-lock" title="Lock: History undo will never change this field"><input type="checkbox" class="bd-lock-check" data-lock-field="${this.escapeHtml(name)}" ${locked ? 'checked' : ''} aria-label="Lock ${this.escapeHtml(name)} against undo"> 🔒</span>`;
    }

    renderEditForm(b) {
        const groups = BookDetailPage.EDIT_GROUPS.map(([title, fields]) => `
            <fieldset class="panel bd-edit-group">
                <legend>${this.escapeHtml(title)}</legend>
                <div class="bd-edit-grid">${fields.map(f => this.renderEditField(b, f)).join('')}</div>
            </fieldset>`).join('');
        const note = this.detail.file
            ? 'Saving changes the library only. Fields that go into ComicInfo.xml mark the file out of date; use "Write metadata to file" to update it.'
            : 'Saving changes the library only.';
        return `
            <form id="bd-edit-form" class="bd-edit-form" novalidate>
                <p class="empty-message">${note} Comma-separate lists such as writers and characters.</p>
                ${groups}
                <div class="bd-edit-buttons">
                    <button type="submit" class="btn btn-primary" id="bd-edit-save" ${this.busy ? 'disabled' : ''}>Save</button>
                    <button type="button" class="btn btn-secondary" id="bd-edit-cancel">Cancel</button>
                </div>
            </form>`;
    }

    // Only the fields the user changed are sent, so a change made elsewhere
    // to another field in the meantime is kept.
    collectEdits() {
        const b = this.detail.book;
        const fields = {};
        document.querySelectorAll('#bd-edit-form [data-field]').forEach(el => {
            const name = el.dataset.field, type = el.dataset.type;
            if (type === 'bool') {
                if (el.checked !== !!b[name]) fields[name] = el.checked;
                return;
            }
            const now = type === 'textarea' ? el.value.trim() : el.value.trim().replace(/\s+/g, ' ');
            const before = this.editValue(b, name, type);
            const same = (type === 'int' || type === 'num') ? Number(now || 0) === Number(before || 0) : now === before.trim();
            if (!same) fields[name] = now;
        });
        return fields;
    }

    async saveEdits() {
        const form = document.getElementById('bd-edit-form');
        const bad = form && [...form.querySelectorAll('input[type=number]')].find(el => !el.checkValidity());
        if (bad) {
            dialogs.toast(`${bad.closest('label').querySelector('span').textContent}: ${bad.validationMessage}`, 'error', 6000);
            bad.focus();
            return;
        }
        const fields = this.collectEdits();
        if (!Object.keys(fields).length) {
            this.editing = false;
            this.render();
            return;
        }
        if (this.busy) return;
        this.busy = true;
        const save = document.getElementById('bd-edit-save');
        if (save) save.disabled = true;
        try {
            const res = await this.postJSON(`/api/library/books/${encodeURIComponent(this.bookId)}/edit`, { fields });
            const n = (res.changed || []).length;
            dialogs.toast(n ? `Saved ${n} field${n === 1 ? '' : 's'}.` : 'Nothing changed.', 'success');
            this.editing = false;
        } catch (e) {
            // Keep the form and what was typed; the server says which field is wrong.
            dialogs.toast(`Could not save: ${e.message}`, 'error', 8000);
            if (save) save.disabled = false;
            this.busy = false;
            return;
        }
        this.busy = false;
        await Promise.all([this.loadDetail(), this.loadHistory()]);
        if (this.comicInfo && this.comicInfo !== 'loading') this.comicInfo = null;
        this.render();
        window.scrollTo(0, 0);
    }

    attachEditListeners() {
        document.querySelectorAll('.bd-lock-check').forEach(el => el.addEventListener('change', () =>
            this.setFieldLock(el.dataset.lockField, el.checked, el)));
        const form = document.getElementById('bd-edit-form');
        if (!form) return;
        form.addEventListener('submit', e => { e.preventDefault(); this.saveEdits(); });
        document.getElementById('bd-edit-cancel').addEventListener('click', async () => {
            if (Object.keys(this.collectEdits()).length) {
                const ok = await dialogs.confirm({
                    title: 'Discard changes',
                    message: 'Leave the form without saving your changes?',
                    confirmLabel: 'Discard',
                    danger: true,
                });
                if (!ok) return;
            }
            this.editing = false;
            this.render();
        });
    }

    attachListeners() {
        const back = document.getElementById('bd-back');
        if (back) back.addEventListener('click', () => {
            if (window.history.length > 1) window.history.back();
            else router.navigate('/browse');
        });
        const on = (id, fn) => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('click', fn);
        };
        on('bd-scrape-btn', () => this.rescrape());
        document.querySelectorAll('.bd-accept-btn').forEach(el => el.addEventListener('click', () =>
            this.setAccepted(el.dataset.flag, el.dataset.accept === '1')));
        on('bd-write-btn', () => this.writeMetadata());
        on('bd-rollback-btn', () => this.rollback());
        const cvHost = document.getElementById('bd-cvmatch-host');
        if (cvHost && typeof CVMatchBox !== 'undefined') {
            new CVMatchBox(this.bookId, cvHost, () => this.runAction(async () => {}));
        }
        on('bd-nomatch-btn', () => this.setNoMatch(true));
        on('bd-nomatch-undo-btn', () => this.setNoMatch(false));
        on('bd-variants-btn', () => router.navigate(`/variant-covers?book=${encodeURIComponent(this.bookId)}`));
        on('bd-read-btn', () => router.navigate(`/reader?book=${encodeURIComponent(this.bookId)}`));
        on('bd-edit-btn', () => { this.editing = true; this.render(); window.scrollTo(0, 0); });
        on('bd-mark-read-btn', () => this.toggleRead());
        on('bd-pages-btn', () => router.navigate(`/page-manager?book=${encodeURIComponent(this.bookId)}`));
        on('bd-relink-btn', () => this.relinkFile());
        on('bd-remove-record-btn', () => this.removeRecord());
        on('bd-add-list-btn', () => this.addToList());
        document.querySelectorAll('.bd-list-remove').forEach(el => el.addEventListener('click', () =>
            this.removeFromList(el.dataset.listId, el.dataset.listName)));
        const ciBtn = document.getElementById('bd-comicinfo-btn');
        if (ciBtn) ciBtn.addEventListener('click', () => this.loadComicInfo());
        document.querySelectorAll('.bd-section').forEach(el => el.addEventListener('toggle', () => {
            this.openSections[el.dataset.title] = el.open;
        }));
    }
}

// Edit form layout: [group title, [[field, label, type]...]]. Types: text,
// int, num, textarea, bool, or a key of EDIT_CHOICES.
BookDetailPage.EDIT_GROUPS = [
    ['Series', [
        ['Series', 'Series', 'text'], ['Number', 'Number', 'text'], ['Count', 'Issue count', 'int'],
        ['Volume', 'Volume', 'int'], ['Title', 'Title', 'text'], ['Format', 'Format', 'text'],
        ['AlternateSeries', 'Alternate series', 'text'], ['AlternateNumber', 'Alternate number', 'text'],
        ['AlternateCount', 'Alternate count', 'int'], ['StoryArc', 'Story arc', 'text'],
        ['SeriesGroup', 'Series group', 'text'], ['SeriesComplete', 'Series complete', 'yesno'],
    ]],
    ['Publishing', [
        ['Publisher', 'Publisher', 'text'], ['Imprint', 'Imprint', 'text'],
        ['Year', 'Year', 'int'], ['Month', 'Month', 'int'], ['Day', 'Day', 'int'],
        ['Genre', 'Genre', 'text'], ['LanguageISO', 'Language (ISO code)', 'text'],
        ['AgeRating', 'Age rating', 'text'], ['BlackAndWhite', 'Black and white', 'yesno'],
        ['Manga', 'Manga', 'manga'], ['Web', 'Web', 'text'], ['ISBN', 'ISBN', 'text'],
    ]],
    ['Credits', [
        ['Writer', 'Writer', 'text'], ['Penciller', 'Penciller', 'text'], ['Inker', 'Inker', 'text'],
        ['Colorist', 'Colorist', 'text'], ['Letterer', 'Letterer', 'text'], ['CoverArtist', 'Cover artist', 'text'],
        ['Editor', 'Editor', 'text'], ['Translator', 'Translator', 'text'],
    ]],
    ['Story', [
        ['Characters', 'Characters', 'text'], ['MainCharacterOrTeam', 'Main character or team', 'text'],
        ['Teams', 'Teams', 'text'], ['Locations', 'Locations', 'text'], ['Tags', 'Tags', 'text'],
        ['Summary', 'Summary', 'textarea'],
    ]],
    ['Personal', [
        ['Rating', 'My rating (0-5)', 'num'], ['CommunityRating', 'Community rating (0-5)', 'num'],
        ['Checked', 'Checked', 'bool'], ['Notes', 'Notes', 'textarea'], ['Review', 'Review', 'textarea'],
    ]],
    ['Collection', [
        ['BookCollectionStatus', 'Collection status', 'text'], ['BookLocation', 'Location', 'text'],
        ['BookCondition', 'Condition', 'text'], ['BookAge', 'Age', 'text'], ['BookOwner', 'Owner', 'text'],
        ['BookStore', 'Store', 'text'], ['BookPrice', 'Price', 'num'], ['ScanInformation', 'Scan information', 'text'],
        ['BookNotes', 'Collection notes', 'textarea'],
    ]],
];
BookDetailPage.EDIT_CHOICES = {
    yesno: [['', 'Unknown'], ['Yes', 'Yes'], ['No', 'No']],
    manga: [['', 'Unknown'], ['No', 'No'], ['Yes', 'Yes'], ['YesAndRightToLeft', 'Yes, right to left']],
};
BookDetailPage.NO_MATCH_REASONS = [['collected','Collected edition'],['not_listed','Not listed'],['web_comic','Web comic'],['other','Other']];
