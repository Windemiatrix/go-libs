package uti

import "strings"

// PreferredExtension returns the preferred extension of a UTI from the table
// of uti.py (the third column; uti.py:521-526, 576-580, 611-614), compared in
// lower case, without the leading "."; "" for a UTI the table does not have.
// The CoreServices path of osxphotos (594-609) is not ported.
func PreferredExtension(uti string) string {
	return extByUTI[strings.ToLower(uti)]
}
