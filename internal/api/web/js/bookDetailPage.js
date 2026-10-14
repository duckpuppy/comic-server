// Book detail page (comic-server-br14): one book's record, file, workflow
// stage, attention flags, ComicVine links, history and the file's own
// ComicInfo.xml compared with the library. View-only; actions come later
// (comic-server-2s4f).
class BookDetailPage {
    constructor() {
        this.bookId = null;
        this.detail = null;
        this.history = null;
        this.comicInfo = null; // null = not loaded, 'loading', or the response
        this.error = null;
        this.loading = true;
        this.openSections = {};
    }

    async init(ctx, bookId) {
        this.bookId = bookId;
        this.detail = null;
        this.history = null;
        this.comicInfo = null;
        this.error = null;
        this.loading = true;
        this.openSections = {};
        this.busy = false;
        this.render();
        await Promise.all([this.loadDetail(), this.loadHistory()]);
        if (ctx && ctx.aborted) return;
        this.loading = false;
        this.render();
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
        if (d.book.ComicInfoIsDirty) badges.push('<span class="bd-badge">ComicInfo not yet written</span>');
        return badges.length ? `<div class="bd-badges">${badges.join('')}</div>` : '';
    }

    renderComicVine(cv) {
        if (!cv) return '<p class="empty-message">Not tagged with ComicVine.</p>';
        const links = [];
        if (cv.volume_url) links.push(`<a href="${this.escapeHtml(cv.volume_url)}" target="_blank" rel="noopener">ComicVine volume ${cv.volume_id}</a>`);
        if (cv.issue_url) links.push(`<a href="${this.escapeHtml(cv.issue_url)}" target="_blank" rel="noopener">ComicVine issue ${cv.issue_id}</a>`);
        return `<p class="bd-links">${links.join(' &middot; ')}</p>`;
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
                    ${rollback}
                </div>
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
            await Promise.all([this.loadDetail(), this.loadHistory()]);
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
                await this.postJSON('/api/scrape', { book_ids: [this.bookId] });
                dialogs.toast('Re-scrape started - see Dashboard for progress. Reload this page when it finishes.', 'success', 6000);
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
        const grid = this.renderGrid([
            ['Characters', b.Characters], ['Main character', b.MainCharacterOrTeam],
            ['Teams', b.Teams], ['Locations', b.Locations], ['Tags', b.Tags],
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
        on('bd-variants-btn', () => router.navigate(`/variant-covers?book=${encodeURIComponent(this.bookId)}`));
        const ciBtn = document.getElementById('bd-comicinfo-btn');
        if (ciBtn) ciBtn.addEventListener('click', () => this.loadComicInfo());
        document.querySelectorAll('.bd-section').forEach(el => el.addEventListener('toggle', () => {
            this.openSections[el.dataset.title] = el.open;
        }));
    }
}
