package template

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
)

func TestParseDefaultKV(t *testing.T) {
	t.Parallel()
	plus := func(pairs ...string) map[string]string {
		m := maps.Clone(mediaTypeDefaults)
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i]] = pairs[i+1]
		}
		return m
	}
	for _, tt := range []struct {
		def  []string
		want map[string]string
	}{
		{nil, mediaTypeDefaults},
		{[]string{""}, mediaTypeDefaults},
		// t021
		{[]string{"video=vid\U000000e9o;time_lapse=vid\U000000e9o_acc\U000000e9l\U000000e9r\U000000e9e"},
			plus("video", "vid\U000000e9o", "time_lapse", "vid\U000000e9o_acc\U000000e9l\U000000e9r\U000000e9e")},
		// t023: the ValueError of "nonsense" and "a=b=c" is passed (phototemplate.py:1603-1608)
		{[]string{" video = film ;nonsense;a=b=c"}, plus("video", "film")},
		{[]string{"\U0000001cvideo\t=\U0000001ffilm"}, plus("video", "film")},
		{[]string{"x=y"}, plus("x", "y")},
		{[]string{"video="}, plus("video", "")},
		// default[0] only (1600)
		{[]string{"photo=a", "video=b"}, plus("photo", "a")},
	} {
		got := parseDefaultKV(tt.def, mediaTypeDefaults)
		if !maps.Equal(got, tt.want) {
			t.Errorf("parseDefaultKV(%q) = %v, want %v", tt.def, got, tt.want)
		}
	}
	if mediaTypeDefaults["video"] != "video" || len(mediaTypeDefaults) != 11 || len(photoVideoDefaults) != 2 {
		t.Errorf("parseDefaultKV changed the tables: %v, %v", mediaTypeDefaults, photoVideoDefaults)
	}
}

func mediaPhoto(kind string) fakePhoto {
	other := "photo"
	if kind == "video" {
		other = "video"
	}
	return fakePhoto{"media_type": strs(kind), "photo_or_video": strs(other), "title": strs("photo=P")}
}

func TestRenderMediaType(t *testing.T) {
	t.Parallel()
	m, o := mediaPhoto, RenderOptions{}
	one := func(s string) []string { return []string{s} }
	const t022 = "{media_type,selfie=autoportrait;burst=rafale;live_photo=live;photo=foto}"
	const t025 = "{photo_or_video,photo=fotos;video=videos}"
	checkRender(t, []renderCase{
		{"t020 selfie", "{media_type}", m("selfie"), o, one("selfie"), nil},
		{"t020 video", "{media_type}", m("video"), o, one("video"), nil},
		{"t020 photo", "{media_type}", m("photo"), o, one("photo"), nil},
		{"t021", "{media_type,video=vid\U000000e9o;time_lapse=vid\U000000e9o_acc\U000000e9l\U000000e9r\U000000e9e}", m("time_lapse"), o,
			one("vid\U000000e9o_acc\U000000e9l\U000000e9r\U000000e9e"), nil},
		{"t022 selfie", t022, m("selfie"), o, one("autoportrait"), nil},
		{"t022 video", t022, m("video"), o, one("video"), nil},
		{"t022 photo", t022, m("photo"), o, one("foto"), nil},
		{"t023", "{media_type, video = film ;nonsense;a=b=c}", m("video"), o, one("film"), nil},
		{"t023 photo", "{media_type, video = film ;nonsense;a=b=c}", m("photo"), o, one("photo"), nil},
		{"unknown key unused", "{media_type,x=y}", m("burst"), o, one("burst"), nil},
		{"t024", "{photo_or_video}", m("selfie"), o, one("photo"), nil},
		{"t025 photo", t025, m("selfie"), o, one("fotos"), nil},
		{"t025 video", t025, m("video"), o, one("videos"), nil},
		{"default is a template", "{media_type,{title}}", m("photo"), o, one("P"), nil},
		// "" is None (phototemplate.py:1012), so the default itself is rendered (770-772)
		{"empty value", "{media_type,video=}", m("video"), o, one("video="), nil},
		{"bool", "{media_type?Y,video=v}", m("video"), o, one("Y"), nil},
		{"sanitized", "{media_type,video=a/b}", m("video"), RenderOptions{Dirname: true}, one("a:b"), nil},
	})
}

func TestRenderMediaTypeErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		in    string
		photo fakePhoto
	}{
		{"{media_type}", mediaPhoto("Holidays")},
		{"{media_type,Holidays=x}", mediaPhoto("Holidays")},
		{"{photo_or_video}", fakePhoto{"photo_or_video": strs("selfie")}},
		{"{media_type}", fakePhoto{"media_type": {}}},
		{"{media_type}", fakePhoto{"media_type": strs("photo", "video")}},
	} {
		got, _, err := render(t, tt.in, tt.photo, RenderOptions{})
		name, _, _ := strings.Cut(strings.Trim(tt.in, "{}"), ",")
		if msg := fmt.Sprint(err); !errors.Is(err, ErrRender) || !strings.Contains(msg, name) || strings.Contains(msg, "Holidays") || got != nil {
			t.Errorf("Render(%q) = %q, %v; want ErrRender that names %s and not the kind", tt.in, got, err, name)
		}
	}
}
