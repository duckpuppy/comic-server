// Reader (comic-server-65o8): a basic page viewer for one book. Pages come
// straight from the archive (GET /api/library/books/:id/pages/:n?full=1).
// No reading progress is written back; that belongs to device sync.
class ReaderPage {
    constructor() {
        this.bookId = null;
        this.info = null;
        this.page = 0;
        this.fit = 'screen'; // 'screen' | 'width'
        this.error = '';
        this.keyHandler = null;
    }

    async init() {
        const params = new URLSearchParams(window.location.search);
        this.bookId = params.get('book');
        this.page = Math.max(0, parseInt(params.get('page') || '0', 10) || 0);
        this.error = '';
        this.info = null;
        this.detach();
        this.render();
        if (!this.bookId) {
            this.error = 'No book selected. Open a book and choose "Read".';
            this.render();
            return;
        }
        try {
            const res = await fetch(`/api/library/books/${encodeURIComponent(this.bookId)}/pages`);
            if (!res.ok) throw new Error((await res.text()) || `HTTP ${res.status}`);
            this.info = await res.json();
            if (!this.info.pages.length) throw new Error('This book has no readable pages.');
            this.page = Math.min(this.page, this.info.pages.length - 1);
        } catch (e) {
            this.error = e.message;
            this.info = null;
        }
        this.render();
        if (this.info) {
            this.keyHandler = (e) => this.onKey(e);
            document.addEventListener('keydown', this.keyHandler);
        }
    }

    detach() {
        if (this.keyHandler) {
            document.removeEventListener('keydown', this.keyHandler);
            this.keyHandler = null;
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text == null ? '' : String(text);
        return div.innerHTML;
    }

    count() { return this.info ? this.info.pages.length : 0; }

    pageUrl(n) {
        const v = encodeURIComponent(this.info.version || '');
        return `/api/library/books/${encodeURIComponent(this.bookId)}/pages/${n}?full=1&v=${v}`;
    }

    onKey(e) {
        // The router has no teardown hook: once another page replaced ours,
        // drop the listener instead of re-rendering over it.
        if (!document.querySelector('.reader-page')) {
            this.detach();
            return;
        }
        if (e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)) return;
        if (e.ctrlKey || e.metaKey || e.altKey) return;
        switch (e.key) {
            case 'ArrowRight': case 'PageDown': case ' ': e.preventDefault(); this.go(this.page + 1); break;
            case 'ArrowLeft': case 'PageUp': e.preventDefault(); this.go(this.page - 1); break;
            case 'Home': e.preventDefault(); this.go(0); break;
            case 'End': e.preventDefault(); this.go(this.count() - 1); break;
            case 'f': this.fit = this.fit === 'screen' ? 'width' : 'screen'; this.render(); break;
        }
    }

    go(n) {
        if (!this.info) return;
        const next = Math.max(0, Math.min(this.count() - 1, n));
        if (next === this.page) return;
        this.page = next;
        try {
            const url = new URL(window.location.href);
            url.searchParams.set('page', String(next));
            window.history.replaceState(window.history.state, '', url);
        } catch (e) { /* ignore */ }
        this.render();
    }

    render() {
        const app = document.getElementById('app');
        if (!app) return;
        const info = this.info;
        const total = this.count();
        app.innerHTML = `
            <div class="reader-page">
                <div class="reader-bar">
                    ${this.bookId ? `<a href="/books/${encodeURIComponent(this.bookId)}" data-route>← Back to book</a>` : ''}
                    <span class="reader-title">${info ? this.escapeHtml(info.series) + (info.number ? ' #' + this.escapeHtml(info.number) : '') : ''}</span>
                    ${info ? `
                    <span class="reader-controls">
                        <button class="btn btn-secondary" id="rd-first" ${this.page === 0 ? 'disabled' : ''} aria-label="First page">«</button>
                        <button class="btn btn-secondary" id="rd-prev" ${this.page === 0 ? 'disabled' : ''} aria-label="Previous page">‹</button>
                        <span id="rd-counter">${this.page + 1} / ${total}</span>
                        <button class="btn btn-secondary" id="rd-next" ${this.page >= total - 1 ? 'disabled' : ''} aria-label="Next page">›</button>
                        <button class="btn btn-secondary" id="rd-last" ${this.page >= total - 1 ? 'disabled' : ''} aria-label="Last page">»</button>
                        <button class="btn btn-secondary" id="rd-fit" title="Toggle fit (f)">${this.fit === 'screen' ? 'Fit width' : 'Fit screen'}</button>
                    </span>` : ''}
                </div>
                ${this.error ? `<div class="panel"><p class="datamanager-errors" id="rd-error">${this.escapeHtml(this.error)}</p></div>` : ''}
                ${info ? `
                <div class="reader-stage reader-fit-${this.fit}">
                    <img id="rd-image" src="${this.escapeHtml(this.pageUrl(this.page))}" alt="Page ${this.page + 1} of ${total}">
                </div>
                <p class="empty-message reader-hint">Arrow keys or space to turn pages, Home / End to jump, f to change fit.</p>` : ''}
            </div>`;
        this.attach();
        this.preload();
    }

    preload() {
        if (!this.info) return;
        [this.page + 1, this.page - 1].forEach(n => {
            if (n >= 0 && n < this.count()) {
                const img = new Image();
                img.src = this.pageUrl(n);
            }
        });
    }

    attach() {
        const on = (id, fn) => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('click', fn);
        };
        on('rd-first', () => this.go(0));
        on('rd-prev', () => this.go(this.page - 1));
        on('rd-next', () => this.go(this.page + 1));
        on('rd-last', () => this.go(this.count() - 1));
        on('rd-fit', () => { this.fit = this.fit === 'screen' ? 'width' : 'screen'; this.render(); });
        // Click the left third for the previous page, the rest for the next.
        const img = document.getElementById('rd-image');
        if (img) {
            img.addEventListener('click', (e) => {
                const rect = img.getBoundingClientRect();
                this.go((e.clientX - rect.left) < rect.width / 3 ? this.page - 1 : this.page + 1);
            });
        }
    }
}
