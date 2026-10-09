# purerawcli

Batch-process raw photos with DxO PureRAW 6 from the command line, on macOS and Windows.

> Unofficial. Not affiliated with or endorsed by DxO. DxO, PureRAW, DeepPRIME and Smart Lighting are
> trademarks of DxO Labs, used here only to describe compatibility.

## How it works

For each run, purerawcli:

1. adds a temporary processing preset with your settings to PureRAW's `ProcessingPresets.json` and selects it as the default preset (plus one preference that hides PureRAW's "Done!" dialog);
2. starts PureRAW in Lightroom-plugin "process using last settings" mode with a batch file of your images;
3. waits for PureRAW's completion signal (the trigger file the Lightroom importer watches) and reads the list of outputs;
4. restores your presets file and preferences exactly as they were, also on errors, timeouts and Ctrl-C.

What it changes is first recorded in a journal (`~/Library/Caches/purerawcli` on macOS, `%LOCALAPPDATA%\purerawcli`
on Windows). If a run is killed or the machine goes down mid-run, the next run restores the original settings
before doing anything else.

PureRAW shows a small progress window while it works and quits by itself when done.

## Requirements

- DxO PureRAW 6, installed and activated. Tested: 6.7 on macOS 27 (Apple Silicon), 6.6 on Windows 11 (x64). Other versions emit a warning.
- macOS 13.3 or later (PureRAW 6.7's own minimum; only macOS 27 has been tested), or Windows x64.
- To build: Go 1.27+, GNU make. On macOS also the Xcode Command Line Tools (`xcode-select --install`).

## Build

```sh
make              # native binary: bin/purerawcli (bin\purerawcli.exe on Windows)
make universal    # macOS: one binary for Apple Silicon and Intel
make windows      # Windows x64 binary, cross-built from macOS
make test         # needs PureRAW and a test image, see Testing
```

The macOS binary targets macOS 13.0 and later. On Windows, GNU make is available via scoop or Chocolatey
(`scoop install make`).

## Usage

```
purerawcli -o DIR [flags] FILE|DIR...
```

Quit PureRAW first. On Windows, closing its window leaves it in the notification area: quit it from the tray
icon. Directories are scanned for raw files, not recursively. Wildcards such as `shoot\*.arw` also work in
PowerShell and cmd, which leave them to the program (matched case-insensitively, file names only).
Outputs are written to `DIR` as `<name>.tif`, `<name>.dng` or `<name>.jpg`, and their paths are printed to stdout.

```sh
# DeepPRIME 3, 16-bit TIFF (the defaults)
purerawcli -o ~/Pictures/processed ~/Pictures/shoot

# DeepPRIME XD3, TIFF and compressed DNG, no lens distortion correction
purerawcli -o out -engine xd3 -format tiff,dng -dng-compression 1 -distortion=false shoot/*.ARW

# Process only what isn't done yet, and show the settings PureRAW applied
purerawcli -o out -skip-existing -v shoot
```

Existing outputs are refused unless you pass `-overwrite` or `-skip-existing`. An output that would replace
one of the input files is always refused, e.g. DNG to DNG in the same folder.

Files named explicitly must be raw files, and values outside the GUI's slider ranges are rejected.

Run `purerawcli -h` for the full list and ranges. Boolean flags are switched off with `=false`, e.g. `-vignetting=false`.

Exit status: 0 success, 1 failure (including images PureRAW didn't produce), 2 invalid arguments or inputs
(also `-h`), 130 interrupted or terminated.

## Notes

- Open PureRAW normally once after installing or updating it, so activation and welcome screens are out of the way.
- On macOS, the first run on files in Documents, Desktop, Downloads or external drives may show a privacy prompt
  for PureRAW.
- Only one job runs at a time; PureRAW itself is single-instance.
- Files PureRAW can't import (unsupported, or already processed by PureRAW) make it show an "Import Error" dialog
  that blocks the run until `-timeout` expires. On Windows, purerawcli then asks PureRAW to quit, which closes the
  dialog; if PureRAW finishes the other images, their outputs are reported and the bad file is listed as failed.
  On macOS PureRAW ignores the quit request while the dialog is open, so the whole batch fails: leave such files out.
- If Lightroom Classic with DxO's importer plugin is running, it may import the outputs into its catalog. Untested as I don't have Lightroom.

## Testing

`make test` includes an end-to-end test that processes one image with the installed PureRAW and checks that
PureRAW's settings are restored. Put a single camera raw file named `input.<ext>` (e.g. `input.dng`) in the
repository root; it's gitignored. It must not be a file PureRAW has already processed. PureRAW must be activated and not running. The test fails if the file is missing.

## AI disclosure

This tool is certified AI slop.
