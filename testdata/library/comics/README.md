# Test Comic Files

This directory should contain 3-5 small comic book files for testing sync functionality.

## Setup

Place 3-5 comic files (CBZ, CBR, etc.) in this directory. Recommended sources:

### Public Domain Comics

**Comic Book Plus** (requires free registration):
- https://comicbookplus.com/
- Golden Age comics (1930s-1950s)
- Public domain due to expired/unrenewed copyrights

**Digital Comic Museum**:
- https://digitalcomicmuseum.com/
- Public domain Golden Age comics
- Free registration required

**Internet Archive**:
- https://archive.org/details/comics
- Large collection of public domain comics
- Direct downloads available

### For Development/Testing Only

You can also use any comic files you own for local testing (not committed to git).

## File Naming

The ComicDb.xml references these filenames:
- `league-001.cbz` - The League of Extraordinary Gentlemen: Century #1
- `league-002.cbz` - The League of Extraordinary Gentlemen: Century #2
- `kids-club-002.cbz` - Top Shelf Kids Club #2

If you use different comics, update the `File` attributes in `../ComicDb.xml` to match your filenames.

## Ideal Comics for Testing

Look for:
- **Small file sizes** (< 10MB each)
- **Low page counts** (20-50 pages)
- **Diverse metadata** (different publishers, years, genres)
- **Public domain** (if committing to git)

## Note

Comic files are excluded from git via `.gitignore` to avoid copyright issues.
