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
