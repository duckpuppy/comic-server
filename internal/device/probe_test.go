package device

import (
	"errors"
	"fmt"
	"testing"
)

type fakeReader struct {
	data []byte
	err  error
}

func (f fakeReader) ReadFile(string) ([]byte, error) { return f.data, f.err }

// withFakeDevice makes Probe talk to a fake that answers with data/err.
func withFakeDevice(t *testing.T, data []byte, err error) {
	t.Helper()
	old := dial
	dial = func(string) iniReader { return fakeReader{data: data, err: err} }
	t.Cleanup(func() { dial = old })
}

func validINI(id string) []byte {
	info := &Info{Model: "SM-T970", Manufacturer: "Samsung", Serial: "S1", Edition: EditionAndroidFull, Version: 103}
	return []byte(fmt.Sprintf("Name=Tab\nModel=%s\nManufacturer=%s\nSerial=%s\nID=%s\nHash=%s\nVersion=%d\nEdition=%s\n",
		info.Model, info.Manufacturer, info.Serial, id, info.ComputeHash(), info.Version, info.Edition))
}

func TestProbe_OK(t *testing.T) {
	withFakeDevice(t, validINI("dev-1"), nil)
	res, err := Probe("tablet.example.ts.net", "dev-1")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if res.Info.ID != "dev-1" {
		t.Errorf("ID = %q, want dev-1", res.Info.ID)
	}
}

func TestProbe_WrongDeviceIsMismatch(t *testing.T) {
	withFakeDevice(t, validINI("other-device"), nil)
	_, err := Probe("100.1.2.3", "dev-1")
	if !errors.Is(err, ErrAddressMismatch) {
		t.Fatalf("err = %v, want ErrAddressMismatch", err)
	}
}

func TestProbe_NoWantIDAcceptsAnyValidDevice(t *testing.T) {
	withFakeDevice(t, validINI("anything"), nil)
	if _, err := Probe("100.1.2.3", ""); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbe_Unreachable(t *testing.T) {
	withFakeDevice(t, nil, errors.New("i/o timeout"))
	_, err := Probe("100.1.2.3", "dev-1")
	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed", err)
	}
}

func TestProbe_BadHashIsProbeFailed(t *testing.T) {
	bad := []byte("Name=Tab\nModel=m\nManufacturer=x\nSerial=s\nID=dev-1\nHash=nothex\nVersion=103\nEdition=Android Full\n")
	withFakeDevice(t, bad, nil)
	_, err := Probe("100.1.2.3", "dev-1")
	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed", err)
	}
}
