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
	"context"
	"errors"
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

func check(r C.pk_result) error {
	if r.message != nil {
		defer C.free(unsafe.Pointer(r.message)) //nolint:gosec // G103: the strdup buffer of bridge.m is freed here
	}
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
