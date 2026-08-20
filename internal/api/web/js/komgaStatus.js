// Komga Sync Status - shows each configured target's most recent push
// result, including books skipped because they couldn't be matched in
// Komga (see comic-server-1c0).
class KomgaStatus {
    constructor() {
        this.snapshot = null;
        this.notConfigured = false;
        this.error = null;
    }

    async init(ctx) {
        await this.load();
        if (ctx && ctx.aborted) return;
        this.render();
    }

    async load() {
        try {
            const response = await fetch('/api/komga/status');
            if (response.status === 503) {
                this.notConfigured = true;
                return;
            }
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            this.snapshot = await response.json();
        } catch (error) {
            console.error('Failed to load Komga sync status:', error);
            this.error = 'Failed to load Komga sync status. Please try again.';
        }
    }

    render() {
        const app = document.getElementById('app');
        app.innerHTML = `
            <div class="komga-status-page">
                <div class="komga-status-header">
                    <h1>Komga Sync</h1>
                    <p class="komga-status-subtitle">Smart lists pushed to Komga collections and read lists</p>
                </div>
                ${this.renderBody()}
            </div>
        `;
    }

    renderBody() {
        if (this.notConfigured) {
            return `
                <div class="empty-state">
                    <p>Komga sync is not configured</p>
                    <p class="empty-hint">Set server.komga.enabled: true in your config to enable it</p>
                </div>
            `;
        }
        if (this.error) {
            return `<div class="empty-state"><p>${this.escapeHtml(this.error)}</p></div>`;
        }

        const targets = (this.snapshot && this.snapshot.targets) || [];

        return `
            ${this.renderIndexError()}
            ${targets.length === 0 ? `
                <div class="empty-state">
                    <p>No Komga sync targets configured</p>
                    <p class="empty-hint">Add entries under server.komga.targets to sync smart lists to Komga</p>
                </div>
            ` : `
                <div class="komga-targets">
                    ${targets.map(t => this.renderTarget(t)).join('')}
                </div>
            `}
        `;
    }

    renderIndexError() {
        if (!this.snapshot || !this.snapshot.last_index_error) return '';
        return `
            <div class="komga-index-error">
                <strong>Failed to fetch data from Komga:</strong> ${this.escapeHtml(this.snapshot.last_index_error)}
                <div class="komga-index-error-time">${this.formatTimestamp(this.snapshot.last_index_error_time)}</div>
            </div>
        `;
    }

    renderTarget(target) {
        const unmatched = target.unmatched || [];
        return `
            <div class="komga-target-card">
                <div class="komga-target-header">
                    <div>
                        <strong>${this.escapeHtml(target.komga_name)}</strong>
                        <span class="komga-target-type">${target.type === 'collection' ? 'Collection' : 'Read List'}</span>
                    </div>
                    <div class="komga-target-timestamp">${this.formatTimestamp(target.last_sync_time)}</div>
                </div>
                <div class="komga-target-stats">
                    <div class="komga-stat">
                        <span class="komga-stat-label">Matched:</span>
                        <span class="komga-stat-value">${target.matched_count}</span>
                    </div>
                    <div class="komga-stat">
                        <span class="komga-stat-label">Unmatched:</span>
                        <span class="komga-stat-value ${unmatched.length > 0 ? 'komga-stat-warning' : ''}">${unmatched.length}</span>
                    </div>
                </div>
                ${target.error ? `
                    <div class="komga-target-error">
                        <strong>Error:</strong> ${this.escapeHtml(target.error)}
                    </div>
                ` : ''}
                ${unmatched.length > 0 ? this.renderUnmatched(unmatched) : ''}
            </div>
        `;
    }

    renderUnmatched(unmatched) {
        return `
            <details class="komga-unmatched">
                <summary>${unmatched.length} unmatched book${unmatched.length !== 1 ? 's' : ''}</summary>
                <table class="komga-unmatched-table">
                    <thead>
                        <tr>
                            <th>Series</th>
                            <th>Number</th>
                            <th>File Path</th>
                            <th>Reason</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${unmatched.map(u => `
                            <tr>
                                <td>${this.escapeHtml(u.series || '')}</td>
                                <td>${this.escapeHtml(u.number || '')}</td>
                                <td class="komga-unmatched-path">${this.escapeHtml(u.file_path || '')}</td>
                                <td>${this.escapeHtml(u.reason || '')}</td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </details>
        `;
    }

    formatTimestamp(timestamp) {
        if (!timestamp) return 'never';
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;

        const seconds = Math.floor(diff / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

        if (days > 7) return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
        if (days >= 1) return `${days} day${days !== 1 ? 's' : ''} ago`;
        if (hours >= 1) return `${hours} hour${hours !== 1 ? 's' : ''} ago`;
        if (minutes >= 1) return `${minutes} minute${minutes !== 1 ? 's' : ''} ago`;
        return 'just now';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
