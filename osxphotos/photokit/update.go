package photokit

import (
	"context"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photokit/internal/bridge"
)

// AssetPatch is what Update changes; a nil field is left as it is. A location
// cannot be removed. Location must be a valid coordinate: a latitude within
// ±90, a longitude within ±180, and not -180, -180, which osxphotos reads as no
// location; PhotoKit stores what it is given.
// extension: not in osxphotos
type AssetPatch struct {
	CreationDate     *time.Time
	Favorite, Hidden *bool
	Location         *Location
}

// Update changes the asset id, a UUID or a LocalIdentifier as for Fetch, in
// one change. Hidden true asks the user once; after it the asset is not found
// by Fetch or Update any more, as a hidden asset is not, so Update cannot show
// it again. The other fields change without a dialog, and so does Hidden false
// on a visible asset. An empty patch changes nothing and calls nothing. Before
// the change, and without a dialog, an asset that is missing (a hidden one
// too) or that cannot be changed is ErrNotFound. A refusal of the user is
// ErrUserCanceled and nothing was changed; a failure of PhotoKit ErrChange. A
// ctx done before the call is ctx.Err(); a ctx done before the outcome is
// ErrOutcomeUnknown with ctx.Err(): the change may still take effect, and with
// Hidden true the dialog stays open and a later answer still hides the asset,
// so read the library again before the next change.
// extension: not in osxphotos
func (l *Library) Update(ctx context.Context, id string, p AssetPatch) error {
	if p == (AssetPatch{}) {
		return nil
	}
	rec := patchRecord(p)
	_, err := collectionChange(ctx, "asset", []string{id}, func(timeout time.Duration) (string, error) {
		return "", l.b.UpdateAsset(id, rec, timeout)
	})
	return err
}

func patchRecord(p AssetPatch) bridge.PatchRecord {
	var r bridge.PatchRecord
	if p.CreationDate != nil {
		r.HasCreationDate, r.CreationDate = true, float64(p.CreationDate.UnixMicro())/1e6
	}
	if p.Favorite != nil {
		r.HasFavorite, r.Favorite = true, *p.Favorite
	}
	if p.Hidden != nil {
		r.HasHidden, r.Hidden = true, *p.Hidden
	}
	if p.Location != nil {
		r.HasLocation, r.Latitude, r.Longitude = true, p.Location.Latitude, p.Location.Longitude
	}
	return r
}
