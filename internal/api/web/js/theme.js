// Dark mode (comic-server-8qk). Two controls:
//   - Settings page: a server-side default (light/dark/system), stored
//     in config.db via GET/PUT /api/settings/theme.
//   - Header toggle: flips the CURRENT WINDOW's theme only. Stored in
//     sessionStorage (never localStorage) so a brand-new window/tab -
//     which gets its own, empty sessionStorage - always starts from the
//     server-side default instead of inheriting whatever a different,
//     already-open window was toggled to.
//
// A tiny synchronous copy of the override-read logic also runs inline in
// index.html's <head>, before this file (and the rest of the page) even
// loads, so a same-window reload/navigation applies the override
// immediately rather than flashing the wrong theme for a moment. This
// file's job is everything that inline snippet can't do without an async
// fetch: resolving the server-side default on a window's first load, and
// wiring up the header toggle button.
const THEME_OVERRIDE_KEY = 'comic-server-theme-override';

const themeManager = {
    // effective() returns what's actually showing right now: an explicit
    // override/default ("light"/"dark") if one is set via data-theme, or
    // "system" if none is - meaning the prefers-color-scheme media query
    // is what's actually deciding.
    effective() {
        const attr = document.documentElement.getAttribute('data-theme');
        return attr === 'light' || attr === 'dark' ? attr : 'system';
    },

    // resolvedForToggle() is effective(), but with "system" resolved to
    // whichever of light/dark the OS/browser is actually rendering right
    // now - the toggle needs a concrete light/dark to flip, not "system"
    // itself (there's no "opposite of system").
    resolvedForToggle() {
        const eff = this.effective();
        if (eff !== 'system') return eff;
        return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    },

    applyOverride(theme) {
        if (theme === 'light' || theme === 'dark') {
            document.documentElement.setAttribute('data-theme', theme);
        } else {
            document.documentElement.removeAttribute('data-theme');
        }
    },

    toggle() {
        const next = this.resolvedForToggle() === 'dark' ? 'light' : 'dark';
        this.applyOverride(next);
        try {
            sessionStorage.setItem(THEME_OVERRIDE_KEY, next);
        } catch (e) {
            // Private-browsing/storage-disabled: the toggle still works
            // for this page view, it just won't survive a reload.
        }
        this.updateToggleButton();
    },

    updateToggleButton() {
        const btn = document.getElementById('theme-toggle-btn');
        if (!btn) return;
        const isDark = this.resolvedForToggle() === 'dark';
        btn.textContent = isDark ? '☀️' : '🌙';
        btn.title = isDark ? 'Switch to light mode (this window only)' : 'Switch to dark mode (this window only)';
    },

    // init resolves the server-side default for a window that has no
    // per-window override yet, then wires up the toggle button. Safe to
    // call once per page load (app.js calls this at startup).
    async init() {
        try {
            const hasOverride = sessionStorage.getItem(THEME_OVERRIDE_KEY) !== null;
            if (!hasOverride) {
                const response = await fetch('/api/settings/theme');
                if (response.ok) {
                    const data = await response.json();
                    this.applyOverride(data.theme);
                }
            }
        } catch (e) {
            console.error('Failed to load theme setting:', e);
        }
        this.updateToggleButton();

        const btn = document.getElementById('theme-toggle-btn');
        if (btn) btn.addEventListener('click', () => this.toggle());
    },
};
