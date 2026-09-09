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
    }

    async init(ctx) {
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
        let html = `
            <div class="panel">
                <div class="datamanager-page-header">
                    <button class="btn btn-secondary" id="workflow-back-btn">&larr; Back</button>
                    <h2>${this.escapeHtml(d.label)} (${d.total})</h2>
                </div>
        `;
        if (this.loadingDrillIn && d.comics.length === 0) {
            html += '<p class="empty-message">Loading…</p>';
        } else if (d.comics.length === 0) {
            html += '<p class="empty-message">No books at this stage.</p>';
        } else {
            html += '<table class="datamanager-diff-table"><thead><tr><th>Series</th><th>Number</th><th>Title</th><th>Publisher</th><th>Year</th></tr></thead><tbody>';
            for (const c of d.comics) {
                html += `<tr><td>${this.escapeHtml(c.series)}</td><td>${this.escapeHtml(c.number)}</td><td>${this.escapeHtml(c.title)}</td><td>${this.escapeHtml(c.publisher)}</td><td>${c.year || ''}</td></tr>`;
            }
            html += '</tbody></table>';
            if (d.hasMore) {
                html += `<div class="load-more-container"><button id="workflow-load-more-btn" class="btn btn-secondary">Load More</button></div>`;
            }
        }
        html += '</div>';
        return html;
    }

    attachListeners() {
        document.querySelectorAll('.workflow-view-btn').forEach(el => {
            el.addEventListener('click', () => this.openDrillIn(el.dataset.stage, el.dataset.label));
        });

        const backBtn = document.getElementById('workflow-back-btn');
        if (backBtn) backBtn.addEventListener('click', () => { this.drillIn = null; this.render(); this.attachListeners(); });

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
        if (!reset) { this.render(); this.attachListeners(); }

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
                throw new Error(text || `Failed to run ${stage}`);
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
