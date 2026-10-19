# Web UI

Web UI structure, routes, navigation and WebSocket events.

Moved from `CLAUDE.md`. See the index there for the other developer reference files.

## Web UI (v0.8)

**Access**: http://localhost:7620/ (served by the REST API server)

**Technology Stack**:
- Vanilla JavaScript (no framework dependencies)
- Client-side routing using History API
- WebSocket for real-time updates
- Go's embed package for static file serving
- Responsive CSS Grid/Flexbox layout

**Architecture** (`internal/api/web/`):
- `index.html` - Main dashboard structure with templates
- `css/style.css` - Modern, responsive styling with CSS custom properties
- `css/navigation.css` - Navigation tab styling
- `css/lists.css` - Smart lists browser and detail page styling
- `css/lists-tree.css` - Tree sidebar navigation styling
- `css/deviceDetail.css` - Device detail page styling
- `css/devicesBrowser.css` - Devices browser page styling
- `js/router.js` - Client-side router (History API, no framework)
- `js/navigation.js` - Navigation component with tabs
- `js/websocket.js` - WebSocket client with automatic reconnection
- `js/devices.js` - Device management and registration controls
- `js/deviceDetail.js` - Device detail page with sync history
- `js/devicesBrowser.js` - Devices browser page with filtering
- `js/sync.js` - Sync progress monitoring with progress bars
- `js/syncHistoryBrowser.js` - Sync history browser with pagination
- `js/listsBrowser.js` - Smart lists browser with search/filter
- `js/listDetail.js` - List detail page with preview
- `js/listsTree.js` - Tree sidebar navigation for smart lists
- `js/app.js` - Application initialization and route registration

**Client-Side Routing**:

The UI now features multi-page navigation with the following routes:

- `/` - Dashboard (overview with stats, devices, sync history)
- `/lists` - Smart lists browser with search and filtering
- `/lists/:listId` - List detail page with matchers, devices, and comic preview (with tree sidebar)
- `/devices` - Devices browser page with filtering and search
- `/devices/:deviceId` - Device detail page with sync history
- `/sync` - Sync history browser with pagination
- `/books/:bookId` - Book detail page (comic-server-br14): record, file, stage, attention flags, ComicVine links, this book's audit history, and an on-demand compare of the file's ComicInfo.xml with what Write Metadata would write (`GET /api/library/books/:id` and `/:id/comicinfo`; `Pages` skipped, empty == absent). Actions (comic-server-2s4f): Re-scrape (`POST /api/scrape {book_ids}`), Write metadata to file (`POST .../workflow/write-metadata {book_ids}` writes those CBZs even when not stale; no body = every stale book), Variant covers (`/variant-covers?book=`), stage rollback (stages before the current one). `BookDetail.actions` carries a `*_blocker` reason per action so a disabled button says why. Characters, teams and the main character on the book page are chips linking to Browse with a `Characters`/`Teams` (list membership) or `MainCharacterOrTeam` (equals) matcher in the `m` URL param (comic-server-n2yb). In-app links use `<a href=... data-route>` (one delegated click handler in app.js); Browse, list cards, workflow drill-ins, Duplicates and History link here.

**Routing Features**:
- Browser back/forward button support (popstate events)
- URL updates without page reload (pushState)
- Bookmarkable URLs for direct access
- Parameterized routes (e.g., `/lists/:listId`, `/devices/:deviceId`)
- Automatic route handling on page load

**Tree Sidebar Navigation**:

- **Persistent sidebar** on list detail pages showing library structure
- **Hierarchical display** of smart lists organized in folders
- **Collapsible folders** to navigate nested list structures
- **Active list highlighting** shows current list in the tree
- **State persistence** maintains expanded/collapsed folders across navigation
- **Click to navigate** instantly switch between lists without leaving detail view
- **Responsive design** toggles with hamburger menu on mobile

**Smart Lists as First-Class Entities**:

- **Lists Browser** (`/lists`):
  - Grid view of all smart lists
  - Search by list name (debounced 300ms)
  - Sort by name (A-Z, Z-A) or book count
  - Shows book count and matcher count per list
  - Click card to navigate to detail page

- **List Detail Page** (`/lists/:listId`):
  - Breadcrumb navigation
  - Human-readable matcher display
  - Device assignments section
  - Paginated comic preview (20 per page, max 100)
  - "Load More" button for additional previews
  - Navigate to assigned devices

**Performance Optimizations**:
- List count caching (15 min TTL) for large libraries (65K+ comics)
- Paginated comic previews to avoid loading entire lists
- Debounced search input (300ms delay)
- Cached list evaluation results

**Features**:
- **Real-time Dashboard** - Live server statistics (devices, syncs, uptime, WebSocket clients)
- **Navigation** - five top entries (comic-server-gkzt): Dashboard, Library ▾, Pipeline ▾, Devices ▾, Settings. Groups are defined in `Navigation.ENTRIES` (`js/navigation.js`); a group button is active when one of its pages is, and its badge sums the items marked `alert` (Scrape Review, Trash). Icons only on mobile. The bar does not scroll (overflow would clip the menus) - add a page to a group, not a new top entry.
  - Data Manager rules editor (`/datamanager/rules`) is full width; each tree row is one line, with Rename/Move/Delete (and New ruleset/subfolder on folders) in a "⋯" row menu
- **Device Sidebar** - Shows all discovered devices with status indicators:
  - Connected (< 2 min since last_seen)
  - Idle (2-30 min)
  - Offline (> 30 min)
  - Syncing (during active sync)
- **Device Management**:
  - Register button for unregistered discovered devices
  - Unregister button with confirmation dialog
  - Device info: model, IP address, last seen timestamp
- **Sync Progress Monitoring**:
  - Active syncs with real-time progress bars
  - Current file being transferred
  - File counts (completed/total)
  - Data transferred and transfer speed
- **Sync History**:
  - Recent sync operations (last 10 by default)
  - File statistics (added/updated/deleted)
  - Duration and completion status
  - Timestamp with relative time display

**WebSocket Events** (`internal/websocket/`):
- `device_discovered` - New device found on network
- `device_connected` - Device connected to server
- `device_disconnected` - Device disconnected
- `device_registered` - Device added to registry
- `device_unregistered` - Device removed from registry
- `sync_started` - Sync operation began
- `sync_progress` - Progress update during sync
- `sync_completed` - Sync finished successfully
- `sync_failed` - Sync encountered error

**Browser Compatibility**:
- Modern browsers with WebSocket support
- ES6+ JavaScript features (classes, async/await, template literals)
- CSS Grid and Flexbox required
- History API support required
- Named capture groups in regex (ES2018)
- Tested: Chrome, Firefox, Safari (latest versions)

**Static File Serving**:
- Files embedded in binary using `//go:embed web` directive
- No external dependencies for deployment
- Served from root path `/` by API server
- Automatic content-type detection
