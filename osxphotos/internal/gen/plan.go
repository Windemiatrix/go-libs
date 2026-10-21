package gen

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"slices"
	"time"
)

// The plan is the whole library in memory, built from a Config without any
// I/O; write.go turns it into rows. Values that depend on other rows (sizes
// of the original, dates of albums, pairs of the same content) are settled
// here, so the rows are written in one pass.

// Streams of the random generator, one per task: a task added later takes a
// new stream and does not shift the values of the others.
const (
	streamKinds uint64 = iota + 1
	streamMedia
	streamSizes
	streamIDs
	streamFlags
	streamNames
	streamDates
	streamAlbums
	streamMembers
)

// rng draws only Uint64 from PCG: the algorithm of PCG is fixed, while the
// derived methods of math/rand/v2 (IntN, Shuffle, Perm) do not promise the
// same output across Go versions, and the reference outputs built from
// synthetic libraries depend on the content.
//
// Stored values must also be the same on every CPU architecture, so they are
// made from integers. The compiler may fuse x*y + z into one FMA instruction
// on one architecture (arm64) and not on another (amd64), and math.Exp,
// math.Log and the like have per-architecture code. A float from float() is
// only compared with a share or truncated to an index, never stored and
// never added to a product.
type rng struct{ src *rand.PCG }

func newRNG(seed, stream uint64) *rng {
	return &rng{src: rand.NewPCG(seed, stream)} //nolint:gosec // G404: deterministic synthetic data
}

// below returns a number in [0, n) for n > 0 from a uniform u: the high word
// of u × n.
func below(u uint64, n int64) int64 {
	hi, _ := bits.Mul64(u, uint64(n)) //nolint:gosec // G115: n > 0
	return int64(hi)                  //nolint:gosec // G115: hi < n
}

// intn returns a number in [0, n) for n > 0.
func (r *rng) intn(n int) int { return int(below(r.src.Uint64(), int64(n))) }

// float returns a number in [0, 1).
func (r *rng) float() float64 { return float64(r.src.Uint64()>>11) * 0x1p-53 }

// perm returns a random permutation of [0, n) (Fisher–Yates).
func (r *rng) perm(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := r.intn(i + 1)
		p[i], p[j] = p[j], p[i]
	}
	return p
}

// pick returns k random indexes of [0, n) as a set.
func (r *rng) pick(n, k int) []bool {
	set := make([]bool, n)
	for _, i := range r.perm(n)[:k] {
		set[i] = true
	}
	return set
}

// uuid returns an upper-case version 4 UUID. It is not cryptographic and
// need not be: the data are synthetic.
func (r *rng) uuid() string {
	hi := r.src.Uint64()&^0xF000 | 0x4000
	lo := r.src.Uint64()&^(0xC<<60) | 0x8<<60
	return fmt.Sprintf("%08X-%04X-%04X-%04X-%012X", hi>>32, hi>>16&0xFFFF, hi&0xFFFF, lo>>48, lo&0xFFFFFFFFFFFF)
}

// fingerprint returns 28 base64 characters starting with "A" (a byte 1 and
// 20 random bytes), the form of the fingerprints in the osxphotos test
// library Test-26.1.
func (r *rng) fingerprint() string {
	var b [25]byte
	b[0] = 1
	for i := range 3 {
		binary.BigEndian.PutUint64(b[1+8*i:], r.src.Uint64())
	}
	return base64.StdEncoding.EncodeToString(b[:21])
}

// countOf is the number of assets of a share: 0 for a zero share or no
// assets, otherwise max(1, round(share × n)) (round, not ceil: 0.07 × 100 is
// 7.000000000000001 in float64).
func countOf(share float64, n int) int {
	if share == 0 || n == 0 {
		return 0
	}
	return max(1, int(math.Round(share*float64(n))))
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// counts are the numbers of assets of each kind and group that follow from a
// Config; validation and the plan use the same numbers.
type counts struct {
	videos, live, screenshots, raw int
	bursts                         []int // frames per burst group
	frames, invisible              int   // burst frames, hidden burst frames
	photos                         int   // ordinary photos
	trashed, hidden                int
	outside, several               int // assets outside albums, in several albums
}

func countsOf(cfg Config) counts {
	n := cfg.Assets
	c := counts{
		videos:      countOf(cfg.Videos, n),
		live:        countOf(cfg.LivePhotos, n),
		screenshots: countOf(cfg.Screenshots, n),
		raw:         countOf(cfg.RAWPairs, n),
		trashed:     countOf(cfg.Trashed, n),
		hidden:      countOf(cfg.Hidden, n),
		several:     countOf(cfg.SeveralAlbums, n),
	}
	if cfg.BurstFrames > 0 && n > 0 {
		// Groups alternate: A of 2 frames, B of 3, until there are enough frames.
		target := max(5, countOf(cfg.BurstFrames, n))
		for c.frames < target {
			size := len(burstA)
			if len(c.bursts)%2 == 1 {
				size = len(burstB)
			}
			c.bursts = append(c.bursts, size)
			c.frames += size
			c.invisible++ // one hidden frame in either group
		}
	}
	c.photos = n - c.videos - c.live - c.screenshots - c.raw - c.frames
	c.outside = max(countOf(cfg.NoAlbum, n), c.invisible)
	return c
}

// frame is a burst frame: ZAVALANCHEPICKTYPE and ZVISIBILITYSTATE.
type frame struct{ pick, visibility int }

var (
	// burstA: two frames of pick type 0, the second not shown, as in a real library.
	burstA = []frame{{0, 0}, {0, 2}}
	// burstB: the key frame, a frame the user picked, a frame not picked.
	burstB = []frame{{16, 0}, {8, 0}, {2, 2}}
)

// format describes a file type.
type format struct {
	uti      string // ZUNIFORMTYPEIDENTIFIER
	ext      string // extension of the original file name
	fileExt  string // extension of ZFILENAME
	compact  int    // ZINTERNALRESOURCE.ZCOMPACTUTI
	resource int    // ZINTERNALRESOURCE.ZRESOURCETYPE
	min, max int64  // file sizes in bytes, [min, max)
	logSize  bool   // sizes are about log-uniform (max / min is a power of two), otherwise uniform
}

// size returns a file size from two uniform numbers. A log-uniform size
// takes a power-of-two bucket [min·2ᵏ, min·2ᵏ⁺¹) by uBucket, then a uniform
// size inside it; a uniform size ignores uBucket.
func (f *format) size(uBucket, u uint64) int64 {
	lo, hi := f.min, f.max
	if f.logSize {
		k := below(uBucket, int64(bits.Len64(uint64(hi/lo))-1)) //nolint:gosec // G115: hi / lo > 0
		lo, hi = lo<<k, lo<<(k+1)
	}
	return lo + below(u, hi-lo)
}

const mib = 1 << 20

var (
	// ZFILENAME extensions, ZCOMPACTUTI and ZRESOURCETYPE of JPEG, HEIC and
	// DNG originals as in the osxphotos test library Test-26.1.
	jpeg = &format{uti: "public.jpeg", ext: "JPG", fileExt: "jpeg", compact: 1, resource: 0, min: 1 * mib, max: 6 * mib}
	heic = &format{uti: "public.heic", ext: "HEIC", fileExt: "heic", compact: 3, resource: 0, min: 1 * mib, max: 4 * mib}
	dng  = &format{uti: "com.adobe.raw-image", ext: "DNG", fileExt: "dng", compact: 9, resource: 0,
		min: 20 * mib, max: 50 * mib}
	// synthetic: not observed in a test library (ZCOMPACTUTI, ZRESOURCETYPE, ZFILENAME extension)
	png = &format{uti: "public.png", ext: "PNG", fileExt: "png", compact: 7, resource: 0, min: 200 << 10, max: 3 * mib}
	mov = &format{uti: "com.apple.quicktime-movie", ext: "MOV", fileExt: "mov", compact: 21, resource: 1,
		min: 1 * mib, max: 2048 * mib, logSize: true}
	mp4 = &format{uti: "public.mpeg-4", ext: "MP4", fileExt: "mp4", compact: 22, resource: 1,
		min: 1 * mib, max: 2048 * mib, logSize: true}
	// The second original of a RAW+JPEG pair: ZRESOURCETYPE and ZCOMPACTUTI
	// of the RAW resource of the osxphotos test library Test-26.1 (a CR2).
	cr2 = &format{compact: 11, resource: 4, min: 20 * mib, max: 30 * mib}
	// The paired video of a Live photo (resource type 3, data store subtype
	// 18 in a real library); its ZCOMPACTUTI is synthetic.
	liveVideo = &format{compact: mov.compact, resource: 3, min: 1 * mib, max: 4 * mib}
)

// hugeVideo is the size of the first video: more than 2³² bytes.
const hugeVideo = 4608 * mib

type weighted struct {
	f *format
	w float64
}

var (
	photoFormats = []weighted{{jpeg, 0.5}, {heic, 0.3}, {dng, 0.2}}
	videoFormats = []weighted{{mov, 0.75}, {mp4, 0.25}}
)

func choose(ws []weighted, u float64) *format {
	for _, w := range ws {
		if u < w.w {
			return w.f
		}
		u -= w.w
	}
	return ws[len(ws)-1].f
}

// Pixel sizes: 12, 24 and 48 MP photos, a Full HD video, a portrait screenshot.
var (
	photoSizes = [][2]int{{4032, 3024}, {5712, 4284}, {8064, 6048}}
	videoSize  = [2]int{1920, 1080}
	screenSize = [2]int{1179, 2556}
)

// savedImported is the share of ZSAVEDASSETTYPE 3 (imported on this Mac); the
// rest is 6 (from iCloud).
const savedImported = 0.3

// timeZone is ZTIMEZONEOFFSET (and ZINFERREDTIMEZONEOFFSET) with
// ZTIMEZONENAME; an empty name is NULL in all three columns. The names are of
// the form of the osxphotos test library Test-26.1 ("GMT-0400").
type timeZone struct {
	offset int
	name   string
}

var timeZones = []timeZone{{0, ""}, {0, "GMT"}, {10800, "GMT+0300"}, {-18000, "GMT-0500"},
	{19800, "GMT+0530"}, {20700, "GMT+0545"}, {50400, "GMT+1400"}}

// Dates are Core Data seconds since 2001-01-01 UTC.
var (
	coreDataEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	createdFrom   = coreData(time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC))
	createdTo     = coreData(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	addedFromZero = coreData(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) // added date of zero creation dates
	dateEnd       = coreData(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) // end of every date, not time.Now
	unixEpoch     = coreData(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
	maxAddDelay   = (3 * 365 * 24 * time.Hour).Seconds()
)

func coreData(t time.Time) float64 { return t.Sub(coreDataEpoch).Seconds() }

// between returns a whole number of seconds in [lo, hi) from a uniform u; lo
// and hi are whole numbers of seconds, lo < hi.
func between(lo, hi float64, u uint64) float64 { return lo + float64(below(u, int64(hi-lo))) }

// Synthetic words of album and folder titles, mostly non-ASCII.
var titleWords = []string{"Отпуск", "Дача", "Море", "Café", "Straße", "Holidays", "Поездки"}

// Prefixes of non-ASCII file names; the first is "Café" in NFD, so that any
// library with non-ASCII names has one in NFD.
var namePrefixes = []string{"Café", "Фото", "Café", "Zürich", "Ёлка"}

const (
	rootAlbumsShare = 0.02 // share of albums in the root folder
	albumGUIDShare  = 0.6  // share of albums and folders whose ZCLOUDGUID is ZUUID
	coordsShare     = 0.5  // share of assets with coordinates
	noCoordinate    = -180 // ZLATITUDE and ZLONGITUDE of an asset without coordinates
)

const (
	kindPhoto = iota // the class of an asset: what the generator made it
	kindVideo
	kindLive
	kindScreenshot
	kindRAWPair
	kindBurst
)

type asset struct {
	class                     int
	kind, subtype             int // ZKIND, ZKINDSUBTYPE
	f                         *format
	width, height             int
	savedType                 int
	lat, lon                  float64
	uuid, cloudGUID, master   string // ZUUID, ZCLOUDASSETGUID, ZCLOUDMASTERGUID
	resources                 []resource
	burst                     int // burst group + 1, 0 outside bursts
	pick, visibility          int
	trashed, hidden, favorite bool
	adjustments               int // ZADJUSTMENTSSTATE: 0, 1 or 2
	cloudOnly, shared         bool
	original                  string // ZORIGINALFILENAME
	created, added, modified  float64
	trashedAt, adjustedAt     float64
}

type resource struct {
	subtype     int // ZDATASTORESUBTYPE
	f           *format
	length      int64
	fingerprint string
}

type album struct {
	kind            int // 3999 root, 4000 folder, 2 album
	parent          int // index in plan.albums, -1 for none
	uuid, cloudGUID string
	title           string // "" is NULL
	created         float64
	members         []int // asset indexes
}

type plan struct {
	libraryUUID string
	scopeUUID   string // the shared library scope: ZSHARE.ZUUID and ZSCOPEIDENTIFIER
	scopeID     string
	assets      []asset
	bursts      []string // ZAVALANCHEUUID by group
	albums      []album  // root, folders, user albums
}

func (p *plan) shared() bool {
	return slices.ContainsFunc(p.assets, func(a asset) bool { return a.shared })
}

// buildPlan builds the library of a valid Config.
func buildPlan(cfg Config) *plan {
	c := countsOf(cfg)
	p := &plan{assets: make([]asset, cfg.Assets)}
	ids := newRNG(cfg.Seed, streamIDs)
	p.libraryUUID = ids.uuid()
	if cfg.Assets > 0 {
		pairs := planKinds(p, cfg, c)
		planMedia(p, cfg, pairs)
		planResources(p, cfg)
		planIDs(p, ids, pairs)
		planFlags(p, cfg, c)
		planNames(p, cfg)
		planDates(p, cfg)
	}
	planAlbums(p, cfg, c)
	return p
}

// planKinds places the kinds at random indexes and returns the pairs of
// ordinary photos of the same content.
func planKinds(p *plan, cfg Config, c counts) [][2]int {
	r := newRNG(cfg.Seed, streamKinds)
	rest := r.perm(cfg.Assets)
	next := func(k int) []int {
		s := slices.Sorted(slices.Values(rest[:k]))
		rest = rest[k:]
		return s
	}
	set := func(k, class, kind, subtype int) {
		for _, i := range next(k) {
			p.assets[i].class, p.assets[i].kind, p.assets[i].subtype = class, kind, subtype
		}
	}
	set(c.videos, kindVideo, 1, 0)
	set(c.live, kindLive, 0, 2)
	set(c.screenshots, kindScreenshot, 0, 10)
	set(c.raw, kindRAWPair, 0, 0)
	for g, size := range c.bursts {
		frames := burstA
		if size == len(burstB) {
			frames = burstB
		}
		for k, i := range next(size) {
			a := &p.assets[i]
			a.class, a.burst, a.pick, a.visibility = kindBurst, g+1, frames[k].pick, frames[k].visibility
		}
	}
	photos := slices.Sorted(slices.Values(rest))
	order := r.perm(len(photos))
	var pairs [][2]int
	for j := range cfg.SameContentPairs {
		a, b := photos[order[2*j]], photos[order[2*j+1]]
		pairs = append(pairs, [2]int{min(a, b), max(a, b)})
	}
	return pairs
}

// planMedia sets the file type, the pixel size, the saved type and the
// coordinates; the second asset of a pair takes the type and size of the first.
func planMedia(p *plan, cfg Config, pairs [][2]int) {
	r := newRNG(cfg.Seed, streamMedia)
	for i := range p.assets {
		a := &p.assets[i]
		// Coordinates in micro-degrees: lat in [-90, 90), lon in [-180, 180).
		uFormat, uSize, uSaved, lat, lon := r.float(), r.float(), r.float(), r.intn(180_000_000), r.intn(360_000_000)
		size := photoSizes[int(uSize*float64(len(photoSizes)))]
		switch a.class {
		case kindVideo:
			a.f, size = choose(videoFormats, uFormat), videoSize
		case kindLive:
			a.f = heic
		case kindScreenshot:
			a.f, size = png, screenSize
		case kindRAWPair:
			a.f = jpeg
		case kindPhoto, kindBurst:
			a.f = choose(photoFormats, uFormat)
		}
		a.width, a.height = size[0], size[1]
		a.savedType = 6
		if uSaved < savedImported {
			a.savedType = 3
		}
		a.lat, a.lon = float64(lat)/1e6-90, float64(lon)/1e6-180
	}
	withCoords := r.pick(len(p.assets), countOf(coordsShare, len(p.assets)))
	for i := range p.assets {
		if !withCoords[i] {
			p.assets[i].lat, p.assets[i].lon = noCoordinate, noCoordinate
		}
	}
	for _, pr := range pairs {
		a, b := &p.assets[pr[0]], &p.assets[pr[1]]
		b.f, b.width, b.height = a.f, a.width, a.height
	}
}

// planResources adds the original, the second original of a RAW+JPEG pair
// and the paired video of a Live photo with their sizes.
func planResources(p *plan, cfg Config) {
	r := newRNG(cfg.Seed, streamSizes)
	firstVideo := true
	for i := range p.assets {
		a := &p.assets[i]
		// Three numbers for every asset, so the sizes of an asset do not
		// depend on the kinds of the assets before it.
		uBucket, uOriginal, uExtra := r.src.Uint64(), r.src.Uint64(), r.src.Uint64()
		original := resource{subtype: 1, f: a.f, length: a.f.size(uBucket, uOriginal)}
		if a.class == kindVideo && firstVideo {
			original.length, firstVideo = hugeVideo, false
		}
		a.resources = []resource{original}
		switch a.class {
		case kindRAWPair:
			a.resources = append(a.resources, resource{subtype: 3, f: cr2, length: cr2.size(0, uExtra)})
		case kindLive:
			a.resources = append(a.resources, resource{subtype: 18, f: liveVideo, length: liveVideo.size(0, uExtra)})
		}
	}
}

// planIDs sets the identifiers and fingerprints; the second asset of a pair
// takes the size and the fingerprint of the original of the first.
func planIDs(p *plan, ids *rng, pairs [][2]int) {
	for i := range p.assets {
		a := &p.assets[i]
		a.uuid, a.cloudGUID, a.master = ids.uuid(), ids.uuid(), ids.uuid()
		for k := range a.resources {
			a.resources[k].fingerprint = ids.fingerprint()
		}
	}
	for _, pr := range pairs {
		a, b := &p.assets[pr[0]], &p.assets[pr[1]]
		b.resources[0].length, b.resources[0].fingerprint = a.resources[0].length, a.resources[0].fingerprint
	}
	groups := 0
	for _, a := range p.assets {
		groups = max(groups, a.burst)
	}
	for range groups {
		p.bursts = append(p.bursts, ids.uuid())
	}
	p.scopeUUID, p.scopeID = ids.uuid(), "SharedSync-"+ids.uuid() // synthetic: not observed in a test library
}

// planFlags sets the flags by independent permutations; hidden assets are
// chosen among the ones not in the trash.
func planFlags(p *plan, cfg Config, c counts) {
	r := newRNG(cfg.Seed, streamFlags)
	n := len(p.assets)
	trashed := r.pick(n, c.trashed)
	var kept []int
	for i := range p.assets {
		p.assets[i].trashed = trashed[i]
		if !trashed[i] {
			kept = append(kept, i)
		}
	}
	hidden := r.pick(len(kept), c.hidden)
	for k, i := range kept {
		p.assets[i].hidden = hidden[k]
	}
	favorite := r.pick(n, countOf(cfg.Favorites, n))
	for i := range p.assets {
		p.assets[i].favorite = favorite[i]
	}
	for j, i := range r.perm(n)[:countOf(cfg.Edited, n)] {
		p.assets[i].adjustments = 1 + j%2 // a state, not a flag
	}
	cloudOnly := r.pick(n, countOf(cfg.CloudOnly, n))
	shared := r.pick(n, countOf(cfg.SharedLibrary, n))
	for i := range p.assets {
		p.assets[i].cloudOnly, p.assets[i].shared = cloudOnly[i], shared[i]
	}
}

// planNames sets the original file names: IMG_0001.JPG or Screenshot
// 0001.PNG, a non-ASCII prefix on some, and on others the name of the nearest
// earlier asset with the same extension.
func planNames(p *plan, cfg Config) {
	r := newRNG(cfg.Seed, streamNames)
	n := len(p.assets)
	for i := range p.assets {
		a := &p.assets[i]
		if a.class == kindScreenshot {
			a.original = fmt.Sprintf("Screenshot %04d.%s", i+1, a.f.ext)
		} else {
			a.original = fmt.Sprintf("IMG_%04d.%s", i+1, a.f.ext)
		}
	}
	nonASCII := make([]bool, n)
	for j, i := range r.perm(n)[:countOf(cfg.NonASCIINames, n)] {
		nonASCII[i] = true
		p.assets[i].original = namePrefixes[j%len(namePrefixes)] + "_" + p.assets[i].original
	}
	var candidates []int
	seen := map[string]bool{}
	for i, a := range p.assets {
		if seen[a.f.ext] && !nonASCII[i] {
			candidates = append(candidates, i)
		}
		seen[a.f.ext] = true
	}
	duplicate := make([]bool, n)
	order := r.perm(len(candidates))
	for _, k := range order[:min(countOf(cfg.DuplicateNames, n), len(candidates))] {
		duplicate[candidates[k]] = true
	}
	last := map[string]int{}
	for i := range p.assets {
		a := &p.assets[i]
		if duplicate[i] {
			a.original = p.assets[last[a.f.ext]].original
		}
		last[a.f.ext] = i
	}
}

// planDates sets the creation, added, modification, trash and adjustment dates.
func planDates(p *plan, cfg Config) {
	r := newRNG(cfg.Seed, streamDates)
	n := len(p.assets)
	u := make([][5]uint64, n)
	for i := range u {
		for k := range u[i] {
			u[i][k] = r.src.Uint64()
		}
	}
	zero := make([]bool, n)
	for j, i := range r.perm(n)[:countOf(cfg.ZeroDates, n)] {
		zero[i] = true
		p.assets[i].created = 0 // the Core Data epoch
		if j%2 == 1 {
			p.assets[i].created = unixEpoch
		}
	}
	var candidates []int
	for i := range p.assets {
		if !zero[i] {
			p.assets[i].created = between(createdFrom, createdTo, u[i][0])
			if i > 0 && !zero[i-1] {
				candidates = append(candidates, i)
			}
		}
	}
	same := make([]bool, n)
	order := r.perm(len(candidates))
	for _, k := range order[:min(countOf(cfg.SameSecond, n), len(candidates))] {
		same[candidates[k]] = true
	}
	for i := range p.assets {
		a := &p.assets[i]
		if same[i] {
			a.created = p.assets[i-1].created
		}
		lo, hi := a.created, min(a.created+maxAddDelay, dateEnd)
		if zero[i] {
			lo, hi = addedFromZero, dateEnd
		}
		a.added = between(lo, hi, u[i][1])
		a.modified = between(a.added, dateEnd, u[i][2])
		a.trashedAt = between(a.added, dateEnd, u[i][3])
		a.adjustedAt = between(a.added, dateEnd, u[i][4])
	}
}

// planAlbums builds the root folder, the folders, the albums and the
// membership.
func planAlbums(p *plan, cfg Config, c counts) {
	r := newRNG(cfg.Seed, streamAlbums)
	created := func() float64 { return between(addedFromZero, dateEnd, r.src.Uint64()) } // synthetic
	guids := func() (string, string) {
		uuid, other, u := r.uuid(), r.uuid(), r.float()
		if u < albumGUIDShare {
			return uuid, uuid
		}
		return uuid, other
	}
	root := album{kind: 3999, parent: -1, uuid: r.uuid(), cloudGUID: r.uuid(), created: created()}
	p.albums = []album{root}
	n := cfg.Assets
	if n == 0 {
		return
	}
	albums := max(4, ceilDiv(n, cfg.AssetsPerAlbum))
	folders := max(1, ceilDiv(albums, cfg.AlbumsPerFolder))
	for f := range folders {
		a := album{kind: 4000, parent: 0, created: created()}
		a.uuid, a.cloudGUID = guids()
		year, word := 2010+r.intn(16), titleWords[f%len(titleWords)]
		a.title = fmt.Sprintf("%d/%s %d", year, word, f+1)
		if f == folders-1 {
			a.title = fmt.Sprintf("%s %d", word, f+1) // one folder without "/"
		}
		p.albums = append(p.albums, a)
	}
	inRoot := r.pick(albums, countOf(rootAlbumsShare, albums))
	for j := range albums {
		a := album{kind: 2, created: created()}
		a.uuid, a.cloudGUID = guids()
		year, month, day, folder := 2010+r.intn(16), 1+r.intn(12), 1+r.intn(28), r.intn(folders)
		a.parent = 1 + folder
		if inRoot[j] {
			a.parent = 0
		}
		word := titleWords[j%len(titleWords)]
		a.title = fmt.Sprintf("%d/%02d-%02d - %s %d", year, month, day, word, j+1)
		if j == albums-1 {
			a.title = fmt.Sprintf("%s %d", word, j+1) // one album without "/"
		}
		p.albums = append(p.albums, a)
	}
	planMembers(p, cfg, c, p.albums[1+folders:])
}

// planMembers puts the visible assets into albums: album 0 stays empty,
// album 1 gets one asset, the others are chosen with a bias to the first ones
// (a few large albums, many small).
func planMembers(p *plan, cfg Config, c counts, albums []album) {
	r := newRNG(cfg.Seed, streamMembers)
	var visible []int
	for i, a := range p.assets {
		if a.visibility == 0 {
			visible = append(visible, i)
		}
	}
	order := r.perm(len(visible))[c.outside-c.invisible:]
	biased := func() int {
		u := r.float()
		return 2 + int(float64(len(albums)-2)*u*u)
	}
	albums[1].members = append(albums[1].members, visible[order[0]])
	for k, o := range order[1:] {
		i := visible[o]
		if k >= c.several {
			j := biased()
			albums[j].members = append(albums[j].members, i)
			continue
		}
		var chosen []int
		for want := min(2+r.intn(2), len(albums)-2); len(chosen) < want; {
			if j := biased(); !slices.Contains(chosen, j) {
				chosen = append(chosen, j)
			}
		}
		for _, j := range chosen {
			albums[j].members = append(albums[j].members, i)
		}
	}
}
