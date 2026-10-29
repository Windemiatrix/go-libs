package template

import "errors"

// ErrSyntax is wrapped by every error of Parse; the error names the byte
// offset and what was expected there.
var ErrSyntax = errors.New("template: syntax error")

// The errors of Render; each names the field or the construct.
var (
	// ErrUnsupportedField is a field that upstream has but the Photo does not
	// give, or one that runs code or a program.
	ErrUnsupportedField = errors.New("template: field not supported")
	// ErrUnsupportedConstruct is a part of the language not rendered yet.
	ErrUnsupportedConstruct = errors.New("template: construct not supported yet")
	// ErrTooManyValues is a render of more than RenderOptions.MaxResults strings.
	ErrTooManyValues = errors.New("template: too many values")
)
