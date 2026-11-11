//go:build darwin && cgo

package bridge

// -Werror is not here: a warning of a newer clang would break go build for
// every user of the library. CI adds it through CGO_CFLAGS. CoreLocation holds
// the CLLocation of an imported asset, which Photos declares but does not link.
// ImageIO reads the properties of an image, as Quartz does upstream
// (photokit.py:728-732). AVFoundation holds the AVURLAsset of a video, whose
// URL upstream copies (photokit.py:968), and the AVAssetExportSession that
// renders a slow-motion video (photokit.py:866-873).

/*
#cgo CFLAGS: -Wall -Wextra -fobjc-arc
#cgo LDFLAGS: -framework Photos -framework Foundation -framework CoreLocation
#cgo LDFLAGS: -framework ImageIO
#cgo LDFLAGS: -framework AVFoundation
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
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
	cids, free := cStrings(ids)
	defer free()
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

func (*cgoBridge) RequestResource(id string, typ int, sink ResourceSink) (Request, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var rid C.int32_t
	return startRequest(&cgoRequest{sink: sink, cancel: func() { C.pk_cancel_resource(rid) }}, func(h C.uintptr_t) C.pk_result {
		return C.pk_request_resource(cid, C.int64_t(typ), h, &rid)
	})
}

// The request has no id to cancel: pk_go_progress stops it.
func (*cgoBridge) RequestImage(id string, version int, sink ImageSink) (Request, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	return startRequest(&cgoRequest{sink: sink}, func(h C.uintptr_t) C.pk_result { return C.pk_request_image(cid, C.int64_t(version), h) })
}

func (*cgoBridge) RequestVideo(id string, version int, path func(string)) (Request, error) {
	return requestVideo(id, version, nil, &cgoRequest{path: path})
}

func (*cgoBridge) ExportComposition(id string, version int, path string, file func(string)) (Request, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	return requestVideo(id, version, cpath, &cgoRequest{path: file})
}

// The cancel needs the handle: bridge.m keeps the session of a render by it.
func requestVideo(id string, version int, renderPath *C.char, r *cgoRequest) (Request, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var rid C.int32_t
	var handle C.uintptr_t
	r.cancel = func() { C.pk_cancel_video(rid, handle) }
	return startRequest(r, func(h C.uintptr_t) C.pk_result {
		handle = h
		return C.pk_request_video(cid, C.int64_t(version), renderPath, h, &rid)
	})
}

// The completion handler may run long after the export returned, or never: the
// request stays known until it runs.
func startRequest(r *cgoRequest, start func(C.uintptr_t) C.pk_result) (Request, error) {
	r.done = make(chan error, 1)
	h := lastRequest.Add(1)
	requests.Store(h, r)
	if err := check(start(C.uintptr_t(h))); err != nil {
		requests.Delete(h)
		return nil, err
	}
	return r, nil
}

func (*cgoBridge) Import(r ImportRecord, timeout time.Duration) (string, error) {
	strs, free := cStrings(append([]string{r.Photo, r.Video, r.PairedVideo, r.AlternatePhoto, r.OriginalFilename}, r.Albums...))
	defer free()
	req := C.pk_import_request{
		photo: strs[0], video: strs[1], paired_video: strs[2], alternate_photo: strs[3], original_filename: strs[4],
		has_creation_date: C.bool(r.HasCreationDate), creation_date: C.double(r.CreationDate),
		has_location: C.bool(r.HasLocation), latitude: C.double(r.Latitude), longitude: C.double(r.Longitude),
		favorite: C.bool(r.Favorite), hidden: C.bool(r.Hidden),
	}
	var albums **C.char
	if len(r.Albums) > 0 {
		albums = &strs[5]
	}
	var id, domain *C.char
	var code C.int64_t
	err := check(C.pk_import(&req, albums, C.int64_t(len(r.Albums)), C.int64_t(timeoutMS(timeout)), &id, &code, &domain))
	defer C.free(unsafe.Pointer(id))     //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	if err != nil {
		return "", err
	}
	return changeResult(domain != nil, C.GoString(domain), int64(code), C.GoString(id))
}

func (*cgoBridge) Collections() ([]CollectionRecord, error) {
	var out *C.pk_collection
	var count C.int
	r := C.pk_collections(&out, &count)
	defer C.pk_free_collections(out, count)
	if err := check(r); err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, nil
	}
	collections := unsafe.Slice(out, count) //nolint:gosec // G103: count is the length of the array bridge.m allocated
	recs := make([]CollectionRecord, len(collections))
	for i, c := range collections {
		id, idErr := cString(C.GoString(c.local_identifier))
		title, titleErr := cString(C.GoString(c.title))
		parent, parentErr := cString(C.GoString(c.parent))
		if err := cmp.Or(idErr, titleErr, parentErr); err != nil {
			return nil, err
		}
		recs[i] = CollectionRecord{LocalIdentifier: id, Title: title, Parent: parent, IsFolder: bool(c.is_folder)}
	}
	return recs, nil
}

func (*cgoBridge) CreateCollection(title, parent string, folder bool, timeout time.Duration) (string, error) {
	ctitle, cparent := C.CString(title), C.CString(parent)
	defer C.free(unsafe.Pointer(ctitle))  //nolint:gosec // G103: the C.CString buffer of this call is freed here
	defer C.free(unsafe.Pointer(cparent)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var id, domain *C.char
	var code C.int64_t
	err := check(C.pk_create_collection(ctitle, cparent, C.bool(folder), C.int64_t(timeoutMS(timeout)), &id, &code, &domain))
	defer C.free(unsafe.Pointer(id))     //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	if err != nil {
		return "", err
	}
	return changeResult(domain != nil, C.GoString(domain), int64(code), C.GoString(id))
}

func (*cgoBridge) RenameCollection(id, title string, timeout time.Duration) error {
	cid, ctitle := C.CString(id), C.CString(title)
	defer C.free(unsafe.Pointer(cid))    //nolint:gosec // G103: the C.CString buffer of this call is freed here
	defer C.free(unsafe.Pointer(ctitle)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	var domain *C.char
	var code C.int64_t
	err := check(C.pk_rename_collection(cid, ctitle, C.int64_t(timeoutMS(timeout)), &code, &domain))
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return cmp.Or(err, nsError(domain != nil, C.GoString(domain), int64(code)))
}

func (*cgoBridge) DeleteCollections(ids []string, timeout time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	cids, free := cStrings(ids)
	defer free()
	var domain *C.char
	var code C.int64_t
	err := check(C.pk_delete_collections(&cids[0], C.int64_t(len(cids)), C.int64_t(timeoutMS(timeout)), &code, &domain))
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return cmp.Or(err, nsError(domain != nil, C.GoString(domain), int64(code)))
}

func (*cgoBridge) AlbumAssets(album string, assets []string, add bool, timeout time.Duration) error {
	if len(assets) == 0 {
		return nil
	}
	strs, free := cStrings(append([]string{album}, assets...))
	defer free()
	var domain *C.char
	var code C.int64_t
	err := check(C.pk_album_assets(strs[0], &strs[1], C.int64_t(len(assets)), C.bool(add), C.int64_t(timeoutMS(timeout)), &code, &domain))
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return cmp.Or(err, nsError(domain != nil, C.GoString(domain), int64(code)))
}

func (*cgoBridge) DeleteAssets(ids []string, timeout time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	cids, free := cStrings(ids)
	defer free()
	var domain *C.char
	var code C.int64_t
	err := check(C.pk_delete_assets(&cids[0], C.int64_t(len(cids)), C.int64_t(timeoutMS(timeout)), &code, &domain))
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return cmp.Or(err, nsError(domain != nil, C.GoString(domain), int64(code)))
}

func (*cgoBridge) UpdateAsset(id string, r PatchRecord, timeout time.Duration) error {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid)) //nolint:gosec // G103: the C.CString buffer of this call is freed here
	p := C.pk_patch{
		has_creation_date: C.bool(r.HasCreationDate), creation_date: C.double(r.CreationDate),
		has_favorite: C.bool(r.HasFavorite), favorite: C.bool(r.Favorite),
		has_hidden: C.bool(r.HasHidden), hidden: C.bool(r.Hidden),
		has_location: C.bool(r.HasLocation), latitude: C.double(r.Latitude), longitude: C.double(r.Longitude),
	}
	var domain *C.char
	var code C.int64_t
	err := check(C.pk_update_asset(cid, &p, C.int64_t(timeoutMS(timeout)), &code, &domain))
	defer C.free(unsafe.Pointer(domain)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	return cmp.Or(err, nsError(domain != nil, C.GoString(domain), int64(code)))
}

func (*cgoBridge) CloudIdentifiers(ids []string) ([]IdentifierPair, error) {
	return pairs(ids, func(cids **C.char, n C.int64_t, out **C.pk_identifier_pair, count *C.int) C.pk_result {
		return C.pk_cloud_identifiers(cids, n, out, count)
	})
}

func (*cgoBridge) LocalIdentifiers(cloudIDs []string) ([]IdentifierPair, error) {
	return pairs(cloudIDs, func(cids **C.char, n C.int64_t, out **C.pk_identifier_pair, count *C.int) C.pk_result {
		return C.pk_local_identifiers(cids, n, out, count)
	})
}

func pairs(ids []string, call func(**C.char, C.int64_t, **C.pk_identifier_pair, *C.int) C.pk_result) ([]IdentifierPair, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cids, free := cStrings(ids)
	defer free()
	var out *C.pk_identifier_pair
	var count C.int
	r := call(&cids[0], C.int64_t(len(cids)), &out, &count)
	defer C.pk_free_identifier_pairs(out, count)
	if err := check(r); err != nil {
		return nil, err
	}
	var recs []IdentifierPair
	for _, p := range unsafe.Slice(out, count) { //nolint:gosec // G103: count is the length of the array bridge.m allocated, 0 with out NULL
		local, localErr := cString(C.GoString(p.local_identifier))
		cloud, cloudErr := cString(C.GoString(p.cloud_identifier))
		if err := cmp.Or(localErr, cloudErr); err != nil {
			return nil, err
		}
		recs = append(recs, IdentifierPair{Local: local, Cloud: cloud})
	}
	return recs, nil
}

func cStrings(ss []string) ([]*C.char, func()) {
	cs := make([]*C.char, len(ss))
	for i, s := range ss {
		cs[i] = C.CString(s)
	}
	return cs, func() {
		for _, c := range cs {
			C.free(unsafe.Pointer(c)) //nolint:gosec // G103: the C.CString buffers of cStrings are freed here
		}
	}
}

type cgoRequest struct {
	sink             ResourceSink
	path             func(string)
	cancel           func()
	done             chan error
	failed, canceled atomic.Bool
}

var _ Request = (*cgoRequest)(nil)

// PhotoKit calls the handlers on queues of its own, in no order: a
// handler after the completion finds no request here, where the Value of a
// deleted cgo.Handle would panic.
var (
	requests    sync.Map
	lastRequest atomic.Uintptr
)

func (r *cgoRequest) Cancel() {
	if r.canceled.CompareAndSwap(false, true) && r.cancel != nil {
		r.cancel()
	}
}

func (r *cgoRequest) Done() <-chan error { return r.done }

// The exported functions below do not recover: a panic recovered here would
// leave a partial file looking whole or Export waiting forever; unrecovered,
// it ends the process, as in any goroutine.

//export pk_go_write
func pk_go_write(h C.uintptr_t, p unsafe.Pointer, n C.size_t) {
	if r, ok := requests.Load(uintptr(h)); ok && n > 0 {
		r.(*cgoRequest).write(unsafe.Slice((*byte)(p), n)) //nolint:gosec // G103: n is the length of the NSData of the call, kept by PhotoKit until it returns
	}
}

// The answer is the stop of the progress handler of an image request.
//
//export pk_go_progress
func pk_go_progress(h C.uintptr_t, fraction C.double) C.bool {
	r, ok := requests.Load(uintptr(h))
	if !ok {
		return true
	}
	r.(*cgoRequest).sink.Progress(float64(fraction))
	return C.bool(r.(*cgoRequest).canceled.Load())
}

// Every pointer of info, as p, lives until the call returns, so everything is
// copied here.
//
//export pk_go_image
func pk_go_image(h C.uintptr_t, p unsafe.Pointer, n C.size_t, info *C.pk_image_info) {
	r, ok := requests.Load(uintptr(h))
	if !ok {
		return
	}
	r.(*cgoRequest).write(unsafe.Slice((*byte)(p), n)) //nolint:gosec // G103: n is the length of the NSData of the call, kept by bridge.m until it returns
	r.(*cgoRequest).sink.(ImageSink).Info(ImageRecord{
		UTI:         C.GoString(info.uti),
		Orientation: int(info.orientation),
		Degraded:    bool(info.degraded),
		Path:        C.GoString(info.path),
		Metadata:    metadataBytes(info.metadata, int64(info.metadata_len)),
	})
}

// NULL is a URL that names no file: no file, as for an AVComposition.
//
//export pk_go_video
func pk_go_video(h C.uintptr_t, path *C.char) {
	if r, ok := requests.Load(uintptr(h)); ok && path != nil {
		r.(*cgoRequest).path(C.GoString(path))
	}
}

// pk_go_progress cannot answer a render, which has no sink; an ended request
// needs no render either.
//
//export pk_go_canceled
func pk_go_canceled(h C.uintptr_t) C.bool {
	r, ok := requests.Load(uintptr(h))
	return C.bool(!ok || r.(*cgoRequest).canceled.Load())
}

//export pk_go_done
func pk_go_done(h C.uintptr_t, code C.int64_t, domain *C.char) {
	r, ok := requests.LoadAndDelete(uintptr(h))
	if !ok {
		// A second completion is a bug of the bridge; returning would hide it.
		panic("photokit: completion of an unknown request")
	}
	r.(*cgoRequest).finish(domain != nil, C.GoString(domain), int64(code))
}

// write, metadataBytes, finish and changeResult take Go values, so that a test can reach them without PhotoKit.

func (r *cgoRequest) write(p []byte) {
	if !r.failed.Load() && r.sink.Write(p) != nil {
		r.failed.Store(true)
	}
}

// photokit drops a list above 1 MiB unread (version.go), so it is not copied
// either.
func metadataBytes(p unsafe.Pointer, n int64) []byte {
	if n > 1<<20 {
		return nil
	}
	return bytes.Clone(unsafe.Slice((*byte)(p), n)) //nolint:gosec // G103: n is the length of the NSData of the call, kept by bridge.m until it returns
}

// Without an NSError, code is one of bridge.h: an image request reports with
// it no data or an exception in its handler, a video request an exception, a
// composition another AVAsset or a session neither completed nor failed.
func (r *cgoRequest) finish(failed bool, domain string, code int64) {
	err := codeError(int(code), "")
	if failed {
		err = nsError(true, domain, code)
	}
	if r.canceled.Load() {
		err = ErrCanceled
	}
	r.done <- err
}

func changeResult(failed bool, domain string, code int64, id string) (string, error) {
	if err := nsError(failed, domain, code); err != nil {
		return "", err
	}
	return cString(id)
}

// Done and the changes give an NSError only as a *PhotoKitError, so a domain that is not UTF-8 is dropped, not the error.
func nsError(failed bool, domain string, code int64) error {
	if !failed {
		return nil
	}
	d, _ := cString(domain)
	return &PhotoKitError{Domain: d, Code: code}
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
	case C.PK_NO_RESOURCE:
		return ErrNoResource
	case C.PK_NOT_FOUND:
		return ErrNoAlbum
	case C.PK_NO_ASSET:
		return ErrNoAsset
	case C.PK_NO_COMPOSITION:
		return ErrNoComposition
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
