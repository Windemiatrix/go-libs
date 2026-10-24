package sqlread

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSweep(t *testing.T) {
	t.Parallel()
	const (
		self = 3001
		live = 2001
	)
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	removed := []string{"osxphotos-sqlread-1001-a", "osxphotos-sqlread-1002-"}
	kept := []string{
		"osxphotos-sqlread-2001-a",                 // a live process
		"osxphotos-sqlread-3001-a",                 // this process
		"osxphotos-sqlread-0-a",                    // pid 0
		"osxphotos-sqlread--5-a",                   // negative
		"osxphotos-sqlread-+5-a",                   // a sign is not a decimal pid
		"osxphotos-sqlread-abc-a",                  // not a number
		"osxphotos-sqlread-1003",                   // no "-" after the pid
		"osxphotos-sqlread-",                       // nothing after the prefix
		"osxphotos-sqlread-99999999999999999999-x", // does not fit in int
		"osxphotos-sqlread-4294967297-x",           // 2^32+1: kill(2) would see pid 1
		"osxphotos-sqlread-2147483648-x",           // 2^31: above the 32-bit pid_t
		"osxphotos-sqlreadx-1004-a",                // another prefix
		"other-1005-a",
	}
	for _, n := range slices.Concat(removed, kept) {
		if err := os.Mkdir(filepath.Join(dir, n), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n, "Photos.sqlite"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	file := "osxphotos-sqlread-1006-f" // a regular file with a matching name
	if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := "osxphotos-sqlread-1007-l" // a symbolic link to a directory outside
	if err := os.Symlink(outside, filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	kept = append(kept, file, link)

	var asked []int
	sweep(dir, self, func(pid int) bool {
		asked = append(asked, pid)
		return pid == live
	})

	want := slices.Clone(kept)
	slices.Sort(want)
	if got := names(t, dir); !slices.Equal(got, want) {
		t.Errorf("left %q, want %q", got, want)
	}
	if got := names(t, outside); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("the link target holds %q, want keep", got)
	}
	slices.Sort(asked)
	if want := []int{1001, 1002, live}; !slices.Equal(asked, want) {
		t.Errorf("asked about pids %v, want %v", asked, want)
	}
}

func TestSweepMissingDir(t *testing.T) {
	t.Parallel()
	sweep(filepath.Join(t.TempDir(), "none"), 1, func(int) bool {
		t.Error("alive called for a missing directory")
		return true
	})
}

func TestProcessAlive(t *testing.T) {
	t.Parallel()
	if !processAlive(os.Getpid()) {
		t.Error("this process is not alive")
	}
	if processAlive(1 << 30) { // no such pid on darwin or linux
		t.Error("pid 1<<30 is alive")
	}
	// kill(1, 0) of a user other than root fails with EPERM: a process of
	// another user counts as alive.
	if os.Geteuid() != 0 && !processAlive(1) {
		t.Error("pid 1 (a process of another user) is not alive")
	}
}
