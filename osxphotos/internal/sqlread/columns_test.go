package sqlread

import (
	"errors"
	"testing"
)

func TestCheckColumns(t *testing.T) {
	t.Parallel()
	d, err := Open(t.Context(), syntheticDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()

	for _, tc := range []struct {
		name string
		want map[string][]string
		err  string // "" is nil
	}{
		{"all present", map[string][]string{
			"ZASSET":     {"Z_PK", "ZUUID", "ZDATECREATED"},
			"Z_METADATA": {"Z_PLIST", "Z_UUID", "Z_VERSION"},
		}, ""},
		{"missing column", map[string][]string{"ZASSET": {"Z_PK", "ZNOPE"}}, "ZASSET.ZNOPE"},
		{"first missing column in the given order", map[string][]string{"ZASSET": {"ZNOPE2", "ZNOPE1"}}, "ZASSET.ZNOPE2"},
		{"missing table", map[string][]string{"ZNOPE": {"X"}}, "table ZNOPE"},
		{"tables in sorted order", map[string][]string{"ZB": {"X"}, "ZA": {"Y"}}, "table ZA"},
		{"missing table before a missing column", map[string][]string{"ZASSET": {"ZNOPE"}, "ZA": {"Y"}}, "table ZA"},
		{"table name case", map[string][]string{"zasset": {"Z_PK"}}, "table zasset"},
		{"column name case", map[string][]string{"ZASSET": {"zuuid"}}, "ZASSET.zuuid"},
		{"double quote in the name", map[string][]string{`ZASSET"; DROP TABLE ZASSET; --`: {"Z_PK"}}, `table ZASSET"; DROP TABLE ZASSET; --`},
		{"single quote in the name", map[string][]string{`ZASSET'; DROP TABLE ZASSET; --`: {"Z_PK"}}, `table ZASSET'; DROP TABLE ZASSET; --`},
		{"existing table without columns", map[string][]string{"ZASSET": nil}, ""},
		{"missing table without columns", map[string][]string{"ZNOPE": {}}, "table ZNOPE"},
		{"empty map", map[string][]string{}, ""},
		{"nil map", nil, ""},
	} {
		err := d.CheckColumns(t.Context(), tc.want)
		switch {
		case tc.err == "" && err != nil:
			t.Errorf("%s: %v, want nil", tc.name, err)
		case tc.err == "":
		case !errors.Is(err, ErrMissingColumn):
			t.Errorf("%s: %v, want ErrMissingColumn", tc.name, err)
		case err.Error() != "sqlread: missing table or column: "+tc.err:
			t.Errorf("%s: %q, want %q", tc.name, err, tc.err)
		}
	}
	// The names went in as parameters: ZASSET is intact.
	if n := count(t, d.db, "SELECT COUNT(*) FROM sqlite_master WHERE name = 'ZASSET'"); n != 1 {
		t.Errorf("ZASSET: %d rows in sqlite_master, want 1", n)
	}
}

func TestCheckColumnsCanceled(t *testing.T) {
	t.Parallel()
	d, err := Open(t.Context(), syntheticDB(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	ctx, cancel := contextCanceled(t)
	defer cancel()
	if err := d.CheckColumns(ctx, map[string][]string{"ZASSET": {"Z_PK"}}); err == nil || errors.Is(err, ErrMissingColumn) {
		t.Errorf("%v, want an error of the query", err)
	}
}
