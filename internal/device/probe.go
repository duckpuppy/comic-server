package device

import (
	"errors"
	"fmt"
	"time"

	"github.com/duckpuppy/comic-server/internal/protocol"
)

// ErrAddressMismatch is returned by Probe when the device that answers at an
// address is not the device we expected there.
var ErrAddressMismatch = errors.New("a different device answered at that address")

// ErrProbeFailed is returned by Probe when the address gave no usable
// device: unreachable, unreadable info or failed validation.
var ErrProbeFailed = errors.New("device not reachable at that address")

// iniReader is the part of protocol.Client that Probe needs.
type iniReader interface {
	ReadFile(filename string) ([]byte, error)
}

// dial opens a client to a device address. A variable so tests can fake it.
var dial = func(address string) iniReader {
	return protocol.NewClient(address, DevicePort)
}

// quickDial is like dial but gives up fast and does not retry. It is for a
// check made on every click, where an address that does not answer must cost
// about a second, not several.
var quickDial = func(address string) iniReader {
	c := protocol.NewClient(address, DevicePort)
	c.SetTimeouts(quickProbeTimeout, quickProbeTimeout, quickProbeTimeout)
	c.SetRetries(0)
	return c
}

// quickProbeTimeout bounds connect, read and write in ProbeQuick.
const quickProbeTimeout = time.Second

// ProbeResult is what a successful Probe learned about the device.
type ProbeResult struct {
	Info *Info
	RTT  time.Duration // time to read comicrack.ini
}

// Probe connects to a device at address (an IP or hostname, device port
// 7614), reads comicrack.ini and validates it. If wantID is not empty, the
// device must report that ID, otherwise ErrAddressMismatch is returned: a
// wrong hostname must never sync another device. Probe changes nothing on
// the device.
func Probe(address, wantID string) (*ProbeResult, error) {
	return probeWith(dial, address, wantID)
}

// ProbeQuick is Probe with a one-second limit and no retry, for checking an
// address that is probably right but may be stale (the last LAN address).
func ProbeQuick(address, wantID string) (*ProbeResult, error) {
	return probeWith(quickDial, address, wantID)
}

func probeWith(open func(string) iniReader, address, wantID string) (*ProbeResult, error) {
	start := time.Now()
	iniData, err := open(address).ReadFile("comicrack.ini")
	if err != nil {
		return nil, fmt.Errorf("%w (%s): %w", ErrProbeFailed, address, err)
	}
	rtt := time.Since(start)

	info, err := ParseINI(iniData)
	if err != nil {
		return nil, fmt.Errorf("%w (%s): unreadable info: %w", ErrProbeFailed, address, err)
	}
	if err := info.Validate(); err != nil {
		return nil, fmt.Errorf("%w (%s): failed validation: %w", ErrProbeFailed, address, err)
	}
	if wantID != "" && info.ID != wantID {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrAddressMismatch, wantID, info.ID)
	}
	return &ProbeResult{Info: info, RTT: rtt}, nil
}
