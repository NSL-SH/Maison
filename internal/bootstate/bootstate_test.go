package bootstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	now := time.Now()
	check := func(boot string, want Verdict) {
		t.Helper()
		if v, err := Check(path, boot, now); err != nil || v != want {
			t.Fatalf("boot %s: %v, %v; want %v", boot, v, err, want)
		}
	}

	check("a", First)
	check("a", SameBoot) // Maison's container restarted; the server did not

	if err := MarkClean(path); err != nil {
		t.Fatal(err)
	}
	check("b", CleanReboot)

	// No SIGTERM before the next boot: the server stopped without shutting down.
	check("c", UncleanReboot)

	// A clean stop of Maison alone, then the same boot: still the same boot, and the
	// record is back to running.
	if err := MarkClean(path); err != nil {
		t.Fatal(err)
	}
	check("c", SameBoot)
	check("d", UncleanReboot)
}

// A damaged record is not evidence of a crash.
func TestCheckUnreadableRecordIsFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(path, []byte("{not json"), 0o644)
	if v, err := Check(path, "a", time.Now()); err != nil || v != First {
		t.Fatalf("%v, %v; want first start", v, err)
	}
	if v, _ := Check(path, "b", time.Now()); v != UncleanReboot {
		t.Fatalf("the record was not rewritten: %v", v)
	}
}
