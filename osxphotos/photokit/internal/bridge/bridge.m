//go:build darwin && cgo

#import <AVFoundation/AVFoundation.h>
#import <Foundation/Foundation.h>
#import <ImageIO/ImageIO.h>
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
// photokit.Version, which upstream takes from PhotoKit (photokit.py:77-85).
_Static_assert(PHImageRequestOptionsVersionCurrent == 0, "PHImageRequestOptionsVersionCurrent");
_Static_assert(PHImageRequestOptionsVersionUnadjusted == 1, "PHImageRequestOptionsVersionUnadjusted");
_Static_assert(PHImageRequestOptionsVersionOriginal == 2, "PHImageRequestOptionsVersionOriginal");
// What ExportVideo passes for a photokit.Version.
_Static_assert(PHVideoRequestOptionsVersionCurrent == 0, "PHVideoRequestOptionsVersionCurrent");
_Static_assert(PHVideoRequestOptionsVersionOriginal == 1, "PHVideoRequestOptionsVersionOriginal");
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
			// A string that breaks the rule of bridge.h gives nil, on which addObject: raises.
			[identifiers addObject:[NSString stringWithUTF8String:ids[i]]];
		}
		return [PHAsset fetchAssetsWithLocalIdentifiers:identifiers options:options];
	}, out, count);
}

pk_result pk_fetch_burst(const char *burst_id, bool include_all, pk_asset **out, int *count) {
	return pk_fetch(^(PHFetchOptions *options) {
		options.includeAllBurstAssets = include_all;
		NSString *burst = [NSString stringWithUTF8String:burst_id];
		// A string that breaks the rule of bridge.h gives nil, on which
		// fetchAssetsWithBurstIdentifier: is not known to raise, as addObject: and @[ ] do.
		if (burst == nil) {
			[NSException raise:NSInvalidArgumentException format:@"burst_id breaks the rule of bridge.h"];
		}
		return [PHAsset fetchAssetsWithBurstIdentifier:burst options:options];
	}, out, count);
}

void pk_free_assets(pk_asset *assets, int count) {
	for (int i = 0; i < count; i++) {
		free(assets[i].local_identifier);
		free(assets[i].burst_identifier);
	}
	free(assets);
}

// A string that breaks the rule of bridge.h gives nil, on which @[ ] raises.
static PHAsset *pk_asset_of(const char *local_id) {
	return [PHAsset fetchAssetsWithLocalIdentifiers:@[ [NSString stringWithUTF8String:local_id] ] options:nil].firstObject;
}

static NSArray<PHAssetResource *> *pk_resources(const char *local_id) {
	PHAsset *asset = pk_asset_of(local_id);
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

// The dictionary that upstream keeps (photokit.py:669-673) crosses to Go as a
// binary property list; nil when ImageIO reads none or it holds a value that a
// property list cannot.
static NSData *pk_image_properties(NSData *data) {
	NSDictionary *options = @{(__bridge NSString *)kCGImageSourceShouldCache : @NO};
	id source = CFBridgingRelease(CGImageSourceCreateWithData((__bridge CFDataRef)data, (__bridge CFDictionaryRef)options));
	NSDictionary *properties = source ? CFBridgingRelease(CGImageSourceCopyPropertiesAtIndex((__bridge CGImageSourceRef)source, 0, (__bridge CFDictionaryRef)options)) : nil;
	return properties ? [NSPropertyListSerialization dataWithPropertyList:properties format:NSPropertyListBinaryFormat_v1_0 options:0 error:NULL] : nil;
}

// pk_go_done comes once, after the request, which PhotoKit may end without
// calling the handler; an exception must not leave the handler or the queue.
// The description of an NSError is left out, as in pk_request_resource.
static void pk_image(PHAsset *asset, PHImageRequestOptions *options, uintptr_t handle) {
	@autoreleasepool {
		__block int64_t code = PK_NO_RESOURCE;
		__block NSString *domain = nil;
		@try {
			[[PHImageManager defaultManager] requestImageDataAndOrientationForAsset:asset
				options:options
				resultHandler:^(NSData *data, NSString *uti, CGImagePropertyOrientation orientation, NSDictionary *info) {
					@autoreleasepool {
						@try {
							NSError *error = info[PHImageErrorKey];
							if (error != nil) {
								code = error.code;
								domain = error.domain ?: @"";
							} else if (data.length > 0) {
								// No constant names the key; upstream reads it so too (photokit.py:502, 511).
								NSURL *url = info[@"PHImageFileURLKey"];
								// metadata.bytes is an inner pointer: ARC may free metadata after its last use, before pk_go_image reads them.
								NS_VALID_UNTIL_END_OF_SCOPE NSData *metadata = pk_image_properties(data);
								pk_image_info image = {
									.uti = uti.UTF8String,
									.path = url.fileSystemRepresentation,
									.metadata = metadata.bytes,
									.metadata_len = (int64_t)metadata.length,
									.orientation = (int)orientation,
									.degraded = [info[PHImageResultIsDegradedKey] boolValue],
								};
								pk_go_image(handle, (void *)data.bytes, data.length, &image);
								code = PK_OK;
							}
						} @catch (NSException *e) {
							code = PK_EXCEPTION;
							domain = nil;
						}
					}
				}];
		} @catch (NSException *e) {
			code = PK_EXCEPTION;
			domain = nil;
		}
		pk_go_done(handle, code, domain == nil ? NULL : (char *)(domain.UTF8String ?: ""));
	}
}

// _request_image_data (photokit.py:625-691) is synchronous (654): the handler of
// an asynchronous image request may run on the main queue, which nothing runs
// in a Go process. The request runs on a global queue and holds its thread
// until PhotoKit returns, so that Go waits as for pk_request_resource.
pk_result pk_request_image(const char *local_id, int64_t version, uintptr_t handle) {
	pk_result r = {PK_NO_RESOURCE, NULL};
	@autoreleasepool {
		@try {
			PHAsset *asset = pk_asset_of(local_id);
			if (asset != nil) {
				PHImageRequestOptions *options = [[PHImageRequestOptions alloc] init];
				// An original kept only in iCloud is downloaded (photokit.py:653).
				options.networkAccessAllowed = YES;
				options.synchronous = YES;
				options.version = (PHImageRequestOptionsVersion)version;
				// One result, not degraded (photokit.py:656-657).
				options.deliveryMode = PHImageRequestOptionsDeliveryModeHighQualityFormat;
				// extension: not in osxphotos; it feeds ExportOptions.Progress and
				// stops the request, which has no id for cancelImageRequest:.
				options.progressHandler = ^(double progress, __unused NSError *error, BOOL *stop, __unused NSDictionary *info) {
					@autoreleasepool {
						*stop = pk_go_progress(handle, progress);
					}
				};
				dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
					pk_image(asset, options, handle);
				});
				r.code = PK_OK;
			}
		} @catch (NSException *e) {
			r = pk_exception(e);
		}
	}
	return r;
}

// _request_video_data (photokit.py:967-1015) waits on an event; here Go waits,
// as for pk_request_resource. PhotoKit calls the handler on a serial queue of
// its own, not the main queue, so unlike pk_request_image the request is
// asynchronous and has an id for cancelImageRequest:. The handler holds only
// handle and calls pk_go_done once; an exception must not leave it. The
// description of an NSError is left out, as in pk_request_resource.
pk_result pk_request_video(const char *local_id, int64_t version, uintptr_t handle, int32_t *request_id) {
	pk_result r = {PK_NO_RESOURCE, NULL};
	@autoreleasepool {
		@try {
			PHAsset *asset = pk_asset_of(local_id);
			if (asset != nil) {
				PHVideoRequestOptions *options = [[PHVideoRequestOptions alloc] init];
				// A video kept only in iCloud is downloaded (photokit.py:988).
				options.networkAccessAllowed = YES;
				options.version = (PHVideoRequestOptionsVersion)version;
				options.deliveryMode = PHVideoRequestOptionsDeliveryModeHighQualityFormat;
				*request_id = [[PHImageManager defaultManager] requestAVAssetForVideo:asset
					options:options
					resultHandler:^(AVAsset *video, __unused AVAudioMix *mix, NSDictionary *info) {
						@autoreleasepool {
							int64_t code = PK_OK;
							NSString *domain = nil;
							@try {
								NSError *error = info[PHImageErrorKey];
								if (error != nil) {
									code = error.code;
									domain = error.domain ?: @"";
								} else if ([video isKindOfClass:AVURLAsset.class] && ((AVURLAsset *)video).URL.isFileURL) {
									// An AVComposition, a slow-motion video of the current version, has no file
									// (photokit.py:876-884, 901), and a URL of another scheme names none.
									pk_go_video(handle, (char *)((AVURLAsset *)video).URL.fileSystemRepresentation);
								}
							} @catch (NSException *e) {
								code = PK_EXCEPTION;
								domain = nil;
							}
							pk_go_done(handle, code, domain == nil ? NULL : (char *)(domain.UTF8String ?: ""));
						}
					}];
				r.code = PK_OK;
			}
		} @catch (NSException *e) {
			r = pk_exception(e);
		}
	}
	return r;
}

// cancelImageRequest: of an ended request does nothing, as cancelDataRequest: in pk_cancel_resource.
void pk_cancel_video(int32_t request_id) {
	@autoreleasepool {
		@try {
			[[PHImageManager defaultManager] cancelImageRequest:request_id];
		} @catch (NSException *e) {
			// An exception must not unwind into Go; ExportVideo stops waiting after its grace.
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

// A change that creates an object sets request.identifier; the others pass
// identifier NULL. PhotoKit commits what it requested before an exception: the
// outcome is then unknown.
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
			} else if (identifier != NULL) {
				*identifier = pk_strdup(request.identifier);
			}
		} @catch (NSException *e) {
			r = pk_exception_name(e);
		}
	}
	return r;
}

// Each identifier is kept once, since PhotoKit finds an object once however
// often it is named and the callers compare counts. A string that breaks the
// rule of bridge.h gives nil, on which addObject: raises.
static NSArray<NSString *> *pk_identifiers(const char *const *ids, int64_t n) {
	NSMutableOrderedSet<NSString *> *out = [NSMutableOrderedSet orderedSetWithCapacity:(NSUInteger)n];
	for (int64_t i = 0; i < n; i++) {
		[out addObject:[NSString stringWithUTF8String:ids[i]]];
	}
	return out.array;
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
			// In the change, a set PhotoKit refuses would raise after a part is requested.
			if (![PHAssetCreationRequest supportsAssetResourceTypes:types]) [NSException raise:NSInvalidArgumentException format:@"pk_import_request"];
			NSDate *date = r->has_creation_date ? [NSDate dateWithTimeIntervalSince1970:r->creation_date] : nil;
			CLLocation *location = r->has_location ? [[CLLocation alloc] initWithLatitude:r->latitude longitude:r->longitude] : nil;
			BOOL favorite = r->favorite, hidden = r->hidden;
			NSArray<NSString *> *ids = pk_identifiers(albums, n_albums);
			PHFetchResult<PHAssetCollection *> *collections = [PHAssetCollection fetchAssetCollectionsWithLocalIdentifiers:ids options:nil];
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
					// A new asset is neither a favorite nor hidden: only YES needs a write.
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

// Collection is a folder or an album: a smart folder or a smart album, which
// takes no change, is left out wherever PhotoKit lists one.
static void pk_add_collections(PHFetchResult<PHCollection *> *fetch, NSString *parent, NSMutableArray<PHCollection *> *all, NSMutableArray<NSString *> *parents) {
	for (PHCollection *c in fetch) {
		PHCollectionList *list = [c isKindOfClass:PHCollectionList.class] ? (PHCollectionList *)c : nil;
		BOOL user = list ? list.collectionListType == PHCollectionListTypeFolder : ((PHAssetCollection *)c).assetCollectionType == PHAssetCollectionTypeAlbum;
		if (!user) continue;
		[all addObject:c];
		[parents addObject:parent];
		if (list) pk_add_collections([PHCollection fetchCollectionsInCollectionList:list options:nil], list.localIdentifier, all, parents);
	}
}

// extension: not in osxphotos, which reads albums from the database.
pk_result pk_collections(pk_collection **out, int *count) {
	pk_result r = {PK_OK, NULL};
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		@try {
			NSMutableArray<PHCollection *> *all = [NSMutableArray array];
			NSMutableArray<NSString *> *parents = [NSMutableArray array];
			pk_add_collections([PHCollection fetchTopLevelUserCollectionsWithOptions:nil], @"", all, parents);
			if (all.count > INT32_MAX) [NSException raise:NSGenericException format:@"too many collections"];
			if (all.count > 0) {
				*out = calloc(all.count, sizeof(pk_collection));
				if (*out == NULL) [NSException raise:NSGenericException format:@"no memory for collections"];
				*count = (int)all.count;
				for (int i = 0; i < *count; i++) {
					(*out)[i].local_identifier = pk_strdup(all[i].localIdentifier);
					(*out)[i].title = pk_strdup(all[i].localizedTitle);
					(*out)[i].parent = pk_strdup(parents[i]);
					(*out)[i].is_folder = [all[i] isKindOfClass:PHCollectionList.class];
				}
			}
		} @catch (NSException *e) {
			// calloc zeroed the collections not yet filled, so all of them are freed.
			pk_free_collections(*out, *count);
			*out = NULL;
			*count = 0;
			// The reason may name a collection.
			r = pk_exception_name(e);
		}
	}
	return r;
}

void pk_free_collections(pk_collection *collections, int count) {
	for (int i = 0; i < count; i++) {
		free(collections[i].local_identifier);
		free(collections[i].title);
		free(collections[i].parent);
	}
	free(collections);
}

// A string that breaks the rule of bridge.h gives nil, on which @[ ] raises.
static PHCollection *pk_collection_of(const char *local_id) {
	NSString *identifier = [NSString stringWithUTF8String:local_id];
	PHCollection *list = [PHCollectionList fetchCollectionListsWithLocalIdentifiers:@[ identifier ] options:nil].firstObject;
	return list ?: [PHAssetCollection fetchAssetCollectionsWithLocalIdentifiers:@[ identifier ] options:nil].firstObject;
}

// extension: not in osxphotos, which changes albums through AppleScript. As in
// pk_import, every string becomes an object before performChanges; a nil title
// (stringWithUTF8String: does not raise) would not raise until the change.
pk_result pk_create_collection(const char *title, const char *parent, bool folder, int64_t timeout_ms, char **identifier, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NOT_FOUND, NULL};
	@autoreleasepool {
		@try {
			NSString *name = [NSString stringWithUTF8String:title];
			if (name == nil) [NSException raise:NSInvalidArgumentException format:@"title"];
			PHCollectionList *list = *parent ? (PHCollectionList *)pk_collection_of(parent) : nil;
			// An album takes no collection.
			if (*parent && (![list isKindOfClass:PHCollectionList.class] || ![list canPerformEditOperation:PHCollectionEditOperationAddContent])) return result;
			result = pk_perform_changes(^(OSXPhotosChangeRequest *request) {
				PHCollectionListChangeRequest *add = list ? [PHCollectionListChangeRequest changeRequestForCollectionList:list] : nil;
				// Before the collection is requested, which would otherwise be left at the top level.
				if (list && add == nil) [NSException raise:NSInternalInconsistencyException format:@"parent"];
				PHObjectPlaceholder *placeholder = folder ? [PHCollectionListChangeRequest creationRequestForCollectionListWithTitle:name].placeholderForCreatedCollectionList
				                                          : [PHAssetCollectionChangeRequest creationRequestForAssetCollectionWithTitle:name].placeholderForCreatedAssetCollection;
				request.identifier = placeholder.localIdentifier;
				[add addChildCollections:@[ placeholder ]];
			}, timeout_ms, identifier, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, as pk_create_collection.
pk_result pk_rename_collection(const char *local_id, const char *title, int64_t timeout_ms, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NOT_FOUND, NULL};
	@autoreleasepool {
		@try {
			NSString *name = [NSString stringWithUTF8String:title];
			if (name == nil) [NSException raise:NSInvalidArgumentException format:@"title"];
			PHCollection *c = pk_collection_of(local_id);
			// A smart album or a smart folder cannot be renamed.
			if (![c canPerformEditOperation:PHCollectionEditOperationRename]) return result;
			BOOL folder = [c isKindOfClass:PHCollectionList.class];
			result = pk_perform_changes(^(__unused OSXPhotosChangeRequest *request) {
				PHCollectionListChangeRequest *listChange = folder ? [PHCollectionListChangeRequest changeRequestForCollectionList:(PHCollectionList *)c] : nil;
				PHAssetCollectionChangeRequest *albumChange = folder ? nil : [PHAssetCollectionChangeRequest changeRequestForAssetCollection:(PHAssetCollection *)c];
				// Without a request the change would succeed and rename nothing.
				if (listChange == nil && albumChange == nil) [NSException raise:NSInternalInconsistencyException format:@"collection"];
				listChange.title = name;
				albumChange.title = name;
			}, timeout_ms, NULL, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, as pk_create_collection. Every collection is
// checked before the change, so that it deletes all or none. PhotoKit does not
// say what a deletion of one object twice does, and two forms of an identifier
// name one collection.
pk_result pk_delete_collections(const char *const *ids, int64_t n, int64_t timeout_ms, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NOT_FOUND, NULL};
	@autoreleasepool {
		@try {
			NSMutableArray<PHCollection *> *lists = [NSMutableArray array], *albums = [NSMutableArray array];
			NSMutableSet<NSString *> *seen = [NSMutableSet set];
			for (int64_t i = 0; i < n; i++) {
				PHCollection *c = pk_collection_of(ids[i]);
				if (![c canPerformEditOperation:PHCollectionEditOperationDelete]) return result;
				if ([seen containsObject:c.localIdentifier]) continue;
				[seen addObject:c.localIdentifier];
				[([c isKindOfClass:PHCollectionList.class] ? lists : albums) addObject:c];
			}
			result = pk_perform_changes(^(__unused OSXPhotosChangeRequest *request) {
				if (albums.count > 0) [PHAssetCollectionChangeRequest deleteAssetCollections:albums];
				if (lists.count > 0) [PHCollectionListChangeRequest deleteCollectionLists:lists];
			}, timeout_ms, NULL, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, as pk_create_collection. The assets are read
// before the album, so that the CI tests reach an asset that breaks the rule of
// bridge.h without a fetch of PhotoKit.
pk_result pk_album_assets(const char *album, const char *const *assets, int64_t n, bool add, int64_t timeout_ms, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NOT_FOUND, NULL};
	@autoreleasepool {
		@try {
			NSArray<NSString *> *ids = pk_identifiers(assets, n);
			PHCollection *c = pk_collection_of(album);
			// A folder holds no asset, and a smart or a shared album takes no change.
			if (![c isKindOfClass:PHAssetCollection.class] || ![c canPerformEditOperation:(add ? PHCollectionEditOperationAddContent : PHCollectionEditOperationRemoveContent)]) return result;
			// The default options leave a hidden asset out, as for pk_fetch_assets.
			PHFetchResult<PHAsset *> *found = [PHAsset fetchAssetsWithLocalIdentifiers:ids options:nil];
			if (found.count != ids.count) return (pk_result){PK_NO_ASSET, NULL};
			result = pk_perform_changes(^(__unused OSXPhotosChangeRequest *request) {
				PHAssetCollectionChangeRequest *change = [PHAssetCollectionChangeRequest changeRequestForAssetCollection:(PHAssetCollection *)c];
				// Without a request the change would succeed and change nothing.
				if (change == nil) [NSException raise:NSInternalInconsistencyException format:@"album"];
				if (add) {
					[change addAssets:found];
				} else {
					[change removeAssets:found];
				}
			}, timeout_ms, NULL, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, whose photokit.py does not delete. Every asset
// is checked before the change, so that it deletes all or none.
pk_result pk_delete_assets(const char *const *ids, int64_t n, int64_t timeout_ms, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NO_ASSET, NULL};
	@autoreleasepool {
		@try {
			NSArray<NSString *> *list = pk_identifiers(ids, n);
			// The default options leave a hidden asset out, as for pk_fetch_assets.
			PHFetchResult<PHAsset *> *found = [PHAsset fetchAssetsWithLocalIdentifiers:list options:nil];
			if (found.count != list.count) return result;
			for (PHAsset *asset in found) {
				if (![asset canPerformEditOperation:PHAssetEditOperationDelete]) return result;
			}
			result = pk_perform_changes(^(__unused OSXPhotosChangeRequest *request) {
				[PHAssetChangeRequest deleteAssets:found];
			}, timeout_ms, NULL, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, whose photokit.py only reads the properties of
// an asset (photokit.py:436-464). As in pk_import, every value of p is copied
// before performChanges.
pk_result pk_update_asset(const char *local_id, const pk_patch *p, int64_t timeout_ms, int64_t *ns_code, char **ns_domain) {
	pk_result result = {PK_NO_ASSET, NULL};
	@autoreleasepool {
		@try {
			NSArray<NSString *> *ids = pk_identifiers(&local_id, 1);
			// The default options leave a hidden asset out, as for pk_fetch_assets.
			PHFetchResult<PHAsset *> *found = [PHAsset fetchAssetsWithLocalIdentifiers:ids options:nil];
			PHAsset *asset = found.firstObject;
			if (found.count != 1 || ![asset canPerformEditOperation:PHAssetEditOperationProperties]) return result;
			NSDate *date = p->has_creation_date ? [NSDate dateWithTimeIntervalSince1970:p->creation_date] : nil;
			CLLocation *location = p->has_location ? [[CLLocation alloc] initWithLatitude:p->latitude longitude:p->longitude] : nil;
			BOOL hasFavorite = p->has_favorite, favorite = p->favorite, hasHidden = p->has_hidden, hidden = p->hidden;
			result = pk_perform_changes(^(__unused OSXPhotosChangeRequest *request) {
				PHAssetChangeRequest *change = [PHAssetChangeRequest changeRequestForAsset:asset];
				// Without a request the change would succeed and change nothing.
				if (change == nil) [NSException raise:NSInternalInconsistencyException format:@"asset"];
				if (date) change.creationDate = date;
				if (location) change.location = location;
				if (hasFavorite) change.favorite = favorite;
				if (hasHidden) change.hidden = hidden;
			}, timeout_ms, NULL, ns_code, ns_domain);
		} @catch (NSException *e) {
			result = pk_exception_name(e);
		}
	}
	return result;
}

// extension: not in osxphotos, whose photokit.py has no cloud identifiers.
static pk_result pk_identifier_pairs(const char *const *ids, int64_t n, NSArray<NSArray<NSString *> *> *(^map)(NSArray<NSString *> *), pk_identifier_pair **out, int *count) {
	pk_result r = {PK_OK, NULL};
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		@try {
			NSArray<NSString *> *list = pk_identifiers(ids, n);
			NSArray<NSArray<NSString *> *> *pairs = list.count > 0 ? map(list) : nil;
			if (pairs.count > INT32_MAX) [NSException raise:NSGenericException format:@"too many identifiers"];
			if (pairs.count > 0) {
				*out = calloc(pairs.count, sizeof(pk_identifier_pair));
				if (*out == NULL) [NSException raise:NSGenericException format:@"no memory for identifiers"];
				*count = (int)pairs.count;
				for (int i = 0; i < *count; i++) {
					(*out)[i].local_identifier = pk_strdup(pairs[i][0]);
					(*out)[i].cloud_identifier = pk_strdup(pairs[i][1]);
				}
			}
		} @catch (NSException *e) {
			// calloc zeroed the pairs not yet filled, so all of them are freed.
			pk_free_identifier_pairs(*out, *count);
			*out = NULL;
			*count = 0;
			r = pk_exception_name(e);
		}
	}
	return r;
}

// The dictionaries are walked, not looked up by the identifiers given: a key
// is what PhotoKit made of them. An asset without a cloud identifier has an
// error in its mapping and is left out; the call does not fail for it.
pk_result pk_cloud_identifiers(const char *const *ids, int64_t n, pk_identifier_pair **out, int *count) {
	return pk_identifier_pairs(ids, n, ^NSArray<NSArray<NSString *> *> *(NSArray<NSString *> *list) {
		// The mapping takes a LocalIdentifier, not the UUID that Fetch takes; the
		// default options leave a hidden asset out, as for pk_fetch_assets.
		NSMutableArray<NSString *> *locals = [NSMutableArray array];
		for (PHAsset *asset in [PHAsset fetchAssetsWithLocalIdentifiers:list options:nil]) [locals addObject:asset.localIdentifier];
		NSDictionary<NSString *, PHCloudIdentifierMapping *> *mappings = [[PHPhotoLibrary sharedPhotoLibrary] cloudIdentifierMappingsForLocalIdentifiers:locals];
		NSMutableArray<NSArray<NSString *> *> *pairs = [NSMutableArray array];
		for (NSString *local in mappings) {
			NSString *cloud = mappings[local].cloudIdentifier.stringValue;
			if (mappings[local].error == nil && cloud != nil) [pairs addObject:@[ local, cloud ]];
		}
		return pairs;
	}, out, count);
}

pk_result pk_local_identifiers(const char *const *cloud_ids, int64_t n, pk_identifier_pair **out, int *count) {
	return pk_identifier_pairs(cloud_ids, n, ^NSArray<NSArray<NSString *> *> *(NSArray<NSString *> *list) {
		NSMutableArray<PHCloudIdentifier *> *clouds = [NSMutableArray array];
		for (NSString *s in list) {
			// initWithStringValue: may give nil for a string it cannot parse, and
			// addObject: raises on nil; such a string names no asset, as an unknown one.
			PHCloudIdentifier *cloud = [[PHCloudIdentifier alloc] initWithStringValue:s];
			if (cloud != nil) [clouds addObject:cloud];
		}
		// What PhotoKit does with an empty array is not documented.
		if (clouds.count == 0) return nil;
		NSDictionary<PHCloudIdentifier *, PHLocalIdentifierMapping *> *mappings = [[PHPhotoLibrary sharedPhotoLibrary] localIdentifierMappingsForCloudIdentifiers:clouds];
		NSMutableArray<NSArray<NSString *> *> *pairs = [NSMutableArray array];
		for (PHCloudIdentifier *cloud in mappings) {
			NSString *local = mappings[cloud].localIdentifier, *s = cloud.stringValue;
			if (mappings[cloud].error == nil && local != nil && s != nil) [pairs addObject:@[ local, s ]];
		}
		return pairs;
	}, out, count);
}

void pk_free_identifier_pairs(pk_identifier_pair *pairs, int count) {
	for (int i = 0; i < count; i++) {
		free(pairs[i].local_identifier);
		free(pairs[i].cloud_identifier);
	}
	free(pairs);
}
