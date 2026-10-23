// Package viewfile keeps the saved view (core.View) on disk: one file per
// agent command, under the state directory, so reopening relay-tui
// against the same agent looks the way the operator left it.
//
// The file is JSON in the strict dialect (package strictjson):
//
//	{"version": 1, "argv": [...], "sha256": "<hex>", "view": {...}}
//
// argv is the agent's command, as given after --, and the file is named
// by a hash of it; a file whose argv is not this command's is not used.
// sha256 is the digest of the view's bytes as written, so a file damaged
// anywhere in the view is not used. A paste the operator trusted carries
// "trust": a digest of its number and text; a paste without one, or with
// one that does not match, comes back untrusted. So no damage to the
// file can make a paste more trusted than it was saved.
//
// Encode and Decode are pure. Load, Write and Saver are the IO.
package viewfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// Version is the file format's version.
const Version = 1

// MaxSize is the largest file read or written. A view too large to save
// whole is saved without its drafts.
const MaxSize = 8 << 20

// Errors Decode gives. None quotes the file: it may hold drafts.
var (
	ErrDamaged     = errors.New("the saved view is damaged")
	ErrVersion     = errors.New("the saved view is of a version this relay-tui does not know")
	ErrOtherAgent  = errors.New("the saved view is for another agent command")
	ErrTooLarge    = errors.New("the saved view is too large")
	errUnencodable = errors.New("the view cannot be encoded")
)

type file struct {
	Version int            `json:"version"`
	Argv    []string       `json:"argv"`
	SHA256  string         `json:"sha256"`
	View    jsontext.Value `json:"view"`
}

type wireView struct {
	Hub       *wireHub    `json:"hub,omitempty"`
	PaneFocus bool        `json:"paneFocus"`
	Items     bool        `json:"items"`
	Card      int64       `json:"card"`
	Detail    bool        `json:"detail"`
	Line      int         `json:"line"`
	Col       int         `json:"col"`
	Drafts    []wireDraft `json:"drafts"`
}

type wireHub struct {
	Focus  string `json:"focus"`
	Cursor string `json:"cursor"`
	List   bool   `json:"list"`
}

type wireDraft struct {
	Role      string      `json:"role"`
	Text      string      `json:"text"`
	NextPaste int         `json:"nextPaste"`
	Pastes    []wirePaste `json:"pastes"`
}

type wirePaste struct {
	ID    int    `json:"id"`
	Text  string `json:"text"`
	Trust string `json:"trust,omitempty"`
}

// trustSeal is what a trusted paste carries: it binds the trust to the
// paste's number and its exact text.
func trustSeal(id int, text string) string {
	h := sha256.New()
	h.Write([]byte("relay-tui trusted paste\x00"))
	h.Write([]byte(strconv.Itoa(id)))
	h.Write([]byte{0})
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Name is the file's name for an agent command.
func Name(argv []string) string {
	b, _ := json.Marshal(argv)
	return "view-" + digest(b)[:32] + ".json"
}

// Path is where the saved view for an agent command lives under dir.
func Path(dir string, argv []string) string { return filepath.Join(dir, Name(argv)) }

// DefaultDir is the state directory under the home directory.
func DefaultDir(home string) string { return filepath.Join(home, ".local", "state", "relay-tui") }

func toWire(v core.View) wireView {
	w := wireView{PaneFocus: v.PaneFocus, Items: v.Items, Card: v.Card, Detail: v.Detail, Line: v.Line, Col: v.Col}
	if v.Hub != nil {
		w.Hub = &wireHub{Focus: v.Hub.Focus, Cursor: v.Hub.Cursor, List: v.Hub.List}
	}
	w.Drafts = []wireDraft{}
	for _, d := range v.Drafts {
		wd := wireDraft{Role: d.Role, Text: d.Text, NextPaste: d.NextPaste, Pastes: []wirePaste{}}
		for _, p := range d.Pastes {
			wp := wirePaste{ID: p.ID, Text: p.Text}
			if p.Trusted {
				wp.Trust = trustSeal(p.ID, p.Text)
			}
			wd.Pastes = append(wd.Pastes, wp)
		}
		w.Drafts = append(w.Drafts, wd)
	}
	return w
}

// Encode is the file for the view of an agent command. A view too large
// is encoded without its drafts; dropped says so.
func Encode(argv []string, v core.View) (b []byte, dropped bool, err error) {
	b, err = encode(argv, v)
	if err == nil && len(b) > MaxSize {
		v.Drafts = nil
		b, err = encode(argv, v)
		dropped = true
	}
	return b, dropped, err
}

func encode(argv []string, v core.View) ([]byte, error) {
	// A paste's bytes need not be UTF-8. They are saved with the bad
	// bytes replaced, so a trusted one no longer matches its seal and
	// comes back untrusted.
	vb, err := json.Marshal(toWire(v), jsontext.AllowInvalidUTF8(true))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnencodable, err)
	}
	if argv == nil {
		argv = []string{}
	}
	b, err := json.Marshal(file{Version: Version, Argv: argv, SHA256: digest(vb), View: vb})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnencodable, err)
	}
	return append(b, '\n'), nil
}

// Decode reads the file for an agent command. Any error means the
// defaults. Untrusted says some paste had a trust that did not match it
// and came back untrusted.
func Decode(argv []string, b []byte) (v core.View, untrusted bool, err error) {
	if len(b) > MaxSize {
		return core.View{}, false, ErrTooLarge
	}
	var f file
	if err := strictjson.Unmarshal(b, &f); err != nil {
		return core.View{}, false, ErrDamaged
	}
	if f.Version != Version {
		return core.View{}, false, ErrVersion
	}
	if !sameArgv(f.Argv, argv) {
		return core.View{}, false, ErrOtherAgent
	}
	if len(f.View) == 0 || digest(f.View) != f.SHA256 {
		return core.View{}, false, ErrDamaged
	}
	var w wireView
	if err := strictjson.Unmarshal(f.View, &w); err != nil {
		return core.View{}, false, ErrDamaged
	}
	v, untrusted = fromWire(w)
	return v, untrusted, nil
}

func sameArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fromWire is the view a file holds. A paste is trusted only when its
// trust matches its number and text.
func fromWire(w wireView) (core.View, bool) {
	v := core.View{PaneFocus: w.PaneFocus, Items: w.Items, Card: w.Card, Detail: w.Detail, Line: w.Line, Col: w.Col}
	if w.Hub != nil {
		v.Hub = &core.HubView{Focus: w.Hub.Focus, Cursor: w.Hub.Cursor, List: w.Hub.List}
	}
	untrusted := false
	for _, wd := range w.Drafts {
		d := core.Draft{Role: wd.Role, Text: wd.Text, NextPaste: wd.NextPaste}
		for _, wp := range wd.Pastes {
			p := core.Paste{ID: wp.ID, Text: wp.Text}
			switch {
			case wp.Trust == "":
			case wp.Trust == trustSeal(wp.ID, wp.Text):
				p.Trusted = true
			default:
				untrusted = true
			}
			d.Pastes = append(d.Pastes, p)
		}
		v.Drafts = append(v.Drafts, d)
	}
	return v, untrusted
}

// equalBytes is bytes.Equal, named for the saver.
func equalBytes(a, b []byte) bool { return bytes.Equal(a, b) }
