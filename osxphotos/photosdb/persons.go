package photosdb

import (
	"maps"
	"slices"
)

// Persons returns the names of the persons on the photos of Photos(), each
// once, in byte order, "_UNKNOWN_" for the persons without a name; nil when no
// photo has a face of a person. The slice is new on every call
// (photosdb/photosdb.py:553-557).
func (db *DB) Persons() []string {
	return slices.Sorted(maps.Keys(db.PersonCounts()))
}

// PersonCounts returns the number of faces of each name of Persons on the
// photos of Photos(); the map is new on every call (photosdb/photosdb.py:498-509).
func (db *DB) PersonCounts() map[string]int {
	counts := map[string]int{}
	for _, p := range db.photos {
		for _, name := range p.Persons {
			counts[name]++
		}
	}
	return counts
}
