package viewfile

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kirahowe/relay-tui/internal/core"
)

// Load reads the saved view for an agent command from dir. With no file
// it is the defaults and no note. With a file it cannot use, it is the
// defaults and a note saying why, which never quotes the file.
func Load(dir string, argv []string) (core.View, string) {
	f, err := os.Open(Path(dir, argv))
	if errors.Is(err, fs.ErrNotExist) {
		return core.View{}, ""
	}
	if err != nil {
		return core.View{}, "the saved view could not be read, so the view starts afresh"
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return core.View{}, "the saved view could not be read, so the view starts afresh"
	}
	v, untrusted, err := Decode(argv, b)
	if err != nil {
		return core.View{}, err.Error() + ", so the view starts afresh"
	}
	if untrusted {
		return v, "a saved paste's trust did not check out, so it is back as untrusted"
	}
	return v, ""
}

// Write puts b in the file at path in one step: a file of its own in the
// same directory, then a rename over the old one, so a reader sees the
// old file or the new one, never part of one. The directory is made 0700
// if it is not there, and the file is 0600.
func Write(path string, b []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".view-*.tmp") // 0600
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Saver writes the saved view in the background, at most once every
// delay while it changes, so a crash or a kill loses little and the
// screen never waits on the disk. Close writes the last one.
type Saver struct {
	path  string
	argv  []string
	delay time.Duration
	note  func(string)

	mu      sync.Mutex // guards latest and pending
	latest  core.View
	pending bool
	written []byte // what the last write wrote; the writer's own
	kick    chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

// NewSaver starts a saver of the view for argv to path. note is told
// about a failure to save, in words that never quote the view.
func NewSaver(path string, argv []string, delay time.Duration, note func(string)) *Saver {
	s := &Saver{path: path, argv: argv, delay: delay, note: note,
		kick: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go s.run()
	return s
}

// Save hands the saver a view to write. It never blocks on the disk.
func (s *Saver) Save(v core.View) {
	s.mu.Lock()
	s.latest, s.pending = v, true
	s.mu.Unlock()
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Saver) run() {
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case <-s.kick:
		}
		select {
		case <-s.stop:
			return
		case <-time.After(s.delay):
		}
		s.flush()
	}
}

// flush writes the latest view, if it has changed since the last write.
// Only the saver's goroutine, or Close once that has stopped, calls it,
// so the lock is held only to take the view: Save never waits on the
// disk.
func (s *Saver) flush() {
	s.mu.Lock()
	v, pending := s.latest, s.pending
	s.pending = false
	s.mu.Unlock()
	if !pending {
		return
	}
	b, dropped, err := Encode(s.argv, v)
	if err != nil {
		s.note("the view could not be saved")
		return
	}
	if dropped {
		s.note("the drafts are too large to save; the rest of the view is saved")
	}
	if equalBytes(b, s.written) {
		return
	}
	if err := Write(s.path, b); err != nil {
		s.note("the view could not be saved: " + err.Error())
		return
	}
	s.written = b
}

// Close stops the saver and writes v, the view as the client closes.
func (s *Saver) Close(v core.View) {
	close(s.stop)
	<-s.done
	s.Save(v)
	s.flush()
}
