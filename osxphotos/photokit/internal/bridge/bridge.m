//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Photos/Photos.h>
#include <stdlib.h>
#include <string.h>

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
			// Hidden assets too, unlike upstream (photokit.py:1234, 1281): photosdb lists them.
			PHFetchOptions *options = [[PHFetchOptions alloc] init];
			options.includeHiddenAssets = YES;
			PHFetchResult<PHAsset *> *assets = fetch(options);
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
