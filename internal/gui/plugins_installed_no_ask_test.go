package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/plugin"
)

// The Plugins page's Installed list is what was added, each row naming the
// subscriptions its plugin signs in to. Those names are what the plugins said
// the last time magpie asked them — but asking them is the whole of the
// providers RPC, which walks the plugins one at a time and has each fetch its
// vendor's model list over the network. The page here has no use for those
// models, so waiting on them made Installed take seconds to draw: one vendor
// after another, every time it was opened, the whole list held for the
// slowest of them.
//
// What the page reads instead is what plugin.Cached keeps: the names, without
// starting the host or asking anyone, with the plugins asked again in the
// background (Cached's own refresh) when a sign-in or the plugins changed
// since. So a row's names come from there, and asking the plugins is not on
// the way to drawing the list.

// pluginsSandbox points the magpie folders at a test's own, so the page reads
// a plugins.json the test wrote.
func pluginsSandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	return appdir.Config()
}

// TestPluginsStateNamesWithoutAsking: a row's subscriptions are named from
// what the plugins last said, and the host is not asked for them to draw the
// Installed list. What the ask would cost is the whole of the providers RPC —
// every plugin's vendor, one at a time — and nothing here needs its models.
func TestPluginsStateNamesWithoutAsking(t *testing.T) {
	cfg := pluginsSandbox(t)
	const spec = "@magpie-community/opencode-copilot-auth@latest"
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "plugins.json"),
		[]byte(`{"plugins":[{"spec":"`+spec+`"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// what the plugins said the last time magpie asked them
	plugin.UseCached([]plugin.Provider{{ID: "github-copilot", Spec: spec, Name: "GitHub Copilot"}})
	t.Cleanup(func() { plugin.UseCached(nil) })

	// with a context that would give up at once if anything waited on the host
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := pluginsState(ctx, nil)

	var got []string
	found := false
	for _, e := range s.Plugins {
		if e.Spec == spec {
			got, found = e.Providers, true
		}
	}
	if !found {
		t.Fatal("the installed plugin isn't in the list")
	}
	if len(got) != 1 || got[0] != "GitHub Copilot" {
		t.Fatalf("the row's subscriptions = %v, want [GitHub Copilot]", got)
	}
}
