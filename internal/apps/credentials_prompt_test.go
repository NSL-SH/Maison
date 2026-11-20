package apps

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/yundera/maison/internal/xcomposeapp"
)

// prompt: false keeps a credential out of the first-click dialog (it stays in
// the tile menu) on a box with single sign-on; absent or true keeps the old
// behaviour. Without single sign-on, prompt: false is shown too: it is a gate's
// fallback password, and there it is the only way in.
func TestCredentialPrompt(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml           string
		withSSO, noSSO bool
	}{
		"absent":      {"credentials: [{username: admin, password: x}]", true, true},
		"true":        {"credentials: [{username: admin, password: x, prompt: true}]", true, true},
		"false":       {"credentials: [{username: admin, password: x, prompt: false}]", false, true},
		"one of two":  {"credentials: [{username: a, password: x, prompt: false}, {username: b, password: y}]", true, true},
		"empty entry": {"credentials: [{username: a, password: x, prompt: false}, {username: \"\"}]", false, true},
		"none at all": {"title: {en_us: x}", false, false},
	} {
		var ca xcomposeapp.App
		if err := yaml.Unmarshal([]byte(tc.yaml), &ca); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := promptsCredentials(&ca, true); got != tc.withSSO {
			t.Errorf("%s, with SSO: promptsCredentials = %v, want %v", name, got, tc.withSSO)
		}
		if got := promptsCredentials(&ca, false); got != tc.noSSO {
			t.Errorf("%s, without SSO: promptsCredentials = %v, want %v", name, got, tc.noSSO)
		}
	}
	if promptsCredentials(nil, false) {
		t.Error("nil app prompts")
	}
}

func TestSSOConfigured(t *testing.T) {
	for name, tc := range map[string]struct {
		env  map[string]string
		want bool
	}{
		"bare Maison":     {nil, false},
		"other keys only": {map[string]string{"APP_DOMAIN": "x"}, false},
		"registrar":       {map[string]string{"OIDC_REGISTRAR_URL": "http://auth-registrar:9092"}, true},
		"own provider":    {map[string]string{"OIDC_ISSUER_URL": "https://id.example.com"}, true},
		"client id only":  {map[string]string{"OIDC_CLIENT_ID": "x"}, false},
	} {
		if got := ssoConfigured(tc.env); got != tc.want {
			t.Errorf("%s: ssoConfigured = %v, want %v", name, got, tc.want)
		}
	}
}
