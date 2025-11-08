#!/usr/bin/env node
// Automated test script for router.js
// This tests the core router logic without requiring a browser

// Mock browser APIs
global.window = {
    location: { pathname: '/' },
    history: {
        pushState: function(state, title, path) {
            console.log(`✓ pushState called: ${path}`);
            global.window.location.pathname = path;
        }
    },
    addEventListener: function() {}
};

global.document = {
    addEventListener: function() {},
    getElementById: function(id) {
        return {
            innerHTML: ''
        };
    }
};

// Load router code
const fs = require('fs');
const routerCode = fs.readFileSync('./router.js', 'utf8');

// Create a class context for evaluation
let Router, router;
eval(routerCode);
// The eval should have defined Router class and instantiated router

console.log('=== Router Tests ===\n');

// Verify router is available
if (typeof router === 'undefined') {
    console.log('✗ Router not instantiated');
    process.exit(1);
}

// Test 1: Pattern to Regex conversion
console.log('Test 1: Pattern to Regex conversion');
const pattern1 = '/lists/:listId';
const regex1 = router.patternToRegex(pattern1);
console.log(`Pattern: ${pattern1}`);
console.log(`Regex: ${regex1}`);
const match1 = '/lists/123'.match(regex1);
if (match1 && match1.groups.listId === '123') {
    console.log('✓ Successfully extracted listId: 123\n');
} else {
    console.log('✗ Failed to extract listId\n');
    process.exit(1);
}

// Test 2: Register routes
console.log('Test 2: Register routes');
let routesCalled = [];
router.register('/', () => {
    routesCalled.push('home');
    console.log('✓ Home route called');
});
router.register('/lists', () => {
    routesCalled.push('lists');
    console.log('✓ Lists route called');
});
router.register('/lists/:listId', (params) => {
    routesCalled.push(`list-${params.listId}`);
    console.log(`✓ List detail route called with ID: ${params.listId}`);
});
router.register('/devices/:deviceId', (params) => {
    routesCalled.push(`device-${params.deviceId}`);
    console.log(`✓ Device detail route called with ID: ${params.deviceId}`);
});
console.log(`Registered ${router.routes.size} routes\n`);

// Test 3: Navigate to home
console.log('Test 3: Navigate to home (/)');
router.navigate('/');
if (routesCalled[routesCalled.length - 1] === 'home') {
    console.log('✓ Home route triggered\n');
} else {
    console.log('✗ Home route not triggered\n');
    process.exit(1);
}

// Test 4: Navigate to lists
console.log('Test 4: Navigate to lists (/lists)');
router.navigate('/lists');
if (routesCalled[routesCalled.length - 1] === 'lists') {
    console.log('✓ Lists route triggered\n');
} else {
    console.log('✗ Lists route not triggered\n');
    process.exit(1);
}

// Test 5: Navigate to specific list
console.log('Test 5: Navigate to list detail (/lists/abc-123)');
router.navigate('/lists/abc-123');
if (routesCalled[routesCalled.length - 1] === 'list-abc-123') {
    console.log('✓ List detail route triggered with correct ID\n');
} else {
    console.log('✗ List detail route not triggered correctly\n');
    process.exit(1);
}

// Test 6: Navigate to specific device
console.log('Test 6: Navigate to device detail (/devices/xyz-789)');
router.navigate('/devices/xyz-789');
if (routesCalled[routesCalled.length - 1] === 'device-xyz-789') {
    console.log('✓ Device detail route triggered with correct ID\n');
} else {
    console.log('✗ Device detail route not triggered correctly\n');
    process.exit(1);
}

// Test 7: 404 handling
console.log('Test 7: 404 handling (/nonexistent)');
global.document.getElementById = function(id) {
    return {
        innerHTML: '',
        set innerHTML(value) {
            if (value.includes('404 - Page Not Found')) {
                console.log('✓ 404 page rendered correctly\n');
            }
        }
    };
};
router.navigate('/nonexistent');

// Test 8: Same path navigation (should be no-op)
console.log('Test 8: Same path navigation');
const beforeLength = routesCalled.length;
router.navigate('/devices/xyz-789'); // Already on this path
if (routesCalled.length === beforeLength) {
    console.log('✓ Same path navigation was skipped (no duplicate route call)\n');
} else {
    console.log('✗ Same path navigation should not trigger route handler again\n');
    process.exit(1);
}

console.log('=== All Tests Passed! ===');
console.log(`\nRoute call sequence: ${routesCalled.join(' → ')}`);
console.log('\nManual testing required:');
console.log('1. Open router_test.html in a browser');
console.log('2. Test back/forward buttons');
console.log('3. Test page refresh (should maintain route)');
console.log(`4. Server running at: http://localhost:8888/router_test.html`);

process.exit(0);
