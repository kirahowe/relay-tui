package viewfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/kirahowe/relay-tui/internal/core"
)

var argv = []string{"relay", "acp", "--approvals", "--data", "/tmp/x"}

func genView(t *rapid.T) core.View {
	v := core.View{
		PaneFocus: rapid.Bool().Draw(t, "pane"), Items: rapid.Bool().Draw(t, "items"),
		Card: rapid.Int64Range(0, 50).Draw(t, "card"), Detail: rapid.Bool().Draw(t, "detail"),
		Line: rapid.IntRange(0, 9).Draw(t, "line"), Col: rapid.IntRange(0, 9).Draw(t, "col"),
	}
	if rapid.Bool().Draw(t, "hub") {
		v.Hub = &core.HubView{Focus: rapid.StringMatching(`[a-z]{0,5}`).Draw(t, "focus"),
			Cursor: rapid.StringMatching(`[a-z]{0,5}`).Draw(t, "cursor"), List: rapid.Bool().Draw(t, "list")}
	}
	for i := rapid.IntRange(0, 3).Draw(t, "drafts"); i > 0; i-- {
		d := core.Draft{Role: rapid.StringMatching(`[a-z]{0,5}`).Draw(t, "role"),
			Text: rapid.String().Draw(t, "text"), NextPaste: rapid.IntRange(0, 9).Draw(t, "next")}
		for j := rapid.IntRange(0, 4).Draw(t, "pastes"); j > 0; j-- {
			d.Pastes = append(d.Pastes, core.Paste{ID: rapid.IntRange(1, 9).Draw(t, "id"),
				Text: rapid.String().Draw(t, "paste")})
		}
		v.Drafts = append(v.Drafts, d)
	}
	return v
}

func mustEncode(t interface {
	Helper()
	Fatalf(string, ...any)
}, v core.View) []byte {
	t.Helper()
	b, dropped, err := Encode(argv, v)
	if err != nil || dropped {
		t.Fatalf("encode: %v, dropped %v", err, dropped)
	}
	return b
}

// Save then load gives back the view exactly.
func TestRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := genView(t)
		got, err := Decode(argv, mustEncode(t, v))
		if err != nil || !reflect.DeepEqual(got, v) {
			t.Fatalf("decode: %v\n got %+v\nwant %+v", err, got, v)
		}
	})
}

// Any damage to the bytes of the file means the defaults, or, where it
// changed nothing that is read, the same view.
func TestDamagedBytesNeverChangeTheView(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := genView(t)
		b := mustEncode(t, v)
		for n := rapid.IntRange(1, 4).Draw(t, "damage"); n > 0; n-- {
			i := rapid.IntRange(0, len(b)-1).Draw(t, "at")
			switch rapid.IntRange(0, 3).Draw(t, "how") {
			case 0:
				b[i] = rapid.Byte().Draw(t, "byte")
			case 1:
				b[i] ^= 1 << rapid.IntRange(0, 7).Draw(t, "bit")
			case 2:
				b = b[:i]
			case 3:
				b = append(b[:i:i], append([]byte{rapid.Byte().Draw(t, "insert")}, b[i:]...)...)
			}
			if len(b) == 0 {
				break
			}
		}
		got, err := Decode(argv, b)
		if err != nil {
			if !reflect.DeepEqual(got, core.View{}) {
				t.Fatalf("an error with a view: %+v", got)
			}
			return
		}
		if !reflect.DeepEqual(got, v) {
			t.Fatalf("damage changed the view\n got %+v\nwant %+v", got, v)
		}
	})
}

func TestDecodeRefusals(t *testing.T) {
	v := core.View{Card: 3, Drafts: []core.Draft{{Role: "dev", Text: "zqxw"}}}
	b := mustEncode(t, v)
	if _, err := Decode([]string{"relay", "acp"}, b); !errors.Is(err, ErrOtherAgent) {
		t.Errorf("other argv: %v", err)
	}
	if _, err := Decode(argv, []byte(strings.Replace(string(b), `"version":2`, `"version":3`, 1))); !errors.Is(err, ErrVersion) {
		t.Errorf("version: %v", err)
	}
	dup := strings.Replace(string(b), `"version":2`, `"version":2,"version":2`, 1)
	if _, err := Decode(argv, []byte(dup)); !errors.Is(err, ErrDamaged) {
		t.Errorf("duplicate member: %v", err)
	}
	if _, err := Decode(argv, make([]byte, MaxSize+1)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	for _, s := range []string{"", "null", "[]", `{"version":2}`, string(b[:len(b)/2])} {
		if _, err := Decode(argv, []byte(s)); err == nil {
			t.Errorf("%q decoded", s)
		}
	}
	for _, err := range []error{ErrDamaged, ErrVersion, ErrOtherAgent, ErrTooLarge} {
		if strings.Contains(err.Error(), "zqxw") {
			t.Errorf("an error quotes the file: %v", err)
		}
	}
}

func TestEncodeTooLargeDropsTheDrafts(t *testing.T) {
	v := core.View{Card: 3, Drafts: []core.Draft{{Role: "dev", Text: strings.Repeat("x", MaxSize)}}}
	b, dropped, err := Encode(argv, v)
	if err != nil || !dropped {
		t.Fatalf("%v %v", err, dropped)
	}
	got, err := Decode(argv, b)
	if err != nil || got.Card != 3 || got.Drafts != nil {
		t.Fatalf("%v %+v", err, got)
	}
}

// A paste of bytes that are not UTF-8 is saved with them replaced.
func TestInvalidUTF8IsSaved(t *testing.T) {
	v := core.View{Drafts: []core.Draft{{Role: "dev", Text: "a \xff", Pastes: []core.Paste{{ID: 1, Text: "b \xfe"}}}}}
	got, err := Decode(argv, mustEncode(t, v))
	if err != nil || got.Drafts[0].Text != "a \ufffd" || got.Drafts[0].Pastes[0].Text != "b \ufffd" {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestNameIsByCommand(t *testing.T) {
	a, b := Name([]string{"a b", "c"}), Name([]string{"a", "b c"})
	if a == b || a != Name([]string{"a b", "c"}) || !strings.HasPrefix(a, "view-") || !strings.HasSuffix(a, ".json") {
		t.Fatalf("%s %s", a, b)
	}
	if got := DefaultDir("/Users/k"); got != "/Users/k/.local/state/relay-tui" {
		t.Fatal(got)
	}
}

func TestWriteIsAtomicAndPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state", "relay-tui")
	path := Path(dir, argv)
	if err := Write(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Fatalf("%q", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v", fi.Mode())
	}
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if fi, _ := os.Stat(d); fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode %v", d, fi.Mode())
		}
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("left behind: %v", es)
	}
	// A failed write leaves the old file whole and nothing else.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if err := Write(path, []byte("three")); err == nil {
		t.Fatal("wrote into a read-only directory")
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Fatalf("%q", b)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if v, note := Load(dir, argv); !reflect.DeepEqual(v, core.View{}) || note != "" {
		t.Fatalf("no file: %+v %q", v, note)
	}
	want := core.View{Card: 4, Drafts: []core.Draft{{Role: "dev", Text: "secret words",
		Pastes: []core.Paste{{ID: 1, Text: "t"}}}}}
	if err := Write(Path(dir, argv), mustEncode(t, want)); err != nil {
		t.Fatal(err)
	}
	if v, note := Load(dir, argv); !reflect.DeepEqual(v, want) || note != "" {
		t.Fatalf("saved: %+v %q", v, note)
	}
	// Another command has a file of its own.
	if v, note := Load(dir, []string{"agentloop", "acp"}); v.Card != 0 || note != "" {
		t.Fatalf("other: %+v %q", v, note)
	}
	// Damaged, too large, unreadable: the defaults and a note, which
	// never quotes the file.
	b := mustEncode(t, want)
	for name, content := range map[string][]byte{
		"damaged":   b[:len(b)-9],
		"too large": append(b, make([]byte, MaxSize)...),
	} {
		if err := os.WriteFile(Path(dir, argv), content, 0o600); err != nil {
			t.Fatal(err)
		}
		v, note := Load(dir, argv)
		if !reflect.DeepEqual(v, core.View{}) || note == "" || strings.Contains(note, "secret") {
			t.Errorf("%s: %+v %q", name, v, note)
		}
	}
	if err := os.Chmod(Path(dir, argv), 0o000); err != nil {
		t.Fatal(err)
	}
	if v, note := Load(dir, argv); !reflect.DeepEqual(v, core.View{}) || note == "" {
		t.Errorf("unreadable: %+v %q", v, note)
	}
}

func TestSaverWritesSoonAndOnClose(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir, argv)
	var notes []string
	s := NewSaver(path, argv, 20*time.Millisecond, func(n string) { notes = append(notes, n) })
	s.Save(core.View{Card: 1})
	s.Save(core.View{Card: 2})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if v, _ := Load(dir, argv); v.Card == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never written")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Save(core.View{Card: 3})
	s.Close(core.View{Card: 4})
	if v, note := Load(dir, argv); v.Card != 4 || note != "" {
		t.Fatalf("after close: %+v %q", v, note)
	}
	if len(notes) != 0 {
		t.Fatalf("notes %q", notes)
	}
}

// A version 1 file keeps its drafts: the placeholders lose the trust
// word and the pastes still resolve.
func TestDecodeVersion1KeepsDrafts(t *testing.T) {
	old := core.View{Drafts: []core.Draft{{Role: "dev", NextPaste: 2,
		Text:   "half [pasted 2 lines · untrusted #1] and [pasted 1 line · trusted #2] done",
		Pastes: []core.Paste{{ID: 1, Text: "a\nb"}, {ID: 2, Text: "c"}}}}}
	b := mustEncode(t, old)
	b = []byte(strings.Replace(string(b), `"version":2`, `"version":1`, 1))
	v, err := Decode(argv, b)
	if err != nil {
		t.Fatal(err)
	}
	d := v.Drafts[0]
	want := "half [pasted 2 lines #1] and [pasted 1 line #2] done"
	if d.Text != want || len(d.Pastes) != 2 {
		t.Fatalf("got %q %v", d.Text, d.Pastes)
	}
	for _, p := range d.Pastes {
		if !strings.Contains(d.Text, p.Token()) {
			t.Errorf("paste %d does not match its placeholder", p.ID)
		}
	}
}
