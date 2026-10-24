// Package sqlread reads a Photos.sqlite database through a read-only copy
// that it makes and checks in a directory of its own. The files of the
// library are only read as bytes; SQLite never opens them.
package sqlread
