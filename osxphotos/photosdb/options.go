package photosdb

// Options configures Open.
type Options struct {
	// TempDir is where the copy of the database is made; "" is os.TempDir().
	// The copy is the whole database, with locations, names of persons and
	// captions: the directory must be private to the user and must not be
	// inside the library. The copy is made in a new directory with
	// permissions 0700, its files have permissions 0600, and it is removed
	// before Open returns.
	TempDir string
}
