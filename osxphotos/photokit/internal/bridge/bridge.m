//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Photos/Photos.h>
#include <stdlib.h>
#include <string.h>

#include "_cgo_export.h"
#include "bridge.h"

// photokit maps these numbers in Go, where the PhotoKit headers cannot be
// read: a change in them must fail the build.
_Static_assert(PHAuthorizationStatusNotDetermined == 0, "PHAuthorizationStatusNotDetermined");
_Static_assert(PHAuthorizationStatusRestricted == 1, "PHAuthorizationStatusRestricted");
_Static_assert(PHAuthorizationStatusDenied == 2, "PHAuthorizationStatusDenied");
_Static_assert(PHAuthorizationStatusAuthorized == 3, "PHAuthorizationStatusAuthorized");
_Static_assert(PHAuthorizationStatusLimited == 4, "PHAuthorizationStatusLimited");
_Static_assert(PHAssetMediaTypeUnknown == 0, "PHAssetMediaTypeUnknown");
_Static_assert(PHAssetMediaTypeImage == 1, "PHAssetMediaTypeImage");
_Static_assert(PHAssetMediaTypeVideo == 2, "PHAssetMediaTypeVideo");
_Static_assert(PHAssetMediaTypeAudio == 3, "PHAssetMediaTypeAudio");
_Static_assert(PHAssetMediaSubtypePhotoPanorama == (1 << 0), "PHAssetMediaSubtypePhotoPanorama");
_Static_assert(PHAssetMediaSubtypePhotoHDR == (1 << 1), "PHAssetMediaSubtypePhotoHDR");
_Static_assert(PHAssetMediaSubtypePhotoScreenshot == (1 << 2), "PHAssetMediaSubtypePhotoScreenshot");
_Static_assert(PHAssetMediaSubtypePhotoLive == (1 << 3), "PHAssetMediaSubtypePhotoLive");
_Static_assert(PHAssetMediaSubtypePhotoDepthEffect == (1 << 4), "PHAssetMediaSubtypePhotoDepthEffect");
_Static_assert(PHAssetMediaSubtypeVideoStreamed == (1 << 16), "PHAssetMediaSubtypeVideoStreamed");
_Static_assert(PHAssetMediaSubtypeVideoHighFrameRate == (1 << 17), "PHAssetMediaSubtypeVideoHighFrameRate");
_Static_assert(PHAssetMediaSubtypeVideoTimelapse == (1 << 18), "PHAssetMediaSubtypeVideoTimelapse");
_Static_assert(PHAssetResourceTypePhoto == 1, "PHAssetResourceTypePhoto");
_Static_assert(PHAssetResourceTypeVideo == 2, "PHAssetResourceTypeVideo");
_Static_assert(PHAssetResourceTypeAudio == 3, "PHAssetResourceTypeAudio");
_Static_assert(PHAssetResourceTypeAlternatePhoto == 4, "PHAssetResourceTypeAlternatePhoto");
_Static_assert(PHAssetResourceTypeFullSizePhoto == 5, "PHAssetResourceTypeFullSizePhoto");
_Static_assert(PHAssetResourceTypeFullSizeVideo == 6, "PHAssetResourceTypeFullSizeVideo");
_Static_assert(PHAssetResourceTypeAdjustmentData == 7, "PHAssetResourceTypeAdjustmentData");
_Static_assert(PHAssetResourceTypeAdjustmentBasePhoto == 8, "PHAssetResourceTypeAdjustmentBasePhoto");
_Static_assert(PHAssetResourceTypePairedVideo == 9, "PHAssetResourceTypePairedVideo");
_Static_assert(PHAssetResourceTypeFullSizePairedVideo == 10, "PHAssetResourceTypeFullSizePairedVideo");
_Static_assert(PHAssetResourceTypeAdjustmentBasePairedVideo == 11, "PHAssetResourceTypeAdjustmentBasePairedVideo");
_Static_assert(PHAssetResourceTypeAdjustmentBaseVideo == 12, "PHAssetResourceTypeAdjustmentBaseVideo");
_Static_assert(PHPhotosErrorUserCancelled == 3072, "PHPhotosErrorUserCancelled");
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000 // the name came with the macOS 26 SDK; the build targets macOS 13
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wunguarded-availability-new"
_Static_assert(PHAssetMediaSubtypeVideoScreenRecording == (1 << 19), "PHAssetMediaSubtypeVideoScreenRecording");
#pragma clang diagnostic pop
#endif
// The handler may run after pk_request_authorization has returned on its
// timeout, so it holds only this object, which ARC frees after the last of
// the two lets it go. The semaphore starts at 0: disposing of one whose value
// is below its initial value ends the process. The class prefix is not PK,
// which Apple uses: two classes of one name in a process make the runtime
// pick either.
@interface OSXPhotosAuthRequest : NSObject
@property(atomic) NSInteger status;
@property(nonatomic, strong, readonly) dispatch_semaphore_t done;
@end

@implementation OSXPhotosAuthRequest
- (instancetype)init {
	if ((self = [super init])) {
		_done = dispatch_semaphore_create(0);
	}
	return self;
}
@end

// The string or its UTF8String may be nil, and strdup(NULL) crashes.
static char *pk_strdup(NSString *s) { return s.UTF8String ? strdup(s.UTF8String) : NULL; }

static pk_result pk_exception(NSException *e) {
	return (pk_result){PK_EXCEPTION, pk_strdup(e.reason ?: e.name)};
}

pk_result pk_authorization_status(int *status) {
	pk_result r = {PK_OK, NULL};
	@autoreleasepool {
		@try {
			*status = (int)[PHPhotoLibrary authorizationStatusForAccessLevel:PHAccessLevelReadWrite];
		} @catch (NSException *e) {
			r = pk_exception(e);
		}
	}
	return r;
}

// One request with requestAuthorizationForAccessLevel:handler:, which replaced
// the requestAuthorization: that upstream calls (photokit.py:139-160).
pk_result pk_request_authorization(int64_t timeout_ms, int *status) {
	pk_result r = {PK_OK, NULL};
	@autoreleasepool {
		@try {
			OSXPhotosAuthRequest *request = [[OSXPhotosAuthRequest alloc] init];
			[PHPhotoLibrary requestAuthorizationForAccessLevel:PHAccessLevelReadWrite
			                                           handler:^(PHAuthorizationStatus answer) {
				// The handler runs on a queue of PhotoKit, outside the @try
				// of this function: an exception there would end the process.
				@autoreleasepool {
					@try {
						request.status = answer;
					} @catch (NSException *e) {
						// -1 is outside PHAuthorizationStatus, so Go reports it as an error.
						request.status = -1;
					}
				}
				dispatch_semaphore_signal(request.done);
			}];
			dispatch_time_t deadline = dispatch_time(DISPATCH_TIME_NOW, timeout_ms * (int64_t)NSEC_PER_MSEC);
			if (dispatch_semaphore_wait(request.done, deadline) == 0) {
				*status = (int)request.status;
			} else {
				r.code = PK_TIMEOUT;
			}
		} @catch (NSException *e) {
			r = pk_exception(e);
		}
	}
	return r;
}

static void pk_fill_asset(PHAsset *asset, pk_asset *out) {
	out->local_identifier = pk_strdup(asset.localIdentifier);
	out->burst_identifier = pk_strdup(asset.burstIdentifier);
	out->media_type = (int)asset.mediaType;
	out->media_subtypes = asset.mediaSubtypes;
	out->source_type = (int)asset.sourceType;
	out->pixel_width = (int)asset.pixelWidth;
	out->pixel_height = (int)asset.pixelHeight;
	out->has_creation_date = asset.creationDate != nil;
	out->creation_date = asset.creationDate.timeIntervalSince1970;
	out->has_modification_date = asset.modificationDate != nil;
	out->modification_date = asset.modificationDate.timeIntervalSince1970;
	CLLocation *location = asset.location;
	out->has_location = location != nil;
	out->latitude = location.coordinate.latitude;
	out->longitude = location.coordinate.longitude;
	out->duration = asset.duration;
	out->favorite = asset.favorite;
	out->hidden = asset.hidden;
}

static pk_result pk_fetch(PHFetchResult<PHAsset *> *(^fetch)(PHFetchOptions *), pk_asset **out, int *count) {
	pk_result r = {PK_OK, NULL};
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		@try {
			PHFetchResult<PHAsset *> *assets = fetch([[PHFetchOptions alloc] init]);
			if (assets.count > 0) {
				*out = calloc(assets.count, sizeof(pk_asset));
				if (*out == NULL) {
					[NSException raise:NSGenericException format:@"no memory for %lu assets", (unsigned long)assets.count];
				}
				*count = (int)assets.count;
				for (int i = 0; i < *count; i++) {
					pk_fill_asset(assets[i], &(*out)[i]);
				}
			}
		} @catch (NSException *e) {
			// calloc zeroed the assets not yet filled, so all of them are freed.
			pk_free_assets(*out, *count);
			*out = NULL;
			*count = 0;
			r = pk_exception(e);
		}
	}
	return r;
}

pk_result pk_fetch_assets(const char *const *ids, int64_t n, pk_asset **out, int *count) {
	return pk_fetch(^(PHFetchOptions *options) {
		NSMutableArray<NSString *> *identifiers = [NSMutableArray arrayWithCapacity:(NSUInteger)n];
		for (int64_t i = 0; i < n; i++) {
			// A string that is not UTF-8 names no asset.
			NSString *identifier = [NSString stringWithUTF8String:ids[i]];
			if (identifier != nil) {
				[identifiers addObject:identifier];
			}
		}
		return [PHAsset fetchAssetsWithLocalIdentifiers:identifiers options:options];
	}, out, count);
}

pk_result pk_fetch_burst(const char *burst_id, bool include_all, pk_asset **out, int *count) {
	return pk_fetch(^(PHFetchOptions *options) {
		options.includeAllBurstAssets = include_all;
		NSString *burst = [NSString stringWithUTF8String:burst_id];
		return burst != nil ? [PHAsset fetchAssetsWithBurstIdentifier:burst options:options] : nil;
	}, out, count);
}

void pk_free_assets(pk_asset *assets, int count) {
	for (int i = 0; i < count; i++) {
		free(assets[i].local_identifier);
		free(assets[i].burst_identifier);
	}
	free(assets);
}

static NSArray<PHAssetResource *> *pk_resources(const char *local_id) {
	// A string that is not UTF-8 names no asset, and @[nil] would raise.
	NSString *identifier = [NSString stringWithUTF8String:local_id];
	NSArray<NSString *> *identifiers = identifier != nil ? @[ identifier ] : @[];
	PHAsset *asset = [PHAsset fetchAssetsWithLocalIdentifiers:identifiers options:nil].firstObject;
	return asset != nil ? [PHAssetResource assetResourcesForAsset:asset] : nil;
}

pk_result pk_asset_resources(const char *local_id, pk_resource **out, int *count) {
	pk_result r = {PK_OK, NULL};
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		@try {
			NSArray<PHAssetResource *> *resources = pk_resources(local_id);
			if (resources.count > 0) {
				*out = calloc(resources.count, sizeof(pk_resource));
				if (*out == NULL) {
					[NSException raise:NSGenericException format:@"no memory for %lu resources", (unsigned long)resources.count];
				}
				*count = (int)resources.count;
				for (int i = 0; i < *count; i++) {
					(*out)[i].original_filename = pk_strdup(resources[i].originalFilename);
					(*out)[i].uti = pk_strdup(resources[i].uniformTypeIdentifier);
					(*out)[i].resource_type = (int)resources[i].type;
				}
			}
		} @catch (NSException *e) {
			// calloc zeroed the resources not yet filled, so all of them are freed.
			pk_free_resources(*out, *count);
			*out = NULL;
			*count = 0;
			r = pk_exception(e);
		}
	}
	return r;
}

void pk_free_resources(pk_resource *resources, int count) {
	for (int i = 0; i < count; i++) {
		free(resources[i].original_filename);
		free(resources[i].uti);
	}
	free(resources);
}

// _request_resource_data (photokit.py:693-736). The handlers may run on queues
// of PhotoKit after this function has returned, so they hold only handle, a
// number: C keeps no pointer of Go. The request is the last call of the @try:
// after an exception no handler runs, and Go forgets the handle.
pk_result pk_request_resource(const char *local_id, int64_t type, uintptr_t handle, int32_t *request_id) {
	pk_result r = {PK_NO_RESOURCE, NULL};
	@autoreleasepool {
		@try {
			for (PHAssetResource *resource in pk_resources(local_id)) {
				if (resource.type != type) {
					continue;
				}
				PHAssetResourceRequestOptions *options = [[PHAssetResourceRequestOptions alloc] init];
				// An original kept only in iCloud is downloaded (photokit.py:705).
				options.networkAccessAllowed = YES;
				// extension: not in osxphotos; it feeds ExportOptions.Progress.
				options.progressHandler = ^(double progress) {
					@autoreleasepool {
						pk_go_progress(handle, progress);
					}
				};
				*request_id = [[PHAssetResourceManager defaultManager] requestDataForAssetResource:resource
					options:options
					dataReceivedHandler:^(NSData *data) {
						// Go copies the bytes before it returns; data is not kept.
						@autoreleasepool {
							pk_go_write(handle, (void *)data.bytes, data.length);
						}
					}
					completionHandler:^(NSError *error) {
						// A message to a nil error gives 0. A domain whose UTF8String
						// is NULL must still fail the request. The description is left
						// out: it may name a file.
						@autoreleasepool {
							pk_go_done(handle, error.code, error == nil ? NULL : (char *)(error.domain.UTF8String ?: ""));
						}
					}];
				r.code = PK_OK;
				break;
			}
		} @catch (NSException *e) {
			r = pk_exception(e);
		}
	}
	return r;
}

// cancelDataRequest: of an ended request does nothing, so a Cancel that races the completion is harmless.
void pk_cancel_resource(int32_t request_id) {
	@autoreleasepool {
		@try {
			[[PHAssetResourceManager defaultManager] cancelDataRequest:request_id];
		} @catch (NSException *e) {
			// An exception must not unwind into Go; Export stops waiting after its grace.
		}
	}
}

// The change block and the handler may run after pk_perform_changes has
// returned on its timeout: as for OSXPhotosAuthRequest, they hold only this.
@interface OSXPhotosChangeRequest : NSObject
@property(atomic) BOOL ok;
@property(atomic, strong) NSError *error;
@property(atomic, copy) NSString *identifier, *exception;
@property(nonatomic, strong, readonly) dispatch_semaphore_t done;
@end

@implementation OSXPhotosChangeRequest
- (instancetype)init {
	if ((self = [super init])) {
		_done = dispatch_semaphore_create(0);
	}
	return self;
}
@end

// The reason of an exception may name a file or an identifier of the caller,
// and the message reaches the error of Go: a change gives the name only.
static pk_result pk_exception_name(NSException *e) { return (pk_result){PK_EXCEPTION, pk_strdup(e.name)}; }

// changes sets request.identifier. PhotoKit commits what it requested before an exception: the outcome is then unknown.
static pk_result pk_perform_changes(void (^changes)(OSXPhotosChangeRequest *), int64_t timeout_ms, char **identifier, int64_t *ns_code, char **ns_domain) {
	pk_result r = {PK_OK, NULL};
	@autoreleasepool {
		@try {
			OSXPhotosChangeRequest *request = [[OSXPhotosChangeRequest alloc] init];
			[[PHPhotoLibrary sharedPhotoLibrary] performChanges:^{
				@autoreleasepool {
					@try {
						changes(request);
					} @catch (NSException *e) {
						request.exception = e.name;
					}
				}
			} completionHandler:^(BOOL ok, NSError *error) {
				@autoreleasepool {
					@try {
						request.ok = ok;
						request.error = error;
					} @catch (NSException *e) {
						request.exception = e.name;
					}
				}
				dispatch_semaphore_signal(request.done);
			}];
			dispatch_time_t deadline = dispatch_time(DISPATCH_TIME_NOW, timeout_ms * (int64_t)NSEC_PER_MSEC);
			if (dispatch_semaphore_wait(request.done, deadline) != 0 || (request.ok && request.exception != nil)) {
				r.code = PK_TIMEOUT;
			} else if (request.exception != nil) {
				r = (pk_result){PK_EXCEPTION, pk_strdup(request.exception)};
			} else if (!request.ok) {
				// A nil error or a code of 0 must still fail; the description may name a file.
				*ns_code = request.error.code;
				*ns_domain = strdup(request.error.domain.UTF8String ?: "");
			} else {
				*identifier = pk_strdup(request.identifier);
			}
		} @catch (NSException *e) {
			r = pk_exception_name(e);
		}
	}
	return r;
}

// extension: not in osxphotos, which imports through AppleScript. Every value
// of r and albums is copied before performChanges: on its timeout this returns,
// Go frees the strings, and the change block may not have run yet.
pk_result pk_import(const pk_import_request *r, const char *const *albums, int64_t n_albums, int64_t timeout_ms, char **identifier, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NOT_FOUND, NULL};
	@autoreleasepool {
		@try {
			// The bytes of a path are taken as they are: Go does not require UTF-8 of a path.
			const char *paths[] = {*r->photo ? r->photo : r->video, r->paired_video, r->alternate_photo};
			PHAssetResourceType kinds[] = {*r->photo ? PHAssetResourceTypePhoto : PHAssetResourceTypeVideo, PHAssetResourceTypePairedVideo, PHAssetResourceTypeAlternatePhoto};
			NSMutableArray<NSNumber *> *types = [NSMutableArray array];
			NSMutableArray<NSURL *> *urls = [NSMutableArray array];
			for (int i = 0; i < 3; i++) {
				if (*paths[i]) {
					[types addObject:@(kinds[i])];
					[urls addObject:[NSURL fileURLWithFileSystemRepresentation:paths[i] isDirectory:NO relativeToURL:nil]];
				}
			}
			NSString *name = *r->original_filename ? [NSString stringWithUTF8String:r->original_filename] : nil;
			// In the change, a set PhotoKit refuses would raise after a part is requested, and a nil name would keep that of the file.
			if (![PHAssetCreationRequest supportsAssetResourceTypes:types] || (*r->original_filename && name == nil)) [NSException raise:NSInvalidArgumentException format:@"pk_import_request"];
			NSDate *date = r->has_creation_date ? [NSDate dateWithTimeIntervalSince1970:r->creation_date] : nil;
			CLLocation *location = r->has_location ? [[CLLocation alloc] initWithLatitude:r->latitude longitude:r->longitude] : nil;
			BOOL favorite = r->favorite, hidden = r->hidden;
			// A string that is not UTF-8 names no album.
			NSMutableSet<NSString *> *ids = [NSMutableSet setWithCapacity:(NSUInteger)n_albums];
			for (int64_t i = 0; i < n_albums; i++) {
				NSString *s = [NSString stringWithUTF8String:albums[i]];
				if (s == nil) return result;
				[ids addObject:s];
			}
			PHFetchResult<PHAssetCollection *> *collections = [PHAssetCollection fetchAssetCollectionsWithLocalIdentifiers:ids.allObjects options:nil];
			// A smart or shared album takes no asset, which would be created outside it.
			BOOL editable = collections.count == ids.count;
			for (PHAssetCollection *album in collections) {
				editable = editable && [album canPerformEditOperation:PHCollectionEditOperationAddContent];
			}
			if (editable) {
				result = pk_perform_changes(^(OSXPhotosChangeRequest *request) {
					// Albums first: a nil request raises before any change, a later exception leaves the asset in them.
					NSMutableArray<PHAssetCollectionChangeRequest *> *adds = [NSMutableArray array];
					for (PHAssetCollection *album in collections) {
						[adds addObject:[PHAssetCollectionChangeRequest changeRequestForAssetCollection:album]];
					}
					PHAssetCreationRequest *asset = [PHAssetCreationRequest creationRequestForAsset];
					PHObjectPlaceholder *placeholder = asset.placeholderForCreatedAsset;
					request.identifier = placeholder.localIdentifier;
					for (PHAssetCollectionChangeRequest *add in adds) {
						[add addAssets:@[ placeholder ]];
					}
					for (NSUInteger i = 0; i < urls.count; i++) {
						PHAssetResourceCreationOptions *o = [[PHAssetResourceCreationOptions alloc] init];
						// The file of the caller must stay: Photos copies it.
						o.shouldMoveFile = NO;
						o.originalFilename = i == 0 ? name : nil;
						[asset addResourceWithType:(PHAssetResourceType)types[i].integerValue fileURL:urls[i] options:o];
					}
					if (date) {
						asset.creationDate = date;
					}
					if (location) {
						asset.location = location;
					}
					// Only YES is written: a write of hidden, even of NO, may ask the user.
					if (favorite) asset.favorite = YES;
					if (hidden) asset.hidden = YES;
				}, timeout_ms, identifier, ns_code, ns_domain);
			}
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}
