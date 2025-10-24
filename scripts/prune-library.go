package main

import (
	"encoding/xml"
	"fmt"
	"os"
)

// Simplified structures for pruning
type ComicDatabase struct {
	XMLName    xml.Name   `xml:"ComicDatabase"`
	ID         string     `xml:"Id,attr"`
	Books      Books      `xml:"Books"`
	ComicLists ComicLists `xml:"ComicLists"`
}

type Books struct {
	Book []RawMessage `xml:"Book"`
}

type ComicLists struct {
	Items []RawMessage `xml:"Item"`
}

// RawMessage is a simplified version of xml.RawMessage for raw XML preservation
type RawMessage []byte

func (r *RawMessage) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var content []byte
	if err := d.DecodeElement(&content, &start); err != nil {
		return err
	}
	*r = content
	return nil
}

func (r RawMessage) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement([]byte(r), start)
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input.xml> <output.xml> <num_books>\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := os.Args[1]
	outputPath := os.Args[2]
	var numBooks int
	fmt.Sscanf(os.Args[3], "%d", &numBooks)

	// Read input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse XML
	var db ComicDatabase
	if err := xml.Unmarshal(data, &db); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing XML: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Original library: %d books, %d comic lists\n", len(db.Books.Book), len(db.ComicLists.Items))

	// Keep only first N books
	if len(db.Books.Book) > numBooks {
		db.Books.Book = db.Books.Book[:numBooks]
	}

	// Keep only first 10 comic lists
	if len(db.ComicLists.Items) > 10 {
		db.ComicLists.Items = db.ComicLists.Items[:10]
	}

	fmt.Printf("Pruned library: %d books, %d comic lists\n", len(db.Books.Book), len(db.ComicLists.Items))

	// Marshal back to XML
	output, err := xml.MarshalIndent(&db, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling XML: %v\n", err)
		os.Exit(1)
	}

	// Write output file with XML declaration
	outFile, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	outFile.WriteString(`<?xml version="1.0"?>` + "\n")
	outFile.Write(output)

	// Show file sizes
	inStat, _ := os.Stat(inputPath)
	outStat, _ := os.Stat(outputPath)
	fmt.Printf("\nFile size: %d MB -> %d KB\n", inStat.Size()/1024/1024, outStat.Size()/1024)
	fmt.Printf("Reduction: %.1f%%\n", (1.0-float64(outStat.Size())/float64(inStat.Size()))*100)
}
