#!/usr/bin/env node
// Simple test to verify router.js syntax and structure

const fs = require('fs');

console.log('=== Router.js Validation Tests ===\n');

// Test 1: File exists and is readable
console.log('Test 1: File exists and is readable');
try {
    const routerCode = fs.readFileSync('./router.js', 'utf8');
    console.log(`✓ router.js loaded (${routerCode.length} bytes)\n`);
} catch (err) {
    console.log(`✗ Failed to read router.js: ${err.message}\n`);
    process.exit(1);
}

// Test 2: Check for required class definition
console.log('Test 2: Check for Router class definition');
const routerCode = fs.readFileSync('./router.js', 'utf8');
if (routerCode.includes('class Router')) {
    console.log('✓ Router class found\n');
} else {
    console.log('✗ Router class not found\n');
    process.exit(1);
}

// Test 3: Check for required methods
console.log('Test 3: Check for required methods');
const requiredMethods = [
    'constructor',
    'register',
    'patternToRegex',
    'navigate',
    'handleRoute',
    'handle404'
];

let allMethodsPresent = true;
for (const method of requiredMethods) {
    if (routerCode.includes(method)) {
        console.log(`  ✓ ${method} method found`);
    } else {
        console.log(`  ✗ ${method} method missing`);
        allMethodsPresent = false;
    }
}
console.log();

if (!allMethodsPresent) {
    process.exit(1);
}

// Test 4: Check for History API usage
console.log('Test 4: Check for History API usage');
if (routerCode.includes('window.history.pushState')) {
    console.log('✓ Uses History API (pushState)\n');
} else {
    console.log('✗ History API usage not found\n');
    process.exit(1);
}

// Test 5: Check for popstate listener
console.log('Test 5: Check for popstate event listener');
if (routerCode.includes('popstate')) {
    console.log('✓ Popstate event listener found\n');
} else {
    console.log('✗ Popstate listener not found\n');
    process.exit(1);
}

// Test 6: Check for named capture groups in regex
console.log('Test 6: Check for named capture groups in regex conversion');
if (routerCode.includes('?<') || routerCode.includes('match.groups')) {
    console.log('✓ Named capture groups used\n');
} else {
    console.log('✗ Named capture groups not found\n');
    process.exit(1);
}

// Test 7: Check for global router instance
console.log('Test 7: Check for global router instance');
if (routerCode.includes('const router = new Router')) {
    console.log('✓ Global router instance created\n');
} else {
    console.log('✗ Global router instance not found\n');
    process.exit(1);
}

// Test 8: Pattern to regex conversion logic check
console.log('Test 8: Pattern to regex conversion logic');
if (routerCode.includes(':param') || routerCode.includes(':(\\w+)')) {
    console.log('✓ Pattern parameter matching logic found\n');
} else {
    console.log('✗ Pattern parameter matching not found\n');
    process.exit(1);
}

console.log('=== All Validation Tests Passed! ===\n');
console.log('Router Implementation Summary:');
console.log('- Uses History API for navigation');
console.log('- Supports parameterized routes (e.g., /lists/:id)');
console.log('- Handles browser back/forward buttons (popstate)');
console.log('- Includes 404 error handling');
console.log('- Creates global router instance');
console.log('\nNext Steps:');
console.log('1. Test manually: open http://localhost:8888/router_test.html in browser');
console.log('2. Verify navigation works correctly');
console.log('3. Test browser back/forward buttons');
console.log('4. Test page refresh maintains route');

process.exit(0);
