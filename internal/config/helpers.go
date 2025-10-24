package config

import (
	"fmt"
	"strings"

	"github.com/duckpuppy/comic-server/internal/library"
)

// ResolveDevice attempts to find a device by name or ID
// Returns the device ID and device config, or an error if:
// - No devices match
// - Multiple devices match (ambiguous)
func ResolveDevice(config *Config, nameOrID string) (string, *DeviceConfig, error) {
	if len(config.Devices) == 0 {
		return "", nil, fmt.Errorf("no devices configured")
	}

	// Try exact ID match first
	if device, ok := config.Devices[nameOrID]; ok {
		return nameOrID, device, nil
	}

	// Try exact friendly name match
	var matches []string
	for id, device := range config.Devices {
		if device.FriendlyName == nameOrID {
			matches = append(matches, id)
		}
	}

	if len(matches) == 1 {
		return matches[0], config.Devices[matches[0]], nil
	}

	if len(matches) > 1 {
		return "", nil, fmt.Errorf("ambiguous device name %q matches multiple devices: %v (use device ID instead)", nameOrID, matches)
	}

	// Try partial name match (case-insensitive)
	searchLower := strings.ToLower(nameOrID)
	matches = nil
	for id, device := range config.Devices {
		if strings.Contains(strings.ToLower(device.FriendlyName), searchLower) {
			matches = append(matches, id)
		}
	}

	if len(matches) == 1 {
		return matches[0], config.Devices[matches[0]], nil
	}

	if len(matches) > 1 {
		// Build list of friendly names for error message
		names := make([]string, len(matches))
		for i, id := range matches {
			names[i] = fmt.Sprintf("%s (%s)", config.Devices[id].FriendlyName, id)
		}
		return "", nil, fmt.Errorf("ambiguous device name %q matches multiple devices:\n  %s\nUse device ID to specify which one", nameOrID, strings.Join(names, "\n  "))
	}

	return "", nil, fmt.Errorf("device %q not found", nameOrID)
}

// ResolveSmartList finds a smart list by name and returns its GUID
// Returns error if list not found or multiple lists match
func ResolveSmartList(lib *library.ComicLibrary, name string) (string, string, error) {
	var matches []library.ComicListItem

	for _, list := range lib.ComicLists {
		// Only consider smart lists
		if !strings.Contains(list.Type, "SmartList") {
			continue
		}

		if list.Name == name {
			matches = append(matches, list)
		}
	}

	if len(matches) == 0 {
		return "", "", fmt.Errorf("smart list %q not found in library", name)
	}

	if len(matches) > 1 {
		return "", "", fmt.Errorf("multiple smart lists named %q found (list names should be unique)", name)
	}

	// Return GUID (use ID field) and name
	return matches[0].ID, matches[0].Name, nil
}

// FindListByGUID finds a smart list in the library by its GUID
// Returns nil if not found
func FindListByGUID(lib *library.ComicLibrary, guid string) *library.ComicListItem {
	for i := range lib.ComicLists {
		list := &lib.ComicLists[i]
		if list.ID == guid && strings.Contains(list.Type, "SmartList") {
			return list
		}
	}
	return nil
}

// FormatDeviceList returns a human-readable list of devices
func FormatDeviceList(devices map[string]*DeviceConfig) string {
	if len(devices) == 0 {
		return "No devices configured"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Configured Devices (%d):\n", len(devices)))

	for id, device := range devices {
		name := device.FriendlyName
		if name == "" {
			name = "(unnamed)"
		}
		builder.WriteString(fmt.Sprintf("  • %s\n", name))
		builder.WriteString(fmt.Sprintf("    ID: %s\n", id))
		builder.WriteString(fmt.Sprintf("    Lists: %d configured\n", len(device.Lists)))
		if !device.LastSeen.IsZero() {
			builder.WriteString(fmt.Sprintf("    Last seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05")))
		}
	}

	return builder.String()
}

// FormatSmartListConfig returns a human-readable representation of a list config
func FormatSmartListConfig(list *SharedListConfig) string {
	var builder strings.Builder

	status := "enabled"
	if !list.Enabled {
		status = "disabled"
	}

	builder.WriteString(fmt.Sprintf("• %s (%s)\n", list.ListName, status))
	builder.WriteString(fmt.Sprintf("  ID: %s\n", list.ListID))

	if list.Settings != nil {
		builder.WriteString("  Settings:\n")
		if list.Settings.OnlyUnread {
			builder.WriteString("    - Only unread books\n")
		}
		if list.Settings.KeepLastRead {
			builder.WriteString("    - Keep last read books\n")
		}
		if list.Settings.OnlyChecked {
			builder.WriteString("    - Only checked books\n")
		}
		if list.Settings.Limit {
			builder.WriteString(fmt.Sprintf("    - Limit: %d %s\n", list.Settings.LimitValue, list.Settings.LimitValueType))
		}
		if list.Settings.Sort {
			builder.WriteString(fmt.Sprintf("    - Sort: %s\n", list.Settings.ListSortType))
		}
	} else {
		builder.WriteString("  Settings: (using device defaults)\n")
	}

	return builder.String()
}
