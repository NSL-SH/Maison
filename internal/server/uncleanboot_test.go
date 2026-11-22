package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yundera/maison/internal/incident"
)

// The unclean-boot warnings close a day after they were last asserted; a failed
// repair stays open until a repair succeeds or the owner closes it.
func TestCloseUncleanBootIncidents(t *testing.T) {
	s := &Server{incidents: incident.New(filepath.Join(t.TempDir(), "incidents.json"))}
	at := time.Date(2026, 11, 22, 17, 0, 0, 0, time.UTC)
	s.incidents.Now = func() time.Time { return at }
	s.incidents.Report(uncleanBootIncident("9f2c1a7e", []string{"seafile"}, at))
	s.incidents.Report(incident.Report{ID: "app.repaired:seafile:9f2c1a7e", Kind: incident.KindAppRepaired, Severity: incident.Warning, Title: "repaired"})
	s.incidents.Report(incident.Report{ID: "app.repair-failed:immich", Kind: incident.KindAppRepairFailed, Severity: incident.Critical, Title: "failed"})

	open := func() map[string]bool {
		m := map[string]bool{}
		for _, inc := range s.incidents.Snapshot().Open {
			m[inc.ID] = true
		}
		return m
	}
	s.closeUncleanBootIncidents(at.Add(23 * time.Hour))
	if len(open()) != 3 {
		t.Fatalf("closed before a day: %v", open())
	}
	s.closeUncleanBootIncidents(at.Add(24 * time.Hour))
	if got := open(); len(got) != 1 || !got["app.repair-failed:immich"] {
		t.Fatalf("open after a day: %v, want only the failed repair", got)
	}
}
