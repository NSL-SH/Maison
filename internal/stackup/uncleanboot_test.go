package stackup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yundera/maison/internal/config"
	"github.com/yundera/maison/internal/incident"
)

// uncleanBootApp writes an app folder whose compose declares the given unclean-boot
// step script, and a config that records what is reported and resolved.
func uncleanBootApp(t *testing.T, script string) (cfg config.Config, dir string, reports *[]incident.Report, resolved *[]string, run func() error) {
	t.Helper()
	fixtureHookBin(t, "echo", "cat")
	cfg, dir = hookTestApp(t)
	compose := "services:\n  app:\n    image: busybox\nx-compose-app:\n  steps:\n" +
		"    - name: repair\n      at: unclean-boot\n      script: |\n" +
		indent(script, "        ") +
		"    - name: configure\n      at: started\n      script: echo started >> started\n"
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o644)
	reports, resolved = &[]incident.Report{}, &[]string{}
	cfg.Report = func(r incident.Report) { *reports = append(*reports, r) }
	cfg.Resolve = func(id string) { *resolved = append(*resolved, id) }
	files := AppFiles(dir)
	if ok, err := MarkUncleanBoot(cfg, "app", files, "9f2c1a7e"); err != nil || !ok {
		t.Fatalf("mark: %v, %v", ok, err)
	}
	if !UncleanBootPending(cfg, "app") {
		t.Fatal("no marker")
	}
	return cfg, dir, reports, resolved, func() error {
		return RunUncleanBoot(t.Context(), cfg, "app", dir, files)
	}
}

func indent(s, by string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(by + l + "\n")
	}
	return b.String()
}

// A step that finds nothing reports nothing, runs only while the marker is there,
// and never runs the app's other steps.
func TestRunUncleanBootCleanRunIsSilent(t *testing.T) {
	_, dir, reports, resolved, run := uncleanBootApp(t, "echo ran >> runs")
	for range 2 {
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	if got := readLines(t, filepath.Join(dir, "runs")); len(got) != 1 {
		t.Fatalf("ran %d times, want once per marker", len(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "started")); !os.IsNotExist(err) {
		t.Fatal("a started step ran")
	}
	if len(*reports) != 0 {
		t.Fatalf("reported %+v", *reports)
	}
	if len(*resolved) != 1 || (*resolved)[0] != "app.repair-failed:app" {
		t.Fatalf("resolved %v, want the app's failed-repair incident", *resolved)
	}
	// Never a once marker: the next unclean boot runs it again.
	if _, err := os.Stat(filepath.Join(dir, InitStateDir, "repair")); !os.IsNotExist(err) {
		t.Fatal("an unclean-boot step left a once marker")
	}
}

func TestRunUncleanBootRepairIsAWarning(t *testing.T) {
	_, _, reports, _, run := uncleanBootApp(t, `echo "REPAIRED=Library Photos rolled back to 16:58" >> "$MAISON_OUTPUT"`)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if len(*reports) != 1 {
		t.Fatalf("reports %+v", *reports)
	}
	r := (*reports)[0]
	if r.ID != "app.repaired:app:9f2c1a7e" || r.Kind != incident.KindAppRepaired || r.Severity != incident.Warning ||
		!strings.Contains(r.Detail, "Library Photos rolled back to 16:58") {
		t.Fatalf("report %+v", r)
	}
}

func TestRunUncleanBootFailureIsCritical(t *testing.T) {
	cfg, _, reports, _, run := uncleanBootApp(t, "echo fsck failed >&2; exit 3")
	if err := run(); err == nil {
		t.Fatal("a failing step returned no error")
	}
	if len(*reports) != 1 || (*reports)[0].ID != "app.repair-failed:app" || (*reports)[0].Severity != incident.Critical || !strings.Contains((*reports)[0].Detail, "fsck failed") {
		t.Fatalf("reports %+v", *reports)
	}
	// Cleared anyway: rerunning a repair that failed on every start would not fix it.
	if UncleanBootPending(cfg, "app") {
		t.Fatal("marker left after a failed run")
	}
}

// An app without unclean-boot steps is not marked.
func TestMarkUncleanBootSkipsAppsWithoutSteps(t *testing.T) {
	cfg, dir := hookTestApp(t)
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  app:\n    image: busybox\n"), 0o644)
	if ok, err := MarkUncleanBoot(cfg, "app", AppFiles(dir), "9f2c1a7e"); err != nil || ok {
		t.Fatalf("%v, %v; want not marked", ok, err)
	}
	if UncleanBootPending(cfg, "app") {
		t.Fatal("marker written")
	}
}
