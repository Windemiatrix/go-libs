// cgo copies the preamble of cgo_darwin.go, which includes this file, into
// _cgo_export.h, and bridge.m includes both.
#pragma once

#include <stdbool.h>
#include <stdint.h>

enum { PK_OK, PK_EXCEPTION, PK_TIMEOUT, PK_NO_RESOURCE, PK_NOT_FOUND, PK_NO_ASSET };

// message is NULL unless code is PK_EXCEPTION; the caller frees it.
typedef struct {
	int code;
	char *message;
} pk_result;

// Every identifier and title, except the ids of the fetches, the resources, the
// images and the videos, is UTF-8 without NUL: photokit checks it before the
// call, and a pk_* function checks only the state of PhotoKit. One that is not
// UTF-8 is a bug of the caller and raises before any change (PK_EXCEPTION); C
// cuts one at a NUL, which only photokit sees. The excepted ids that are not
// UTF-8 find no asset.

pk_result pk_authorization_status(int *status);
// A timeout_ms of 0 does not wait.
pk_result pk_request_authorization(int64_t timeout_ms, int *status);

// A string is NULL when PhotoKit gives none; a date or the coordinates hold only
// with their has_ field. Dates are seconds since 1970, the duration seconds.
typedef struct {
	char *local_identifier, *burst_identifier;
	int media_type, source_type, pixel_width, pixel_height;
	uint64_t media_subtypes;
	bool has_creation_date, has_modification_date, has_location, favorite, hidden;
	double creation_date, modification_date, latitude, longitude, duration;
} pk_asset;

// Neither finds a hidden asset, as upstream (photokit.py:1234-1237, 1281-1285).
// *out holds *count assets in the order of PhotoKit, or is NULL with *count 0
// on an error or when none is found; pk_free_assets frees it in every case.
pk_result pk_fetch_assets(const char *const *ids, int64_t n, pk_asset **out, int *count);
pk_result pk_fetch_burst(const char *burst_id, bool include_all, pk_asset **out, int *count);
void pk_free_assets(pk_asset *assets, int count);

// A string is NULL when PhotoKit gives none; type is a keyword in Go.
typedef struct {
	char *original_filename, *uti;
	int resource_type;
} pk_resource;

// *out holds the *count resources of the asset local_id in the order of
// PhotoKit, or is NULL with *count 0 on an error or when there is no such
// asset or it is hidden; pk_free_resources frees it in every case.
pk_result pk_asset_resources(const char *local_id, pk_resource **out, int *count);
void pk_free_resources(pk_resource *resources, int count);

// Starts requestDataForAssetResource for the first resource of the type of
// the asset local_id and sets *request_id; no such asset, a hidden asset or no
// such resource is PK_NO_RESOURCE and starts nothing. Once started, the request
// calls pk_go_write, pk_go_progress and, at most once, pk_go_done with handle,
// in an order PhotoKit does not promise; Go ignores a call after pk_go_done. A
// stalled iCloud download may never end.
pk_result pk_request_resource(const char *local_id, int64_t type, uintptr_t handle, int32_t *request_id);
void pk_cancel_resource(int32_t request_id);

// The pointers live until pk_go_image returns. A string is NULL when PhotoKit
// gives none; metadata is NULL when the properties cannot be serialized.
typedef struct {
	const char *uti, *path;
	const void *metadata;
	int64_t metadata_len;
	int orientation;
	bool degraded;
} pk_image_info;

// Starts requestImageDataAndOrientationForAsset for the asset local_id with
// version; no such asset or a hidden one is PK_NO_RESOURCE and starts nothing.
// Once started, the request calls pk_go_progress, whose answer true stops it,
// pk_go_image once when PhotoKit gives data, and pk_go_done once with handle:
// an NSError, or no domain with PK_OK, PK_NO_RESOURCE when PhotoKit gives no
// data, or PK_EXCEPTION.
pk_result pk_request_image(const char *local_id, int64_t version, uintptr_t handle);

// Starts requestAVAssetForVideo for the asset local_id with version, a
// PHVideoRequestOptionsVersion, and sets *request_id; no such asset or a hidden
// one is PK_NO_RESOURCE and starts nothing. Once started, the request calls
// pk_go_video once when PhotoKit gives an AVURLAsset of a file URL, with its
// path (NULL when the URL gives none), and, at most once, pk_go_done with
// handle: an NSError, or no domain with PK_OK or PK_EXCEPTION.
pk_result pk_request_video(const char *local_id, int64_t version, uintptr_t handle, int32_t *request_id);
void pk_cancel_video(int32_t request_id);

// A path or the name is "" when not given, and photo is "" for a video; the
// date, seconds since 1970, and the coordinates hold only with their has_ field.
typedef struct {
	const char *photo, *video, *paired_video, *alternate_photo, *original_filename;
	bool has_creation_date, has_location, favorite, hidden;
	double creation_date, latitude, longitude;
} pk_import_request;

// Creates the asset of r in its n_albums albums, LocalIdentifiers or UUIDs in
// one form per album, in one performChanges, and waits up to timeout_ms for its
// completion (0 does not wait); r and albums are not read after the return. An
// album not found or that cannot take an asset is PK_NOT_FOUND and changes
// nothing; an exception in a change that PhotoKit commits is PK_TIMEOUT.
// An original_filename that is not UTF-8 keeps the name of the file, so the
// caller checks it. A failed change sets *ns_domain, "" when the NSError has
// none, and *ns_code; otherwise *identifier is the LocalIdentifier. The caller
// passes both strings NULL and frees them.
pk_result pk_import(const pk_import_request *r, const char *const *albums, int64_t n_albums, int64_t timeout_ms, char **identifier, int64_t *ns_code, char **ns_domain);

// The title is NULL when PhotoKit gives none, the parent "" at the top level.
typedef struct {
	char *local_identifier, *title, *parent;
	bool is_folder;
} pk_collection;

// *out holds the *count user folders and albums, a folder before what it holds,
// in the order of PhotoKit, or is NULL with *count 0 on an error or when there
// are none; pk_free_collections frees it in every case. Smart folders and smart
// albums are left out.
pk_result pk_collections(pk_collection **out, int *count);
void pk_free_collections(pk_collection *collections, int count);

// Each runs one performChanges and waits as pk_import does, and reads no string
// after the return. A collection, or a parent ("" is the top level), that is not
// found or cannot take the change is PK_NOT_FOUND and changes nothing.
// *ns_code, *ns_domain and the *identifier of the created collection are as for
// pk_import. pk_delete_collections deletes all of the n ids, each once, or
// none. A folder as the album of pk_album_assets is PK_NOT_FOUND; an asset that
// is not found or hidden is PK_NO_ASSET and changes nothing.
pk_result pk_create_collection(const char *title, const char *parent, bool folder, int64_t timeout_ms, char **identifier, int64_t *ns_code, char **ns_domain);
pk_result pk_rename_collection(const char *local_id, const char *title, int64_t timeout_ms, int64_t *ns_code, char **ns_domain);
pk_result pk_delete_collections(const char *const *ids, int64_t n, int64_t timeout_ms, int64_t *ns_code, char **ns_domain);
pk_result pk_album_assets(const char *album, const char *const *assets, int64_t n, bool add, int64_t timeout_ms, int64_t *ns_code, char **ns_domain);

// Deletes the n assets, UUIDs or LocalIdentifiers in one form per asset, in one
// performChanges and waits up to timeout_ms as pk_import does; ids are not read
// after the return. An asset that is not found, hidden or cannot be deleted is
// PK_NO_ASSET and changes nothing. *ns_code and *ns_domain are as for
// pk_import.
pk_result pk_delete_assets(const char *const *ids, int64_t n, int64_t timeout_ms, int64_t *ns_code, char **ns_domain);

// A value holds only with its has_ field; the date is seconds since 1970, the
// coordinates degrees.
typedef struct {
	bool has_creation_date, has_favorite, has_hidden, has_location, favorite, hidden;
	double creation_date, latitude, longitude;
} pk_patch;

// Changes the asset local_id, a UUID or a LocalIdentifier, in one
// performChanges and waits up to timeout_ms as pk_import does; local_id and p
// are not read after the return. An asset that is not found, hidden or cannot
// be changed is PK_NO_ASSET and changes nothing. *ns_code and *ns_domain are as
// for pk_import.
pk_result pk_update_asset(const char *local_id, const pk_patch *p, int64_t timeout_ms, int64_t *ns_code, char **ns_domain);

typedef struct {
	char *local_identifier, *cloud_identifier;
} pk_identifier_pair;

// *out holds the *count pairs of the assets of ids, UUIDs or LocalIdentifiers,
// that have a cloud identifier, or of the cloud identifiers cloud_ids that the
// library resolves, or is NULL with *count 0 on an error or when there are
// none; pk_free_identifier_pairs frees it in every case. pk_cloud_identifiers
// does not find a hidden asset, as pk_fetch_assets.
pk_result pk_cloud_identifiers(const char *const *ids, int64_t n, pk_identifier_pair **out, int *count);
pk_result pk_local_identifiers(const char *const *cloud_ids, int64_t n, pk_identifier_pair **out, int *count);
void pk_free_identifier_pairs(pk_identifier_pair *pairs, int count);
