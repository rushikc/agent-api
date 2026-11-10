package tmux

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in          string
		maj, min    int
		ok          bool
		wantVersion bool
	}{
		{"tmux 3.3", 3, 3, true, true},
		{"tmux 3.3a", 3, 3, true, true},
		{"tmux 3.4", 3, 4, true, true},
		{"tmux 3.2a", 3, 2, true, false},
		{"tmux 2.9", 2, 9, true, false},
		{"tmux 4.0", 4, 0, true, true},
	}
	for _, tc := range cases {
		maj, min, err := ParseVersion(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if maj != tc.maj || min != tc.min {
			t.Errorf("%s: got %d.%d want %d.%d", tc.in, maj, min, tc.maj, tc.min)
		}
		if VersionOK(maj, min) != tc.wantVersion {
			t.Errorf("%s: VersionOK=%v want %v", tc.in, VersionOK(maj, min), tc.wantVersion)
		}
	}
}

func TestProjectFromSession(t *testing.T) {
	p, ok := ProjectFromSession("dh-krayon")
	if !ok || p != "krayon" {
		t.Fatalf("got %q %v", p, ok)
	}
	if _, ok := ProjectFromSession("dh-main"); ok {
		t.Fatal("dh-main should not be a project")
	}
	if _, ok := ProjectFromSession("dh-shadow-krayon"); ok {
		t.Fatal("shadow session should not be a project")
	}
}
