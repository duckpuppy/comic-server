#!/bin/bash
# Prune ComicDB.xml to a smaller test file

INPUT="${1:-testdata/ComicDB.xml}"
OUTPUT="${2:-testdata/ComicDB-small.xml}"
NUM_BOOKS="${3:-150}"

echo "Pruning $INPUT to $NUM_BOOKS books..."

# Use xmlstarlet or sed to extract just the header, first N books, and footer
# This is a simple approach that keeps the XML structure valid

# Extract everything up to </Book> for the Nth book, then add closing tags

awk -v n="$NUM_BOOKS" '
BEGIN { book_count = 0; in_books = 0; printed_books = 0 }

# Print header until we reach Books section
/<Books>/ { in_books = 1; print; next }

# Count and print books
in_books && /<Book / { book_count++ }

# Print books up to limit
in_books && book_count <= n {
    print
    if (/<\/Book>/) {
        printed_books++
        if (printed_books == n) {
            print "  </Books>"
            print "  <ComicLists>"
            print "  </ComicLists>"
            print "</ComicDatabase>"
            exit
        }
    }
    next
}

# Skip everything after limit in Books section
in_books { next }

# Print everything before Books section
!in_books { print }
' "$INPUT" > "$OUTPUT"

echo "Created $OUTPUT"
ls -lh "$OUTPUT"
echo "Book count: $(grep -c '<Book Id=' "$OUTPUT")"
