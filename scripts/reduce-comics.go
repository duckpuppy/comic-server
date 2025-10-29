// reduce-comics.go - Reduce CBZ file sizes by keeping only first N pages
//
// Usage: go run scripts/reduce-comics.go <pages-to-keep>
//
// This script:
// 1. Finds all .cbz files in testdata/real-comics/
// 2. Reduces each to first N pages (default: 3)
// 3. Updates page counts in testdata/ComicDB.xml
// 4. Creates backups before modification

package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultPages   = 3
	comicsDir      = "testdata/real-comics"
	libraryPath    = "testdata/ComicDB.xml"
	backupSuffix   = ".backup"
	maxFileSizeMB  = 5 // Target max size per file
)

// Library XML structures (minimal - only what we need to update)
type Library struct {
	XMLName xml.Name `xml:"Books"`
	Books   []Book   `xml:"Book"`
}

type Book struct {
	File      string `xml:"File,attr"`
	PageCount int    `xml:"PageCount,attr"`
	// Preserve all other attributes as raw XML
	InnerXML string `xml:",innerxml"`
}

func main() {
	// Get pages to keep from command line or use default
	pagesToKeep := defaultPages
	if len(os.Args) > 1 {
		if n, err := strconv.Atoi(os.Args[1]); err == nil && n > 0 {
			pagesToKeep = n
		}
	}

	fmt.Printf("Reducing comic files to %d pages each...\n", pagesToKeep)

	// Find all CBZ files
	files, err := findCBZFiles(comicsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding CBZ files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("No CBZ files found in", comicsDir)
		return
	}

	fmt.Printf("Found %d CBZ files to process\n", len(files))

	// Track file updates for library XML
	updates := make(map[string]int) // file path -> new page count

	// Process each file
	for _, file := range files {
		fmt.Printf("\nProcessing: %s\n", filepath.Base(file))

		newPages, err := reduceCBZ(file, pagesToKeep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		updates[file] = newPages
		fmt.Printf("  Reduced to %d pages\n", newPages)

		// Show new size
		if info, err := os.Stat(file); err == nil {
			sizeMB := float64(info.Size()) / 1024 / 1024
			fmt.Printf("  New size: %.2f MB\n", sizeMB)
		}
	}

	// Update library XML
	if len(updates) > 0 {
		fmt.Printf("\nUpdating %s with new page counts...\n", libraryPath)
		if err := updateLibraryXML(libraryPath, updates); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating library: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Library updated successfully!")
	}

	fmt.Println("\nDone! All comic files reduced.")
	fmt.Printf("Backup files created with suffix '%s'\n", backupSuffix)
}

// findCBZFiles finds all .cbz files in the given directory
func findCBZFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".cbz") {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// reduceCBZ reduces a CBZ file to keep only first N pages
func reduceCBZ(cbzPath string, pagesToKeep int) (int, error) {
	// Create backup
	backupPath := cbzPath + backupSuffix
	if err := copyFile(cbzPath, backupPath); err != nil {
		return 0, fmt.Errorf("failed to create backup: %w", err)
	}

	// Open original CBZ (which is a ZIP file)
	reader, err := zip.OpenReader(cbzPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer reader.Close()

	// Find image files (pages)
	imageFiles := findImageFiles(reader.File)
	if len(imageFiles) == 0 {
		return 0, fmt.Errorf("no image files found in CBZ")
	}

	// Sort by name to maintain page order
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	// Determine how many pages to keep
	actualPages := pagesToKeep
	if actualPages > len(imageFiles) {
		actualPages = len(imageFiles)
	}

	// Create new CBZ with only first N pages
	tempPath := cbzPath + ".tmp"
	if err := createReducedCBZ(tempPath, imageFiles[:actualPages]); err != nil {
		return 0, fmt.Errorf("failed to create reduced CBZ: %w", err)
	}

	// Replace original with reduced version
	if err := os.Rename(tempPath, cbzPath); err != nil {
		os.Remove(tempPath)
		return 0, fmt.Errorf("failed to replace original: %w", err)
	}

	return actualPages, nil
}

// findImageFiles filters ZIP entries for image files
func findImageFiles(files []*zip.File) []*zip.File {
	var images []*zip.File

	imageExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".gif": true, ".bmp": true, ".webp": true,
	}

	for _, f := range files {
		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Check extension
		ext := strings.ToLower(filepath.Ext(f.Name))
		if imageExts[ext] {
			images = append(images, f)
		}
	}

	return images
}

// createReducedCBZ creates a new CBZ with only specified files
func createReducedCBZ(outputPath string, files []*zip.File) error {
	// Create new ZIP file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	writer := zip.NewWriter(outFile)
	defer writer.Close()

	// Copy each file
	for _, file := range files {
		if err := copyZipFile(writer, file); err != nil {
			return fmt.Errorf("failed to copy %s: %w", file.Name, err)
		}
	}

	return nil
}

// copyZipFile copies a file from one ZIP to another
func copyZipFile(writer *zip.Writer, file *zip.File) error {
	// Open source file
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// Create destination file
	dst, err := writer.Create(file.Name)
	if err != nil {
		return err
	}

	// Copy content
	_, err = io.Copy(dst, src)
	return err
}

// copyFile copies a file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// updateLibraryXML updates page counts in the library XML file
func updateLibraryXML(libPath string, updates map[string]int) error {
	// Create backup
	backupPath := libPath + backupSuffix
	if err := copyFile(libPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Read library XML
	data, err := os.ReadFile(libPath)
	if err != nil {
		return fmt.Errorf("failed to read library: %w", err)
	}

	// Parse XML
	var library Library
	if err := xml.Unmarshal(data, &library); err != nil {
		return fmt.Errorf("failed to parse XML: %w", err)
	}

	// Update page counts
	updatedCount := 0
	for i := range library.Books {
		book := &library.Books[i]

		// Check if this book's file was updated
		for updatePath, newPages := range updates {
			// Match by filename (books use absolute paths)
			if strings.Contains(book.File, filepath.Base(updatePath)) {
				fmt.Printf("  Updating %s: %d -> %d pages\n",
					filepath.Base(book.File), book.PageCount, newPages)
				book.PageCount = newPages
				updatedCount++
				break
			}
		}
	}

	if updatedCount == 0 {
		fmt.Println("  Warning: No matching books found in library to update")
	} else {
		fmt.Printf("  Updated %d book(s) in library\n", updatedCount)
	}

	// Write updated XML
	output, err := xml.MarshalIndent(library, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal XML: %w", err)
	}

	// Write back to file
	if err := os.WriteFile(libPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write library: %w", err)
	}

	return nil
}
