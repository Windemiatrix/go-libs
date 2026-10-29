//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Photos/Photos.h>
#include <string.h>

#include "bridge.h"

// photokit maps these numbers to its Status in Go, where the PhotoKit headers
// cannot be read: a change in them must fail the build.
_Static_assert(PHAuthorizationStatusNotDetermined == 0, "PHAuthorizationStatusNotDetermined");
_Static_assert(PHAuthorizationStatusRestricted == 1, "PHAuthorizationStatusRestricted");
_Static_assert(PHAuthorizationStatusDenied == 2, "PHAuthorizationStatusDenied");
_Static_assert(PHAuthorizationStatusAuthorized == 3, "PHAuthorizationStatusAuthorized");
_Static_assert(PHAuthorizationStatusLimited == 4, "PHAuthorizationStatusLimited");

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

// reason may be nil, and strdup(NULL) crashes.
static pk_result pk_exception(NSException *e) {
	const char *text = ((e.reason ?: e.name) ?: @"exception").UTF8String;
	return (pk_result){PK_EXCEPTION, strdup(text ?: "exception")};
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
