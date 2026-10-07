package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// Ante's catalog.json is merged on top of the providers it ships: magpie's
// entry is added to the providers map under its own id, with the user's own
// providers and the models section left as they are, and it goes again when
// magpie steps out. A file magpie made goes with it.
func TestAnte(t *testing.T) {
	home, _ := museHome(t)
	path := filepath.Join(home, ".ante", "catalog.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	// a user's own: a provider of theirs, a model of its, and a patch
	orig := `{
  "providers": {
    "together": {
      "display_name": "Together AI",
      "base_url": "https://api.together.xyz/v1",
      "wire_style": "OpenAiCompatible",
      "auth": { "bearer": { "env_key": "TOGETHER_API_KEY" } }
    }
  },
  "models": { "claude-opus-5-5": { "effort": "max" } }
}
`
	os.WriteFile(path, []byte(orig), 0o644)
	read := func() map[string]any {
		t.Helper()
		var m map[string]any
		b, _ := os.ReadFile(path)
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%v\n%s", err, b)
		}
		return m
	}
	providers := func() map[string]any {
		t.Helper()
		ps, _ := read()["providers"].(map[string]any)
		return ps
	}
	var want map[string]any
	json.Unmarshal([]byte(orig), &want)

	a := ante(home)
	if a.Path != path || a.Dir != filepath.Dir(path) || a.Bin != "ante" {
		t.Fatalf("path %q dir %q bin %q", a.Path, a.Dir, a.Bin)
	}
	f := a.Field("provider")
	// not wired yet: the user's provider is there, magpie's is not, and
	// nothing is said about wiring magpie never did
	if got := f.Get(); got != "" || a.Check() != "" {
		t.Fatalf("own: %q %q", got, a.Check())
	}
	var vals []string
	for _, o := range f.Options(a.Values()) {
		vals = append(vals, o.Value)
	}
	if !reflect.DeepEqual(vals, []string{"magpie"}) {
		t.Fatalf("options: %v", vals)
	}

	for range 2 { // twice: one magpie provider, and the user's left alone
		if err := f.Set("magpie"); err != nil {
			t.Fatal(err)
		}
	}
	m := read()
	ps := providers()
	if len(ps) != 2 || !reflect.DeepEqual(ps["together"], want["providers"].(map[string]any)["together"]) {
		t.Fatalf("providers: %v", ps)
	}
	if !reflect.DeepEqual(m["models"], want["models"]) {
		t.Fatalf("models: %v", m["models"])
	}
	mine, _ := ps["magpie"].(map[string]any)
	if mine["base_url"] != gatewayV1() || mine["wire_style"] != "OpenAiCompatible" || mine["display_name"] != "magpie" {
		t.Fatalf("magpie provider: %v", mine)
	}
	// no auth block: the gateway lets this machine in with any token, and
	// Ante counts a provider with none as authenticated
	if _, ok := mine["auth"]; ok {
		t.Fatalf("auth written: %v", mine["auth"])
	}
	ms, _ := mine["preferred_models"].([]any)
	if len(ms) == 0 {
		t.Fatalf("no models: %v", mine)
	}
	for _, x := range ms {
		e, _ := x.(map[string]any)
		id, _ := e["id"].(string)
		if id == "" {
			t.Fatalf("a model without an id: %v", e)
		}
		// support_vision is never written: Ante takes a model as seeing
		// images unless told otherwise, and magpie's own answer is only as
		// good as a provider's list — a relay's models are often not
		// described — so writing false would drop the reader's images
		if _, ok := e["support_vision"]; ok {
			t.Fatalf("%s: support_vision written: %v", id, e)
		}
	}
	if f.Get() != "magpie" || a.Check() != "" {
		t.Fatalf("on: %q %q", f.Get(), a.Check())
	}
	// magpie's provider pointed elsewhere
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), gatewayV1(), "http://elsewhere/v1", 1)), 0o644)
	if !strings.Contains(a.Check(), "http://elsewhere/v1") {
		t.Fatalf("check: %q", a.Check())
	}
	os.WriteFile(path, b, 0o644)

	// Sync keeps it pointed at the gateway magpie is on now
	os.WriteFile(path, []byte(strings.Replace(string(b), gatewayV1(), "http://elsewhere/v1", 1)), 0o644)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if got, _ := providers()["magpie"].(map[string]any)["base_url"]; got != gatewayV1() {
		t.Fatalf("sync: %v", got)
	}

	// stepping out takes magpie's provider away and leaves the user's file
	if err := f.Set(""); err != nil {
		t.Fatal(err)
	}
	if m := read(); !reflect.DeepEqual(m, want) {
		t.Fatalf("restore: %v", m)
	}

	// a file magpie made goes again
	os.Remove(path)
	if err := f.Set("magpie"); err != nil {
		t.Fatal(err)
	}
	if _, ok := providers()["magpie"]; !ok {
		t.Fatalf("new: %v", read())
	}
	if err := f.Set(""); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err == nil {
		t.Fatalf("made file left: %s", b)
	}
}

// A catalog Ante can't parse costs magpie's provider, not the user's: Ante
// skips an entry that doesn't deserialize and loads the rest, and magpie
// writes only its own key.
func TestAnteKeepsTheUsersFile(t *testing.T) {
	home, _ := museHome(t)
	path := filepath.Join(home, ".ante", "catalog.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	orig := `{"providers":{"together":{"base_url":"https://api.together.xyz/v1","wire_style":"OpenAiCompatible"}}}`
	os.WriteFile(path, []byte(orig), 0o644)
	a := ante(home)
	if err := a.Field("provider").Set("magpie"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("the file is not JSON: %v\n%s", err, b)
	}
	ps, _ := m["providers"].(map[string]any)
	if _, ok := ps["together"]; !ok {
		t.Fatalf("the user's provider went: %v", ps)
	}
	if _, ok := ps[magpieID]; !ok {
		t.Fatalf("magpie's provider went: %v", ps)
	}
}

// A routing group says neither what it reasons at nor what it takes in: which
// member answers, and what that member can do, is the group's own to decide
// per turn. magpie's other agents leave both off for one, and Ante would
// otherwise filter images out before a group whose member takes them, or ask
// for an effort the member the group picked doesn't have.
func TestAnteGroupSaysNoCapabilities(t *testing.T) {
	home, _ := museHome(t)
	path := filepath.Join(home, ".ante", "catalog.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	// a group of magpie's own, over a model of a provider of the user's
	provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k",
		Chat: "http://127.0.0.1:1/v1", Models: []string{"m1"}})
	if err := provider.SaveGroup(provider.Group{ID: "g", Name: "G", Members: []string{"relay/m1"}}); err != nil {
		t.Fatal(err)
	}
	a := ante(home)
	if err := a.Field("provider").Set("magpie"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	ms, _ := m["providers"].(map[string]any)["magpie"].(map[string]any)["preferred_models"].([]any)
	var sawGroup bool
	for _, x := range ms {
		e, _ := x.(map[string]any)
		id, _ := e["id"].(string)
		if !strings.HasPrefix(id, provider.GroupPrefix) {
			continue
		}
		sawGroup = true
		for _, k := range []string{"effort", "supported_efforts", "support_vision"} {
			if _, ok := e[k]; ok {
				t.Fatalf("the group %s declares %s: %v", id, k, e)
			}
		}
	}
	if !sawGroup {
		t.Fatalf("no group in the list: %v", ms)
	}
}

// The effort ladder is Ante's (min, low, …, max): magpie's "none" and
// "minimal" — its two lowest — are Ante's "min", the rungs come out in
// Ante's order whatever order magpie lists them in, and a model magpie
// knows no level of carries no effort at all, leaving Ante its built-in
// ladder.
func TestAnteEfforts(t *testing.T) {
	for _, c := range []struct {
		name   string
		levels []string
		want   []string
		effort string
	}{
		{"a ladder", []string{"low", "medium", "high"}, []string{"low", "medium", "high"}, "high"},
		{"none is min", []string{"none", "high"}, []string{"min", "high"}, "high"},
		{"magpie's order, Ante's ladder", []string{"max", "low"}, []string{"low", "max"}, "max"},
		{"none alone is min", []string{"none"}, []string{"min"}, "min"},
		{"minimal is Ante's min", []string{"minimal"}, []string{"min"}, "min"},
		{"nothing known", nil, []string{}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := anteEfforts(c.levels)
			if len(got) != len(c.want) {
				t.Fatalf("anteEfforts(%v) = %v, want %v", c.levels, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("anteEfforts(%v) = %v, want %v", c.levels, got, c.want)
				}
			}
			if e := anteEffort(c.levels); e != c.effort {
				t.Fatalf("anteEffort(%v) = %q, want %q", c.levels, e, c.effort)
			}
		})
	}
}
