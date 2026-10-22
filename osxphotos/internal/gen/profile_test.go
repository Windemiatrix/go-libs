package gen

import (
	"fmt"
	"testing"
)

// TestProfileDefault checks every count of DefaultConfig at 2000 assets. The
// expected numbers are worked out by hand from the count rule
// max(1, round(share × assets)), not computed by the code under test.
func TestProfileDefault(t *testing.T) {
	t.Parallel()
	sc := schemas[DefaultSchema]
	cfg := DefaultConfig()
	cfg.Assets = 2000
	_, db := generate(t, cfg)

	mt, ma, ms := sc.members, sc.memberAlbum, sc.memberAsset
	perAsset := "SELECT COUNT(*) FROM (SELECT a.Z_PK, COUNT(m." + ms + ") AS n FROM ZASSET a LEFT JOIN " + mt +
		" m ON m." + ms + " = a.Z_PK GROUP BY a.Z_PK) WHERE "
	members := "(SELECT COUNT(*) FROM " + mt + " m WHERE m." + ma + " = g.Z_PK)"
	runChecks(t, db, []check{
		{"assets", "SELECT COUNT(*) FROM ZASSET", "2000"},
		{"videos 0.05", "SELECT COUNT(*) FROM ZASSET WHERE ZKIND = 1 AND ZKINDSUBTYPE = 0", "100"},
		{"live photos 0.003", "SELECT COUNT(*) FROM ZASSET WHERE ZKIND = 0 AND ZKINDSUBTYPE = 2", "6"},
		{"screenshots 0.003", "SELECT COUNT(*) FROM ZASSET WHERE ZKIND = 0 AND ZKINDSUBTYPE = 10", "6"},
		{"screenshots are PNG", "SELECT COUNT(*) FROM ZASSET WHERE ZKINDSUBTYPE = 10 AND ZUNIFORMTYPEIDENTIFIER = 'public.png'", "6"},
		{"live photos are HEIC", "SELECT COUNT(*) FROM ZASSET WHERE ZKINDSUBTYPE = 2 AND ZUNIFORMTYPEIDENTIFIER = 'public.heic'", "6"},
		{"RAW pairs 0.04", "SELECT COUNT(DISTINCT ZASSET) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 3", "80"},
		{"RAW pairs are JPEG", "SELECT COUNT(*) FROM ZASSET a JOIN ZINTERNALRESOURCE r ON r.ZASSET = a.Z_PK " +
			"WHERE r.ZDATASTORESUBTYPE = 3 AND a.ZUNIFORMTYPEIDENTIFIER = 'public.jpeg'", "80"},
		// max(5, count(0.001, 2000) = 2) = 5 frames: group A (2) and group B (3)
		{"burst frames", "SELECT COUNT(*) FROM ZASSET WHERE ZAVALANCHEUUID IS NOT NULL", "5"},
		{"burst groups", "SELECT COUNT(DISTINCT ZAVALANCHEUUID) FROM ZASSET", "2"},
		{"burst marks", "SELECT group_concat(p, ',') FROM (SELECT DISTINCT ZAVALANCHEPICKTYPE AS p FROM ZASSET " +
			"WHERE ZAVALANCHEUUID IS NOT NULL ORDER BY 1)", "0,2,8,16"},
		{"invisible frames", "SELECT COUNT(*) FROM ZASSET WHERE ZVISIBILITYSTATE = 2", "2"},
		{"non-burst pick type", "SELECT COUNT(*) FROM ZASSET WHERE ZAVALANCHEUUID IS NULL AND ZAVALANCHEPICKTYPE != 0", "0"},
		// 2000 - 100 - 6 - 6 - 80 - 5
		{"ordinary photos", "SELECT COUNT(*) FROM ZASSET a WHERE ZKIND = 0 AND ZKINDSUBTYPE = 0 AND ZAVALANCHEUUID IS NULL " +
			"AND NOT EXISTS (SELECT 1 FROM ZINTERNALRESOURCE r WHERE r.ZASSET = a.Z_PK AND r.ZDATASTORESUBTYPE = 3)", "1803"},
		{"trashed 0.02", "SELECT COUNT(*) FROM ZASSET WHERE ZTRASHEDSTATE = 1", "40"},
		{"hidden 0.001", "SELECT COUNT(*) FROM ZASSET WHERE ZHIDDEN = 1", "2"},
		{"favorites 0.001", "SELECT COUNT(*) FROM ZASSET WHERE ZFAVORITE = 1", "2"},
		{"edited 0.1", "SELECT COUNT(*) FROM ZASSET WHERE " + sc.adjustments + " > 0", "200"},
		{"edited state 1", "SELECT COUNT(*) FROM ZASSET WHERE " + sc.adjustments + " = 1", "100"},
		{"edited state 2", "SELECT COUNT(*) FROM ZASSET WHERE " + sc.adjustments + " = 2", "100"},
		{"cloud only 0.9", "SELECT COUNT(DISTINCT ZASSET) FROM ZINTERNALRESOURCE WHERE ZLOCALAVAILABILITY = -1", "1800"},
		{"cloud only is whole", "SELECT COUNT(*) FROM (SELECT ZASSET FROM ZINTERNALRESOURCE GROUP BY ZASSET " +
			"HAVING MIN(ZLOCALAVAILABILITY) != MAX(ZLOCALAVAILABILITY))", "0"},
		{"shared library 0.65", "SELECT COUNT(*) FROM ZASSET WHERE ZLIBRARYSCOPE IS NOT NULL", "1300"},
		{"library scope rows", "SELECT COUNT(*) FROM ZSHARE", "1"},
		{"outside albums 0.45", perAsset + "n = 0", "900"},
		{"in several albums 0.01", perAsset + "n >= 2", "20"},
		{"in one album", perAsset + "n = 1", "1080"},
		{"in at most three albums", perAsset + "n > 3", "0"},
		// Every other name is unique by its index, so each duplicate removes one distinct name.
		{"duplicate names 0.2", "SELECT COUNT(*) - COUNT(DISTINCT ZORIGINALFILENAME) FROM ZADDITIONALASSETATTRIBUTES", "400"},
		// at least: a duplicate name may copy a non-ASCII one
		{"non-ASCII names 0.03", "SELECT COUNT(*) >= 60 FROM ZADDITIONALASSETATTRIBUTES WHERE ZORIGINALFILENAME GLOB '*[^ -~]*'", "1"},
		{"NFD name", "SELECT COUNT(*) > 0 FROM ZADDITIONALASSETATTRIBUTES WHERE instr(ZORIGINALFILENAME, 'e' || char(769)) > 0", "1"},
		{"screenshot names", "SELECT COUNT(*) FROM ZASSET a JOIN ZADDITIONALASSETATTRIBUTES x ON x.ZASSET = a.Z_PK " +
			"WHERE a.ZKINDSUBTYPE = 10 AND x.ZORIGINALFILENAME LIKE '%Screenshot %.PNG'", "6"},
		{"zero dates 0.004", "SELECT COUNT(*) FROM ZASSET WHERE ZDATECREATED IN (0, -978307200)", "8"},
		{"Core Data epoch", "SELECT COUNT(*) FROM ZASSET WHERE ZDATECREATED = 0", "4"},
		// Each same-second asset removes one distinct date (seed 1 has no chance collision).
		{"same second 0.1", "SELECT COUNT(*) - COUNT(DISTINCT ZDATECREATED) FROM ZASSET WHERE ZDATECREATED NOT IN (0, -978307200)", "200"},
		{"created range", "SELECT COUNT(*) FROM ZASSET WHERE ZDATECREATED NOT IN (0, -978307200) " +
			"AND (ZDATECREATED < 283996800 OR ZDATECREATED >= 788918400)", "0"},
		// Whole dates are stored as INTEGER, dates with a fraction as REAL.
		{"date types", "SELECT SUM(typeof(ZDATECREATED) = 'integer') > 0 AND SUM(typeof(ZDATECREATED) = 'real') > 0 " +
			"AND SUM(typeof(ZADDEDDATE) = 'integer') > 0 AND SUM(typeof(ZADDEDDATE) = 'real') > 0 FROM ZASSET", "1"},
		{"modified range", "SELECT COUNT(*) FROM ZASSET WHERE ZMODIFICATIONDATE < ZADDEDDATE OR ZMODIFICATIONDATE >= 801964800", "0"},
		{"same content 1", "SELECT COUNT(*) - COUNT(DISTINCT " + sc.fingerprint + ") FROM ZADDITIONALASSETATTRIBUTES", "1"},
		{"huge video", "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATALENGTH > 4294967296", "1"},
		{"coordinates", "SELECT COUNT(*) FROM ZASSET WHERE ZLATITUDE != -180", "1000"},
		{"saved types", "SELECT group_concat(t, ',') FROM (SELECT DISTINCT ZSAVEDASSETTYPE AS t FROM ZASSET ORDER BY 1)", "3,6"},
		// A = max(4, ceil(2000 / 40)) = 50, F = max(1, ceil(50 / 25)) = 2
		{"albums", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 2", "50"},
		{"folders", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 4000", "2"},
		{"roots", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 3999", "1"},
		{"albums in the root 0.02", "SELECT COUNT(*) FROM ZGENERICALBUM g JOIN ZGENERICALBUM p ON p.Z_PK = g.ZPARENTFOLDER " +
			"WHERE g.ZKIND = 2 AND p.ZKIND = 3999", "1"},
		{"empty album", "SELECT COUNT(*) > 0 FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND " + members + " = 0", "1"},
		{"album of one", "SELECT COUNT(*) > 0 FROM ZGENERICALBUM g WHERE g.ZKIND = 2 AND " + members + " = 1", "1"},
		{"titles without a slash", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 4000) AND instr(ZTITLE, '/') = 0", "2"},
		// words by album index: one in seven is "Holidays"; folders 0 and 1 are non-ASCII
		{"non-ASCII titles", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND IN (2, 4000) AND ZTITLE GLOB '*[^ -~]*'", "45"},
		{"title length", "SELECT MAX(length(ZTITLE)) <= 54 FROM ZGENERICALBUM", "1"},
		{"distinct titles in a folder", "SELECT COUNT(*) FROM (SELECT ZPARENTFOLDER, ZTITLE FROM ZGENERICALBUM " +
			"GROUP BY 1, 2 HAVING COUNT(*) > 1)", "0"},
		// round(0.6 × 52 albums and folders) = 31
		{"cloud GUID = UUID", "SELECT SUM(ZCLOUDGUID = ZUUID) FROM ZGENERICALBUM WHERE ZKIND IN (2, 4000)", "31"},
		{"root cloud GUID", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 3999 AND ZCLOUDGUID = ZUUID", "0"},
		{"order step", "SELECT MIN(" + sc.memberOrder + ") || '/' || COUNT(DISTINCT " + sc.memberOrder + " % 1024) FROM " + mt, "1024/1"},
		// 2000 originals, 80 second originals, 6 paired videos
		{"resources", "SELECT COUNT(*) FROM ZINTERNALRESOURCE", "2086"},
		{"fingerprint form", "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE length(ZFINGERPRINT) != 28 OR substr(ZFINGERPRINT, 1, 1) != 'A'", "0"},
		{"time zones", "SELECT COUNT(DISTINCT ZTIMEZONENAME) FROM ZADDITIONALASSETATTRIBUTES", "6"},
	})
}

// TestProfileZero: with every share zero there are only ordinary photos, each
// in exactly one album.
func TestProfileZero(t *testing.T) {
	t.Parallel()
	sc := schemas[DefaultSchema]
	_, db := generate(t, zeroConfig(100))
	mt, ms := sc.members, sc.memberAsset
	runChecks(t, db, []check{
		{"assets", "SELECT COUNT(*) FROM ZASSET", "100"},
		{"ordinary photos", "SELECT COUNT(*) FROM ZASSET WHERE ZKIND = 0 AND ZKINDSUBTYPE = 0 AND ZAVALANCHEUUID IS NULL " +
			"AND ZTRASHEDSTATE = 0 AND ZHIDDEN = 0 AND ZFAVORITE = 0 AND " + sc.adjustments + " = 0 AND ZLIBRARYSCOPE IS NULL", "100"},
		{"resources", "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE = 1 AND ZLOCALAVAILABILITY = 1", "100"},
		{"other resources", "SELECT COUNT(*) FROM ZINTERNALRESOURCE WHERE ZDATASTORESUBTYPE != 1", "0"},
		{"one album each", "SELECT COUNT(*) FROM (SELECT a.Z_PK FROM ZASSET a JOIN " + mt + " m ON m." + ms + " = a.Z_PK " +
			"GROUP BY a.Z_PK HAVING COUNT(*) = 1)", "100"},
		{"albums", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 2", "4"},
		{"folders", "SELECT COUNT(*) FROM ZGENERICALBUM WHERE ZKIND = 4000", "1"},
		{"no library scope", "SELECT COUNT(*) FROM ZSHARE", "0"},
		{"distinct fingerprints", "SELECT COUNT(DISTINCT " + sc.fingerprint + ") FROM ZADDITIONALASSETATTRIBUTES", "100"},
		{"distinct names", "SELECT COUNT(DISTINCT ZORIGINALFILENAME) FROM ZADDITIONALASSETATTRIBUTES", "100"},
		{"ASCII names", "SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES WHERE ZORIGINALFILENAME GLOB '*[^ -~]*'", "0"},
		{"zero dates", "SELECT COUNT(*) FROM ZASSET WHERE ZDATECREATED IN (0, -978307200)", "0"},
	})
}

// TestProfileEmpty: an empty library has the schema, the metadata, the
// primary keys and the root folder only.
func TestProfileEmpty(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 0
	_, db := generate(t, cfg)
	sc := schemas[DefaultSchema]
	runChecks(t, db, []check{
		{"assets", "SELECT COUNT(*) FROM ZASSET", "0"},
		{"attributes", "SELECT COUNT(*) FROM ZADDITIONALASSETATTRIBUTES", "0"},
		{"resources", "SELECT COUNT(*) FROM ZINTERNALRESOURCE", "0"},
		{"albums", "SELECT COUNT(*) FROM ZGENERICALBUM", "1"},
		{"root", "SELECT ZKIND FROM ZGENERICALBUM", "3999"},
		{"shares", "SELECT COUNT(*) FROM ZSHARE", "0"},
		{"metadata", "SELECT COUNT(*) FROM Z_METADATA", "1"},
		{"primary keys", "SELECT COUNT(*) FROM Z_PRIMARYKEY", fmt.Sprint(len(sc.entities))},
		{"asset max", "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_NAME = 'Asset'", "0"},
		{"album max", "SELECT Z_MAX FROM Z_PRIMARYKEY WHERE Z_NAME = 'GenericAlbum'", "1"},
	})
}

// TestPrimaryKeysDiffer: the Z_PK sequences of related tables do not line up,
// so a join on a wrong column in a reader test does not pass by chance.
func TestPrimaryKeysDiffer(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Assets = 301
	_, db := generate(t, cfg)
	runChecks(t, db, []check{
		{"attributes", "SELECT COUNT(*) <= 1 FROM ZASSET WHERE Z_PK = ZADDITIONALATTRIBUTES", "1"},
		{"masters", "SELECT COUNT(*) FROM ZASSET WHERE Z_PK = ZMASTER", "0"},
	})
}
