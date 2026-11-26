package bt

import "testing"

func TestParseDevices(t *testing.T) {
	out := `Device 6D:51:00:E3:8F:F9 6D-51-00-E3-8F-F9
Device 40:8C:4C:49:C3:C3 GTX-R10i Pro
Device A5:C2:37:12:AD:4C SP22S003BP8S100A
`
	devices := ParseDevices(out)
	if len(devices) != 3 {
		t.Fatalf("devices = %d, want 3: %#v", len(devices), devices)
	}
	// Named devices come first, even when they appear later in the output.
	if devices[0].Name != "GTX-R10i Pro" || !devices[0].Named {
		t.Fatalf("first = %#v", devices[0])
	}
	if devices[1].Name != "SP22S003BP8S100A" || !devices[1].Named {
		t.Fatalf("second = %#v", devices[1])
	}
	if devices[2].Named || devices[2].MAC != "6D:51:00:E3:8F:F9" {
		t.Fatalf("alias row = %#v", devices[2])
	}
}
