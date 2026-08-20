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

        nav.innerHTML = `
            <div class="nav-tabs">
                <a href="/" class="nav-tab" data-tab="dashboard">
                    <span class="nav-icon">📊</span>
                    <span class="nav-label">Dashboard</span>
                </a>
                <a href="/lists" class="nav-tab" data-tab="lists">
                    <span class="nav-icon">📚</span>
                    <span class="nav-label">Lists</span>
                    <span class="nav-badge" id="lists-count">0</span>
                </a>
                <a href="/devices" class="nav-tab" data-tab="devices">
                    <span class="nav-icon">📱</span>
                    <span class="nav-label">Devices</span>
                    <span class="nav-badge" id="devices-count">0</span>
                </a>
                <a href="/sync" class="nav-tab" data-tab="sync">
                    <span class="nav-icon">🔄</span>
                    <span class="nav-label">Sync History</span>
                </a>
                <a href="/komga" class="nav-tab" data-tab="komga">
                    <span class="nav-icon">📖</span>
                    <span class="nav-label">Komga</span>
                </a>
            </div>
        `;
    }

    attachListeners() {
        const tabs = document.querySelectorAll('.nav-tab');
        tabs.forEach(tab => {
            tab.addEventListener('click', (e) => {
                e.preventDefault();
                const path = tab.getAttribute('href');
                router.navigate(path);
                this.setActive(tab.dataset.tab);
            });
        });
    }

    setActive(tabName) {
        this.currentTab = tabName;

        // Update active state
        document.querySelectorAll('.nav-tab').forEach(tab => {
            if (tab.dataset.tab === tabName) {
                tab.classList.add('active');
            } else {
                tab.classList.remove('active');
            }
        });
    }

    updateBadge(tabName, count) {
        const badge = document.getElementById(`${tabName}-count`);
        if (badge) {
            badge.textContent = count;
            badge.style.display = count > 0 ? 'inline-block' : 'none';
        }
    }
}

// Global navigation instance
const navigation = new Navigation();
