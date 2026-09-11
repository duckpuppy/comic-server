// Workflow Dashboard (comic-server-1iv.3) - the native replacement for
// the manual ComicRack ingest-pipeline smart lists ("0 Day Folder",
// "01 Convert to CBZ", ..., "08 To Move"). Shows a live count per stage,
// lets the user drill into the actual book list for one stage, and run
// that stage's action directly - no smart list ever needs to be created
// or maintained just to track pipeline progress.
//
// Scrape's button starts the existing background scrape job (POST
// /api/scrape, already whole-library-scoped by default - untagged books
// only) rather than a new endpoint; convert_cbz/scan_info/data_manager
// call new whole-library workflow endpoints added alongside this page.
// ToMove's button is different from the rest: it needs a profile picker
// and a preview/approve step first (multiple Library Organizer profiles,
// and this is comic-server's first feature that moves/renames the user's
// own files), so instead of running in place it navigates to a dedicated
// page (comic-server-3bz.6) rather than firing one POST.
class WorkflowPage {
    constructor() {
        this.summary = null; // { stages: [...], cvdb_skip }
        this.error = null;
        this.running = null; // stage string currently running an action, or null
        this.lastResult = null; // { stage, message }
        this.drillIn = null; // { stage, label, comics: [], total, limit, offset, hasMore } or null
        this.loadingDrillIn = false;
        // New Files (comic-server-chh) - "stage 0", files sitting in a
        // configured watch folder that aren't backed by any library book
        // yet. Loaded alongside the summary so its count shows on the
        // dashboard without a separate round trip; kept as flat state
        // (not folded into `summary`) since it comes from a different
        // endpoint and has its own selection/start-processing flow.
        this.newFiles = { total: 0, files: [] };
        this.newFilesDrillIn = false; // true when showing the New Files list instead of a stage drill-in
        this.selectedNewFiles = new Set();
        this.startingProcessing = false;
    }

    async init(ctx) {
        // Render once BEFORE the fetch so the "Loading…" state (below, in
        // renderBody) actually appears - this page's summary comes from a
        // full-library GetAllBooks() scan on the server, which takes
        // multiple seconds on a large library, so without this the tab
        // just sits blank until the fetch resolves.
        this.render();
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
        this.attachListeners();
    }

    async load() {
        try {
            const response = await fetch('/api/library/workflow');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            this.summary = await response.json();
        } catch (error) {
            console.error('Failed to load workflow summary:', error);
            this.error = 'Failed to load workflow summary. Please try again.';
        }
        await this.loadNewFiles();
    }

    async loadNewFiles() {
        try {
            const response = await fetch('/api/library/workflow/new-files');
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            this.newFiles = await response.json();
        } catch (error) {
            console.error('Failed to load watch folder new files:', error);
        }
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="workflow-page">
                <div class="workflow-page-header">
                    <h1>Workflow</h1>
                    <p class="empty-message">Tracks every book's progress through comic-server's native ingest pipeline, replacing the manual "0 Day Folder" / "01 Convert to CBZ" / ... smart lists.</p>
                </div>
                ${this.renderBody()}
            </div>
        `;
    }

    renderBody() {
        if (this.error) {
            return `<div class="panel"><p class="datamanager-errors">${this.escapeHtml(this.error)}</p></div>`;
        }
        if (!this.summary) {
            return `<div class="panel"><p class="empty-message">Loading…</p></div>`;
        }

        if (this.newFilesDrillIn) {
            return this.renderNewFilesDrillIn();
        }
        if (this.drillIn) {
            return this.renderDrillIn();
        }

        const actionFor = {
            convert_cbz: 'run-workflow-convert-cbz',
            scrape: 'run-workflow-scrape',
            scan_info: 'run-workflow-scan-info',
            data_manager: 'run-workflow-data-manager',
        };
        const labelFor = {
            convert_cbz: 'Convert to CBZ',
            scrape: 'Scrape',
            scan_info: 'Run Scan Info',
            data_manager: 'Run Data Manager',
        };

        let html = '<div class="workflow-cards">';
        html += `
            <div class="panel workflow-card">
                <h2>New Files</h2>
                <p class="workflow-card-count">${this.newFiles.total}</p>
                <p class="empty-message">Comic files sitting in a watch folder that aren't in the library yet.</p>
                <div class="workflow-card-actions">
                    <button class="btn btn-secondary" id="workflow-view-new-files-btn" ${this.newFiles.total === 0 ? 'disabled' : ''}>View</button>
                </div>
            </div>
        `;
        for (const s of this.summary.stages) {
            const btnId = actionFor[s.stage];
            const running = this.running === s.stage;
            const isToMove = s.stage === 'to_move';
            html += `
                <div class="panel workflow-card">
                    <h2>${this.escapeHtml(s.label)}</h2>
                    <p class="workflow-card-count">${s.count}</p>
                    <div class="workflow-card-actions">
                        <button class="btn btn-secondary workflow-view-btn" data-stage="${s.stage}" data-label="${this.escapeAttr(s.label)}" ${s.count === 0 ? 'disabled' : ''}>View</button>
                        ${isToMove ? `
                        <button class="btn btn-primary" id="workflow-organize-btn" ${s.count === 0 ? 'disabled' : ''}>Organize</button>` : ''}
                        ${btnId ? `
                        <button class="btn btn-primary" id="${btnId}" data-stage="${s.stage}" ${running || s.count === 0 ? 'disabled' : ''}>
                            ${running ? 'Running…' : labelFor[s.stage]}
                        </button>` : ''}
                    </div>
                </div>
            `;
        }
        html += `
            <div class="panel workflow-card workflow-card-info">
                <h2>CVDBSKIP</h2>
                <p class="workflow-card-count">${this.summary.cvdb_skip}</p>
                <p class="empty-message">Books tagged to skip ComicVine processing. Informational only.</p>
            </div>
        `;
        html += '</div>';

        if (this.lastResult) {
            html += `<div class="panel"><p>${this.escapeHtml(this.lastResult)}</p></div>`;
        }

        return html;
    }

    renderDrillIn() {
        const d = this.drillIn;
        // Fetching this stage's real book list touches every book in the
        // library to find which ones are at this stage, no matter how
        // few actually match - on a large library that reliably takes
        // several seconds. d.total starts at 0 before that finishes, so
        // showing it unconditionally read as "0 books here" rather than
        // "still counting" (a real user report - comic-server-x5r).
        const initialLoad = this.loadingDrillIn && d.comics.length === 0 && d.offset === 0;
        const countLabel = initialLoad ? '…' : d.total;
        let html = `
            <div class="panel">
                <div class="datamanager-page-header">
                    <button class="btn btn-secondary" id="workflow-back-btn">&larr; Back</button>
                    <h2>${this.escapeHtml(d.label)} (${countLabel})</h2>
                </div>
        `;
        if (this.loadingDrillIn && d.comics.length === 0) {
            html += '<p class="empty-message">Loading…</p>';
        } else if (d.comics.length === 0) {
            html += '<p class="empty-message">No books at this stage.</p>';
        } else {
            // To Move is the only stage the server computes a target path
            // for (it's the one place "where would this end up" means
            // anything - see ComicPreview.TargetPath's own doc comment),
            // so it gets its own path-focused column set instead of the
            // generic series/number/title/publisher/year table every
            // other stage uses.
            const showPaths = d.stage === 'to_move';
            if (showPaths) {
                html += '<table class="datamanager-diff-table"><thead><tr><th>Series</th><th>Current Path</th><th>Target Path</th></tr></thead><tbody>';
                for (const c of d.comics) {
                    const label = `${c.series}${c.number ? ' #' + c.number : ''}`;
                    // An empty target path always comes with a reason
                    // (comic-server-1qb's follow-up) - never a silent
                    // dash that looks indistinguishable from a bug.
                    const target = c.target_path || (c.target_path_note ? `— (${c.target_path_note})` : '—');
                    html += `<tr><td>${this.escapeHtml(label)}</td><td>${this.escapeHtml(c.current_path)}</td><td>${this.escapeHtml(target)}</td></tr>`;
                }
                html += '</tbody></table>';
            } else {
                html += '<table class="datamanager-diff-table"><thead><tr><th>Series</th><th>Number</th><th>Title</th><th>Publisher</th><th>Year</th></tr></thead><tbody>';
                for (const c of d.comics) {
                    html += `<tr><td>${this.escapeHtml(c.series)}</td><td>${this.escapeHtml(c.number)}</td><td>${this.escapeHtml(c.title)}</td><td>${this.escapeHtml(c.publisher)}</td><td>${c.year || ''}</td></tr>`;
                }
                html += '</tbody></table>';
            }
            if (d.hasMore) {
                html += `<div class="load-more-container"><button id="workflow-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
            }
        }
        html += '</div>';
        return html;
    }

    renderNewFilesDrillIn() {
        const files = this.newFiles.files || [];
        let html = `
            <div class="panel">
                <div class="datamanager-page-header">
                    <button class="btn btn-secondary" id="workflow-back-btn">&larr; Back</button>
                    <h2>New Files (${files.length})</h2>
                </div>
        `;
        if (files.length === 0) {
            html += '<p class="empty-message">No new files in a watch folder right now.</p>';
        } else {
            const allSelected = files.length > 0 && files.every(f => this.selectedNewFiles.has(f.path));
            html += `
                <p class="empty-message">Files found in a watch folder that aren't in the library yet. Select the ones to bring into the pipeline.</p>
                <table class="datamanager-diff-table">
                    <thead><tr>
                        <th><input type="checkbox" id="workflow-new-files-select-all" ${allSelected ? 'checked' : ''}></th>
                        <th>Path</th><th>Size</th>
                    </tr></thead>
                    <tbody>
            `;
            for (const f of files) {
                const checked = this.selectedNewFiles.has(f.path) ? 'checked' : '';
                html += `<tr>
                    <td><input type="checkbox" class="workflow-new-file-checkbox" data-path="${this.escapeAttr(f.path)}" ${checked}></td>
                    <td>${this.escapeHtml(f.path)}</td>
                    <td>${this.formatSize(f.size)}</td>
                </tr>`;
            }
            html += '</tbody></table>';
            html += `
                <div class="workflow-card-actions">
                    <button class="btn btn-primary" id="workflow-start-processing-btn" ${this.startingProcessing || this.selectedNewFiles.size === 0 ? 'disabled' : ''}>
                        ${this.startingProcessing ? 'Starting…' : `Start Processing (${this.selectedNewFiles.size})`}
                    </button>
                </div>
            `;
        }
        html += '</div>';
        return html;
    }

    formatSize(bytes) {
        if (!bytes) return '';
        const units = ['B', 'KB', 'MB', 'GB'];
        let size = bytes, i = 0;
        while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
        return `${size.toFixed(size >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
    }

    attachListeners() {
        const viewNewFilesBtn = document.getElementById('workflow-view-new-files-btn');
        if (viewNewFilesBtn) viewNewFilesBtn.addEventListener('click', () => {
            this.newFilesDrillIn = true;
            this.selectedNewFiles = new Set();
            this.render();
            this.attachListeners();
        });

        const selectAll = document.getElementById('workflow-new-files-select-all');
        if (selectAll) selectAll.addEventListener('change', () => {
            const files = this.newFiles.files || [];
            if (selectAll.checked) {
                files.forEach(f => this.selectedNewFiles.add(f.path));
            } else {
                this.selectedNewFiles.clear();
            }
            this.render();
            this.attachListeners();
        });

        document.querySelectorAll('.workflow-new-file-checkbox').forEach(el => {
            el.addEventListener('change', () => {
                if (el.checked) this.selectedNewFiles.add(el.dataset.path);
                else this.selectedNewFiles.delete(el.dataset.path);
                this.render();
                this.attachListeners();
            });
        });

        const startBtn = document.getElementById('workflow-start-processing-btn');
        if (startBtn) startBtn.addEventListener('click', () => this.startProcessing());

        document.querySelectorAll('.workflow-view-btn').forEach(el => {
            el.addEventListener('click', () => this.openDrillIn(el.dataset.stage, el.dataset.label));
        });

        const backBtn = document.getElementById('workflow-back-btn');
        if (backBtn) backBtn.addEventListener('click', () => {
            this.drillIn = null;
            this.newFilesDrillIn = false;
            this.render();
            this.attachListeners();
        });

        const loadMoreBtn = document.getElementById('workflow-load-more-btn');
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', () => this.loadDrillInPage(false));

        const cbzBtn = document.getElementById('run-workflow-convert-cbz');
        if (cbzBtn) cbzBtn.addEventListener('click', () => this.runAction('convert_cbz', '/api/library/workflow/convert-cbz', r => `Converted ${r.converted} of ${r.processed} books.`));

        const scanInfoBtn = document.getElementById('run-workflow-scan-info');
        if (scanInfoBtn) scanInfoBtn.addEventListener('click', () => this.runAction('scan_info', '/api/library/workflow/scan-info', r => `Updated ${r.updated} of ${r.processed} books.`));

        const dmBtn = document.getElementById('run-workflow-data-manager');
        if (dmBtn) dmBtn.addEventListener('click', () => this.runAction('data_manager', '/api/library/datamanager-apply', r => `Applied changes to ${r.changed} book(s).`));

        const scrapeBtn = document.getElementById('run-workflow-scrape');
        if (scrapeBtn) {
            scrapeBtn.addEventListener('click', () => this.runAction('scrape', '/api/scrape', () => 'Scrape job started - see Dashboard for progress.'));
        }

        const organizeBtn = document.getElementById('workflow-organize-btn');
        if (organizeBtn) organizeBtn.addEventListener('click', () => router.navigate('/organize'));
    }

    async openDrillIn(stage, label) {
        this.drillIn = { stage, label, comics: [], total: 0, limit: 20, offset: 0, hasMore: false };
        this.render();
        this.attachListeners();
        await this.loadDrillInPage(true);
    }

    async loadDrillInPage(reset) {
        const d = this.drillIn;
        this.loadingDrillIn = true;
        // Render right after flipping loadingDrillIn - openDrillIn's own
        // render() call happens BEFORE this, so skipping this one on a
        // reset (the old behavior) left the very first render as the
        // last thing on screen for the whole fetch, with loadingDrillIn
        // still false at that point - the header's book count showed a
        // bare "0" instead of the "still counting" state for however
        // long the fetch took (comic-server-x5r).
        this.render();
        this.attachListeners();

        try {
            const offset = reset ? 0 : d.offset;
            const url = `/api/library/workflow/${d.stage}/books?limit=${d.limit}&offset=${offset}`;
            const response = await fetch(url);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            d.total = data.total;
            d.hasMore = !!data.has_more;
            d.comics = reset ? (data.comics || []) : d.comics.concat(data.comics || []);
            d.offset = offset + d.limit;
        } catch (error) {
            console.error('Failed to load workflow stage books:', error);
        } finally {
            this.loadingDrillIn = false;
            this.render();
            this.attachListeners();
        }
    }

    async runAction(stage, url, describe) {
        this.running = stage;
        this.lastResult = null;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch(url, { method: 'POST' });
            const text = await response.text();
            if (!response.ok && response.status !== 202) {
                throw new Error(friendlyErrorText(response, text, `Failed to run ${stage}`));
            }
            const result = text ? JSON.parse(text) : {};
            this.lastResult = describe(result);
            await this.load();
        } catch (error) {
            console.error(`Failed to run ${stage}:`, error);
            this.lastResult = `Failed: ${error.message}`;
        } finally {
            this.running = null;
            this.render();
            this.attachListeners();
        }
    }

    async startProcessing() {
        if (this.selectedNewFiles.size === 0) return;
        this.startingProcessing = true;
        this.lastResult = null;
        this.render();
        this.attachListeners();

        try {
            const response = await fetch('/api/library/workflow/new-files/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ paths: Array.from(this.selectedNewFiles) }),
            });
            const text = await response.text();
            if (!response.ok) {
                throw new Error(friendlyErrorText(response, text, 'Failed to start processing'));
            }
            const result = text ? JSON.parse(text) : {};
            const failed = (result.results || []).filter(r => r.error);
            this.lastResult = failed.length > 0
                ? `Started processing ${result.created} file(s); ${failed.length} could not be started: ${failed.map(f => f.error).join('; ')}`
                : `Started processing ${result.created} file(s).`;
            this.selectedNewFiles = new Set();
            this.newFilesDrillIn = false;
            await this.load();
        } catch (error) {
            console.error('Failed to start processing:', error);
            this.lastResult = `Failed: ${error.message}`;
        } finally {
            this.startingProcessing = false;
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

window.WorkflowPage = WorkflowPage;
