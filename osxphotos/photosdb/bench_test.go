package photosdb

import (
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestOpenSynthetic(t *testing.T) {
	t.Parallel()
	lib := testlib.Synthetic(t, 2)
	db, err := Open(t.Context(), lib, Options{TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if n, want := len(db.Photos()), count(t, roDB(t, lib), "SELECT COUNT(*) FROM ZASSET"); n != want || want != 2000 || db.Info().SkippedAssets != 0 {
		t.Errorf("%d photos, %d skipped; want %d of 2000 assets, none skipped", n, db.Info().SkippedAssets, want)
	}
}

// BenchmarkOpen20k has a goal of less than 10 s per Open.
func BenchmarkOpen20k(b *testing.B) {
	lib := testlib.Synthetic(b, 20)
	tmp := b.TempDir()
	b.ResetTimer()
	for b.Loop() {
		db, err := Open(b.Context(), lib, Options{TempDir: tmp})
		if err != nil {
			b.Fatal(err)
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
