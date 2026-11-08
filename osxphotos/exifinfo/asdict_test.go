package exifinfo_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/exifinfo"
)

func ptr[T any](v T) *T { return &v }

func TestAsDict(t *testing.T) {
	date := time.Date(2024, 5, 1, 12, 30, 0, 0, time.FixedZone("", 3*3600))
	filled := exifinfo.ExifInfo{
		FlashFired: ptr(true), ISO: ptr(100), MeteringMode: ptr(5), SampleRate: ptr(44100), TrackFormat: ptr(7), WhiteBalance: ptr(1),
		Aperture: ptr(2.8), BitRate: ptr(1.5), Duration: ptr(3.25), ExposureBias: ptr(-0.5), FocalLength: ptr(4.2), FPS: ptr(29.97),
		Latitude: ptr(10.5), Longitude: ptr(-20.25), ShutterSpeed: ptr(0.01),
		CameraMake: "Example Make", CameraModel: "Example Model", Codec: "avc1", LensModel: "Example Lens",
		Date: &date, TZOffset: ptr(10800), TZName: "GMT+0300",
	}
	tests := []struct {
		name string
		e    exifinfo.ExifInfo
		want map[string]any
	}{
		{
			name: "filled",
			e:    filled,
			want: map[string]any{
				"flash_fired": true, "iso": 100, "metering_mode": 5, "sample_rate": 44100, "track_format": 7, "white_balance": 1,
				"aperture": 2.8, "bit_rate": 1.5, "duration": 3.25, "exposure_bias": -0.5, "focal_length": 4.2, "fps": 29.97,
				"latitude": 10.5, "longitude": -20.25, "shutter_speed": 0.01,
				"camera_make": "Example Make", "camera_model": "Example Model", "codec": "avc1", "lens_model": "Example Lens",
				"date": date, "tzoffset": 10800, "tzname": "GMT+0300",
			},
		},
		{
			name: "no row: None",
			e:    exifinfo.ExifInfo{},
			want: map[string]any{
				"flash_fired": nil, "iso": nil, "metering_mode": nil, "sample_rate": nil, "track_format": nil, "white_balance": nil,
				"aperture": nil, "bit_rate": nil, "duration": nil, "exposure_bias": nil, "focal_length": nil, "fps": nil,
				"latitude": nil, "longitude": nil, "shutter_speed": nil,
				"camera_make": nil, "camera_model": nil, "codec": nil, "lens_model": nil,
				"date": nil, "tzoffset": nil, "tzname": nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}
