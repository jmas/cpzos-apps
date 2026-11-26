package wifi

import "testing"

func TestParseNetworks(t *testing.T) {
	out := `                              Network name  Security  Signal
---------------------------------------------------------------------------
  >                           Home Net      psk       ****
                              Cafe          open      **
`
	got := ParseNetworks(out)
	if len(got) != 2 {
		t.Fatalf("networks = %d, want 2: %#v", len(got), got)
	}
	if got[0].SSID != "Home Net" || got[0].Security != "psk" || got[0].Signal != "****" {
		t.Fatalf("first = %#v", got[0])
	}
	if !got[0].Active {
		t.Fatal("first network should be active")
	}
	if got[1].SSID != "Cafe" || got[1].Security != "open" {
		t.Fatalf("second = %#v", got[1])
	}
	if Secure(got[1]) {
		t.Fatal("open network should not require a passphrase")
	}
	if !Secure(got[0]) {
		t.Fatal("psk network should require a passphrase")
	}
	if !IsSaved([]string{"Cafe"}, "Cafe") || IsSaved([]string{"Cafe"}, "Home Net") {
		t.Fatal("saved lookup failed")
	}
}

func TestPoweredFormats(t *testing.T) {
	if on, ok := parsePoweredShow("Powered: yes\n"); !ok || !on {
		t.Fatalf("show yes: on=%v ok=%v", on, ok)
	}
	if on, ok := parsePoweredShow("Powered  on\n"); !ok || !on {
		t.Fatalf("show on: on=%v ok=%v", on, ok)
	}
	table := `Name    Address    Powered  Adapter  Mode
wlan0   aa:bb:cc   on       phy0     station
wlan1   dd:ee:ff   off      phy1     station
`
	if on, ok := parsePoweredList(table, "wlan0"); !ok || !on {
		t.Fatalf("list on: on=%v ok=%v", on, ok)
	}
	if on, ok := parsePoweredList(table, "wlan1"); !ok || on {
		t.Fatalf("list off: on=%v ok=%v", on, ok)
	}
}

func TestParseRealIWCTLOutput(t *testing.T) {
	raw := "\x1b[1;90m      Network name                      Security            Signal\n" +
		"\x1b[0m\x1b[90m--------------------------------------------------------------------------------\n" +
		"\x1b[0m      SLAVA UKRAINI 🇺🇦                      psk                 **\x1b[1;90m**\x1b[0m\n" +
		"\x1b[0m      eWiFi                             open                *\x1b[1;90m***\x1b[0m\n"
	got := ParseNetworks(stripANSI(raw))
	if len(got) != 2 {
		t.Fatalf("networks = %d, want 2: %#v", len(got), got)
	}
	if got[0].SSID != "SLAVA UKRAINI 🇺🇦" || got[0].Security != "psk" || got[0].Signal != "****" {
		t.Fatalf("first = %#v", got[0])
	}
	if got[1].SSID != "eWiFi" || Secure(got[1]) {
		t.Fatalf("second = %#v", got[1])
	}
}

func TestParsePoweredShowTable(t *testing.T) {
	in := "                                 Device: wlan0\n" +
		"  --------------------------------------------------------------------------------\n" +
		"            Name                  wlan0\n" +
		"         *  Powered               on\n"
	if on, ok := parsePoweredShow(in); !ok || !on {
		t.Fatalf("table: on=%v ok=%v", on, ok)
	}
}
