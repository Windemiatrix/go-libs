package gen

import (
	"path/filepath"
	"testing"
	"time"
)

// scaleLimit is a rough ceiling that catches quadratic algorithms and still
// holds under -race on CI runners.
const scaleLimit = 300 * time.Second

func TestGenerateScale(t *testing.T) {
	if testing.Short() {
		t.Skip("20000 assets: skipped with -short")
	}
	cfg := DefaultConfig()
	cfg.Assets = 20_000
	dir := filepath.Join(t.TempDir(), "Scale.photoslibrary")
	start := time.Now()
	if err := Generate(t.Context(), dir, cfg); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	t.Logf("Generate of %d assets: %v", cfg.Assets, took)
	if took > scaleLimit {
		t.Errorf("Generate of %d assets took %v, more than %v", cfg.Assets, took, scaleLimit)
	}
	db := openDB(t, dbPath(dir))
	start = time.Now()
	assertInvariants(t, db, schemas[DefaultSchema], false)
	t.Logf("invariants: %v", time.Since(start))
	if got := count(t, db, "SELECT COUNT(*) FROM ZASSET"); got != 20_000 {
		t.Errorf("assets: %d", got)
	}
}

func BenchmarkGenerate20k(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Assets = 20_000
	for b.Loop() {
		dir := filepath.Join(b.TempDir(), "Bench.photoslibrary")
		if err := Generate(b.Context(), dir, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
