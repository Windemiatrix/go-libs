package sqlread

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// dirPrefix starts the name of every copy directory:
// osxphotos-sqlread-<pid>-<random>.
const dirPrefix = "osxphotos-sqlread-"

// The order, the database and then its -wal, is that of osxphotos
// (sqlite_utils.py:86-99).
func makeCopy(ctx context.Context, src, tmp string) (dir, copyPath string, err error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("sqlread: %w", err)
	}
	// MkdirTemp creates the directory with 0700.
	dir, err = os.MkdirTemp(tmp, dirPrefix+strconv.Itoa(os.Getpid())+"-*")
	if err != nil {
		return "", "", fmt.Errorf("sqlread: %w", err)
	}
	copyPath = filepath.Join(dir, filepath.Base(src))
	if err := copyFiles(ctx, src, copyPath); err != nil {
		return "", "", errors.Join(fmt.Errorf("sqlread: %w", err), os.RemoveAll(dir))
	}
	return dir, copyPath, nil
}

// Any error of the -wal but a missing file fails: without it the copy would
// silently lose the rows of the log.
func copyFiles(ctx context.Context, src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := copyFile(src+"-wal", dst+"-wal")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src) //nolint:gosec // G304: the database path is given by the caller
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: a fixed name in the copy directory
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}

// Copies of this process are never touched, whatever their age, since several
// processes share one temporary directory. Errors are ignored: the sweep is a
// best effort.
func sweep(dir string, self int, alive func(pid int) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.Type().IsDir() {
			continue
		}
		pid, ok := copyPID(e.Name())
		if !ok || pid == self || alive(pid) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// The pid must fit in the 32-bit pid_t of kill(2): a larger number would be
// truncated there and name another process, so such a name is kept.
func copyPID(name string) (int, bool) {
	rest, ok := strings.CutPrefix(name, dirPrefix)
	if !ok {
		return 0, false
	}
	digits, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	pid, err := strconv.ParseInt(digits, 10, 32)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return int(pid), true
}

// Only ESRCH means the process is gone; EPERM (a process of another user) and
// any other error count as alive, so a copy is kept rather than removed. A
// reused pid also counts as alive: its copy stays until a later sweep.
func processAlive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
