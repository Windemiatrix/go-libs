package template

import "time"

// DefaultMaxResults bounds the product of multi-valued fields
// (phototemplate.py:774-783), whose values come from the library: a template
// is not a way to exhaust memory.
const DefaultMaxResults = 10000

// RenderOptions are RenderOptions of upstream (phototemplate.py:366-400); the
// zero value is RenderOptions() of upstream, and an empty NoneStr, InplaceSep
// or PathSep is its default: "_", "," and "/" (upstream os.path.sep).
type RenderOptions struct {
	NoneStr       string // for a field without a value (770-772)
	ExpandInplace bool   // join the values of a field with InplaceSep, as "+" does (665-667)
	InplaceSep    string
	PathSep       string    // in folder_album without "(sep)" (1255)
	Strip         bool      // strip every rendered string, of nested statements too (534-537)
	EditedVersion bool      // gives {edited_version} its value (946-947)
	ExportDir     string    // {export_dir}; "" renders "None", as the None of upstream (1641)
	Filepath      string    // {filepath}; "" renders "None" (1641)
	Today         time.Time // {today}; the zero time is time.Now() at the start of a Render (910-912)
	MaxResults    int       // the most strings of one statement; 0 is DefaultMaxResults
}

// Renderer renders templates and may be used from several goroutines at once.
// It is the place of the state that outlives one render, the counters of
// {counter} (template_counter.py:28), once they are ported.
type Renderer struct{}

// NewRenderer returns a Renderer.
func NewRenderer() *Renderer { return &Renderer{} }
