package configdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/sync"
)

// Device is one registered device and its sync configuration.
type Device struct {
	DeviceID        string
	FriendlyName    string
	LastSeen        time.Time                // zero value if never set
	DefaultSettings *sync.SharedListSettings // nil if unset
	Lists           []DeviceList
}

// DeviceList is one list assigned to a device for sync.
type DeviceList struct {
	ListID   string
	ListName string
	Enabled  bool
	Settings *sync.SharedListSettings // nil = inherit the device's DefaultSettings
}

func marshalSettings(s *sync.SharedListSettings) (sql.NullString, error) {
	if s == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal settings: %w", err)
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

func unmarshalSettings(ns sql.NullString) (*sync.SharedListSettings, error) {
	if !ns.Valid {
		return nil, nil
	}
	var s sync.SharedListSettings
	if err := json.Unmarshal([]byte(ns.String), &s); err != nil {
		return nil, fmt.Errorf("unmarshal settings: %w", err)
	}
	return &s, nil
}

func nullTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339), Valid: true}
}

func parseNullTime(ns sql.NullString) time.Time {
	if !ns.Valid {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ListDevices returns every registered device with its assigned lists,
// ordered by device_id.
func (db *DB) ListDevices() ([]Device, error) {
	rows, err := db.Query(`SELECT device_id, friendly_name, last_seen, default_settings FROM devices ORDER BY device_id`)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()

	var devices []Device
	byID := make(map[string]*Device)
	for rows.Next() {
		var d Device
		var lastSeen, defaultSettings sql.NullString
		if err := rows.Scan(&d.DeviceID, &d.FriendlyName, &lastSeen, &defaultSettings); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		d.LastSeen = parseNullTime(lastSeen)
		d.DefaultSettings, err = unmarshalSettings(defaultSettings)
		if err != nil {
			return nil, fmt.Errorf("device %s: %w", d.DeviceID, err)
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	for i := range devices {
		byID[devices[i].DeviceID] = &devices[i]
	}

	lists, err := db.listAllDeviceLists()
	if err != nil {
		return nil, err
	}
	for deviceID, dl := range lists {
		if d, ok := byID[deviceID]; ok {
			d.Lists = dl
		}
	}

	return devices, nil
}

// GetDevice returns one device with its assigned lists, or nil if it
// isn't registered.
func (db *DB) GetDevice(deviceID string) (*Device, error) {
	var d Device
	d.DeviceID = deviceID
	var lastSeen, defaultSettings sql.NullString
	err := db.QueryRow(`SELECT friendly_name, last_seen, default_settings FROM devices WHERE device_id = ?`, deviceID).
		Scan(&d.FriendlyName, &lastSeen, &defaultSettings)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device %s: %w", deviceID, err)
	}
	d.LastSeen = parseNullTime(lastSeen)
	d.DefaultSettings, err = unmarshalSettings(defaultSettings)
	if err != nil {
		return nil, fmt.Errorf("device %s: %w", deviceID, err)
	}

	d.Lists, err = db.ListDeviceLists(deviceID)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpsertDevice creates or updates a device's own fields (friendly name,
// last seen, default settings) - it does not touch that device's list
// assignments, which are managed separately via AddDeviceList/
// RemoveDeviceList/UpdateDeviceList.
func (db *DB) UpsertDevice(deviceID, friendlyName string, lastSeen time.Time, defaultSettings *sync.SharedListSettings) error {
	settingsVal, err := marshalSettings(defaultSettings)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO devices (device_id, friendly_name, last_seen, default_settings)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			friendly_name = excluded.friendly_name,
			last_seen = excluded.last_seen,
			default_settings = excluded.default_settings
	`, deviceID, friendlyName, nullTime(lastSeen), settingsVal)
	if err != nil {
		return fmt.Errorf("upsert device %s: %w", deviceID, err)
	}
	return nil
}

// DeleteDevice removes a device and (via ON DELETE CASCADE) all of its
// list assignments.
func (db *DB) DeleteDevice(deviceID string) error {
	if _, err := db.Exec(`DELETE FROM devices WHERE device_id = ?`, deviceID); err != nil {
		return fmt.Errorf("delete device %s: %w", deviceID, err)
	}
	return nil
}

// ListDeviceLists returns the lists assigned to one device, ordered by
// list_id.
func (db *DB) ListDeviceLists(deviceID string) ([]DeviceList, error) {
	rows, err := db.Query(`SELECT list_id, list_name, enabled, settings FROM device_lists WHERE device_id = ? ORDER BY list_id`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list device lists for %s: %w", deviceID, err)
	}
	defer rows.Close()

	var lists []DeviceList
	for rows.Next() {
		var dl DeviceList
		var settings sql.NullString
		if err := rows.Scan(&dl.ListID, &dl.ListName, &dl.Enabled, &settings); err != nil {
			return nil, fmt.Errorf("scan device list: %w", err)
		}
		dl.Settings, err = unmarshalSettings(settings)
		if err != nil {
			return nil, fmt.Errorf("device %s list %s: %w", deviceID, dl.ListID, err)
		}
		lists = append(lists, dl)
	}
	return lists, rows.Err()
}

// listAllDeviceLists returns every device's lists in one query, keyed by
// device_id - used by ListDevices to avoid one query per device.
func (db *DB) listAllDeviceLists() (map[string][]DeviceList, error) {
	rows, err := db.Query(`SELECT device_id, list_id, list_name, enabled, settings FROM device_lists ORDER BY device_id, list_id`)
	if err != nil {
		return nil, fmt.Errorf("list all device lists: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]DeviceList)
	for rows.Next() {
		var deviceID string
		var dl DeviceList
		var settings sql.NullString
		if err := rows.Scan(&deviceID, &dl.ListID, &dl.ListName, &dl.Enabled, &settings); err != nil {
			return nil, fmt.Errorf("scan device list: %w", err)
		}
		dl.Settings, err = unmarshalSettings(settings)
		if err != nil {
			return nil, fmt.Errorf("device %s list %s: %w", deviceID, dl.ListID, err)
		}
		result[deviceID] = append(result[deviceID], dl)
	}
	return result, rows.Err()
}

// AddDeviceList assigns a list to a device. Returns an error if the pair
// already exists (matches the old config.DeviceConfig.AddList's
// duplicate-rejection behavior) - callers that want create-or-update
// should use UpdateDeviceList after checking existence, not this.
func (db *DB) AddDeviceList(deviceID string, dl DeviceList) error {
	settingsVal, err := marshalSettings(dl.Settings)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO device_lists (device_id, list_id, list_name, enabled, settings)
		VALUES (?, ?, ?, ?, ?)
	`, deviceID, dl.ListID, dl.ListName, dl.Enabled, settingsVal)
	if err != nil {
		return fmt.Errorf("add list %s to device %s: %w", dl.ListID, deviceID, err)
	}
	return nil
}

// RemoveDeviceList unassigns a list from a device. Not an error if the
// pair didn't exist - callers check existence first if they need to
// distinguish "removed" from "wasn't there."
func (db *DB) RemoveDeviceList(deviceID, listID string) error {
	if _, err := db.Exec(`DELETE FROM device_lists WHERE device_id = ? AND list_id = ?`, deviceID, listID); err != nil {
		return fmt.Errorf("remove list %s from device %s: %w", listID, deviceID, err)
	}
	return nil
}

// UpdateDeviceList updates a device-list assignment's enabled state
// and/or settings. Pass nil for a field to leave it unchanged.
func (db *DB) UpdateDeviceList(deviceID, listID string, enabled *bool, settings *sync.SharedListSettings) error {
	if enabled != nil {
		if _, err := db.Exec(`UPDATE device_lists SET enabled = ? WHERE device_id = ? AND list_id = ?`, *enabled, deviceID, listID); err != nil {
			return fmt.Errorf("update enabled for list %s on device %s: %w", listID, deviceID, err)
		}
	}
	if settings != nil {
		settingsVal, err := marshalSettings(settings)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE device_lists SET settings = ? WHERE device_id = ? AND list_id = ?`, settingsVal, deviceID, listID); err != nil {
			return fmt.Errorf("update settings for list %s on device %s: %w", listID, deviceID, err)
		}
	}
	return nil
}
