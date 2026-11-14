package dodge

import (
	"crypto/sha256"
	"encoding/hex"
	"go/scanner"
	"go/token"
	"strings"
)

// tok is one scanned token's byte offset and text.
type tok struct {
	offset int
	text   string
}

// scanTokens tokenizes src once, skipping comments and the automatically
// inserted semicolons that depend on line layout, and returns each
// remaining token's byte offset and text. Callers slice the result per
// symbol instead of rescanning.
func scanTokens(src []byte) []tok {
	fs := token.NewFileSet()
	tf := fs.AddFile("", fs.Base(), len(src))

	var s scanner.Scanner
	s.Init(tf, src, nil, 0) // mode 0 skips comments

	var out []tok
	for {
		pos, t, lit := s.Scan()
		if t == token.EOF {
			break
		}
		if t == token.SEMICOLON && lit == "\n" {
			continue
		}
		text := lit
		if text == "" {
			text = t.String()
		}
		out = append(out, tok{offset: tf.Offset(pos), text: text})
	}
	return out
}

// sliceTokenText joins the text of the tokens in toks whose offset falls
// within [start, end), with single spaces.
func sliceTokenText(toks []tok, start, end int) string {
	var parts []string
	for _, t := range toks {
		if t.offset < start || t.offset >= end {
			continue
		}
		parts = append(parts, t.text)
	}
	return strings.Join(parts, " ")
}

// tokenText returns the tokens of src within [start, end), joined by one
// space.
func tokenText(src []byte, start, end int) string {
	return sliceTokenText(scanTokens(src), start, end)
}

// hashTokenText returns the hex SHA-256 of the token text in toks within
// [start, end).
func hashTokenText(toks []tok, start, end int) string {
	sum := sha256.Sum256([]byte(sliceTokenText(toks, start, end)))
	return hex.EncodeToString(sum[:])
}
