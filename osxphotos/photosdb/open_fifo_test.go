//go:build unix

package photosdb

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

// Opening a named pipe for reading blocks until a writer appears.
func TestOpenRefusesNamedPipe(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Photos.sqlite", "Photos.sqlite-wal"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib := testlib.OpenPristine(t, "synthetic")
			pipe := filepath.Join(lib, "database", name)
			if err := os.Remove(pipe); err != nil && !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(pipe, 0o600); err != nil {
				t.Fatal(err)
			}
			tmp := t.TempDir()
			opened := make(chan error, 1)
			go func() {
				db, err := Open(t.Context(), lib, Options{TempDir: tmp})
				if err == nil {
					err = errors.Join(errors.New("Open succeeded"), db.Close())
				}
				opened <- err
			}()
			select {
			case err := <-opened:
				if err == nil || !strings.Contains(err.Error(), "not a regular file") {
					t.Errorf("Open error %v, want one that says the file is not a regular file", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Open blocks on a named pipe")
			}
			assertNoCopies(t, tmp)
		})
	}
}
