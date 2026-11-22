// Package bootstate tells Maison whether the server it is starting on was shut down
// cleanly.
//
// A power cut or a forced reset loses what the kernel had not yet written to disk —
// on a PCS up to ~30 s of it (ext4 commit=30) — and an app can come back with files
// that exist and are empty. Nothing on the host records that the stop was unclean,
// and Maison must not need a host change to find out, so it keeps its own record:
//
//	start: read the boot ID, compare with the stored one, store {boot, clean: false}
//	SIGTERM: store clean: true
//
// A clean shutdown stops Docker, which sends Maison its SIGTERM; a crash sends
// nothing. So a new boot ID with clean still false is a server that stopped without
// shutting down. A new boot ID with clean true is an ordinary reboot, and the same
// boot ID is Maison's container restarting on a server that did not.
//
// Both writes are synced to disk before they return: the record that the box is
// running must survive exactly the crash it is there to detect.
package bootstate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FileName is the record's name in Maison's state folder.
const FileName = "boot.json"

// Verdict is what Check concluded about the stop before this start.
type Verdict int

const (
	// First is a box with no record yet: the first start of a Maison that keeps one.
	// Nothing is known about the stop before it, so nothing is assumed.
	First Verdict = iota
	// SameBoot is Maison restarting on a server that kept running.
	SameBoot
	// CleanReboot is a server that was shut down or rebooted properly.
	CleanReboot
	// UncleanReboot is a server that stopped without shutting down.
	UncleanReboot
)

func (v Verdict) String() string {
	switch v {
	case SameBoot:
		return "same boot"
	case CleanReboot:
		return "clean reboot"
	case UncleanReboot:
		return "unclean reboot"
	}
	return "first start"
}

type record struct {
	Boot    string    `json:"boot"`
	Clean   bool      `json:"clean"`
	Started time.Time `json:"started"`
}

// Path is the record's path under Maison's state folder.
func Path(stateDir string) string { return filepath.Join(stateDir, FileName) }

// Check compares bootID with the stored record, then stores bootID as running
// (clean: false). An unreadable record counts as First: a damaged file is not
// evidence of a crash, and an alert on every Maison upgrade would be worse than
// missing one.
func Check(path, bootID string, now time.Time) (Verdict, error) {
	v := First
	var prev record
	b, err := os.ReadFile(path)
	switch {
	case err == nil && json.Unmarshal(b, &prev) == nil && prev.Boot != "":
		switch {
		case prev.Boot == bootID:
			v = SameBoot
		case prev.Clean:
			v = CleanReboot
		default:
			v = UncleanReboot
		}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return First, err
	}
	return v, write(path, record{Boot: bootID, Started: now})
}

// MarkClean records that Maison is stopping on purpose. Called on SIGTERM; a server
// shutting down stops Docker, which is what sends it.
func MarkClean(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	r.Clean = true
	return write(path, r)
}

// write replaces the record durably: synced temporary, rename, synced folder. A
// rename that is not yet on disk when the power goes would bring back the previous
// record — the "clean" one of the last proper shutdown — and hide this crash.
func write(path string, r record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".partial"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
