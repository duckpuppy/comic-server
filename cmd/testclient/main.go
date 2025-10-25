package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/duckpuppy/comic-server/internal/library"
	"github.com/duckpuppy/comic-server/internal/protocol"
	"github.com/spf13/cobra"
)

var (
	deviceID     string
	deviceName   string
	storageDir   string
	requestSync  bool
	multicastIP  string
	multicastPort int
	devicePort   int
)

var rootCmd = &cobra.Command{
	Use:   "testclient",
	Short: "Test client that simulates a ComicRack device",
	Long: `A test client that broadcasts device discovery messages and responds to
server sync commands. Useful for testing comic-server without physical devices.`,
	RunE: runClient,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&deviceID, "device-id", "test-device-123", "Device ID")
	rootCmd.Flags().StringVar(&deviceName, "name", "Test Device", "Device name")
	rootCmd.Flags().StringVar(&storageDir, "storage", "./test-storage", "Directory to store synced files")
	rootCmd.Flags().BoolVar(&requestSync, "sync", false, "Request sync on discovery")
	rootCmd.Flags().StringVar(&multicastIP, "multicast-ip", "224.34.123.90", "Multicast group IP")
	rootCmd.Flags().IntVar(&multicastPort, "multicast-port", 7615, "Multicast port")
	rootCmd.Flags().IntVar(&devicePort, "device-port", 7614, "Device TCP port")
}

// createComicRackINI creates a comicrack.ini file with device information
func createComicRackINI() error {
	// Device info
	model := "Test Device Model"
	manufacturer := "Test Manufacturer"
	serial := deviceID
	edition := "Android Full"
	version := "100"

	// Calculate hash: SHA1(Model + Manufacturer + Serial + Edition + Version)
	hashInput := model + manufacturer + serial + edition + version
	hash := sha1.Sum([]byte(hashInput))
	hashStr := hex.EncodeToString(hash[:])

	// Create INI content
	iniContent := fmt.Sprintf(`[Device]
Name=%s
Model=%s
Manufacturer=%s
Serial=%s
ID=%s
Hash=%s
Version=%s
Edition=%s
`, deviceName, model, manufacturer, serial, deviceID, hashStr, version, edition)

	// Write to storage directory
	iniPath := filepath.Join(storageDir, "comicrack.ini")
	return os.WriteFile(iniPath, []byte(iniContent), 0644)
}

func runClient(cmd *cobra.Command, args []string) error {
	// Create storage directory
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create comicrack.ini file for device info
	if err := createComicRackINI(); err != nil {
		return fmt.Errorf("failed to create comicrack.ini: %w", err)
	}

	fmt.Println("🤖 ComicRack Test Client")
	fmt.Printf("   Device ID: %s\n", deviceID)
	fmt.Printf("   Device Name: %s\n", deviceName)
	fmt.Printf("   Storage: %s\n", storageDir)
	fmt.Printf("   Request Sync: %v\n", requestSync)
	fmt.Println()

	// Start TCP server to handle commands
	go startTCPServer()

	// Start broadcasting discovery
	if err := startDiscoveryBroadcast(); err != nil {
		return err
	}

	return nil
}

// startTCPServer starts a TCP server on the device port to handle server commands
func startTCPServer() {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", devicePort))
	if err != nil {
		log.Fatalf("Failed to start TCP server: %v", err)
	}
	defer listener.Close()

	fmt.Printf("📡 Listening for commands on port %d\n", devicePort)
	fmt.Println()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go handleConnection(conn)
	}
}

// handleConnection handles a single TCP connection from the server
func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set timeout
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Read command byte
	cmdByte, err := protocol.ReadByte(conn)
	if err != nil {
		log.Printf("Error reading command: %v", err)
		return
	}

	switch cmdByte {
	case protocol.CommandInfo:
		handleCommandInfo(conn)
	case protocol.CommandStart:
		handleCommandStart(conn)
	case protocol.CommandCompleted:
		handleCommandCompleted(conn)
	case protocol.CommandFreeSpace:
		handleCommandFreeSpace(conn)
	case protocol.CommandFileExists:
		handleCommandFileExists(conn)
	case protocol.CommandListFiles:
		handleCommandListFiles(conn)
	case protocol.CommandReadFile:
		handleCommandReadFile(conn)
	case protocol.CommandWriteFile:
		handleCommandWriteFile(conn)
	case protocol.CommandDeleteFile:
		handleCommandDeleteFile(conn)
	case protocol.CommandProgressUpdate:
		handleCommandProgressUpdate(conn)
	case protocol.CommandCheckAbort:
		handleCommandCheckAbort(conn)
	case protocol.CommandReadMultiFile:
		handleCommandReadMultiFile(conn)
	default:
		log.Printf("Unknown command: %d", cmdByte)
	}
}

// handleCommandInfo returns device information
func handleCommandInfo(conn net.Conn) {
	fmt.Println("📥 CommandInfo - Sending device info")

	// Read protocol version
	_, err := protocol.ReadInt(conn)
	if err != nil {
		log.Printf("Error reading protocol version: %v", err)
		return
	}

	// Write response: BOOL(licensed) + INT(versionCode) + STRING(certificate) + BOOL(flag)
	if err := protocol.WriteBool(conn, true); err != nil {
		log.Printf("Error writing licensed flag: %v", err)
		return
	}

	if err := protocol.WriteInt(conn, 100); err != nil {
		log.Printf("Error writing version code: %v", err)
		return
	}

	if err := protocol.WriteString(conn, ""); err != nil {
		log.Printf("Error writing certificate: %v", err)
		return
	}

	if err := protocol.WriteBool(conn, false); err != nil {
		log.Printf("Error writing flag: %v", err)
		return
	}
}

// handleCommandStart acknowledges sync start
func handleCommandStart(conn net.Conn) {
	fmt.Println("📥 CommandStart - Sync session starting")
	// No response needed
}

// handleCommandCompleted acknowledges sync completion
func handleCommandCompleted(conn net.Conn) {
	fmt.Println("📥 CommandCompleted - Sync session complete")
	// No response needed
}

// handleCommandFreeSpace returns available storage
func handleCommandFreeSpace(conn net.Conn) {
	fmt.Println("📥 CommandFreeSpace - Reporting storage")

	// Return 10GB free space for testing
	freeSpace := int64(10 * 1024 * 1024 * 1024)

	if err := protocol.WriteLong(conn, freeSpace); err != nil {
		log.Printf("Error writing free space: %v", err)
	}
}

// handleCommandFileExists checks if a file exists
func handleCommandFileExists(conn net.Conn) {
	filename, err := protocol.ReadString(conn)
	if err != nil {
		log.Printf("Error reading filename: %v", err)
		return
	}

	fmt.Printf("📥 CommandFileExists - Checking %s\n", filename)

	// Check if file exists in storage
	filePath := filepath.Join(storageDir, filename)
	_, err = os.Stat(filePath)
	exists := err == nil

	if err := protocol.WriteBool(conn, exists); err != nil {
		log.Printf("Error writing exists status: %v", err)
	}

	fmt.Printf("   File exists: %v\n", exists)
}

// handleCommandListFiles returns list of files on device
func handleCommandListFiles(conn net.Conn) {
	fmt.Println("📥 CommandListFiles - Listing files")

	// List all .cbp files in storage directory
	var files []string
	filepath.Walk(storageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(storageDir, path)
			files = append(files, relPath)
		}
		return nil
	})

	// Join with newlines
	fileList := ""
	for i, file := range files {
		if i > 0 {
			fileList += "\n"
		}
		fileList += file
	}

	if err := protocol.WriteString(conn, fileList); err != nil {
		log.Printf("Error writing file list: %v", err)
	}

	fmt.Printf("   Found %d files\n", len(files))
}

// handleCommandReadFile reads a file from storage
func handleCommandReadFile(conn net.Conn) {
	filename, err := protocol.ReadString(conn)
	if err != nil {
		log.Printf("Error reading filename: %v", err)
		return
	}

	fmt.Printf("📥 CommandReadFile - Reading %s\n", filename)

	// Read file from storage
	filePath := filepath.Join(storageDir, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Send empty data if file doesn't exist
		protocol.WriteData(conn, []byte{})
		return
	}

	if err := protocol.WriteData(conn, data); err != nil {
		log.Printf("Error writing file data: %v", err)
	}

	fmt.Printf("   Sent %d bytes\n", len(data))
}

// handleCommandWriteFile writes a file to storage
func handleCommandWriteFile(conn net.Conn) {
	filename, err := protocol.ReadString(conn)
	if err != nil {
		log.Printf("Error reading filename: %v", err)
		return
	}

	data, err := protocol.ReadData(conn)
	if err != nil {
		log.Printf("Error reading file data: %v", err)
		return
	}

	fmt.Printf("📥 CommandWriteFile - Writing %s (%d bytes)\n", filename, len(data))

	// Write file to storage
	filePath := filepath.Join(storageDir, filename)

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		log.Printf("Error creating directory: %v", err)
		return
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		log.Printf("Error writing file: %v", err)
		return
	}

	// If it's a sidecar file, parse and display metadata
	if filepath.Ext(filename) == ".xml" {
		var book library.ComicBook
		if err := xml.Unmarshal(data, &book); err == nil {
			fmt.Printf("   📖 Book: %s", book.Title)
			if book.Series != "" {
				fmt.Printf(" (%s #%s)", book.Series, book.Number)
			}
			fmt.Println()
		}
	} else {
		// It's a comic file
		fmt.Printf("   💾 Comic file saved\n")
	}
}

// handleCommandDeleteFile deletes a file from storage
func handleCommandDeleteFile(conn net.Conn) {
	filename, err := protocol.ReadString(conn)
	if err != nil {
		log.Printf("Error reading filename: %v", err)
		return
	}

	fmt.Printf("📥 CommandDeleteFile - Deleting %s\n", filename)

	filePath := filepath.Join(storageDir, filename)
	if err := os.Remove(filePath); err != nil {
		log.Printf("Error deleting file: %v", err)
	}
}

// handleCommandProgressUpdate receives progress updates
func handleCommandProgressUpdate(conn net.Conn) {
	percent, err := protocol.ReadInt(conn)
	if err != nil {
		log.Printf("Error reading progress: %v", err)
		return
	}

	fmt.Printf("📊 Progress: %d%%\n", percent)
}

// handleCommandCheckAbort returns whether sync should abort
func handleCommandCheckAbort(conn net.Conn) {
	// Never abort for testing
	if err := protocol.WriteBool(conn, false); err != nil {
		log.Printf("Error writing abort status: %v", err)
	}
}

// handleCommandReadMultiFile reads multiple files at once
func handleCommandReadMultiFile(conn net.Conn) {
	// Read file list as a single newline-separated string
	fileList, err := protocol.ReadString(conn)
	if err != nil {
		log.Printf("Error reading file list: %v", err)
		return
	}

	// Parse filenames (newline-separated)
	var filenames []string
	if fileList != "" {
		filenames = strings.Split(fileList, "\n")
	}

	fmt.Printf("📥 CommandReadMultiFile - Reading %d files\n", len(filenames))

	// Send count
	if err := protocol.WriteInt(conn, int32(len(filenames))); err != nil {
		log.Printf("Error writing count: %v", err)
		return
	}

	// Send each file: filename + data
	for _, filename := range filenames {
		if err := protocol.WriteString(conn, filename); err != nil {
			log.Printf("Error writing filename: %v", err)
			return
		}

		filePath := filepath.Join(storageDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Send empty data if file doesn't exist
			protocol.WriteData(conn, []byte{})
			continue
		}

		if err := protocol.WriteData(conn, data); err != nil {
			log.Printf("Error writing file data: %v", err)
			return
		}
	}

	fmt.Printf("   Sent %d files\n", len(filenames))
}

// startDiscoveryBroadcast broadcasts discovery messages via UDP multicast
func startDiscoveryBroadcast() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", multicastIP, multicastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial multicast: %w", err)
	}
	defer conn.Close()

	// Build discovery message
	message := fmt.Sprintf("ComicRackAndroid:%s", deviceID)
	if requestSync {
		message += ":Sync"
	}

	fmt.Printf("📡 Broadcasting discovery: %s\n", message)
	fmt.Println("   (Press Ctrl+C to stop)")
	fmt.Println()

	// Broadcast every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Send immediately
	conn.Write([]byte(message))

	for range ticker.C {
		conn.Write([]byte(message))
	}

	return nil
}
