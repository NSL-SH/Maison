package apps

import "testing"

func TestMeasuredPct(t *testing.T) {
	cases := []struct {
		ev   Event
		want float64
	}{
		{Event{Pct: 30}, 30},
		{Event{Pct: 0, Done: 5, Total: 10}, 0}, // a real 0% from the engine wins
		{Event{Pct: PctUnknown, Done: 25, Total: 100}, 25},
		{Event{Pct: PctUnknown}, PctUnknown},
	}
	for _, c := range cases {
		if got := measuredPct(c.ev); got != c.want {
			t.Errorf("measuredPct(%+v) = %v, want %v", c.ev, got, c.want)
		}
	}
}

// Unknown progress must not be scaled into a share of the bar: for the second of two
// engines, (1 + -1/100)/2*100 = 49.5 would read as a real half-way mark.
func TestEngineEventKeepsUnknownUnknown(t *testing.T) {
	r := &Registry{}
	ps := []Provider{&LocalProvider{}, &LocalProvider{}}
	ev := r.engineEvent(ps, 1, BackupEvent{Phase: PhaseCopy, Copy: PctUnknown})
	if ev.Copy != PctUnknown {
		t.Fatalf("Copy = %v, want PctUnknown", ev.Copy)
	}
	ev = r.engineEvent(ps, 1, BackupEvent{Phase: PhaseCopy, Copy: 50})
	if ev.Copy != 75 {
		t.Fatalf("Copy = %v, want 75", ev.Copy)
	}
}

func TestBackupEventServing(t *testing.T) {
	for phase, want := range map[string]bool{
		PhaseCopy: true, PhaseSync: false, PhaseCompress: false, PhaseStart: false,
		PhaseRestore: false, PhaseDone: false, PhaseError: false,
	} {
		if got := (BackupEvent{Phase: phase}).Serving(); got != want {
			t.Errorf("Serving() in %q = %v, want %v", phase, got, want)
		}
	}
}

// A report with no measurable progress keeps the tile's bar where it was instead of
// storing -1, which the tile rendered as "-1%".
func TestTrackBackupHoldsLastKnownPct(t *testing.T) {
	r := &Registry{backups: map[string]*BackupState{"app": {ID: "app", Phase: PhaseCopy, Serving: true}}}
	track := r.trackBackup("app", nil)

	track(BackupEvent{Phase: PhaseCopy, Copy: 40})
	track(BackupEvent{Phase: PhaseCopy, Copy: PctUnknown})
	if st := r.backups["app"]; st.Copy != 40 || !st.Serving {
		t.Fatalf("after unknown copy report: Copy=%v Serving=%v, want 40 true", st.Copy, st.Serving)
	}
	// A new phase that has measured nothing yet stays unknown: it must not inherit
	// the 0 the copy events carried in their Sync field.
	track(BackupEvent{Phase: PhaseCopy, Copy: 100})
	track(BackupEvent{Phase: PhaseSync, Copy: 100, Sync: PctUnknown})
	if st := r.backups["app"]; st.Sync != PctUnknown {
		t.Fatalf("sync just started: Sync=%v, want PctUnknown", st.Sync)
	}
	track(BackupEvent{Phase: PhaseSync, Copy: 100, Sync: 30})
	track(BackupEvent{Phase: PhaseSync, Copy: 100, Sync: PctUnknown})
	if st := r.backups["app"]; st.Sync != 30 || st.Serving {
		t.Fatalf("after unknown sync report: Sync=%v Serving=%v, want 30 false", st.Sync, st.Serving)
	}
}
