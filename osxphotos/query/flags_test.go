package query

import (
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

func TestApplyFlags(t *testing.T) {
	t.Parallel()

	pairs := []applyPair{
		{"portrait", func(o *Options) { o.Portrait = true }, func(o *Options) { o.NotPortrait = true },
			photoinfo.PhotoInfo{Portrait: true}, photoinfo.PhotoInfo{}},
		{"spatial capture", func(o *Options) { o.Spatial = true }, func(o *Options) { o.NotSpatial = true },
			photoinfo.PhotoInfo{Spatial: 1}, photoinfo.PhotoInfo{}},
		{"spatial converted", func(o *Options) { o.Spatial = true }, func(o *Options) { o.NotSpatial = true },
			photoinfo.PhotoInfo{Spatial: 2}, photoinfo.PhotoInfo{}},
		{"screen recording", func(o *Options) { o.ScreenRecording = true }, func(o *Options) { o.NotScreenRecording = true },
			photoinfo.PhotoInfo{ScreenRecording: true}, photoinfo.PhotoInfo{}},
		{"slow mo", func(o *Options) { o.SlowMo = true }, func(o *Options) { o.NotSlowMo = true },
			photoinfo.PhotoInfo{SlowMo: true}, photoinfo.PhotoInfo{}},
		{"time lapse", func(o *Options) { o.TimeLapse = true }, func(o *Options) { o.NotTimeLapse = true },
			photoinfo.PhotoInfo{TimeLapse: true}, photoinfo.PhotoInfo{}},
		{"hdr", func(o *Options) { o.HDR = true }, func(o *Options) { o.NotHDR = true },
			photoinfo.PhotoInfo{HDR: true}, photoinfo.PhotoInfo{}},
		{"selfie", func(o *Options) { o.Selfie = true }, func(o *Options) { o.NotSelfie = true },
			photoinfo.PhotoInfo{Selfie: true}, photoinfo.PhotoInfo{}},
		{"panorama", func(o *Options) { o.Panorama = true }, func(o *Options) { o.NotPanorama = true },
			photoinfo.PhotoInfo{Panorama: true}, photoinfo.PhotoInfo{}},
		{"shared moment", func(o *Options) { o.SharedMoment = true }, func(o *Options) { o.NotSharedMoment = true },
			photoinfo.PhotoInfo{SharedMoment: true}, photoinfo.PhotoInfo{}},
	}
	var tests []applyCase
	for _, pr := range pairs {
		yes, no := pr.yes, pr.no
		yes.UUID, yes.IsPhoto = "Y", true
		no.UUID, no.IsPhoto = "N", true
		photos := []*photoinfo.PhotoInfo{&yes, &no}
		var x, notX, both Options
		pr.x(&x)
		pr.notX(&notX)
		pr.x(&both)
		pr.notX(&both)
		tests = append(tests,
			applyCase{pr.name, x, photos, []string{"Y"}},
			applyCase{"not " + pr.name, notX, photos, []string{"N"}},
			applyCase{pr.name + " and not " + pr.name, both, photos, []string{"Y"}},
		)
	}
	external := []*photoinfo.PhotoInfo{{UUID: "N", IsPhoto: true}, {UUID: "Y", IsPhoto: true, ExternalEdit: true}}
	tests = append(tests, applyCase{"external edit", Options{ExternalEdit: true}, external, []string{"Y"}})
	applyRun(t, tests, nil)
}
