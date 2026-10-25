package photosdb

import (
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/gen"
)

// BenchmarkOpen20k opens a synthetic library of 20000 assets: the copy, the
// checks, the readers and the loading. The goal is less than 10 s per Open.
func BenchmarkOpen20k(b *testing.B) {
	cfg := gen.DefaultConfig()
	cfg.Assets = 20000
	lib := genLibrary(b, cfg)
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
