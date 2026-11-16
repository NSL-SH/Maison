package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yundera/maison/internal/composefile"
	"github.com/yundera/maison/internal/incident"
)

// fakePuller fails each image's first `fails[img]` pulls; `local` images are in the
// local store whatever the pulls do.
type fakePuller struct {
	fails map[string]int
	local map[string]bool
	calls map[string]int
}

func (f *fakePuller) PullImage(_ context.Context, ref string, _ func(float64, string)) error {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[ref]++
	if f.calls[ref] <= f.fails[ref] {
		return errors.New("read: connection reset by peer")
	}
	return nil
}

func (f *fakePuller) HasImage(_ context.Context, ref string) (bool, error) {
	if f.local[ref] {
		return true, nil
	}
	return f.calls[ref] > f.fails[ref], nil
}

func noRetryWait(t *testing.T) {
	saved := pullRetryDelays
	pullRetryDelays = []time.Duration{0, 0}
	t.Cleanup(func() { pullRetryDelays = saved })
}

func TestPullForUpdateRetriesAResetConnection(t *testing.T) {
	noRetryWait(t)
	p := &fakePuller{fails: map[string]int{"ml:v2": 2}}

	if err := pullForUpdate(context.Background(), p, []string{"server:v2", "ml:v2"}, func(Event) {}); err != nil {
		t.Fatalf("two resets then a good pull must succeed, got %v", err)
	}
	if p.calls["ml:v2"] != 3 {
		t.Errorf("ml:v2 pulled %d times, want 3", p.calls["ml:v2"])
	}
}

// mestio, 2026-11-16: an image that keeps failing must refuse the update, naming it,
// instead of letting the update stop the app and fail in compose up.
func TestPullForUpdateRefusesAnImageThatNeverArrives(t *testing.T) {
	noRetryWait(t)
	p := &fakePuller{fails: map[string]int{"ml:v2": 99}}

	err := pullForUpdate(context.Background(), p, []string{"server:v2", "ml:v2"}, func(Event) {})

	var refused *PullRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("want a *PullRefusedError, got %v", err)
	}
	if len(refused.Failed) != 1 || !strings.Contains(refused.Failed[0], "ml:v2") {
		t.Errorf("Failed = %v, want only ml:v2", refused.Failed)
	}
	if p.calls["ml:v2"] != 3 {
		t.Errorf("ml:v2 pulled %d times, want 3", p.calls["ml:v2"])
	}
}

func TestPullForUpdateAcceptsAFailedPullOfALocalImage(t *testing.T) {
	noRetryWait(t)
	p := &fakePuller{fails: map[string]int{"db@sha256:abc": 99}, local: map[string]bool{"db@sha256:abc": true}}

	if err := pullForUpdate(context.Background(), p, []string{"db@sha256:abc"}, func(Event) {}); err != nil {
		t.Fatalf("an image already on the box must not refuse the update, got %v", err)
	}
}

func TestPullForUpdateStopsRetryingOnCancel(t *testing.T) {
	saved := pullRetryDelays
	pullRetryDelays = []time.Duration{time.Hour, time.Hour}
	t.Cleanup(func() { pullRetryDelays = saved })
	ctx, cancel := context.WithCancel(context.Background())
	p := &fakePuller{fails: map[string]int{"ml:v2": 99}}
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()

	done := make(chan error, 1)
	go func() { done <- pullForUpdate(ctx, p, []string{"ml:v2"}, func(Event) {}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled pull must not report success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pullForUpdate kept waiting after its context was cancelled")
	}
}

func TestUpdateImagesSkipsWhatComposeDoesNotPull(t *testing.T) {
	f, err := composefile.Parse([]byte(`
services:
  app:
    image: ghcr.io/x/app:v2
  built:
    image: local/built:dev
    build: .
  never:
    image: local/never:1
    pull_policy: never
  templated:
    image: ${REGISTRY}/thing:1
  dup:
    image: ghcr.io/x/app:v2
`))
	if err != nil {
		t.Fatal(err)
	}
	got := updateImages(f, []string{"busybox:1.36", "ghcr.io/x/app:v2"})
	want := map[string]bool{"ghcr.io/x/app:v2": true, "busybox:1.36": true}
	if len(got) != len(want) {
		t.Fatalf("updateImages = %v, want %v", got, want)
	}
	for _, img := range got {
		if !want[img] {
			t.Errorf("unexpected image %q in %v", img, got)
		}
	}
}

// The order is the fix: an image that cannot be pulled refuses the update before the
// backup and the stop, so the app keeps serving and nothing needs restoring.
func TestApplyUpdateRefusesBeforeTheBackupWhenThePullFails(t *testing.T) {
	noRetryWait(t)
	in, dir, installed, steps := updateFixture(t)
	in.pullSeam = &fakePuller{fails: map[string]int{"example/jellyfin:2": 99}}
	var reports []incident.Report
	in.Report = func(r incident.Report) { reports = append(reports, r) }

	res, err := in.ApplyUpdate(context.Background(), "jellyfin", UpdateOptions{})

	var refused *PullRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want a *PullRefusedError", err)
	}
	if len(*steps) != 0 {
		t.Errorf("steps = %v, want none: no backup, no stop, no rollback", *steps)
	}
	if res.Applied || res.RolledBack || res.Backup != "" {
		t.Errorf("result = %+v, want nothing applied, backed up or rolled back", res)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "docker-compose.yml")); string(raw) != installed {
		t.Error("the installed compose was changed by a refused update")
	}
	if len(reports) != 1 || reports[0].Args["reason"] != UpdateRefusedPull {
		t.Errorf("reports = %+v, want one app.update incident with reason %s", reports, UpdateRefusedPull)
	}
}
