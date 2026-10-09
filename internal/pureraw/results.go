package pureraw

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

// parseImportList parses the file PureRAW writes for its Lightroom importer
// plugin when a job is done:
//
//	<collection set lines…>
//	END_OF_COLLECTION_SET_SECTION
//	<collection name>END_OF_COLLECTION_NAME_SECTION
//	+ <parameter>
//	- <source>      sources, numbered from 0 in order of appearance
//	<n> <output>    output produced from source n
//	<output>        output without a source
//
// It returns the outputs per source path, and outputs without a source.
func parseImportList(data string) (map[string][]string, []string) {
	outputs := map[string][]string{}
	var sources, orphans []string
	inFiles := false
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if !inFiles {
			inFiles = strings.HasSuffix(line, "END_OF_COLLECTION_NAME_SECTION")
			continue
		}
		if line == "" {
			continue
		}
		head, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch {
		case head == "+":
		case head == "-":
			sources = append(sources, rest)
		case isIndex(head):
			n, _ := strconv.Atoi(head)
			if n < len(sources) {
				outputs[sources[n]] = append(outputs[sources[n]], rest)
			} else {
				orphans = append(orphans, rest)
			}
		default:
			orphans = append(orphans, strings.TrimSpace(line))
		}
	}
	return outputs, orphans
}

func isIndex(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil && s != ""
}

// Setting is one line of the "Start processing:" block in PureRAW's log.
type Setting struct{ Name, Value string }

// logLineStart matches the start of a new log record: a timestamp and the
// thread id, hex on macOS ("[0x16f3a7000]"), decimal on Windows ("[20808]").
var logLineStart = regexp.MustCompile(`^\d+\.\d+ \[(0x[0-9a-f]+|\d+)\] `)

// parseEffectiveSettings returns the settings PureRAW logged for the first
// image of a job, i.e. what it actually applied.
func parseEffectiveSettings(log string) []Setting {
	var settings []Setting
	in := false
	sc := bufio.NewScanner(strings.NewReader(log))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !in {
			in = strings.HasSuffix(line, "Start processing:")
			continue
		}
		if logLineStart.MatchString(line) {
			break
		}
		name, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			name, value = strings.TrimSuffix(strings.TrimSpace(line), ":"), ""
		}
		settings = append(settings, Setting{name, value})
	}
	return settings
}
