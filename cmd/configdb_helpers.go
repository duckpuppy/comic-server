package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/duckpuppy/comic-server/internal/configdb"
	"github.com/duckpuppy/comic-server/internal/sync"
)

// openConfigDB opens config.db next to the resolved --config file (same
// rule cmd/server.go uses) - NOT config.GetConfigDir()'s XDG default,
// which ignores --config and would silently open a different config.db
// than the one the server itself is using whenever --config points
// somewhere non-default. Callers are responsible for closing the
// returned DB.
func openConfigDB() (*configdb.DB, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}
	db, err := configdb.Open(filepath.Join(filepath.Dir(configPath), "config.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to open config.db: %w", err)
	}
	return db, nil
}

// resolveConfigDBDevice finds a registered device by exact ID, exact
// friendly name, or case-insensitive partial friendly name - same
// resolution order as the retired config.ResolveDevice.
func resolveConfigDBDevice(db *configdb.DB, nameOrID string) (*configdb.Device, error) {
	devices, err := db.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices configured")
	}

	for i := range devices {
		if devices[i].DeviceID == nameOrID {
			return &devices[i], nil
		}
	}

	var matches []*configdb.Device
	for i := range devices {
		if devices[i].FriendlyName == nameOrID {
			matches = append(matches, &devices[i])
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("ambiguous device name %q matches multiple devices (use device ID instead)", nameOrID)
	}

	searchLower := strings.ToLower(nameOrID)
	matches = nil
	for i := range devices {
		if strings.Contains(strings.ToLower(devices[i].FriendlyName), searchLower) {
			matches = append(matches, &devices[i])
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, d := range matches {
			names[i] = fmt.Sprintf("%s (%s)", d.FriendlyName, d.DeviceID)
		}
		return nil, fmt.Errorf("ambiguous device name %q matches multiple devices:\n  %s\nUse device ID to specify which one", nameOrID, strings.Join(names, "\n  "))
	}

	return nil, fmt.Errorf("device %q not found", nameOrID)
}

// formatConfigDBDeviceList returns a human-readable summary of registered
// devices, matching the retired config.FormatDeviceList's layout.
func formatConfigDBDeviceList(devices []configdb.Device) string {
	if len(devices) == 0 {
		return "No devices configured"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Configured Devices (%d):\n", len(devices)))
	for _, d := range devices {
		name := d.FriendlyName
		if name == "" {
			name = "(unnamed)"
		}
		b.WriteString(fmt.Sprintf("  • %s\n", name))
		b.WriteString(fmt.Sprintf("    ID: %s\n", d.DeviceID))
		b.WriteString(fmt.Sprintf("    Lists: %d configured\n", len(d.Lists)))
		if !d.LastSeen.IsZero() {
			b.WriteString(fmt.Sprintf("    Last seen: %s\n", d.LastSeen.Format("2006-01-02 15:04:05")))
		}
	}
	return b.String()
}

// formatConfigDBDeviceList's per-list equivalent of the retired
// config.FormatSmartListConfig.
func formatConfigDBList(list configdb.DeviceList) string {
	var b strings.Builder

	status := "enabled"
	if !list.Enabled {
		status = "disabled"
	}

	b.WriteString(fmt.Sprintf("• %s (%s)\n", list.ListName, status))
	b.WriteString(fmt.Sprintf("  ID: %s\n", list.ListID))

	if list.Settings != nil {
		b.WriteString("  Settings:\n")
		writeSettingsLines(&b, list.Settings)
	} else {
		b.WriteString("  Settings: (using device defaults)\n")
	}

	return b.String()
}

func writeSettingsLines(b *strings.Builder, settings *sync.SharedListSettings) {
	if settings.OnlyUnread {
		b.WriteString("    - Only unread books\n")
	}
	if settings.KeepLastRead {
		b.WriteString(fmt.Sprintf("    - Keep last read books (%d)\n", sync.EffectiveKeepLastReadCount(settings)))
	}
	if settings.OnlyChecked {
		b.WriteString("    - Only checked books\n")
	}
	if settings.Limit {
		b.WriteString(fmt.Sprintf("    - Limit: %d %s\n", settings.LimitValue, settings.LimitValueType))
	}
	if settings.Sort {
		b.WriteString(fmt.Sprintf("    - Sort: %s\n", settings.ListSortType))
	}
}
