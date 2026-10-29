package template

import "errors"

// ErrSyntax is wrapped by every error of Parse; the error names the byte
// offset and what was expected there.
var ErrSyntax = errors.New("template: syntax error")
