// Command purerawcli batch-processes raw files with an installed, licensed
// DxO PureRAW 6 on macOS or Windows.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Dri0m/purerawcli/internal/pureraw"
)

const usage = `usage: purerawcli -o DIR [flags] FILE|DIR...

Processes raw files with DxO PureRAW 6 and writes the results to DIR.
Directories are scanned (not recursively) for raw files. PureRAW must not be
running while purerawcli runs. Output paths are printed to stdout.

flags:
`

// rawExtensions are the raw formats PureRAW 6 registers for (Info.plist).
var rawExtensions = map[string]bool{
	".3fr": true, ".4rw": true, ".arw": true, ".cr2": true, ".cr3": true, ".crw": true,
	".dng": true, ".fff": true, ".iiq": true, ".mrw": true, ".nef": true, ".nrw": true,
	".orf": true, ".ori": true, ".pef": true, ".raf": true, ".rw2": true, ".rwl": true, ".srw": true,
}

var engines = map[string]int{"dp3": 3, "xd3": 5}

// tiffDepths are TiffCompressionSavedValue values.
var tiffDepths = map[string]int{"8": 0, "8c": 1, "16": 2}

var formatExt = map[string]string{"dng": ".dng", "tiff": ".tif", "jpg": ".jpg"}

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("purerawcli", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage); fs.PrintDefaults() }

	out := fs.String("o", "", "output `directory` (required)")
	engine := fs.String("engine", "dp3", "denoising engine: dp3 (DeepPRIME 3) | xd3 (DeepPRIME XD3)")
	luminance := fs.Int("luminance", 40, "denoising luminance, 0-100")
	forceDetails := fs.Int("force-details", 0, "denoising force details, -100-100")
	lensSharpness := fs.Bool("lens-sharpness", true, "lens sharpness optimization")
	lensIntensity := fs.Int("lens-sharpness-intensity", 100, "lens sharpness intensity, 0-200")
	vignetting := fs.Bool("vignetting", true, "vignetting correction")
	chromatic := fs.Bool("chromatic-aberration", true, "chromatic aberration correction")
	distortion := fs.Bool("distortion", true, "lens distortion correction")
	crop := fs.Int("distortion-crop", 0, "distortion crop: 0 = cropped to original ratio, 1 = maximum rectangle, 2 = complete image area")
	dust := fs.Bool("dust", false, "dust removal")
	dustSensitivity := fs.Int("dust-sensitivity", 20, "dust removal sensitivity, 0-100")
	formats := fs.String("format", "tiff", "output formats, comma-separated: tiff,dng,jpg")
	tiffDepth := fs.String("tiff-depth", "16", "TIFF depth: 16 | 8 | 8c (8-bit compressed)")
	dngCompression := fs.Int("dng-compression", 0, "DNG compression: 0 = original (uncompressed), 1 = compressed")
	jpegQuality := fs.Int("jpeg-quality", 90, "JPG quality, 10-100")
	smartLighting := fs.Bool("smart-lighting", false, "DxO Smart Lighting")
	smartIntensity := fs.Int("smart-lighting-intensity", 25, "DxO Smart Lighting intensity, 0-100")
	var raw rawSettings
	fs.Var(&raw, "set", "raw preset `field=value` override, repeatable (JSON value; for experiments)")
	overwrite := fs.Bool("overwrite", false, "overwrite existing output files")
	skipExisting := fs.Bool("skip-existing", false, "skip inputs whose outputs already exist")
	timeout := fs.Duration("timeout", 0, "give up after this long (default 2m + 2m per file)")
	dryRun := fs.Bool("dry-run", false, "print the preset that would be used and exit")
	verbose := fs.Bool("v", false, "print the settings PureRAW actually applied")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	usageErr := func(format string, args ...any) int {
		fmt.Fprintf(os.Stderr, "purerawcli: "+format+"\n", args...)
		return 2
	}
	if *out == "" || fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	// The GUI's limits; PureRAW silently clamps values outside them.
	for _, r := range []struct {
		flag          string
		val, min, max int
	}{
		{"luminance", *luminance, 0, 100},
		{"force-details", *forceDetails, -100, 100},
		{"lens-sharpness-intensity", *lensIntensity, 0, 200},
		{"distortion-crop", *crop, 0, 2},
		{"dust-sensitivity", *dustSensitivity, 0, 100},
		{"dng-compression", *dngCompression, 0, 1},
		{"jpeg-quality", *jpegQuality, 10, 100},
		{"smart-lighting-intensity", *smartIntensity, 0, 100},
	} {
		if r.val < r.min || r.val > r.max {
			return usageErr("-%s must be between %d and %d", r.flag, r.min, r.max)
		}
	}
	if *timeout < 0 {
		return usageErr("-timeout must not be negative")
	}
	engineValue, ok := engines[*engine]
	if !ok {
		return usageErr("unknown -engine %q", *engine)
	}
	depthValue, ok := tiffDepths[*tiffDepth]
	if !ok {
		return usageErr("unknown -tiff-depth %q", *tiffDepth)
	}
	enabled := map[string]bool{}
	for _, f := range strings.Split(*formats, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "tif" {
			f = "tiff"
		}
		if _, ok := formatExt[f]; !ok {
			return usageErr("unknown -format %q", f)
		}
		enabled[f] = true
	}

	settings := map[string]any{
		"ProcessingTypedValue":             engineValue,
		"LuminanceValue":                   *luminance,
		"NoiseModelValue":                  *forceDetails, // GUI "Force details"
		"LensSoftnessActivatedValue":       *lensSharpness,
		"BlurIntensityValue":               *lensIntensity,
		"VignettingSavedValue":             *vignetting,
		"ChromaticAberrationSavedValue":    *chromatic,
		"LensDistortionActivatedValue":     *distortion,
		"AutoCropValue":                    *crop,
		"DustRemovalActivatedValue":        *dust,
		"DustRemovalSelectivityValue":      *dustSensitivity,
		"DngOutputFormat":                  enabled["dng"],
		"TiffOutputFormat":                 enabled["tiff"],
		"JpegOutputFormat":                 enabled["jpg"],
		"TiffCompressionSavedValue":        depthValue,
		"DngCompressionTypeSavedValue":     *dngCompression,
		"JpegQualitySavedValue":            *jpegQuality,
		"SmartLightingSavedValue":          *smartLighting,
		"SmartLightingIntensitySavedValue": *smartIntensity,
	}
	for k, v := range raw {
		settings[k] = v
	}

	outDir, err := filepath.Abs(*out)
	if err != nil {
		return usageErr("%v", err)
	}
	inputs, err := collectInputs(fs.Args())
	if err != nil {
		return usageErr("%v", err)
	}
	inputs, err = checkCollisions(inputs, outDir, enabled, *overwrite, *skipExisting)
	if err != nil {
		return usageErr("%v", err)
	}

	paths, err := pureraw.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, "purerawcli:", err)
		return 1
	}
	if !pureraw.Tested(paths.Version) {
		fmt.Fprintf(os.Stderr, "warning: DxO PureRAW %s has not been tested with purerawcli (tested: %s)\n",
			paths.Version, strings.Join(pureraw.TestedVersions, ", "))
	}
	if *dryRun {
		preset, unknown, err := pureraw.BuildPreset(paths, settings, outDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "purerawcli:", err)
			return 1
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(preset)
		for _, k := range unknown {
			fmt.Fprintf(os.Stderr, "warning: preset field %q is not in PureRAW's default preset\n", k)
		}
		fmt.Fprintf(os.Stderr, "%d input(s)\n", len(inputs))
		return 0
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "purerawcli: nothing to do")
		return 0
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "purerawcli:", err)
		return 1
	}
	if *timeout == 0 {
		*timeout = 2*time.Minute + time.Duration(len(inputs))*2*time.Minute
	}

	// SIGHUP: the terminal was closed.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	res, err := pureraw.Run(ctx, paths, pureraw.Job{
		Inputs:   inputs,
		OutDir:   outDir,
		Settings: settings,
		Timeout:  *timeout,
		Logf:     func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) },
	})
	if res != nil && *verbose {
		for _, s := range res.Settings {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", s.Name, s.Value)
		}
	}
	if errors.Is(err, context.Canceled) {
		if err != context.Canceled {
			fmt.Fprintln(os.Stderr, "purerawcli:", err) // e.g. restoring settings failed
		}
		fmt.Fprintln(os.Stderr, "purerawcli: interrupted")
		return 130
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "purerawcli:", err)
		return 1
	}
	for _, in := range inputs {
		for _, o := range res.Outputs[in] {
			fmt.Println(o)
		}
	}
	fmt.Fprintf(os.Stderr, "done: %d/%d file(s) in %s\n",
		len(inputs)-len(res.Missing), len(inputs), res.Elapsed.Round(time.Second))
	if len(res.Missing) > 0 {
		for _, m := range res.Missing {
			fmt.Fprintln(os.Stderr, "failed:", m)
		}
		fmt.Fprintf(os.Stderr, "see %s\n", paths.Log())
		return 1
	}
	return 0
}

// collectInputs resolves arguments to absolute raw file paths.
func collectInputs(args []string) ([]string, error) {
	args, err := expandWildcards(args)
	if err != nil {
		return nil, err
	}
	var inputs []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			inputs = append(inputs, p)
		}
	}
	for _, arg := range args {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return nil, err
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			if !rawExtensions[strings.ToLower(filepath.Ext(abs))] {
				return nil, fmt.Errorf("%s is not a raw file", arg)
			}
			add(abs)
			continue
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return nil, err
		}
		var found []string
		for _, e := range entries {
			// Skip hidden files, including macOS AppleDouble "._x.ARW" files.
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if !e.IsDir() && rawExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
				found = append(found, filepath.Join(abs, e.Name()))
			}
		}
		sort.Strings(found)
		for _, f := range found {
			add(f)
		}
	}
	return inputs, nil
}

// expandWildcards expands * and ? in file names on Windows, where the shell
// (cmd, PowerShell) leaves that to the program. Those characters can't occur
// in Windows file names, so there is no ambiguity. Matching ignores case, like
// Windows itself, and skips hidden files as a Unix shell does.
func expandWildcards(args []string) ([]string, error) {
	if runtime.GOOS != "windows" {
		return args, nil
	}
	var out []string
	for _, arg := range args {
		if !strings.ContainsAny(arg, "*?") {
			out = append(out, arg)
			continue
		}
		dir, pattern := filepath.Split(arg)
		if strings.ContainsAny(dir, "*?") {
			return nil, fmt.Errorf("%s: wildcards are only supported in file names", arg)
		}
		entries, err := os.ReadDir(filepath.Join(dir, "."))
		if err != nil {
			return nil, err
		}
		var matches []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(pattern, ".") {
				continue
			}
			ok, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(e.Name()))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", arg, err)
			}
			if ok {
				matches = append(matches, filepath.Join(dir, e.Name()))
			}
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("%s matches no files", arg)
		}
		sort.Strings(matches)
		out = append(out, matches...)
	}
	return out, nil
}

// checkCollisions predicts output names (<base><ext>) and refuses outputs that
// would overwrite existing files or each other. Replacing an input is refused
// even with -overwrite.
func checkCollisions(inputs []string, outDir string, formats map[string]bool, overwrite, skipExisting bool) ([]string, error) {
	var keep []string
	isInput := map[string]bool{}
	for _, in := range inputs {
		isInput[strings.ToLower(in)] = true
	}
	claimed := map[string]string{}
	for _, in := range inputs {
		base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
		allExist := true
		var existing []string
		for f := range formats {
			o := filepath.Join(outDir, base+formatExt[f])
			if isInput[strings.ToLower(o)] {
				return nil, fmt.Errorf("output %s would replace an input file; choose another -o", o)
			}
			if prev, ok := claimed[strings.ToLower(o)]; ok {
				return nil, fmt.Errorf("%s and %s would both write %s", prev, in, o)
			}
			claimed[strings.ToLower(o)] = in
			if _, err := os.Stat(o); err == nil {
				existing = append(existing, o)
			} else {
				allExist = false
			}
		}
		switch {
		case skipExisting && allExist:
			fmt.Fprintln(os.Stderr, "skipping (outputs exist):", in)
			continue
		case len(existing) > 0 && !overwrite:
			return nil, fmt.Errorf("%s exists (use -overwrite or -skip-existing)", existing[0])
		}
		keep = append(keep, in)
	}
	return keep, nil
}

// rawSettings collects -set field=value flags.
type rawSettings map[string]any

func (r *rawSettings) String() string { return fmt.Sprint(map[string]any(*r)) }

func (r *rawSettings) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return errors.New("want field=value")
	}
	if *r == nil {
		*r = rawSettings{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(v), &parsed); err != nil {
		parsed = v // bare string
	}
	(*r)[k] = parsed
	return nil
}
