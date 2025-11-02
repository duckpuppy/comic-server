// Simple client-side router using History API
class Router {
    constructor() {
        this.routes = new Map();
        this.currentPath = window.location.pathname;

        // Handle back/forward navigation
        window.addEventListener('popstate', () => this.handleRoute());

        // Handle initial load
        document.addEventListener('DOMContentLoaded', () => this.handleRoute());
    }

    /**
     * Register a route with a handler function
     * @param {string} pattern - Route pattern (e.g., '/lists/:id')
     * @param {Function} handler - Function to call when route matches
     */
    register(pattern, handler) {
        const regex = this.patternToRegex(pattern);
        this.routes.set(pattern, { regex, handler, pattern });
    }

    /**
     * Convert route pattern to regex with named groups
     * @param {string} pattern - Route pattern
     * @returns {RegExp} Regular expression for matching
     */
    patternToRegex(pattern) {
        // Convert :param to named capture group
        const regexPattern = pattern.replace(/:(\w+)/g, '(?<$1>[^/]+)');
        return new RegExp(`^${regexPattern}$`);
    }

    /**
     * Navigate to a new path
     * @param {string} path - Path to navigate to
     */
    navigate(path) {
        if (path === this.currentPath) return;

        this.currentPath = path;
        window.history.pushState({}, '', path);
        this.handleRoute();
    }

    /**
     * Handle current route
     */
    handleRoute() {
        const path = window.location.pathname;
        this.currentPath = path;

        // Find matching route
        for (const [pattern, { regex, handler }] of this.routes) {
            const match = path.match(regex);
            if (match) {
                // Extract params from named groups
                const params = match.groups || {};
                handler(params);
                return;
            }
        }

        // No route matched - 404
        this.handle404();
    }

    /**
     * Default 404 handler
     */
    handle404() {
        console.warn('No route matched:', this.currentPath);
        document.getElementById('app').innerHTML = `
            <div class="error-page">
                <h1>404 - Page Not Found</h1>
                <p>The page "${this.currentPath}" does not exist.</p>
                <a href="/">Return to Dashboard</a>
            </div>
        `;
    }
}

// Global router instance
const router = new Router();
