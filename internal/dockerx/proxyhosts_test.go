package dockerx

import (
	"sort"
	"testing"
)

func TestProxyHosts(t *testing.T) {
	got := proxyHosts(map[string]string{
		"caddy_0":                    "Admin-Alice.nsl.sh",
		"caddy_1":                    "admin.alice.nsl.sh, admin-1-2-3-4.ip.nsl.sh",
		"caddy_1.reverse_proxy":      "{{upstreams 80}}",
		"caddy":                      "x.example.com",
		"caddy_x":                    "nope.example.com",
		"caddy_":                     "nope2.example.com",
		"com.docker.compose.service": "admin",
	})
	sort.Strings(got)
	want := []string{"admin-1-2-3-4.ip.nsl.sh", "admin-alice.nsl.sh", "admin.alice.nsl.sh", "x.example.com"}
	if len(got) != len(want) {
		t.Fatalf("proxyHosts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("proxyHosts = %v, want %v", got, want)
		}
	}
}
