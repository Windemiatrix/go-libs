package template

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Windemiatrix/go-libs/osxphotos/internal/testlib"
)

func TestShortUUID(t *testing.T) {
	t.Parallel()
	twos := func(n int) string { return strings.Repeat("2", n) }
	for _, tt := range []struct{ in, want string }{
		{"00000000-0000-0000-0000-000000000000", twos(22)},
		{"00000000-0000-0000-0000-000000000001", twos(21) + "3"},
		{"00000000-0000-0000-0000-000000000039", twos(20) + "32"},
		{"00000000-0000-0000-0000-000000000038", twos(21) + "z"},
		// uuid.UUID(hex) of CPython: any case, no dashes, braces, urn:uuid:
		{"00000000-0000-0000-0000-00000000003A", twos(20) + "33"},
		{strings.Repeat("0", 30) + "3a", twos(20) + "33"},
		{"{00000000-0000-0000-0000-00000000003a}", twos(20) + "33"},
		{"urn:uuid:00000000-0000-0000-0000-00000000003a", twos(20) + "33"},
		// int(hex, 16) of Python takes a sign
		{"+" + strings.Repeat("0", 30) + "1", twos(21) + "3"},
	} {
		if got, err := shortUUID(tt.in); err != nil || got != tt.want {
			t.Errorf("shortUUID(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{
		"", "nope", strings.Repeat("0", 31), strings.Repeat("0", 33), "00000000-0000-0000-0000-00000000000g",
		"-" + strings.Repeat("0", 30) + "1",
	} {
		if got, err := shortUUID(in); !errors.Is(err, ErrRender) || !strings.Contains(err.Error(), "shortuuid") {
			t.Errorf("shortUUID(%q) = %q, %v; want ErrRender that names shortuuid", in, got, err)
		}
	}
}

// osxphotos gave t007 ({shortuuid}) of the uuid it gave as t006 ({uuid}).
func TestShortUUIDOracle(t *testing.T) {
	dir := testlib.RepoDir(t, "golden")
	templates := goldenTemplates(t, dir)
	if templates[5] != "{uuid}" || templates[6] != "{shortuuid}" {
		t.Fatalf("t006 and t007 of templates.txt are %q and %q, not {uuid} and {shortuuid}", templates[5], templates[6])
	}
	objects := goldenObjects(t, filepath.Join(dir, "synthetic", "templates.json"))
	for i, obj := range objects {
		uuid, want := oracleValues(t, obj["t006"]), oracleValues(t, obj["t007"])
		if len(uuid) != 1 || len(want) != 1 {
			t.Fatalf("element %d: t006 and t007 are not one value each", i+1)
		}
		if got, err := shortUUID(uuid[0]); err != nil || got != want[0] {
			t.Errorf("element %d: shortUUID of t006 = %q, %v; osxphotos gave %q", i+1, got, err, want[0])
		}
	}
	t.Logf("%d pairs of t006 and t007", len(objects))
}

func TestRenderShortUUID(t *testing.T) {
	t.Parallel()
	p := fakePhoto{"uuid": strs("00000000-0000-0000-0000-000000000039")}
	o := RenderOptions{}
	checkRender(t, []renderCase{
		{"shortuuid", "{shortuuid}", p, o, []string{strings.Repeat("2", 20) + "32"}, nil},
		{"no uuid", "{shortuuid}", fakePhoto{"uuid": {}}, o, []string{"_"}, nil},
		{"uuid and shortuuid", "{uuid}/{shortuuid}", p, o, []string{"00000000-0000-0000-0000-000000000039/" + strings.Repeat("2", 20) + "32"}, nil},
	})
	got, _, err := render(t, "{shortuuid}", fakePhoto{"uuid": strs("Holidays")}, o)
	if msg := fmt.Sprint(err); !errors.Is(err, ErrRender) || !strings.Contains(msg, "shortuuid") || strings.Contains(msg, "Holidays") || got != nil {
		t.Errorf("Render({shortuuid}) of a uuid that is not a UUID = %q, %v; want ErrRender that names shortuuid and not the uuid", got, err)
	}
	if got, _, err := render(t, "{shortuuid}", fakePhoto{}, o); !errors.Is(err, ErrUnsupportedField) || !strings.HasSuffix(err.Error(), ": uuid") || got != nil {
		t.Errorf("Render({shortuuid}) of a photo without uuid = %q, %v; want ErrUnsupportedField", got, err)
	}
}
