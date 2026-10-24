package sqlread

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

func contextCanceled(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx, cancel
}

var uuidForm = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

func TestMetadataGen(t *testing.T) {
	t.Parallel()
	d, err := Open(t.Context(), genDB(t, 0), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	plist, id, err := d.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(plist, []byte("PLModelVersion")) {
		t.Errorf("plist %q has no PLModelVersion", plist)
	}
	if !uuidForm.MatchString(id) {
		t.Errorf("Z_UUID %q is not a UUID", id)
	}
}

func TestMetadata(t *testing.T) {
	t.Parallel()
	// Without a primary key, so that the insertion order (rowid) differs
	// from the order of Z_VERSION.
	const table = "CREATE TABLE Z_METADATA (Z_VERSION INTEGER, Z_UUID VARCHAR(255), Z_PLIST BLOB)"
	for _, tc := range []struct {
		name      string
		rows      string
		plist     []byte
		uuid, err string
	}{
		{name: "one row", rows: "(1, 'U1', X'01')", plist: []byte{1}, uuid: "U1"},
		{name: "highest version inserted first", rows: "(2, 'U2', X'02'), (1, 'U1', X'01')", plist: []byte{2}, uuid: "U2"},
		{name: "highest version in the middle", rows: "(2, 'U2', X'02'), (3, 'U3', X'03'), (1, 'U1', X'01')", plist: []byte{3}, uuid: "U3"},
		{name: "NULL Z_UUID", rows: "(1, NULL, X'01')", plist: []byte{1}, uuid: ""},
		{name: "NULL Z_PLIST", rows: "(1, 'U1', NULL)", plist: nil, uuid: "U1"},
		{name: "no rows", err: "sqlread: Z_METADATA has no rows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stmts := []string{table}
			if tc.rows != "" {
				stmts = append(stmts, "INSERT INTO Z_METADATA VALUES "+tc.rows)
			}
			d, err := Open(t.Context(), makeDB(t, stmts...), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = d.Close() }()
			plist, id, err := d.Metadata(t.Context())
			if tc.err != "" {
				if err == nil || err.Error() != tc.err {
					t.Errorf("error %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plist, tc.plist) || (plist == nil) != (tc.plist == nil) || id != tc.uuid {
				t.Errorf("Metadata = %v, %q; want %v, %q", plist, id, tc.plist, tc.uuid)
			}
		})
	}
}

func TestMetadataNoTable(t *testing.T) {
	t.Parallel()
	d, err := Open(t.Context(), makeDB(t, "CREATE TABLE t(a)"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if _, _, err := d.Metadata(t.Context()); err == nil {
		t.Error("Metadata without Z_METADATA succeeded")
	}
}
