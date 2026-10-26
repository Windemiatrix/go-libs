package sqlread

import (
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSharesValues(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	have, err := openDB(t, path).columns(t.Context(), "ZSHARE")
	if err != nil || len(have) == 0 {
		t.Fatalf("columns of ZSHARE: %q, %v; the case needs the table", have, err)
	}
	if !slices.ContainsFunc(shareColumns, func(c string) bool { return !slices.Contains(have, c) }) {
		t.Fatal("synthetic ZSHARE has every column: a missing column is not tested")
	}
	want := Share{PK: 900001}
	v := reflect.ValueOf(&want).Elem()
	if v.NumField() != len(shareColumns) {
		t.Fatalf("Share has %d fields, shareColumns %d columns", v.NumField(), len(shareColumns))
	}
	cols, values := []string{"Z_PK"}, []string{"900001"}
	for i := 1; i < v.NumField(); i++ {
		var (
			val any
			lit string
		)
		switch v.Field(i).Interface().(type) {
		case sql.NullInt64:
			val, lit = sql.NullInt64{Int64: int64(100 + i), Valid: true}, fmt.Sprint(100+i)
		case sql.NullFloat64:
			val, lit = sql.NullFloat64{Float64: float64(i) + 0.5, Valid: true}, fmt.Sprintf("%d.5", i)
		case sql.NullString:
			val, lit = sql.NullString{String: fmt.Sprintf("TEST-%02d", i), Valid: true}, fmt.Sprintf("'TEST-%02d'", i)
		case []byte:
			val, lit = []byte{byte(i)}, fmt.Sprintf("X'%02X'", i)
		default:
			t.Fatalf("Share.%s: type %s", v.Type().Field(i).Name, v.Field(i).Type())
		}
		if slices.Contains(have, shareColumns[i]) {
			v.Field(i).Set(reflect.ValueOf(val))
			cols, values = append(cols, shareColumns[i]), append(values, lit)
		}
	}
	w := openWriter(t, path)
	exec(t, w, "INSERT INTO ZSHARE ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(values, ", ")+")",
		"INSERT INTO ZSHARE (Z_PK) VALUES (900002)")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	shares, err := openDB(t, path).Shares(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]Share{}
	for i, s := range shares {
		if i > 0 && shares[i-1].PK >= s.PK {
			t.Errorf("row %d out of Z_PK order", i+1)
		}
		got[s.PK] = s
	}
	if !reflect.DeepEqual(got[900001], want) {
		t.Errorf("a value in every column:\n got %+v\nwant %+v", got[900001], want)
	}
	if !reflect.DeepEqual(got[900002], Share{PK: 900002}) {
		t.Errorf("NULL in every column: %+v", got[900002])
	}
}

func TestSharesNoTable(t *testing.T) {
	t.Parallel()

	path := syntheticDB(t)
	w := openWriter(t, path)
	exec(t, w, "DROP TABLE ZSHARE")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if shares, err := openDB(t, path).Shares(t.Context()); shares != nil || err != nil {
		t.Errorf("Shares without ZSHARE: %d rows, %v; want nil, nil", len(shares), err)
	}
}

func TestSharesLibraries(t *testing.T) {
	t.Parallel()

	fields := reflect.TypeFor[Share]()
	for _, lib := range libraryDBs(t) {
		t.Run(lib.name, func(t *testing.T) {
			d := openDB(t, lib.path)
			have, err := d.columns(t.Context(), "ZSHARE")
			if err != nil {
				t.Fatal(err)
			}
			shares, err := d.Shares(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(have) == 0 {
				if shares != nil {
					t.Errorf("Shares: %d rows without a ZSHARE table", len(shares))
				}
				return
			}
			ro := openRO(t, lib.path)
			if want := count(t, ro, "SELECT COUNT(*) FROM ZSHARE"); int64(len(shares)) != want {
				t.Fatalf("Shares: %d rows, want %d", len(shares), want)
			}
			for i, c := range shareColumns {
				var want []any
				if slices.Contains(have, c) {
					want = columnValues(t, ro, "SELECT "+c+" FROM ZSHARE ORDER BY Z_PK", fields.Field(i).Type)
				} else {
					t.Logf("no column ZSHARE.%s", c)
					for range shares {
						want = append(want, reflect.Zero(fields.Field(i).Type).Interface())
					}
				}
				if len(want) != len(shares) {
					t.Fatalf("%s: %d values, %d rows", c, len(want), len(shares))
				}
				for j, s := range shares {
					if !reflect.DeepEqual(reflect.ValueOf(s).Field(i).Interface(), want[j]) {
						t.Errorf("row %d: %s is not the value of ZSHARE.%s", j+1, fields.Field(i).Name, c)
					}
				}
			}
			t.Logf("Shares: %d rows", len(shares))
		})
	}
}

func columnValues(t *testing.T, ro *sql.DB, query string, typ reflect.Type) []any {
	t.Helper()
	rows, err := ro.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []any
	for rows.Next() {
		v := reflect.New(typ)
		if err := rows.Scan(v.Interface()); err != nil {
			t.Fatal(err)
		}
		out = append(out, v.Elem().Interface())
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
