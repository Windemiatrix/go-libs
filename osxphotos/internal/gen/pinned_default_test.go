package gen

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// TestPinnedDefault pins the library of DefaultConfig: the reference data in
// testdata/golden/synthetic is produced by golden-regen from this library
// (go run ./internal/gen/cmd -n 1000 -seed 1), so the SHA-256 of every table
// and of sqlite_master below describe the library that the reference data was
// made from. TestPinned pins 50 assets; everything that depends on the size of
// the library (the numbers of albums and folders, SameContentPairs, the
// max(1, round(share x Assets)) counts) is pinned only here.
//
// Change the values only together with a new golden-regen run and the data of
// testdata/golden/synthetic.
func TestPinnedDefault(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	// The workflow passes -n 1000 -seed 1 explicitly; they must stay equal to
	// DefaultConfig, which the reference data is compared with.
	if cfg.Assets != 1000 || cfg.Seed != 1 {
		t.Fatalf("DefaultConfig has Assets %d and Seed %d; golden-regen.yml generates 1000 assets with seed 1", cfg.Assets, cfg.Seed)
	}
	_, db := generate(t, cfg)
	sc := schemas[DefaultSchema]
	want := map[string]string{
		"ZADDITIONALASSETATTRIBUTES": "a0feab3f0ba6eb1fa1cfd9cdeae1d8e839d6a97deee0fa31e72c2397d144f69e",
		"ZASSET":                     "908e214ba7a9469c3c63af73df18a332b1669bedf1b0f6e51870e2971fa9fd9c",
		"ZCLOUDMASTER":               "0b16088332ad364ce67bf0430bcccfc8cbd138fcca4f2898f9e6a65bbee5fa77",
		"ZGENERICALBUM":              "0edff7f44c9549940cca5fb5ece6afd2ee993e93e1ca41aa3f7491f5b533a8e8",
		"ZINTERNALRESOURCE":          "a0c7073c0e59cfe254990ca61fdf0426231418c89d7a1b93747336597f063080",
		"ZSHARE":                     "91ccadb230eadb3630ded0ca16977f6d9b1d4bb1472effcebf4de6bc1e32780e",
		"Z_METADATA":                 "1499d1544d311f49bf443419b78f525da0f1027e769fea8bce2ee62b9b4d0816",
		"Z_PRIMARYKEY":               "217ea8c9942c73296138c4e5449e5bf4e3d9436fcf8167d8169729bd98136e46",
		"sqlite_master":              "b74f6dfd6522b50741b090d7c8f699dcbb8221ff6863fcbbc32bfd5d5a3d8c17",
		sc.members:                   "185264c4115e8f4d0030fefd689b1a8cb69c3d7b6007361aa60d8e716dc7e044",
	}
	got := dump(t, db)
	for _, table := range slices.Sorted(maps.Keys(want)) {
		if got[table] != want[table] {
			t.Errorf("SHA-256 of %s:\n got %s\nwant %s", table, got[table], want[table])
		}
	}
	// The DDL has many tables that the generator does not fill. A table that it
	// starts to fill must be pinned too. The R-tree shadow tables (Z_RT_*) are
	// written by SQLite, not by the generator, and are left out: their content
	// can change with the SQLite driver.
	for _, table := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[table]; ok || strings.HasPrefix(table, "Z_RT_") {
			continue
		}
		if n := count(t, db, `SELECT COUNT(*) FROM "`+table+`"`); n != 0 {
			t.Errorf("table %s has %d rows and is not pinned", table, n)
		}
	}
}
