package server

import "testing"

func TestDashboardURL(t *testing.T) {
	for host, want := range map[string]string{
		"jellyfin.alice.nsl.sh":      "https://maison.alice.nsl.sh",
		"Jellyfin.Alice.nsl.sh:443":  "https://maison.alice.nsl.sh",
		"jellyfin-alice.nsl.sh":      "https://maison-alice.nsl.sh",
		"jellyfin-1-2-3-4.sslip.io":  "https://maison-alice.nsl.sh",
		"jellyfin-1-2-3-4.ip.nsl.sh": "https://maison-alice.nsl.sh",
		"":                           "https://maison-alice.nsl.sh",
	} {
		if got := dashboardURL(host, "alice.nsl.sh"); got != want {
			t.Errorf("dashboardURL(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestSameForm(t *testing.T) {
	const d = "alice.nsl.sh"
	for _, c := range []struct{ url, host, want string }{
		{"https://admin-alice.nsl.sh/", "maison.alice.nsl.sh", "https://admin.alice.nsl.sh/"},
		{"https://admin-alice.nsl.sh/support", "Maison.Alice.nsl.sh:443", "https://admin.alice.nsl.sh/support"},
		{"https://admin-alice.nsl.sh/?a=b", "maison.alice.nsl.sh", "https://admin.alice.nsl.sh/?a=b"},
		{"https://admin-alice.nsl.sh/", "maison-alice.nsl.sh", "https://admin-alice.nsl.sh/"},
		{"https://admin-alice.nsl.sh/", "maison-1-2-3-4.sslip.io", "https://admin-alice.nsl.sh/"},
		{"https://admin-alice.nsl.sh/", "", "https://admin-alice.nsl.sh/"},
		{"https://support.example.com/", "maison.alice.nsl.sh", "https://support.example.com/"},
		{"https://admin-alice.nsl.sh:8443/", "maison.alice.nsl.sh", "https://admin-alice.nsl.sh:8443/"},
		{"https://x.admin-alice.nsl.sh/", "maison.alice.nsl.sh", "https://x.admin-alice.nsl.sh/"},
		{"", "maison.alice.nsl.sh", ""},
	} {
		if got := sameForm(c.url, c.host, d); got != c.want {
			t.Errorf("sameForm(%q, %q) = %q, want %q", c.url, c.host, got, c.want)
		}
	}
}
