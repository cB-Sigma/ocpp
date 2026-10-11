// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

// The Open Charge Alliance's files are embedded as published: oca/1.6,
// oca/2.0.1 and oca/2.1 each hold the part 3 JSON schemas (and for 1.6 the
// OCPP-S WSDLs, for 2.x the part 2 appendix CSV files) with the folder's
// README.md (attribution) and SHA256SUMS. They are licensed under CC BY-ND
// 4.0, not under this module's licence (see NOTICE), so they are never
// changed: the schemas are compiled and the CSV files parsed in memory when
// a version is first used, and a correction the OCA publishes in an errata
// is applied in code. TestOCAFilesUnmodified checks every file against
// SHA256SUMS.
//
//go:embed oca
var ocaFS embed.FS

// OCAFiles is the embedded OCA files, rooted at the version folders
// ("1.6", "2.0.1", "2.1").
func OCAFiles() fs.FS {
	sub, _ := fs.Sub(ocaFS, "oca")
	return sub
}

// versionDir is the oca/ folder of a version ("ocpp2.1" -> "2.1").
func versionDir(version string) string {
	if ss := schemaSets[version]; ss != nil {
		return ss.dir
	}
	return ""
}

// SchemaFiles reads the schema files of an oca/ folder ("1.6", "2.0.1" or
// "2.1"), by file name without .json, as published.
func SchemaFiles(dir string) (map[string][]byte, error) {
	ents, err := fs.ReadDir(ocaFS, path.Join("oca", dir, "schemas"))
	if err != nil {
		return nil, fmt.Errorf("the OCA schemas of %s are missing: %w", dir, err)
	}
	out := make(map[string][]byte, len(ents))
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := fs.ReadFile(ocaFS, path.Join("oca", dir, "schemas", e.Name()))
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = b
	}
	return out, nil
}

// SchemaFile is the OCA schema file of action's request or response in
// version, as published, or nil when there is none.
func SchemaFile(version, action string, response bool) []byte {
	ss := schemaSets[version]
	if ss == nil {
		return nil
	}
	return ss.raw()[schemaName(version, action, response)]
}

// DMVar is one row of a device model appendix (dm_components_vars.csv):
// the component ("<generic>" for a variable any component may have), the
// variable, its instance, the Required? column as written, the data type
// and the unit.
type DMVar struct {
	Component, Variable, Instance, Required, DataType, Unit string
}

// DeviceModel is what a version's part 2 appendices say about the device
// model: its components and variables, the standardized component names,
// and the security events with their criticality. Descriptions are not
// read.
type DeviceModel struct {
	Vars           []DMVar
	Components     []string
	SecurityEvents map[string]bool // event name -> the Critical column says yes
}

var (
	dmMu    sync.Mutex
	dmCache = map[string]*DeviceModel{}
)

// DeviceModelOf parses the device model appendix of version (V201 or V21)
// from the embedded CSV files, once.
func DeviceModelOf(version string) (*DeviceModel, error) {
	dmMu.Lock()
	defer dmMu.Unlock()
	if dm, ok := dmCache[version]; ok {
		return dm, nil
	}
	dir := versionDir(version)
	if dir == "" || version == V16 {
		return nil, fmt.Errorf("%s has no device model appendix", version)
	}
	base := path.Join("oca", dir, "appendices")
	vars, err := readSemicolonCSV(path.Join(base, "dm_components_vars.csv"), 7)
	if err != nil {
		return nil, err
	}
	comps, err := readSemicolonCSV(path.Join(base, "components.csv"), 2)
	if err != nil {
		return nil, err
	}
	events, err := readSemicolonCSV(path.Join(base, "security_events.csv"), 3)
	if err != nil {
		return nil, err
	}
	dm := &DeviceModel{SecurityEvents: map[string]bool{}}
	for _, r := range vars {
		if r[0] == "" && r[1] == "" {
			continue
		}
		dm.Vars = append(dm.Vars, DMVar{r[0], r[1], r[2], r[3], r[4], r[5]})
	}
	for _, r := range comps {
		if r[0] != "" {
			dm.Components = append(dm.Components, r[0])
		}
	}
	for _, r := range events {
		if r[0] != "" {
			dm.SecurityEvents[r[0]] = strings.EqualFold(r[2], "yes")
		}
	}
	dmCache[version] = dm
	return dm, nil
}

// MustDeviceModel is DeviceModelOf for the embedded files, which are known
// to parse (TestDeviceModelAppendices); it panics otherwise.
func MustDeviceModel(version string) *DeviceModel {
	dm, err := DeviceModelOf(version)
	if err != nil {
		panic(err)
	}
	return dm
}

// readSemicolonCSV reads an OCA appendix file: semicolon-separated, a
// header line, at most cols fields per line (the last one, a description,
// may hold semicolons), fields trimmed.
func readSemicolonCSV(name string, cols int) ([][]string, error) {
	b, err := fs.ReadFile(ocaFS, name)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var rows [][]string
	for i, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header
		}
		f := strings.SplitN(line, ";", cols)
		for len(f) < cols {
			f = append(f, "")
		}
		for j := range f {
			f[j] = strings.TrimSpace(f[j])
		}
		rows = append(rows, f)
	}
	return rows, nil
}
