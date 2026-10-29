//go:build darwin && cgo

package bridge

// -Werror is not here: a warning of a newer clang would break go build for
// every user of the library. CI adds it through CGO_CFLAGS.

/*
#cgo CFLAGS: -Wall -Wextra -fobjc-arc
#cgo LDFLAGS: -framework Photos -framework Foundation
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"cmp"
	"context"
	"errors"
	"math"
	"time"
	"unicode/utf8"
	"unsafe"
)

type cgoBridge struct{}

var _ Bridge = (*cgoBridge)(nil)

// New returns the bridge of this build.
func New() Bridge { return &cgoBridge{} }

func (*cgoBridge) AuthorizationStatus() (int, error) {
	var status C.int
	if err := check(C.pk_authorization_status(&status)); err != nil {
		return 0, err
	}
	return int(status), nil
}

func (*cgoBridge) RequestAuthorization(timeout time.Duration) (int, error) {
	var status C.int
	if err := check(C.pk_request_authorization(C.int64_t(timeoutMS(timeout)), &status)); err != nil {
		return 0, err
	}
	return int(status), nil
}

func (*cgoBridge) FetchAssets(ids []string) ([]AssetRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cids := make([]*C.char, 0, len(ids))
	for _, id := range ids {
		cid := C.CString(id)
		defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffers of this call are freed here
		cids = append(cids, cid)
	}
	var out *C.pk_asset
	var count C.int
	r := C.pk_fetch_assets(&cids[0], C.int64_t(len(cids)), &out, &count)
	return records(r, out, count)
}

func (*cgoBridge) FetchBurst(burstID string, all bool) ([]AssetRecord, error) {
	cid := C.CString(burstID)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var out *C.pk_asset
	var count C.int
	r := C.pk_fetch_burst(cid, C.bool(all), &out, &count)
	return records(r, out, count)
}

func (*cgoBridge) AssetResources(id string) ([]ResourceRecord, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var out *C.pk_resource
	var count C.int
	r := C.pk_asset_resources(cid, &out, &count)
	defer C.pk_free_resources(out, count)
	if err := check(r); err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, nil
	}
	resources := unsafe.Slice(out, count) //nolint:gosec // G103: count is the length of the array bridge.m allocated
	recs := make([]ResourceRecord, len(resources))
	for i := range resources {
		name, nameErr := cString(C.GoString(resources[i].original_filename))
		uti, utiErr := cString(C.GoString(resources[i].uti))
		if err := cmp.Or(nameErr, utiErr); err != nil {
			return nil, err
		}
		recs[i] = ResourceRecord{Type: int(resources[i].resource_type), OriginalFilename: name, UTI: uti}
	}
	return recs, nil
}

func records(r C.pk_result, out *C.pk_asset, count C.int) ([]AssetRecord, error) {
	defer C.pk_free_assets(out, count)
	if err := check(r); err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, nil
	}
	assets := unsafe.Slice(out, count) //nolint:gosec // G103: count is the length of the array bridge.m allocated
	recs := make([]AssetRecord, len(assets))
	for i := range assets {
		var err error
		if recs[i], err = record(&assets[i]); err != nil {
			return nil, err
		}
	}
	return recs, nil
}

func record(a *C.pk_asset) (AssetRecord, error) {
	id, idErr := cString(C.GoString(a.local_identifier))
	burst, burstErr := cString(C.GoString(a.burst_identifier))
	if err := cmp.Or(idErr, burstErr); err != nil {
		return AssetRecord{}, err
	}
	r := AssetRecord{
		LocalIdentifier:  id,
		MediaType:        int(a.media_type),
		MediaSubtypes:    uint64(a.media_subtypes),
		BurstIdentifier:  burst,
		SourceType:       int(a.source_type),
		PixelWidth:       int(a.pixel_width),
		PixelHeight:      int(a.pixel_height),
		CreationDate:     date(bool(a.has_creation_date), float64(a.creation_date)),
		ModificationDate: date(bool(a.has_modification_date), float64(a.modification_date)),
		Duration:         duration(float64(a.duration)),
		Favorite:         bool(a.favorite),
		Hidden:           bool(a.hidden),
	}
	if a.has_location {
		lat, lon := float64(a.latitude), float64(a.longitude)
		r.Latitude, r.Longitude = &lat, &lon
	}
	return r, nil
}

// Below a microsecond, a double of seconds since 1970 carries rounding noise.
func date(ok bool, sec float64) *time.Time {
	if !ok {
		return nil
	}
	t := time.UnixMicro(int64(math.Round(sec * 1e6)))
	return &t
}

func duration(sec float64) time.Duration {
	return time.Duration(math.Round(sec * float64(time.Second)))
}

func check(r C.pk_result) error {
	defer C.free(unsafe.Pointer(r.message)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return codeError(int(r.code), C.GoString(r.message))
}

// codeError takes Go values, so that a test can reach it without cgo.
func codeError(code int, message string) error {
	switch code {
	case C.PK_OK:
		return nil
	case C.PK_TIMEOUT:
		return context.DeadlineExceeded
	}
	reason, err := cString(message)
	if err != nil {
		return err
	}
	return errors.New("photokit: Objective-C exception: " + reason)
}

func cString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("photokit: a string from PhotoKit is not valid UTF-8")
	}
	return s, nil
}
