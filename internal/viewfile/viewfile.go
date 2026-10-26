// Package viewfile keeps the saved view (core.View) on disk: one file per
// agent command, under the state directory, so reopening relay-tui
// against the same agent looks the way the operator left it.
//
// The file is JSON in the strict dialect (package strictjson):
//
//	{"version": 2, "argv": [...], "sha256": "<hex>", "view": {...}}
//
// argv is the agent's command, as given after --, and the file is named
// by a hash of it; a file whose argv is not this command's is not used.
// sha256 is the digest of the view's bytes as written, so a file damaged
// anywhere in the view is not used.
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
	"regexp"
	"strconv"

	"github.com/kirahowe/relay-tui/internal/core"
	"github.com/kirahowe/relay-tui/internal/strictjson"
)

// Version is the file format's version. Version 1 files have the same
// wire format; their paste placeholders carried a trust word, which
// Decode rewrites, so drafts survive the upgrade.
const Version = 2

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
	ID   int    `json:"id"`
	Text string `json:"text"`
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
			wd.Pastes = append(wd.Pastes, wirePaste{ID: p.ID, Text: p.Text})
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
	// bytes replaced.
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
// defaults.
func Decode(argv []string, b []byte) (core.View, error) {
	if len(b) > MaxSize {
		return core.View{}, ErrTooLarge
	}
	var f file
	if err := strictjson.Unmarshal(b, &f); err != nil {
		return core.View{}, ErrDamaged
	}
	if f.Version != Version && f.Version != 1 {
		return core.View{}, ErrVersion
	}
	if !sameArgv(f.Argv, argv) {
		return core.View{}, ErrOtherAgent
	}
	if len(f.View) == 0 || digest(f.View) != f.SHA256 {
		return core.View{}, ErrDamaged
	}
	var w wireView
	if err := strictjson.Unmarshal(f.View, &w); err != nil {
		return core.View{}, ErrDamaged
	}
	v := fromWire(w)
	if f.Version == 1 {
		v = upgradeV1(v)
	}
	return v, nil
}

var oldToken = regexp.MustCompile(`\[pasted \d+ lines? · (?:un)?trusted #(\d+)\]`)

// upgradeV1 rewrites the placeholders of a version 1 view to the ones
// the same pastes have now.
func upgradeV1(v core.View) core.View {
	for i, d := range v.Drafts {
		byID := map[int]core.Paste{}
		dup := map[int]bool{}
		for _, p := range d.Pastes {
			if _, ok := byID[p.ID]; ok {
				dup[p.ID] = true
			}
			byID[p.ID] = p
		}
		v.Drafts[i].Text = oldToken.ReplaceAllStringFunc(d.Text, func(m string) string {
			id, err := strconv.Atoi(oldToken.FindStringSubmatch(m)[1])
			p, ok := byID[id]
			if err != nil || !ok || dup[id] {
				return m
			}
			return p.Token()
		})
	}
	return v
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

// fromWire is the view a file holds.
func fromWire(w wireView) core.View {
	v := core.View{PaneFocus: w.PaneFocus, Items: w.Items, Card: w.Card, Detail: w.Detail, Line: w.Line, Col: w.Col}
	if w.Hub != nil {
		v.Hub = &core.HubView{Focus: w.Hub.Focus, Cursor: w.Hub.Cursor, List: w.Hub.List}
	}
	for _, wd := range w.Drafts {
		d := core.Draft{Role: wd.Role, Text: wd.Text, NextPaste: wd.NextPaste}
		for _, wp := range wd.Pastes {
			d.Pastes = append(d.Pastes, core.Paste{ID: wp.ID, Text: wp.Text})
		}
		v.Drafts = append(v.Drafts, d)
	}
	return v
}

// equalBytes is bytes.Equal, named for the saver.
func equalBytes(a, b []byte) bool { return bytes.Equal(a, b) }
