// Navigation component for top-level tabs
class Navigation {
    constructor() {
        this.currentTab = 'dashboard';
    }

    init() {
        this.render();
        this.attachListeners();
    }

    render() {
        const nav = document.getElementById('main-nav');
        if (!nav) return;

        nav.innerHTML = `<div class="nav-tabs">${Navigation.ENTRIES.map(e => e.items ? this.renderGroup(e) : this.renderTab(e, 'nav-tab')).join('')}</div>`;
    }

    renderTab(t, cls) {
        const badge = t.badge ? `<span class="nav-badge" id="${t.tab}-count">0</span>` : '';
        const role = cls === 'nav-item' ? ' role="menuitem"' : '';
        const alert = t.alert ? ' data-alert' : '';
        return `
                <a href="${t.href}" class="${cls}" data-tab="${t.tab}"${role}${alert}>
                    <span class="nav-icon">${t.icon}</span>
                    <span class="nav-label">${t.label}</span>
                    ${badge}
                </a>`;
    }

    // renderGroup draws one dropdown (comic-server-gkzt). The group badge
    // sums its items' alert badges (Scrape Review, Trash) so a hidden count
    // still shows on the bar.
    renderGroup(g) {
        return `
                <div class="nav-group" data-group="${g.group}">
                    <button type="button" class="nav-tab nav-group-btn" aria-haspopup="menu" aria-expanded="false">
                        <span class="nav-icon">${g.icon}</span>
                        <span class="nav-label">${g.label}</span>
                        <span class="nav-badge nav-group-badge"></span>
                        <span class="nav-caret" aria-hidden="true">▾</span>
                    </button>
                    <div class="nav-menu" role="menu" hidden>
                        ${g.items.map(t => this.renderTab(t, 'nav-item')).join('')}
                    </div>
                </div>`;
    }

    closeMenus(except) {
        document.querySelectorAll('.nav-group').forEach(g => {
            if (g === except) return;
            g.querySelector('.nav-menu').hidden = true;
            g.querySelector('.nav-group-btn').setAttribute('aria-expanded', 'false');
        });
    }

    attachListeners() {
        document.querySelectorAll('#main-nav a[data-tab]').forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                this.closeMenus();
                router.navigate(tab.getAttribute('href'));
                this.setActive(tab.dataset.tab);
            });
        });
        document.querySelectorAll('.nav-group').forEach(g => {
            const btn = g.querySelector('.nav-group-btn');
            const menu = g.querySelector('.nav-menu');
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const open = menu.hidden;
                this.closeMenus(g);
                menu.hidden = !open;
                btn.setAttribute('aria-expanded', String(open));
            });
        });
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.nav-group')) this.closeMenus();
        });
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') this.closeMenus();
        });
    }

    setActive(tabName) {
        this.currentTab = tabName;

        document.querySelectorAll('#main-nav a[data-tab]').forEach(tab => {
            tab.classList.toggle('active', tab.dataset.tab === tabName);
        });
        document.querySelectorAll('.nav-group').forEach(g => {
            g.querySelector('.nav-group-btn').classList.toggle('active', !!g.querySelector(`a[data-tab="${tabName}"]`));
        });
    }

    updateBadge(tabName, count) {
        const badge = document.getElementById(`${tabName}-count`);
        if (badge) {
            badge.textContent = count;
            badge.style.display = count > 0 ? 'inline-block' : 'none';
        }
        document.querySelectorAll('.nav-group').forEach(g => {
            let sum = 0;
            g.querySelectorAll('a[data-alert] .nav-badge').forEach(b => { sum += parseInt(b.textContent, 10) || 0; });
            const gb = g.querySelector('.nav-group-badge');
            gb.textContent = sum;
            gb.style.display = sum > 0 ? 'inline-block' : 'none';
        });
    }
}

// Global navigation instance
// Top-level nav (comic-server-gkzt): five entries instead of fourteen
// tabs, so the bar fits without scrolling. alert: the item's badge also
// counts toward its group's badge.
Navigation.ENTRIES = [
    { tab: 'dashboard', href: '/', icon: '📊', label: 'Dashboard' },
    { group: 'library', icon: '📚', label: 'Library', items: [
        { tab: 'lists', href: '/lists', icon: '📚', label: 'Lists', badge: true },
        { tab: 'browse', href: '/browse', icon: '🔍', label: 'Browse' },
        { tab: 'missing-issues', href: '/missing-issues', icon: '🕳️', label: 'Missing Issues' },
        { tab: 'variant-covers', href: '/variant-covers', icon: '🖼️', label: 'Variant Covers' },
        { tab: 'duplicates', href: '/duplicates', icon: '👯', label: 'Duplicates' },
    ] },
    { group: 'pipeline', icon: '🧭', label: 'Pipeline', items: [
        { tab: 'workflow', href: '/workflow', icon: '🧭', label: 'Workflow' },
        { tab: 'datamanager', href: '/datamanager', icon: '🛠️', label: 'Data Manager' },
        { tab: 'scrape-review', href: '/scrape-review', icon: '🧩', label: 'Scrape Review', badge: true, alert: true },
        { tab: 'history', href: '/history', icon: '📜', label: 'History' },
        { tab: 'trash', href: '/trash', icon: '🗑️', label: 'Trash', badge: true, alert: true },
    ] },
    { group: 'devices', icon: '📱', label: 'Devices', items: [
        { tab: 'devices', href: '/devices', icon: '📱', label: 'Devices', badge: true },
        { tab: 'sync', href: '/sync', icon: '🔄', label: 'Sync History' },
    ] },
    { tab: 'settings', href: '/settings', icon: '⚙️', label: 'Settings' },
];

const navigation = new Navigation();
