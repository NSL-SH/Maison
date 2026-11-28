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
