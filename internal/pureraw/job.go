package pureraw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// lrVersion is passed as --lr-version. PureRAW insists the option is present
// but only logs the value (it looks Lightroom up and carries on when absent).
const lrVersion = "purerawcli"

// Job is one PureRAW launch processing a batch of images with one set of settings.
type Job struct {
	Inputs   []string       // absolute paths of raw files
	OutDir   string         // absolute output directory
	Settings map[string]any // ProcessingPresets.json field overrides
	Timeout  time.Duration
	Logf     func(format string, args ...any)
}

type Result struct {
	Outputs  map[string][]string // input path → output paths
	Missing  []string            // inputs PureRAW produced nothing for
	Settings []Setting           // effective settings from PureRAW's log
	Elapsed  time.Duration
}

// Run processes the job. The user's presets file and the preferences touched
// are restored before Run returns, including on error and cancellation.
func Run(ctx context.Context, p Paths, job Job) (res *Result, err error) {
	logf := job.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	unlock, err := lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if Running() {
		return nil, errors.New("DxO PureRAW 6 is running; quit it first (it only runs one instance at a time)")
	}
	if err := recoverInterrupted(logf); err != nil {
		return nil, fmt.Errorf("restoring PureRAW settings from an interrupted run: %w", err)
	}

	preset, unknown, err := BuildPreset(p, job.Settings, job.OutDir)
	if err != nil {
		return nil, err
	}
	for _, k := range unknown {
		logf("warning: preset field %q is not in PureRAW's default preset", k)
	}

	restore, err := install(p, preset)
	defer func() {
		if rerr := restore(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("restoring PureRAW settings: %w", rerr))
		}
	}()
	if err != nil {
		return nil, err
	}

	for _, f := range []string{p.Trigger(), p.ImportList()} {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	// Don't leave these for a Lightroom importer to pick up later, also when
	// the job times out or is cancelled after PureRAW wrote them.
	defer os.Remove(p.ImportList())
	defer os.Remove(p.Trigger())
	batchDir, err := os.MkdirTemp("", "purerawcli-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(batchDir)
	batch := filepath.Join(batchDir, "batch.txt")
	if err := os.WriteFile(batch, []byte(strings.Join(job.Inputs, "\n")), 0o600); err != nil {
		return nil, err
	}

	start := time.Now()
	logf("processing %d file(s) with DxO PureRAW 6…", len(job.Inputs))
	var exitErr *exec.ExitError
	if lerr := launch(ctx, p, batch, job.Timeout, logf); errors.As(lerr, &exitErr) {
		// Not fatal by itself (and hidden by open on macOS): the import list
		// says which images PureRAW did process.
		logf("warning: PureRAW exited with status %d", int32(exitErr.ExitCode()))
	} else if lerr != nil {
		return nil, lerr
	}
	res = &Result{Elapsed: time.Since(start)}

	if log, err := os.ReadFile(p.Log()); err == nil {
		res.Settings = parseEffectiveSettings(string(log))
	}
	list, err := os.ReadFile(p.ImportList())
	if err != nil {
		return res, fmt.Errorf("PureRAW finished without reporting results (see %s): %w", p.Log(), err)
	}
	outputs, orphans := parseImportList(string(list))
	for _, o := range orphans {
		logf("warning: output without a source: %s", o)
	}
	res.Outputs = matchOutputs(job.Inputs, outputs)
	for _, in := range job.Inputs {
		if len(res.Outputs[in]) == 0 {
			res.Missing = append(res.Missing, in)
		}
	}
	return res, nil
}

// matchOutputs keys PureRAW's reported outputs by our input paths, which may
// be spelled differently (slashes, case on Windows), and returns native paths.
func matchOutputs(inputs []string, reported map[string][]string) map[string][]string {
	byKey := map[string][]string{}
	for src, outs := range reported {
		for _, o := range outs {
			byKey[pathKey(src)] = append(byKey[pathKey(src)], filepath.FromSlash(o))
		}
	}
	matched := map[string][]string{}
	for _, in := range inputs {
		if outs := byKey[pathKey(in)]; len(outs) > 0 {
			matched[in] = outs
		}
	}
	return matched
}

// install adds preset to the user's presets, selects it for plugin mode and
// returns a function undoing both.
func install(p Paths, preset Preset) (restore func() error, err error) {
	restore = func() error { return nil }

	orig := &state{PresetsPath: p.Presets()}
	current, err := os.ReadFile(p.Presets())
	switch {
	case os.IsNotExist(err):
		// PureRAW creates the file from the shipped one on first launch.
		if current, err = os.ReadFile(p.ShippedPresets()); err != nil {
			return restore, err
		}
	case err != nil:
		return restore, err
	default:
		fi, err := os.Stat(p.Presets())
		if err != nil {
			return restore, err
		}
		orig.PresetsExisted, orig.Presets, orig.PresetsMode = true, current, fi.Mode().Perm()
	}
	presets, err := decodePresets(current)
	if err != nil {
		return restore, fmt.Errorf("parse %s: %w", p.Presets(), err)
	}
	presets, index := appendPreset(presets, preset)
	data, err := encodePresets(presets)
	if err != nil {
		return restore, err
	}
	for _, k := range []prefKey{prefDefaultPresetIndex, prefHideFinalDialog} {
		snap, err := readPref(k)
		if err != nil {
			return restore, err
		}
		orig.Prefs = append(orig.Prefs, snap)
	}

	if err := writeJournal(orig); err != nil {
		return restore, fmt.Errorf("writing journal: %w", err)
	}
	restore = func() error {
		if err := orig.restore(); err != nil {
			return err // keep the journal; the next run retries
		}
		return removeJournal()
	}
	if err := writeFileAtomic(p.Presets(), data, 0o644); err != nil {
		return restore, err
	}
	if err := writePref(prefDefaultPresetIndex, strconv.Itoa(index)); err != nil {
		return restore, err
	}
	return restore, writePref(prefHideFinalDialog, hideFinalDialog)
}

// launch starts PureRAW in Lightroom "process using last settings" mode and
// waits for it to exit or to report completion.
func launch(ctx context.Context, p Paths, batch string, timeout time.Duration, logf func(string, ...any)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := appCommand(p,
		"--as-lightroom-last-settings-plugin",
		"--lr-version="+lrVersion,
		"--batch-file="+batch)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false // the launched process has exited
	stop := func() {
		terminate()
		if !exited {
			<-done
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	var finishedAt time.Time
	for {
		select {
		case err := <-done:
			exited, done = true, nil // a nil channel is never ready again
			// Done only if PureRAW is gone too: on macOS the launched process
			// is `open`, which can end before PureRAW does.
			if ctx.Err() == nil && !Running() {
				if err != nil {
					return fmt.Errorf("%s: %w", filepath.Base(cmd.Path), err)
				}
				return nil
			}
		case <-ctx.Done():
			stop()
			return ctx.Err()
		case <-timer.C:
			stop()
			// PureRAW may finish anyway: asking it to quit can just dismiss a
			// blocking dialog (e.g. "Import Error" for one image), or it may
			// complete while terminate waits. The trigger comes after all
			// outputs, so the results are complete.
			if _, err := os.Stat(p.Trigger()); err == nil {
				logf("warning: PureRAW only finished after -timeout (%s) asked it to quit (a dialog may have blocked it)", timeout)
				return nil
			}
			return fmt.Errorf("PureRAW did not finish within %s", timeout)
		case <-poll.C:
			if exited && !Running() {
				return nil
			}
			// The trigger file is written after all outputs. If PureRAW then
			// lingers (its "Done!" dialog waits for a click), quit it.
			if finishedAt.IsZero() {
				if _, err := os.Stat(p.Trigger()); err == nil {
					finishedAt = time.Now()
				}
			} else if time.Since(finishedAt) > lingerGrace {
				stop()
				return nil
			}
		}
	}
}

// lingerGrace is how long PureRAW may stay open after reporting completion.
const lingerGrace = 5 * time.Second

// terminate asks PureRAW to quit, forcing it if it doesn't.
func terminate() {
	requestQuit()
	if waitExit(10 * time.Second) {
		return
	}
	forceQuit()
}

func waitExit(d time.Duration) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		if !Running() {
			return true
		}
	}
	return !Running()
}

// lockFile is the file lock locks, so that concurrent jobs fail fast instead
// of fighting over PureRAW's settings.
func lockFile() (*os.File, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o644)
}
