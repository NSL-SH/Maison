package stackup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yundera/maison/internal/config"
	"github.com/yundera/maison/internal/incident"
	"github.com/yundera/maison/internal/xcomposeapp"
)

// UncleanBootDir, under Maison's state folder, holds one empty file per app whose
// `unclean-boot` steps are still to run after the server stopped without shutting
// down. In the state folder rather than the app's: a marker that travelled with the
// app's backup would run a repair on a restored app that never crashed.
//
// A marker, not a call, because the steps can only run against the running app and
// an app the owner had stopped stays stopped across the reboot. The server runs
// them for each app as soon as it is up; an app that is not comes back through Up,
// which runs them after its started steps.
const UncleanBootDir = "unclean-boot"

// RepairedOutput is what an unclean-boot step writes to $MAISON_OUTPUT when it found
// damage and repaired it: one line saying what it did, in the owner's terms. A step
// that writes nothing found nothing, and nobody hears about it.
const RepairedOutput = "REPAIRED"

// uncleanBootMu serialises the runs: the server's pass and an Up can reach the same
// app at once, and a repair must not run twice over the same data.
var uncleanBootMu sync.Mutex

func uncleanBootMarker(cfg config.Config, project string) string {
	return filepath.Join(cfg.StateDir(), UncleanBootDir, markerName(project))
}

// AppFiles is a managed app's compose files: the base, and its override when present.
func AppFiles(dir string) []string {
	return composeFiles(filepath.Join(dir, "docker-compose.yml"), filepath.Join(dir, "docker-compose.override.yml"))
}

// MarkUncleanBoot records that the app's unclean-boot steps are to run after the
// given boot, and reports whether it declares any. An app without them gets no
// marker.
func MarkUncleanBoot(cfg config.Config, project string, files []string, boot string) (bool, error) {
	if !hasUncleanBootSteps(Load(files).Steps) {
		return false, nil
	}
	path := uncleanBootMarker(cfg, project)
	if err := os.MkdirAll(filepath.Dir(path), DefaultFolderMode); err != nil {
		return true, err
	}
	return true, os.WriteFile(path, []byte(boot), 0o644)
}

// UncleanBootPending reports whether the app still has unclean-boot steps to run.
func UncleanBootPending(cfg config.Config, project string) bool {
	return fileExists(uncleanBootMarker(cfg, project))
}

// RunUncleanBoot runs the app's pending unclean-boot steps, reports what they found
// and clears the marker. A no-op when nothing is pending. The caller makes sure the
// app is running: the steps are written against it.
//
// Every step runs, as at `started`. What each wrote as REPAIRED becomes the detail of
// an app.repaired warning for that boot, left open a day so it is delivered and seen
// (see docs/incidents.md); a failure is an app.repair-failed critical instead, closed
// by the next run that succeeds. The marker is cleared either way — a repair that failed is the
// owner's to look at now, and rerunning it on every start would not fix it.
func RunUncleanBoot(ctx context.Context, cfg config.Config, project, dir string, files []string) error {
	uncleanBootMu.Lock()
	defer uncleanBootMu.Unlock()
	marker := uncleanBootMarker(cfg, project)
	b, err := os.ReadFile(marker)
	if err != nil {
		return nil
	}
	boot := strings.TrimSpace(string(b))
	var repaired []string
	var errs []error
	for _, step := range Load(files).Steps {
		if step.At != xcomposeapp.AtUncleanBoot {
			continue
		}
		outputs := map[string]string{}
		if err := runStep(ctx, cfg, project, dir, step, outputs); err != nil {
			errs = append(errs, fmt.Errorf("step %s: %w", stepLabel(step), err))
			continue
		}
		if r := strings.TrimSpace(outputs[RepairedOutput]); r != "" {
			repaired = append(repaired, r)
		}
	}
	// A cancelled context is Maison stopping, not the step failing: keep the marker
	// so the next start runs it again.
	if ctx.Err() != nil {
		return errors.Join(append(errs, ctx.Err())...)
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	failedID := incident.KindAppRepairFailed + ":" + project
	if len(errs) > 0 {
		cfg.ReportIncident(repairFailedIncident(failedID, project, errors.Join(errs...)))
		return errors.Join(errs...)
	}
	cfg.ResolveIncident(failedID)
	if len(repaired) > 0 {
		id := incident.KindAppRepaired + ":" + project
		if boot != "" {
			id += ":" + boot
		}
		cfg.ReportIncident(repairedIncident(id, project, repaired))
	}
	return nil
}

func hasUncleanBootSteps(steps []xcomposeapp.Step) bool {
	for _, s := range steps {
		if s.At == xcomposeapp.AtUncleanBoot {
			return true
		}
	}
	return false
}

func repairedIncident(id, project string, repaired []string) incident.Report {
	return incident.Report{
		ID:       id,
		Kind:     incident.KindAppRepaired,
		Severity: incident.Warning,
		Title:    project + " was repaired after the server stopped unexpectedly",
		Detail: "The server stopped without shutting down — a power cut or a forced restart — " +
			"and the last seconds of what " + project + " was writing never reached the disk. " +
			"On the way back up it found the damage and repaired it:\n\n" + strings.Join(repaired, "\n") +
			"\n\nThe app is working again. Anything saved in the last moments before the stop may be missing: " +
			"check recent changes, and restore them from last night's backup if they are not there.",
		Args: map[string]string{"app": project},
	}
}

func repairFailedIncident(id, project string, err error) incident.Report {
	return incident.Report{
		ID:       id,
		Kind:     incident.KindAppRepairFailed,
		Severity: incident.Critical,
		Title:    project + " could not check its data after the server stopped unexpectedly",
		Detail: "The server stopped without shutting down — a power cut or a forced restart — " +
			"and " + project + "'s check of its data on the way back up did not complete:\n" + err.Error() +
			"\n\nIts data may be damaged. If something is missing or will not open, restore the app " +
			"from last night's backup.",
		Args: map[string]string{"app": project},
	}
}
