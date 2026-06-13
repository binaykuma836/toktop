package ingest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Scan parses every matching file under each source's roots once and returns
// all events (no watching). Used by `toktop --once` for a one-shot summary.
func Scan(sources []Source) []Event {
	var all []Event
	for _, s := range sources {
		for _, root := range s.Roots() {
			filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !s.Match(path) {
					return nil
				}
				all = append(all, parseWhole(path, s)...)
				return nil
			})
		}
	}
	return all
}

func parseWhole(path string, s Source) []Event {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var evs []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		if e, ok := s.ParseLine(sc.Bytes()); ok {
			evs = append(evs, e)
		}
	}
	return evs
}

// Watcher performs an initial scan of every matching log file, then live-tails
// them via fsnotify, emitting batches of new Events on a channel. It only reads
// complete (newline-terminated) lines, so a half-written line in an active
// session is picked up on the next write.
type Watcher struct {
	sources []Source
	out     chan []Event
	mu      sync.Mutex
	offsets map[string]int64 // path -> bytes consumed up to last newline
}

// NewWatcher creates a watcher over the given sources.
func NewWatcher(sources []Source) *Watcher {
	return &Watcher{
		sources: sources,
		out:     make(chan []Event, 128),
		offsets: make(map[string]int64),
	}
}

// Events returns the channel of new event batches. It is closed when Run exits.
func (w *Watcher) Events() <-chan []Event { return w.out }

func (w *Watcher) sourceFor(path string) Source {
	for _, s := range w.sources {
		if s.Match(path) {
			return s
		}
	}
	return nil
}

// Run scans existing logs, then watches for changes until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	defer close(w.out)

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		fsw = nil
	}
	if fsw != nil {
		defer fsw.Close()
	}

	for _, s := range w.sources {
		for _, root := range s.Roots() {
			w.walk(ctx, fsw, root, s)
		}
	}

	if fsw == nil {
		<-ctx.Done()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			w.handle(ctx, fsw, ev)
		case _, ok := <-fsw.Errors:
			if !ok {
				return
			}
		}
	}
}

// walk registers every directory under root with the watcher and tails every
// matching file it finds.
func (w *Watcher) walk(ctx context.Context, fsw *fsnotify.Watcher, root string, s Source) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if fsw != nil {
				_ = fsw.Add(path)
			}
			return nil
		}
		if s.Match(path) {
			if evs := w.tail(path, s); len(evs) > 0 {
				w.emit(ctx, evs)
			}
		}
		return nil
	})
}

func (w *Watcher) handle(ctx context.Context, fsw *fsnotify.Watcher, ev fsnotify.Event) {
	if ev.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			// A new project directory appeared: watch it and tail its files.
			for _, s := range w.sources {
				w.walk(ctx, fsw, ev.Name, s)
			}
			return
		}
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
		if s := w.sourceFor(ev.Name); s != nil {
			if evs := w.tail(ev.Name, s); len(evs) > 0 {
				w.emit(ctx, evs)
			}
		}
	}
}

// tail reads new complete lines from path since the last recorded offset.
func (w *Watcher) tail(path string, s Source) []Event {
	w.mu.Lock()
	snapOff := w.offsets[path]
	w.mu.Unlock()
	off := snapOff

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	size := fi.Size()
	if size < off { // file truncated/rotated; start over
		off = 0
	}
	if size == off {
		return nil
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}
	data := make([]byte, size-off)
	n, err := io.ReadFull(f, data)
	// A short read at EOF is normal for an actively-appended file; any other
	// error means we can't trust these bytes, so don't advance the offset.
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil
	}
	data = data[:n]

	lastNL := bytes.LastIndexByte(data, '\n')
	if lastNL < 0 {
		return nil // no complete line yet
	}

	var evs []Event
	for _, line := range bytes.Split(data[:lastNL+1], []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if e, ok := s.ParseLine(line); ok {
			evs = append(evs, e)
		}
	}

	// Compare-and-set: only advance if no other tail moved the offset while we
	// were reading. If it changed, drop our advance — the next event re-tails
	// and the store de-dupes any overlap by message id.
	w.mu.Lock()
	if w.offsets[path] == snapOff {
		w.offsets[path] = off + int64(lastNL+1)
	}
	w.mu.Unlock()
	return evs
}

func (w *Watcher) emit(ctx context.Context, evs []Event) {
	select {
	case w.out <- evs:
	case <-ctx.Done():
	}
}
