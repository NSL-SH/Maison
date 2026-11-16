package apps

import "testing"

func TestDotURL(t *testing.T) {
	hosts := map[string]bool{"admin-alice.nsl.sh": true, "admin.alice.nsl.sh": true, "legacy-alice.nsl.sh": true}
	for _, c := range []struct{ url, want string }{
		{"https://admin-alice.nsl.sh/", "https://admin.alice.nsl.sh/"},
		{"https://Admin-Alice.nsl.sh/x?y=1", "https://admin.alice.nsl.sh/x?y=1"},
		{"https://legacy-alice.nsl.sh/", ""},
		{"https://admin.alice.nsl.sh/", ""},
		{"https://admin-alice.nsl.sh:8443/", ""},
		{"http://alice.nsl.sh:8080/", ""},
		{"", ""},
	} {
		if got := dotURL(c.url, "alice.nsl.sh", hosts); got != c.want {
			t.Errorf("dotURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
	if got := dotURL("https://admin-alice.nsl.sh/", "alice.nsl.sh", nil); got != "" {
		t.Errorf("no hosts: %q", got)
	}
}
