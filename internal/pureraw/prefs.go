package pureraw

// PureRAW stores its preferences with Qt's QSettings: in the macOS defaults
// system or the Windows registry.

type prefKind int

const (
	prefInt prefKind = iota
	prefString
)

type prefKey struct {
	name string
	kind prefKind // how Qt stores the value
}

var (
	prefDefaultPresetIndex = prefKey{"ProcessingPresetsDefaultIndex", prefInt}
	prefHideFinalDialog    = prefKey{"dontShowLRFinalDialog", prefString}
)

// hideFinalDialog is the dontShowLRFinalDialog value that suppresses the
// "Done!" dialog. Despite the key's name, "0" hides it and "1" shows it.
const hideFinalDialog = "0"
