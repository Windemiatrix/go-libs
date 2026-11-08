//go:build !(darwin && cgo)

package bridge

import "time"

type stub struct{}

var _ Bridge = (*stub)(nil)

// New returns the bridge of this build.
func New() Bridge { return &stub{} }

func (*stub) AuthorizationStatus() (int, error) { return 0, ErrNotSupported }

func (*stub) RequestAuthorization(time.Duration) (int, error) { return 0, ErrNotSupported }

func (*stub) FetchAssets([]string) ([]AssetRecord, error) { return nil, ErrNotSupported }

func (*stub) FetchBurst(string, bool) ([]AssetRecord, error) { return nil, ErrNotSupported }

func (*stub) AssetResources(string) ([]ResourceRecord, error) { return nil, ErrNotSupported }

func (*stub) RequestResource(string, int, ResourceSink) (Request, error) { return nil, ErrNotSupported }

func (*stub) RequestImage(string, int, ImageSink) (Request, error) { return nil, ErrNotSupported }

func (*stub) Import(ImportRecord, time.Duration) (string, error) { return "", ErrNotSupported }

func (*stub) Collections() ([]CollectionRecord, error) { return nil, ErrNotSupported }

func (*stub) CreateCollection(string, string, bool, time.Duration) (string, error) {
	return "", ErrNotSupported
}

func (*stub) RenameCollection(string, string, time.Duration) error { return ErrNotSupported }

func (*stub) DeleteCollections([]string, time.Duration) error { return ErrNotSupported }

func (*stub) AlbumAssets(string, []string, bool, time.Duration) error { return ErrNotSupported }

func (*stub) DeleteAssets([]string, time.Duration) error { return ErrNotSupported }

func (*stub) UpdateAsset(string, PatchRecord, time.Duration) error { return ErrNotSupported }

func (*stub) CloudIdentifiers([]string) ([]IdentifierPair, error) { return nil, ErrNotSupported }

func (*stub) LocalIdentifiers([]string) ([]IdentifierPair, error) { return nil, ErrNotSupported }
