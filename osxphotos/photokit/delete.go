package photokit

import (
	"context"
	"slices"
	"time"
)

// Delete removes the assets, UUIDs or LocalIdentifiers as for Fetch, from the
// library in one change: Photos asks the user once for the whole batch and
// moves them to Recently Deleted. Before the change, and without a dialog, an
// asset that is missing (a hidden one too, as for Fetch), or that cannot be
// deleted, is ErrNotFound and nothing was changed; the batch is all or none.
// A refusal of the user is ErrUserCanceled and nothing was changed; a failure
// of PhotoKit ErrChange. A ctx done before the call is ctx.Err(); a ctx done
// before the outcome is ErrOutcomeUnknown with ctx.Err(): the dialog stays
// open and a later answer still deletes the assets, so read the library again
// before the next change. An empty assets changes nothing. An asset named both
// by its UUID and by its LocalIdentifier is ErrNotFound.
// extension: not in osxphotos
func (l *Library) Delete(ctx context.Context, assets []string) error {
	if len(assets) == 0 {
		return nil
	}
	ids := slices.Clone(assets)
	_, err := collectionChange(ctx, "asset", ids, func(timeout time.Duration) (string, error) {
		return "", l.b.DeleteAssets(ids, timeout)
	})
	return err
}
