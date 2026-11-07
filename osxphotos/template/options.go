package template

import (
	"sync"
	"time"
)

// DefaultMaxResults bounds the product of multi-valued fields
// (phototemplate.py:774-783), whose values come from the library: a template
// is not a way to exhaust memory.
const DefaultMaxResults = 10000

// RenderOptions are RenderOptions of upstream (phototemplate.py:366-400); the
// zero value is RenderOptions() of upstream, and an empty NoneStr, InplaceSep
// or PathSep is its default: "_", "," and "/" (upstream os.path.sep).
type RenderOptions struct {
	NoneStr       string // for a field without a value (770-772)
	NoneEmpty     bool   // a field without a value renders as "", not as NoneStr: RenderOptions(none_str="") of upstream (photoquery.py:734), which the zero NoneStr cannot say
	ExpandInplace bool   // join the values of a field with InplaceSep, as "+" does (665-667)
	InplaceSep    string
	PathSep       string    // in folder_album without "(sep)" (1255)
	Filename      bool      // the result is a file name: SanitizeFilename of every rendered string, of nested statements too, before Strip, and SanitizeDirname of every value of a field but {%name} (377, 529-532, 1005-1009, 1039-1042, 1388-1390); ".", "..", NUL and a leading "." pass, as upstream: the caller keeps the result inside its directory
	Dirname       bool      // the result is a directory path: SanitizeDirname of every value of a field but {%name}, and of each folder and title of folder_album and folder_album_project before the join (378, 1005-1009, 1039-1042, 1296-1312, 1391-1393); with Filename too, folder_album takes both; the result may still hold "..", and the "/" of the template text, defaults, variables and NoneStr, a leading one too: the caller keeps it inside its directory
	Strip         bool      // strip every rendered string, of nested statements too (534-537)
	EditedVersion bool      // gives {edited_version} its value (946-947)
	ExportDir     string    // {export_dir}; "" renders "None", as the None of upstream (1641), and a property of it is ErrRender (1643)
	Filepath      string    // {filepath}; "" renders "None" (1641), and a property of it is ErrRender (1643)
	Quote         bool      // {export_dir}, {filepath} and their properties through shlex.quote (384, 1641, 1652), before the filters and [find,replace], which act on the quoted string and may break it (663-683)
	Today         time.Time // {today}; the zero time is time.Now() at the start of a Render (910-912)
	MaxResults    int       // the most strings of one statement; 0 is DefaultMaxResults
	DestPath      string    // the destination path of the photo (397): album_seq finds its album by the last element of it, folder_album_seq by the folders and the title at its end (979-992, 1665-1681); "" gives them no value
}

// Renderer renders templates and may be used from several goroutines at once.
// The zero Renderer is ready to use; NewRenderer returns one. It holds the
// counters of {counter} (template_counter.py:28): one Renderer is what one
// process of upstream is, so {counter} grows from photo to photo for as long
// as the Renderer lives.
type Renderer struct {
	mu       sync.Mutex
	counters map[string]*counter
}

// NewRenderer returns a Renderer.
func NewRenderer() *Renderer { return &Renderer{} }
