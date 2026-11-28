package installer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yundera/maison/internal/composefile"
	"github.com/yundera/maison/internal/incident"
)

// UpdateRefusedPull is the reason an update is refused when its new version's
// images could not be pulled. Carried in the app.update incident's args and the
// run item, like the no-rollback and data reasons.
const UpdateRefusedPull = "pull_failed"

// PullRefusedError is an update refused because images its new version needs are
// not on the box and could not be pulled — before anything was backed up, stopped
// or written.
type PullRefusedError struct {
	Failed []string // "image (error)", one per image
}

func (e *PullRefusedError) Error() string {
	return "not updated, nothing was changed: the new version's images could not be downloaded: " +
		strings.Join(e.Failed, "; ")
}

// imagePuller is the part of the Docker client the update's pull uses.
type imagePuller interface {
	PullImage(ctx context.Context, ref string, onProgress func(pct float64, status string)) error
	HasImage(ctx context.Context, ref string) (bool, error)
}

// puller is what the update pulls with: the test seam if set, else the Docker
// client, else nil (no daemon — the pull is skipped and compose up has the last word).
func (in *Installer) puller() imagePuller {
	if in.pullSeam != nil {
		return in.pullSeam
	}
	if in.dx != nil {
		return in.dx
	}
	return nil
}

// pullRetryDelays are the waits between attempts at one image: three tries in all.
// A var so tests need not sleep.
var pullRetryDelays = []time.Duration{5 * time.Second, 15 * time.Second}

// updateImages lists the images the new version needs from a registry: each
// service's image that compose would actually pull (composefile.Service.Pulled),
// then the steps' and migrations' literal images, deduplicated.
func updateImages(f *composefile.File, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(img string) {
		if img != "" && !seen[img] {
			seen[img] = true
			out = append(out, img)
		}
	}
	for _, svc := range f.Services {
		if svc.Pulled() {
			add(svc.Image)
		}
	}
	for _, img := range extra {
		add(img)
	}
	return out
}

// pullForUpdate pulls every image the new version needs, while the old version is
// still serving, and reports the ones that are still not on the box afterwards.
//
// Why it must not be best-effort: the stop comes next, and `compose up` re-pulls
// whatever is missing only once the app is down. A registry that drops the
// connection then fails the update after the stop, and the rollback restores the
// whole backup over data nothing touched. That is how mestio's immich was down for
// 44 minutes on 2026-11-16: ghcr reset the connection pulling
// immich-machine-learning:v3.4.1, the pull here ignored it, and the 92 GiB restore
// followed. Refused here, the same failure costs nothing: the app keeps serving its
// current version and the store keeps offering the update.
//
// Each image gets three attempts, since what fails in practice are connection resets
// mid-download. An image whose pull fails but which is already in the local store
// (a digest pin with the registry down) is fine: compose will start from it.
func pullForUpdate(ctx context.Context, p imagePuller, images []string, emit func(Event)) error {
	if len(images) == 0 || p == nil {
		emit(Event{Phase: "pull", Message: "Preparing images", Download: 100})
		return nil
	}
	span := 100.0 / float64(len(images))
	var failed []string
	for i, img := range images {
		base := float64(i) * span
		err := pullWithRetry(ctx, p, img, func(attempt int, pct float64, status string) {
			msg := img + ": " + status
			if attempt > 1 {
				msg = fmt.Sprintf("%s (attempt %d): %s", img, attempt, status)
			}
			// Docker's opening lines ("Pulling", "Pulling from …") carry no measure:
			// before the first image has moved, that is unknown rather than 0%.
			if i == 0 && pct <= 0 {
				emit(Event{Phase: "pull", Message: msg, Download: unknownPct})
				return
			}
			emit(Event{Phase: "pull", Message: msg, Download: base + pct/100*span})
		})
		if err != nil {
			if ok, herr := p.HasImage(ctx, img); herr != nil || !ok {
				failed = append(failed, fmt.Sprintf("%s (%v)", img, err))
				continue
			}
		}
		emit(Event{Phase: "pull", Message: "Pulled " + img, Download: base + span})
	}
	if len(failed) > 0 {
		return &PullRefusedError{Failed: failed}
	}
	return nil
}

func pullWithRetry(ctx context.Context, p imagePuller, img string, progress func(attempt int, pct float64, status string)) error {
	var err error
	for attempt := 1; ; attempt++ {
		progress(attempt, 0, "Pulling")
		err = p.PullImage(ctx, img, func(pct float64, status string) { progress(attempt, pct, status) })
		if err == nil || ctx.Err() != nil || attempt > len(pullRetryDelays) {
			return err
		}
		log.Printf("pull %s: attempt %d failed, retrying: %v", img, attempt, err)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(pullRetryDelays[attempt-1]):
		}
	}
}

// refusePull raises the app.update incident for an update refused over its pull.
func (in *Installer) refusePull(project string, refused *PullRefusedError) error {
	log.Printf("update %s: refused (%s): %s", project, UpdateRefusedPull, strings.Join(refused.Failed, "; "))
	in.report(incident.Report{
		ID: "app.update:" + project, Kind: incident.KindAppUpdate, Severity: incident.Warning,
		Title: project + " was not updated: its new version could not be downloaded",
		Detail: "These images could not be pulled:\n" + strings.Join(refused.Failed, "\n") +
			"\n\nNothing was changed; the app is still running its current version. This is usually " +
			"the registry or the network failing for a moment: retry the update later.",
		Args: map[string]string{"app": project, "reason": UpdateRefusedPull},
	})
	return refused
}
