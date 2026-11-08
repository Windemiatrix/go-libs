package adjustmentsinfo_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/adjustmentsinfo"
)

func TestAsDict(t *testing.T) {
	stamp := time.Date(2024, 5, 1, 12, 30, 0, 0, time.UTC)
	filled := adjustmentsinfo.AdjustmentsInfo{
		Plist: map[string]any{"adjustmentFormatIdentifier": "com.example.editor"}, Data: []byte{1, 2},
		Editor: "com.example.app", FormatID: "com.example.editor", BaseVersion: uint64(0), FormatVersion: "1.5",
		Timestamp: &stamp, Adjustments: []any{map[string]any{"identifier": "Crop"}}, Metadata: map[string]any{"orientation": int64(6)},
		Orientation: int64(6), AdjustmentFormatVersion: int64(1), VersionInfo: map[string]any{"buildNumber": "1A1"},
	}
	tests := []struct {
		name string
		a    adjustmentsinfo.AdjustmentsInfo
		want map[string]any
	}{
		{
			name: "filled",
			a:    filled,
			want: map[string]any{
				"data": []byte{1, 2}, "editor": "com.example.app", "format_id": "com.example.editor",
				"base_version": uint64(0), "format_version": "1.5", "adjustments": []any{map[string]any{"identifier": "Crop"}},
				"metadata": map[string]any{"orientation": int64(6)}, "orientation": int64(6), "adjustment_format_version": int64(1),
				"version_info": map[string]any{"buildNumber": "1A1"}, "timestamp": stamp,
			},
		},
		{
			name: "adjustmentData that does not decode: adjustments [], orientation 0, the rest nil",
			a:    adjustmentsinfo.AdjustmentsInfo{Orientation: 0},
			want: map[string]any{
				"data": nil, "editor": nil, "format_id": nil, "base_version": nil, "format_version": nil, "adjustments": []any{},
				"metadata": nil, "orientation": 0, "adjustment_format_version": nil, "version_info": nil, "timestamp": nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}
