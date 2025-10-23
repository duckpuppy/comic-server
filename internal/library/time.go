package library

import (
	"encoding/xml"
	"time"
)

// ComicTime is a custom time type that handles ComicRack's datetime format
type ComicTime struct {
	time.Time
}

// Supported time formats from ComicRack
var comicTimeFormats = []string{
	"2006-01-02T15:04:05.9999999-07:00", // .NET format with timezone
	"2006-01-02T15:04:05.9999999",       // .NET format without timezone
	"2006-01-02T15:04:05-07:00",         // ISO 8601 with timezone
	"2006-01-02T15:04:05",               // ISO 8601 without timezone
	time.RFC3339,                        // Standard RFC3339
}

// UnmarshalXML implements xml.Unmarshaler for ComicTime
func (ct *ComicTime) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	// Empty string means zero time
	if s == "" {
		ct.Time = time.Time{}
		return nil
	}

	// Try each format
	var lastErr error
	for _, format := range comicTimeFormats {
		t, err := time.Parse(format, s)
		if err == nil {
			ct.Time = t
			return nil
		}
		lastErr = err
	}

	// If all formats failed, return the last error
	return lastErr
}

// MarshalXML implements xml.Marshaler for ComicTime
func (ct ComicTime) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if ct.Time.IsZero() {
		return nil // Omit zero times
	}

	// Use .NET format without timezone (ComicRack standard)
	s := ct.Time.Format("2006-01-02T15:04:05")
	return e.EncodeElement(s, start)
}

// IsZero reports whether ct represents the zero time instant
func (ct ComicTime) IsZero() bool {
	return ct.Time.IsZero()
}
