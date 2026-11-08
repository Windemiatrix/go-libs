package albuminfo_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/albuminfo"
)

// Upstream gives 2001-01-01, 0 in Core Data seconds, without a creation date.
var noCreated = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// Not in UTC, so a change of the zone is seen.
func dates() (created, start, end time.Time) {
	created = time.Date(2024, 5, 1, 12, 0, 0, 0, time.FixedZone("", 3*3600))
	return created, created.Add(time.Hour), created.Add(2 * time.Hour)
}

func TestAlbumInfoAsDict(t *testing.T) {
	created, start, end := dates()
	tests := []struct {
		name string
		a    *albuminfo.AlbumInfo
		want map[string]any
	}{
		{
			name: "filled: photos and the fields of the port only are not in it",
			a: &albuminfo.AlbumInfo{
				UUID: "AL-1", CloudGUID: "G-1", Title: "Holidays", Owner: "Example Owner", ParentUUID: "F-2",
				FolderNames: []string{"Top", "Sub"}, FolderUUIDs: []string{"F-1", "F-2"}, PhotoUUIDs: []string{"P-1"},
				CreationDate: &created, StartDate: &start, EndDate: &end, InTrash: true,
				SortOrder: albuminfo.AlbumSortNewestFirst, LibraryListOrder: []int{1, 2},
			},
			want: map[string]any{
				"uuid": "AL-1", "creation_date": created, "start_date": start, "end_date": end, "owner": "Example Owner",
				"title": "Holidays", "folder_names": []string{"Top", "Sub"}, "folder_list": []string{"F-1", "F-2"},
				"sort_order": albuminfo.AlbumSortNewestFirst, "parent": "F-2",
			},
		},
		{
			name: "zero: the creation date of upstream, lists [], None nil",
			a:    &albuminfo.AlbumInfo{},
			want: map[string]any{
				"uuid": "", "creation_date": noCreated, "start_date": nil, "end_date": nil, "owner": nil,
				"title": nil, "folder_names": []string{}, "folder_list": []string{}, "sort_order": albuminfo.AlbumSortUnknown, "parent": nil,
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

func TestImportInfoAsDict(t *testing.T) {
	created, start, end := dates()
	tests := []struct {
		name string
		s    *albuminfo.ImportInfo
		want map[string]any
	}{
		{
			name: "filled: photos are not in it",
			s:    &albuminfo.ImportInfo{UUID: "IM-1", Title: "Import", CreationDate: &created, StartDate: &start, EndDate: &end, PhotoUUIDs: []string{"P-1"}},
			want: map[string]any{"uuid": "IM-1", "creation_date": created, "start_date": start, "end_date": end, "title": "Import"},
		},
		{
			name: "zero",
			s:    &albuminfo.ImportInfo{},
			want: map[string]any{"uuid": "", "creation_date": noCreated, "start_date": nil, "end_date": nil, "title": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestFolderInfoAsDict(t *testing.T) {
	tests := []struct {
		name string
		f    *albuminfo.FolderInfo
		want map[string]any
	}{
		{
			name: "filled: subfolders and albums as UUIDs in their order",
			f: &albuminfo.FolderInfo{
				UUID: "F-1", CloudGUID: "G-1", Title: "Top", ParentUUID: "F-0", LibraryListOrder: []int{3},
				Subfolders: []*albuminfo.FolderInfo{{UUID: "F-3"}, {UUID: "F-2"}},
				Albums:     []*albuminfo.AlbumInfo{{UUID: "AL-2"}, {UUID: "AL-1"}},
			},
			want: map[string]any{
				"title": "Top", "uuid": "F-1", "parent": "F-0",
				"subfolders": []string{"F-3", "F-2"}, "albums": []string{"AL-2", "AL-1"},
			},
		},
		{
			name: "zero: lists [], None nil",
			f:    &albuminfo.FolderInfo{},
			want: map[string]any{"title": nil, "uuid": "", "parent": nil, "subfolders": []string{}, "albums": []string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.AsDict(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AsDict =\n%#v\nwant\n%#v", got, tt.want)
			}
		})
	}
}

func TestAlbumInfoAsDictIsNotShared(t *testing.T) {
	a := &albuminfo.AlbumInfo{FolderNames: []string{"Top"}, FolderUUIDs: []string{"F-1"}}
	d := a.AsDict()
	d["folder_names"].([]string)[0] = "changed"
	d["folder_list"].([]string)[0] = "changed"
	if a.FolderNames[0] != "Top" || a.FolderUUIDs[0] != "F-1" {
		t.Errorf("AlbumInfo = %+v, want it apart from the map of AsDict", a)
	}
}
