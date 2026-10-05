package gui

import (
	"os"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// A routing group's editor reads each member off the served catalog entry:
// whether agents are told it takes images, whether anything said so either
// way, the levels it reasons at and the window it holds. The first is the
// same expression the gateway decides by when it describes an image for a
// model that can't see one (blindTo reads Entry.Images), so the editor cannot
// say a member sees images that the gateway would describe them for.
func TestGroupModelsCarryWhatTheyTake(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	yes, no := true, false
	if err := provider.Save(provider.Provider{ID: "p", Name: "P", Key: "k", Chat: "http://127.0.0.1:1/v1",
		Models: []string{"eye", "text", "mystery", "mine"}}); err != nil {
		t.Fatal(err)
	}
	// what each model's own list says: one sees, one takes none, one says
	// nothing at all
	if err := catalog.SaveLive("p", "http://127.0.0.1:1/v1", []catalog.Model{
		{ID: "eye", Context: 1048576, Efforts: []string{"low", "medium", "high"}, Images: true, ImageInput: &yes},
		{ID: "text", Context: 200000, ImageInput: &no},
		{ID: "mystery", Context: 128000},
		{ID: "mine", Context: 64000, ImageInput: &no},
	}); err != nil {
		t.Fatal(err)
	}
	// the user's own answer for one its list said took none
	st := settings.Load()
	st.ModelImages = map[string]bool{"p/mine": true}
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: "g", Name: "G", Members: []string{"p/eye", "p/text", "p/mystery", "p/mine"}}); err != nil {
		t.Fatal(err)
	}

	byID := map[string]modelRef{}
	for _, m := range groupsState().Models {
		byID[m.ID] = m
	}
	for _, tc := range []struct {
		id      string
		images  bool
		said    bool
		context int
		efforts int
	}{
		{"p/eye", true, true, 1048576, 3},      // its list says it sees
		{"p/text", false, true, 200000, 0},     // its list says it takes none
		{"p/mystery", false, false, 128000, 0}, // nothing was read of it either way
		{"p/mine", true, true, 64000, 0},       // the user's answer, over its list
	} {
		m, ok := byID[tc.id]
		if !ok {
			t.Fatalf("%s is not in the group's models", tc.id)
		}
		if m.Images != tc.images || m.ImageSet != tc.said {
			t.Errorf("%s: images %v said %v, want %v %v", tc.id, m.Images, m.ImageSet, tc.images, tc.said)
		}
		if m.Context != tc.context {
			t.Errorf("%s: context %d, want %d", tc.id, m.Context, tc.context)
		}
		if len(m.Efforts) != tc.efforts {
			t.Errorf("%s: efforts %v, want %d of them", tc.id, m.Efforts, tc.efforts)
		}
	}

	// and the group's own members say the same of them
	var g groupJSON
	for _, x := range groupsState().Groups {
		if x.ID == "g" {
			g = x
		}
	}
	if len(g.Info) != 4 {
		t.Fatalf("the group lists %d members", len(g.Info))
	}
	for i, want := range []struct {
		id     string
		images bool
		said   bool
	}{{"p/eye", true, true}, {"p/text", false, true}, {"p/mystery", false, false}, {"p/mine", true, true}} {
		if g.Info[i].ID != want.id || g.Info[i].Images != want.images || g.Info[i].ImageSet != want.said {
			t.Errorf("member %d is %s images %v said %v, want %s %v %v", i, g.Info[i].ID, g.Info[i].Images, g.Info[i].ImageSet, want.id, want.images, want.said)
		}
	}
}
