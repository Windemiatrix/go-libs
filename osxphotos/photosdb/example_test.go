package photosdb_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photosdb"
)

// The example has no Output comment: it needs a real library, so it is
// compiled but not run.
func ExampleOpen() {
	lib, err := photosdb.SystemLibraryPath()
	if err != nil {
		log.Fatal(err) // errors.Is(err, photosdb.ErrLibraryNotFound)
	}
	db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
	if err != nil {
		log.Fatal(err) // errors.Is(err, photosdb.ErrUnsupportedSchema): an unknown Photos version
	}
	defer func() { _ = db.Close() }()

	for _, p := range db.Photos() {
		if p.InTrash { // Photos() returns trashed photos too
			continue
		}
		fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
	}
}
