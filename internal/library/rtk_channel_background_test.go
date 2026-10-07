// #1025's note — "RTK 0.51.0 is out, waiting for winget" — is a grey tag
// beside the version, and it is what the package manager is asked for. Asking
// winget is slow: `winget show` refreshes its sources, and on the machine this
// was measured on it took 7.7 seconds — while the read that draws the card waits
// for it at most 3. So the tag never arrived, and every opening of the tab paid
// the whole three seconds for an answer that was never used.
//
// The release itself (GitHub) is cheap and the tab needs it — it is what says
// "v0.51.0 is out" or "Up to date" — so that stays on the way out. The package
// manager's answer is asked in the background and kept for six hours (as
// rtkChannelLatest already does), so the next read of the tab draws with it.
package library

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/testenv"
)

func TestRTKCheckLatestDoesNotWaitForThePackageManager(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fakes are shell scripts")
	}
	h := sandbox(t)
	latestServer(t, "v0.51.0")

	// an rtk out of date, installed through winget, and a winget as slow as
	// the real one: `winget show` refreshes its sources
	links := filepath.Join(h, "AppData", "Local", "Microsoft", "WinGet", "Links")
	testenv.Program(t, filepath.Join(links, "rtk"), strings.ReplaceAll(versionedRTK, "VERSION", filepath.Join(links, "VERSION")))
	write(t, filepath.Join(links, "VERSION"), "0.50.0")
	tools := filepath.Join(h, "tools")
	testenv.Program(t, filepath.Join(tools, "winget"), `#!/bin/sh
/bin/sleep 5
printf 'Found RTK [rtk-ai.rtk]\r\nVersion: 0.50.0\r\n'
`)
	t.Setenv("PATH", links+string(os.PathListSeparator)+tools)
	up := upgraderOf
	upgraderOf = func(string) []string { return []string{"winget", "show", "--id", "rtk-ai.rtk", "--exact"} }
	t.Cleanup(func() { upgraderOf = up })

	// what the tab draws: the release, and no waiting tag the first time
	start := time.Now()
	v := ReadRTK()
	v.CheckLatest()
	drawn := time.Since(start)

	if v.Latest != "0.51.0" {
		t.Fatalf("Latest = %q, want 0.51.0: the release is what the card needs first", v.Latest)
	}
	// winget takes five seconds here; the draw must not wait for it
	if drawn > 2*time.Second {
		t.Errorf("drawing the card took %s: it waited for the package manager, which takes five", drawn)
	}
	// nothing has been said for the manager yet, so nothing is claimed about it
	if v.Waiting != "" {
		t.Errorf("Waiting = %q before the package manager has been asked, want none", v.Waiting)
	}

	// asked in the background: the answer is kept, and the next read of the tab
	// draws the tag without asking again
	CheckChannel(v)
	has, ok := rtkChannelCached(upgraderOf(v.Path))
	if !ok || has != "0.50.0" {
		t.Fatalf("what the package manager has = %q (kept: %v), want 0.50.0", has, ok)
	}
	start = time.Now()
	again := ReadRTK()
	again.CheckLatest()
	drewAgain := time.Since(start)
	if again.Waiting != "winget" || again.WaitingHas != "0.50.0" {
		t.Fatalf("the tag once the answer came: waiting=%q has=%q, want winget/0.50.0", again.Waiting, again.WaitingHas)
	}
	if drewAgain > 2*time.Second {
		t.Errorf("the second draw took %s: it asked the package manager again", drewAgain)
	}
}
