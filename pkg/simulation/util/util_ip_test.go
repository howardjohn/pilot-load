package util

import "testing"

func TestSetIPRange(t *testing.T) {
	t.Cleanup(func() { _ = SetIPRange(DefaultIPRange) })
	if err := SetIPRange("10.200.0.254/30"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"10.200.0.254", "10.200.0.255"} {
		if got := GetIP(); got != want {
			t.Fatalf("GetIP() = %s, want %s", got, want)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("GetIP handed out an address outside 10.200.0.252/30")
			}
		}()
		GetIP()
	}()
	for _, bad := range []string{"10.0.0.10", "not-an-ip/8", "fd00::1/64"} {
		if err := SetIPRange(bad); err == nil {
			t.Fatalf("SetIPRange(%q) succeeded", bad)
		}
	}
}

func TestDefaultIPRange(t *testing.T) {
	t.Cleanup(func() { _ = SetIPRange(DefaultIPRange) })
	if err := SetIPRange(DefaultIPRange); err != nil {
		t.Fatal(err)
	}
	if got := GetIP(); got != "10.0.0.10" {
		t.Fatalf("GetIP() = %s, want 10.0.0.10 (unchanged default)", got)
	}
}
