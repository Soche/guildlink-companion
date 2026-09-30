// Package luasv parses World of Warcraft SavedVariables files: a sequence of
// `Name = value` assignments where values are Lua literals and tables.
package luasv

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Table is a Lua table. Keys are normalised to strings: string keys as-is,
// numeric keys (explicit or positional) in their shortest decimal form, so
// `[171] = x` and the 171st array entry both become "171".
type Table map[string]any

// Values are string, float64, bool, nil or Table.

// Parse returns the global assignments in a SavedVariables file.
func Parse(src string) (map[string]any, error) {
	p := &parser{src: src}
	out := map[string]any{}
	for {
		p.skipSpace()
		if p.eof() {
			return out, nil
		}
		name, err := p.ident()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if !p.consume('=') {
			return nil, p.errorf("expected '=' after %s", name)
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out[name] = v
		p.skipSpace()
		p.consume(';')
	}
}

type parser struct {
	src string
	pos int
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) consume(c byte) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

func (p *parser) errorf(format string, args ...any) error {
	line := 1 + strings.Count(p.src[:min(p.pos, len(p.src))], "\n")
	return fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...))
}

func (p *parser) skipSpace() {
	for !p.eof() {
		switch c := p.peek(); {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			p.pos++
		case strings.HasPrefix(p.src[p.pos:], "--"):
			p.pos += 2
			if level, ok := p.longBracketOpen(); ok {
				p.skipLongBracket(level)
			} else {
				for !p.eof() && p.peek() != '\n' {
					p.pos++
				}
			}
		default:
			return
		}
	}
}

// longBracketOpen consumes "[[" or "[==[" and returns the number of '='.
func (p *parser) longBracketOpen() (int, bool) {
	if p.peek() != '[' {
		return 0, false
	}
	i := p.pos + 1
	for i < len(p.src) && p.src[i] == '=' {
		i++
	}
	if i < len(p.src) && p.src[i] == '[' {
		level := i - p.pos - 1
		p.pos = i + 1
		return level, true
	}
	return 0, false
}

func (p *parser) skipLongBracket(level int) string {
	closer := "]" + strings.Repeat("=", level) + "]"
	end := strings.Index(p.src[p.pos:], closer)
	if end < 0 {
		s := p.src[p.pos:]
		p.pos = len(p.src)
		return s
	}
	s := p.src[p.pos : p.pos+end]
	p.pos += end + len(closer)
	return s
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

func (p *parser) ident() (string, error) {
	start := p.pos
	if !isIdentStart(p.peek()) {
		return "", p.errorf("expected a name, found %q", p.peek())
	}
	for !p.eof() && isIdentChar(p.peek()) {
		p.pos++
	}
	return p.src[start:p.pos], nil
}

func (p *parser) value() (any, error) {
	p.skipSpace()
	c := p.peek()
	switch {
	case c == '{':
		return p.table()
	case c == '"' || c == '\'':
		return p.quoted()
	case c == '[':
		if level, ok := p.longBracketOpen(); ok {
			s := p.skipLongBracket(level)
			return strings.TrimPrefix(s, "\n"), nil
		}
	case c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.number()
	case isIdentStart(c):
		word, _ := p.ident()
		switch word {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		case "inf", "math.huge":
			return math.Inf(1), nil
		}
		return nil, p.errorf("unexpected %q", word)
	}
	return nil, p.errorf("unexpected character %q", c)
}

func (p *parser) number() (float64, error) {
	start := p.pos
	p.consume('-')
	if strings.HasPrefix(strings.ToLower(p.src[p.pos:]), "0x") {
		p.pos += 2
		for !p.eof() && strings.IndexByte("0123456789abcdefABCDEF", p.peek()) >= 0 {
			p.pos++
		}
		n, err := strconv.ParseInt(p.src[start:p.pos], 0, 64)
		if err != nil {
			return 0, p.errorf("bad number %q", p.src[start:p.pos])
		}
		return float64(n), nil
	}
	for !p.eof() {
		c := p.peek()
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' ||
			((c == '-' || c == '+') && (p.src[p.pos-1] == 'e' || p.src[p.pos-1] == 'E')) {
			p.pos++
			continue
		}
		break
	}
	text := p.src[start:p.pos]
	switch text {
	case "-":
		if isIdentStart(p.peek()) {
			if w, _ := p.ident(); w == "inf" || w == "nan" {
				return math.Inf(-1), nil
			}
		}
		return 0, p.errorf("bad number")
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, p.errorf("bad number %q", text)
	}
	return n, nil
}

func (p *parser) quoted() (string, error) {
	quote := p.src[p.pos]
	p.pos++
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errorf("unterminated string")
		}
		c := p.src[p.pos]
		p.pos++
		switch {
		case c == quote:
			return b.String(), nil
		case c == '\\':
			if p.eof() {
				return "", p.errorf("unterminated string")
			}
			e := p.src[p.pos]
			p.pos++
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'a':
				b.WriteByte('\a')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'v':
				b.WriteByte('\v')
			case '\n':
				b.WriteByte('\n')
			default:
				if e >= '0' && e <= '9' {
					// \ddd: up to three decimal digits, one byte.
					n := int(e - '0')
					for i := 0; i < 2 && p.peek() >= '0' && p.peek() <= '9'; i++ {
						n = n*10 + int(p.src[p.pos]-'0')
						p.pos++
					}
					if n > 255 {
						return "", p.errorf("bad escape \\%d", n)
					}
					b.WriteByte(byte(n))
				} else {
					b.WriteByte(e) // \\ \" \' and anything else literal
				}
			}
		default:
			b.WriteByte(c)
		}
	}
}

func formatKey(v any) (string, bool) {
	switch k := v.(type) {
	case string:
		return k, true
	case float64:
		return strconv.FormatFloat(k, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(k), true
	}
	return "", false
}

func (p *parser) table() (Table, error) {
	p.pos++ // {
	t := Table{}
	next := 1
	for {
		p.skipSpace()
		if p.consume('}') {
			return t, nil
		}
		var key any
		switch {
		case p.peek() == '[' && !strings.HasPrefix(p.src[p.pos:], "[[") && !strings.HasPrefix(p.src[p.pos:], "[="):
			p.pos++
			k, err := p.value()
			if err != nil {
				return nil, err
			}
			p.skipSpace()
			if !p.consume(']') {
				return nil, p.errorf("expected ']'")
			}
			p.skipSpace()
			if !p.consume('=') {
				return nil, p.errorf("expected '='")
			}
			key = k
		case isIdentStart(p.peek()):
			// Either `name = value` or a bare true/false/nil value.
			save := p.pos
			name, _ := p.ident()
			p.skipSpace()
			if p.consume('=') {
				key = name
			} else {
				p.pos = save
			}
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if key == nil {
			key = float64(next)
			next++
		}
		if ks, ok := formatKey(key); ok && v != nil {
			t[ks] = v
		}
		p.skipSpace()
		if !p.consume(',') && !p.consume(';') {
			p.skipSpace()
			if !p.consume('}') {
				return nil, p.errorf("expected ',' or '}'")
			}
			return t, nil
		}
	}
}

// String returns t[key] if it is a string.
func (t Table) String(key string) (string, bool) {
	s, ok := t[key].(string)
	return s, ok
}

// Int returns t[key] if it is a number with an integral value.
func (t Table) Int(key string) (int64, bool) {
	f, ok := t[key].(float64)
	if !ok || f != math.Trunc(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return int64(f), true
}

// Table returns t[key] if it is a table.
func (t Table) Table(key string) (Table, bool) {
	v, ok := t[key].(Table)
	return v, ok
}
