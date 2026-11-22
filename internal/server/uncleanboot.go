package server

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"

	"github.com/yundera/maison/internal/bootstate"
	"github.com/yundera/maison/internal/incident"
	"github.com/yundera/maison/internal/stackup"
	"github.com/yundera/maison/internal/system"
)

// After a power cut or a forced reset, the kernel's last ~30 s of writes are gone,
// and an app can come back with files that exist and are empty — Seafile's library
// that looked empty for a week. Maison notices the unclean stop itself (bootstate),
// tells the owner, and gives every app that declares `unclean-boot` steps its
// chance to repair. No host change: the boot ID and a marker in Maison's own state
// folder are enough.

const (
	// repairWaitFor is how long after Maison starts it waits for an app with a
	// pending repair to be running. Docker brings the apps back on its own at boot,
	// usually within a minute or two; one that is not up by then was stopped by its
	// owner, and gets its repair from Up when it is next started.
	repairWaitFor = 10 * time.Minute
	repairPoll    = 15 * time.Second

	// repairQuiet is how long the unclean-boot warnings stay open. They describe
	// something that happened, not a condition that clears, so nothing would ever
	// resolve them — and one that opened and closed inside a delivery window would
	// never be mailed. A day, like an OOM kill.
	repairQuiet = 24 * time.Hour
)

// checkBoot is run once at start: it records this boot, and after an unclean stop
// opens host.unclean-boot and marks the apps that declare a repair.
func (s *Server) checkBoot() {
	id, ok := system.ReadBootID()
	if !ok {
		log.Printf("boot: no boot ID readable; an unclean stop of the server will not be noticed")
		return
	}
	v, err := bootstate.Check(bootstate.Path(s.cfg.StateDir()), id, time.Now())
	if err != nil {
		log.Printf("boot: %v", err)
	}
	log.Printf("boot: %s", v)
	if v != bootstate.UncleanReboot {
		return
	}
	// The boot ID's first group (it is a UUID): short, and enough to tell this box's
	// boots apart.
	boot, _, _ := strings.Cut(id, "-")
	var checked []string
	if s.apps != nil {
		for _, app := range s.apps.ManagedIDs() {
			dir := filepath.Join(s.cfg.AppsDir(), app)
			ok, err := stackup.MarkUncleanBoot(s.cfg, app, stackup.AppFiles(dir), boot)
			if err != nil {
				log.Printf("%s: marking for a check after the unclean boot: %v", app, err)
			}
			if ok {
				checked = append(checked, app)
			}
		}
	}
	s.incidents.Report(uncleanBootIncident(boot, checked, bootTime()))
}

// bootTime is when the server started, or zero when it cannot be read.
func bootTime() time.Time {
	t, err := host.BootTime()
	if err != nil || t == 0 {
		return time.Time{}
	}
	return time.Unix(int64(t), 0)
}

func uncleanBootIncident(boot string, checked []string, booted time.Time) incident.Report {
	when := ""
	if !booted.IsZero() {
		when = " It started again on " + booted.UTC().Format("2006-01-02 at 15:04 UTC") + "."
	}
	next := "No installed app declares a check of its own. If one misbehaves, restarting it — " +
		"or restoring it from last night's backup — brings it back."
	if len(checked) > 0 {
		next = "These apps check their data on the way back up and say so if they had to repair anything: " +
			strings.Join(checked, ", ") + "."
	}
	return incident.Report{
		ID:       incident.KindHostUncleanBoot + ":" + boot,
		Kind:     incident.KindHostUncleanBoot,
		Severity: incident.Warning,
		Title:    "The server stopped without shutting down",
		Detail: "The server went down without a proper shutdown — a power cut, a forced reset by the hosting " +
			"provider, or the host crashing." + when + " What the apps wrote in the last seconds before the stop " +
			"may not have reached the disk.\n\n" + next,
	}
}

// runPendingRepairs runs the unclean-boot steps of every app that has them pending
// (checkBoot's markers, or ones a restart of Maison interrupted), each as soon as the
// app is running. One that is not running within repairWaitFor keeps its marker for
// Up.
func (s *Server) runPendingRepairs(ctx context.Context) {
	if s.apps == nil {
		return
	}
	var pending []string
	for _, app := range s.apps.ManagedIDs() {
		if stackup.UncleanBootPending(s.cfg, app) {
			pending = append(pending, app)
		}
	}
	deadline := time.Now().Add(repairWaitFor)
	for len(pending) > 0 {
		var waiting []string
		for _, app := range pending {
			if !stackup.UncleanBootPending(s.cfg, app) {
				continue // an Up got there first
			}
			if !s.apps.Running(ctx, app) {
				waiting = append(waiting, app)
				continue
			}
			dir := filepath.Join(s.cfg.AppsDir(), app)
			log.Printf("%s: checking its data after the unclean boot", app)
			err := s.apps.WithBusy(app, func() error {
				return stackup.RunUncleanBoot(ctx, s.cfg, app, dir, stackup.AppFiles(dir))
			})
			if err != nil {
				log.Printf("%s: after the unclean boot: %v", app, err)
			}
		}
		if len(waiting) == 0 {
			return
		}
		if time.Now().After(deadline) {
			log.Printf("boot: not running, checked when next started: %s", strings.Join(waiting, ", "))
			return
		}
		pending = waiting
		select {
		case <-ctx.Done():
			return
		case <-time.After(repairPoll):
		}
	}
}

// closeUncleanBootIncidents resolves the unclean-boot warnings a day after they were
// last asserted. A failed repair is left open: it is closed by a repair that
// succeeds, or by the owner.
func (s *Server) closeUncleanBootIncidents(now time.Time) {
	for _, inc := range s.incidents.Snapshot().Open {
		quiet := inc.Kind == incident.KindHostUncleanBoot || inc.Kind == incident.KindAppRepaired
		if quiet && now.Sub(inc.Updated) >= repairQuiet {
			s.incidents.Resolve(inc.ID)
		}
	}
}
