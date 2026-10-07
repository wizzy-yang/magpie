package agent

// Ante (antigma.ai) takes a provider of one's own in ~/.ante/catalog.json
// (ANTE_HOME moves the folder; the catalog is always $ANTE_HOME/catalog.json),
// merged on top of the providers it ships (docs.antigma.ai,
// reference/catalog-reference):
//
//	{"providers":{"<id>":{"display_name":…,"base_url":…,
//	  "wire_style":"OpenAiCompatible","auth":{"bearer":{"env_key":…}},
//	  "http_headers":{…},"extra_body":{…},
//	  "preferred_models":[{"id":…,"display_name":…,"context_limit":…,
//	    "max_tokens":…,"effort":…,"supported_efforts":[…],
//	    "support_vision":…}]}},
//	 "models":{…}}
//
// The providers map's key is what --provider takes; an id inside an entry is
// ignored. magpie adds the entry "magpie" at the gateway's /v1, on
// OpenAiCompatible — the wire style Ante speaks Chat Completions with, which
// is what the gateway takes every model on — and lists magpie's models in its
// preferred_models, Ante's model picker. The user's own providers and the
// models section are left as they are.
//
// No auth: the gateway listens on loopback and lets any token in from this
// machine (gateway.Token's comment, lan.go's identifyCaller), and Ante counts
// a provider with no auth block as authenticated. Writing a bearer env_key
// instead would mean magpie putting a key into Ante's environment, which the
// user would have to do themselves — the same decision Empryo's entry makes,
// which sends no key of the user's either. A gateway shared on the local
// network is another matter, and the same one every agent magpie wires has.
//
// Ante reads the catalog once per process (restart to pick an edit up) and
// parses it leniently: an entry that doesn't deserialize is skipped with a
// notice and the rest still load, so a bad file magpie wrote costs the user
// magpie's provider, not their own.

import (
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
)

const (
	anteProviders = "providers"
	anteProvider  = anteProviders + "." + magpieID
)

func ante(home string) *Agent { return anteAt(here(home)) }

// anteIn is Ante in a WSL distro (see wsl.go).
func anteIn(at place) *Agent { return anteAt(at) }

// anteAt is Ante with its home at at.home, reaching the gateway as at does.
func anteAt(at place) *Agent {
	gatewayV1 := at.v1
	dir := filepath.Join(at.home, ".ante")
	path := filepath.Join(dir, "catalog.json")
	// keyNew marks a file magpie made, which goes again once magpie's
	// provider is taken out of it and nothing is left
	keyNew := "ante:" + path + ":new"
	// wired: magpie's provider is in the catalog
	wired := func() bool { _, ok := edit.GetJSON(path, anteProvider); return ok }
	return &Agent{
		ID: "ante", Name: "Ante", Icon: "generic",
		Bin: "ante", Dir: dir, Path: path,
		Notice: func() string {
			if Running(`(^|/)ante( |$)`) {
				return "Ante reads its catalog at start-up — restart open ante sessions to use this."
			}
			return ""
		},
		Check: func() string {
			if !wired() {
				return ""
			}
			raw, err := edit.Read(path)
			if err != nil {
				return ""
			}
			e := gjson.GetBytes(raw, anteProvider)
			return wiringOff("Ante", path, func(k string) (string, bool) {
				r := e.Get(k)
				return r.String(), r.Exists()
			}, "base_url", gatewayV1())
		},
		Sync: func() error {
			if !wired() {
				return nil
			}
			return edit.SetJSON(path, edit.KV{Path: anteProvider, Value: anteProviderJSON(gatewayV1())})
		},
		Fields: []Field{{
			Key: "provider", Label: "provider",
			Get: func() string {
				if wired() {
					return magpieID
				}
				return ""
			},
			Set: func(v string) error {
				if v == "" {
					if !isFile(path) {
						return nil
					}
					made := unstash(keyNew) != ""
					// magpie's entry goes; a providers map left with
					// nothing goes with it, as Ante reads a missing
					// catalog as the providers it ships
					if err := edit.DelJSON(path, anteProvider); err != nil {
						return err
					}
					if raw, err := edit.Read(path); err == nil {
						if m := gjson.GetBytes(raw, anteProviders); m.Exists() && len(m.Map()) == 0 {
							if err := edit.DelJSON(path, anteProviders); err != nil {
								return err
							}
						}
					}
					// a file magpie made goes with its last entry
					if made {
						if raw, err := edit.Read(path); err == nil && len(gjson.ParseBytes(raw).Map()) == 0 {
							return edit.Remove(path)
						}
					}
					return nil
				}
				if !wired() && !isFile(path) {
					stash(map[string]string{keyNew: "1"})
				}
				return edit.SetJSON(path, edit.KV{Path: anteProvider, Value: anteProviderJSON(gatewayV1())})
			},
			Options: func(map[string]string) []Option {
				return []Option{{Value: magpieID, Label: "magpie", Icon: "magpie", Note: "every magpie model in Ante's model picker"}}
			},
		}},
	}
}

// anteProviderJSON is magpie's entry in Ante's catalog: the gateway's /v1 on
// OpenAiCompatible, every magpie model in preferred_models.
//
// A routing group is not told what it reasons at: which member answers, and
// what that member takes, is the group's own to decide per turn, so magpie's
// other agents leave an effort off for one too (magpieModels sets no APIs for
// a group for the same reason). Ante would otherwise ask for an effort the
// member the group picked doesn't have.
//
// support_vision is left alone for every model: Ante takes a model as seeing
// images unless it is told otherwise, and what magpie knows is only as good
// as a provider's own list — a relay's models are often not described at all,
// so writing false would have Ante drop the reader's images before the
// gateway, which describes them to a model that can't see them itself
// (provider.Described). No other agent magpie wires declares this either.
func anteProviderJSON(gw string) map[string]any {
	ms := []map[string]any{}
	for _, m := range magpieModels("ante") {
		group := strings.HasPrefix(m.ID, provider.GroupPrefix)
		e := map[string]any{"id": m.ID}
		if m.Name != "" && m.Name != m.ID {
			e["display_name"] = m.Name
		}
		if m.Context > 0 {
			e["context_limit"] = m.Context
		}
		if m.Output > 0 {
			e["max_tokens"] = maxTokens(m)
		}
		// Ante's ladder is per provider and model family; a model magpie
		// knows the levels of says so, and the gateway takes them
		if !group && len(m.Efforts) > 0 {
			e["effort"] = anteEffort(m.Efforts)
			e["supported_efforts"] = anteEfforts(m.Efforts)
		}
		ms = append(ms, e)
	}
	p := map[string]any{
		"display_name": "magpie",
		// gw is the gateway's /v1 already (place.v1), which is what Ante
		// joins its own path onto
		"base_url":   gw,
		"wire_style": "OpenAiCompatible",
	}
	if len(ms) > 0 {
		p["preferred_models"] = ms
	}
	return p
}

// anteEfforts are the levels Ante is told a model accepts, in its order
// (ascending): min, low, medium, high, xhigh, max. Ante's ladder has no
// "none" or "minimal" — magpie's two lowest, which ask a model for no
// thinking at all — so both are Ante's lowest rung, "min", which is the
// level it sends no effort at. A model whose only level is one of those
// accepts no effort setting at all, which Ante writes as a list of one.
func anteEfforts(levels []string) []string {
	out := []string{}
	for _, l := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		if !containsLevel(levels, l) {
			continue
		}
		if l == "none" || l == "minimal" {
			l = "min"
		}
		if !containsLevel(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// anteEffort is the level Ante starts a model on: the highest it accepts,
// as Ante picks a provider's heaviest model — magpie's own default for a
// model it knows no level of is left to Ante.
func anteEffort(levels []string) string {
	es := anteEfforts(levels)
	if len(es) == 0 {
		return ""
	}
	return es[len(es)-1]
}

func containsLevel(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
