package photokit

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// CloudIdentifiers returns the cloud identifier, the stringValue of
// PHCloudIdentifier that LocalIdentifiers takes, of the assets ids, UUIDs or
// LocalIdentifiers as for Fetch, by the identifier as given. An id that
// names no asset (a hidden one too) or whose asset has no cloud identifier
// is left out: the map is empty for none, nil for no ids. An id that is not
// a valid string is ErrNotFound. ctx is checked only before the call.
// extension: not in osxphotos
func (l *Library) CloudIdentifiers(ctx context.Context, ids []string) (map[string]string, error) {
	return identifiers(ctx, ids, l.b.CloudIdentifiers, func(p bridge.IdentifierPair, id string) (string, bool) {
		return p.Cloud, p.Local == id || strings.HasPrefix(p.Local, id+"/")
	})
}

// LocalIdentifiers returns the LocalIdentifier of the assets of cloudIDs, by
// the identifier as given; one not resolved is left out, a hidden asset may
// resolve too. The map, ctx and the errors are as for CloudIdentifiers.
// extension: not in osxphotos
func (l *Library) LocalIdentifiers(ctx context.Context, cloudIDs []string) (map[string]string, error) {
	return identifiers(ctx, cloudIDs, l.b.LocalIdentifiers, func(p bridge.IdentifierPair, id string) (string, bool) { return p.Local, p.Cloud == id })
}

// The caller asks by the UUIDs of photosdb, PhotoKit answers by "UUID/L0/001":
// the map is keyed as asked. C would cut an id at a NUL and name another asset.
func identifiers(ctx context.Context, ids []string, call func([]string) ([]bridge.IdentifierPair, error), match func(bridge.IdentifierPair, string) (string, bool)) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if slices.ContainsFunc(ids, func(id string) bool { return !isCString(id) }) {
		return nil, fmt.Errorf("%w: identifier", ErrNotFound)
	}
	pairs, err := call(ids)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, id := range ids {
		for _, p := range pairs {
			if v, ok := match(p, id); ok && v != "" {
				out[id] = v
				break
			}
		}
	}
	return out, nil
}
