package template

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// A Caser keeps state, and a Renderer is shared by goroutines: one Caser per call.
func pyLower(s string) string { return cases.Lower(language.Und).String(s) }
func pyUpper(s string) string { return cases.Upper(language.Und).String(s) }

func pyCapitalize(s string) string {
	return capitalize(cases.Title(language.Und), cases.Lower(language.Und), s)
}

// str.capitalize of Python 3.8 and later. The rest is cut from the lower case
// of the whole string, where a final sigma after the first letter sees it.
func capitalize(title, lower cases.Caser, s string) string {
	_, n := utf8.DecodeRuneInString(s)
	return title.String(s[:n]) + strings.TrimPrefix(lower.String(s), lower.String(s[:n]))
}

// str.title (phototemplate.py:1088-1089); UPSTREAM.md has its difference at a final sigma.
func pyTitle(s string) string {
	title, lower := cases.Title(language.Und), cases.Lower(language.Und)
	var b strings.Builder
	start, cased := 0, false
	for i, r := range s {
		if i > start && !cased {
			b.WriteString(capitalize(title, lower, s[start:i]))
			start = i
		}
		cased = isCased(r)
	}
	return b.String() + capitalize(title, lower, s[start:])
}

// The Cased property of Unicode, which str.title reads, takes Other_Lowercase
// and Other_Uppercase too, such as U+00AA.
func isCased(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// shlex.quote; its \w is ASCII (re.ASCII).
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "%+,-./0123456789:=@ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
