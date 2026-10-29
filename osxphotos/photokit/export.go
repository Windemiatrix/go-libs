package photokit

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// ExportOptions are the options of Export; Overwrite replaces an existing file
// (photokit.py:534, 601). Progress, when not nil, gets the share received, 0 to
// 1, on a queue of PhotoKit as often as PhotoKit decides and never after Export
// returns, which waits for a running call: Progress must not wait for Export.
type ExportOptions struct {
	Overwrite bool
	Progress  func(fraction float64) // extension: not in osxphotos
}

// A stalled iCloud download may hold the completion handler back, after a cancel too (photokit.py:206-218).
var (
	cancelGrace = 30 * time.Second
	fileWrite   = (*os.File).Write
)

// Export writes the first resource of type kind of the asset uuid to the file
// name of the directory dir (_request_resource_data, photokit.py:693-736, as
// upstream exports a RAW, 571-586, and the photo and video of a Live Photo,
// 1168-1178), downloading it from iCloud when needed (networkAccessAllowed,
// 705). uuid is as for Fetch; name is one path element inside dir, otherwise
// ErrInvalidName. The file is created with mode 0600; an existing one is kept
// and fails with fs.ErrExist, unless o.Overwrite replaces it. After an error a
// file Export created is removed and an existing one is left as it was.
// Upstream names and numbers the file (596-599, 1156-1164); here the caller
// does, with export.Names and export.IncrementName. When ctx is done the
// request is canceled and Export returns ctx.Err() once PhotoKit ends it, 30 s
// later at most; without a deadline in ctx it waits as long as PhotoKit does
// (upstream 300 s, 196-218). No such asset is ErrNotFound; no resource of the
// kind, ErrNoResource; a failure of PhotoKit, ErrExport.
func (l *Library) Export(ctx context.Context, uuid string, kind ResourceType, dir, name string, o ExportOptions) (err error) {
	// Upstream writes into dest itself, by the stem of filename (photokit.py:557-561, 596).
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return ErrInvalidName
	}
	// Upstream checks dest before it reads the resources (photokit.py:563-578).
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	res, err := l.Resources(ctx, uuid)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(res, func(r Resource) bool { return r.Type == kind }) {
		return fmt.Errorf("%w: %d", ErrNoResource, kind)
	}
	// The old file stays until the resource is whole (photokit.py:579, 601); a rename never writes through a hard link at name.
	target := name
	if o.Overwrite {
		name = "." + rand.Text() + ".tmp"
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the name is one path element inside os.Root
	if err != nil {
		return err
	}
	s := &fileSink{f: f, progress: o.Progress, failed: make(chan struct{})}
	if err = s.finish(l.request(ctx, uuid, kind, s)); err == nil && o.Overwrite {
		err = root.Rename(name, target)
	}
	if err != nil {
		return errors.Join(err, root.Remove(name))
	}
	return nil
}

func (l *Library) request(ctx context.Context, uuid string, kind ResourceType, s *fileSink) error {
	req, err := l.b.RequestResource(uuid, int(kind), s)
	if err != nil {
		return err
	}
	select {
	case err := <-req.Done():
		if err != nil {
			return fmt.Errorf("%w: %w", ErrExport, err)
		}
		return nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-s.failed:
	}
	req.Cancel()
	select {
	case <-req.Done(): //nolint:errcheck // after Cancel it reports the cancel; err already says why Export ends
	case <-time.After(cancelGrace):
	}
	return err
}

// PhotoKit may still deliver after a failed write and after Export returns.
type fileSink struct {
	f        *os.File
	progress func(float64)
	failed   chan struct{}

	mu  sync.Mutex
	err error
}

func (s *fileSink) Write(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		if _, s.err = fileWrite(s.f, p); s.err != nil {
			close(s.failed)
		}
	}
	return s.err
}

func (s *fileSink) Progress(fraction float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil && s.progress != nil {
		s.progress(fraction)
	}
}

// The request was canceled for a failed write, so that error wins.
func (s *fileSink) finish(err error) error {
	s.mu.Lock()
	err = cmp.Or(s.err, err)
	s.err = bridge.ErrCanceled
	s.mu.Unlock()
	return errors.Join(err, s.f.Close())
}
