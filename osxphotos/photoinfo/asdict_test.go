package photoinfo_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/photoinfo"
)

// Not parallel: it sets time.Local.
func TestCommentAndLikeAsDict(t *testing.T) {
	local := time.Local
	t.Cleanup(func() { time.Local = local })
	time.Local = time.FixedZone("", 3*3600)

	when := time.Date(2024, 5, 1, 12, 30, 0, 0, time.FixedZone("", 2*3600))
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
	tests := []struct {
		name string
		got  map[string]any
		want map[string]any
	}{
		{
			name: "comment",
			got:  photoinfo.CommentInfo{Datetime: &when, User: "Pat Example", IsMine: true, Text: "Nice"}.AsDict(),
			want: map[string]any{"datetime": when, "user": "Pat Example", "ismine": true, "text": "Nice"},
		},
		{
			name: "comment without a date or a user",
			got:  photoinfo.CommentInfo{Text: "Nice"}.AsDict(),
			want: map[string]any{"datetime": epoch, "user": nil, "ismine": false, "text": "Nice"},
		},
		{
			name: "like",
			got:  photoinfo.LikeInfo{Datetime: &when, User: "Pat Example", IsMine: true}.AsDict(),
			want: map[string]any{"datetime": when, "user": "Pat Example", "ismine": true},
		},
		{
			name: "like without a date or a user",
			got:  photoinfo.LikeInfo{}.AsDict(),
			want: map[string]any{"datetime": epoch, "user": nil, "ismine": false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", tt.got, tt.want)
			}
		})
	}
}
