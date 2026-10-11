// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package ocpp

import "testing"

// The device model appendices parse from the embedded CSV files as they
// are published (the counts are those of the files in oca/).
func TestDeviceModelAppendices(t *testing.T) {
	for _, v := range []string{V201, V21} {
		dm, err := DeviceModelOf(v)
		if err != nil {
			t.Fatal(err)
		}
		if len(dm.Vars) != 438 || len(dm.Components) != 82 || len(dm.SecurityEvents) != 21 {
			t.Errorf("%s: %d variables, %d components, %d security events", v, len(dm.Vars), len(dm.Components), len(dm.SecurityEvents))
		}
		if !dm.SecurityEvents["FirmwareUpdated"] || dm.SecurityEvents["InvalidMessages"] {
			t.Errorf("%s: criticality not read", v)
		}
		found := false
		for _, r := range dm.Vars {
			if r.Component == "<generic>" && r.Variable == "ACCurrent" && r.DataType == "decimal" && r.Unit == "A" {
				found = true
			}
			if len(r.Unit) > 40 || len(r.DataType) > 40 {
				t.Errorf("%s: a description was read as a column: %+v", v, r)
			}
		}
		if !found {
			t.Errorf("%s: <generic> ACCurrent missing", v)
		}
	}
	if _, err := DeviceModelOf(V16); err == nil {
		t.Error("1.6 has no device model appendix")
	}
}
