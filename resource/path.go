package resource

import (
	"errors"
	"strings"
)

var ErrBadPath = errors.New("scimply: invalid attribute path")

type AttributePath struct {
	Schema string

	AttributeName string

	SubAttribute string
}

func (p AttributePath) String() string {
	var b strings.Builder
	if p.Schema != "" {
		b.WriteString(p.Schema)
		b.WriteByte(':')
	}
	b.WriteString(p.AttributeName)
	if p.SubAttribute != "" {
		b.WriteByte('.')
		b.WriteString(p.SubAttribute)
	}
	return b.String()
}

func ParsePath(s string) (AttributePath, error) {
	if s == "" {
		return AttributePath{}, ErrBadPath
	}

	if s[0] == '.' {
		return AttributePath{}, ErrBadPath
	}

	var schema, remainder string
	if len(s) >= 4 && strings.EqualFold(s[:4], "urn:") {
		lastColon := strings.LastIndex(s, ":")
		if lastColon != -1 {
			candidate := s[lastColon+1:]
			attrPart := candidate
			if dot := strings.Index(candidate, "."); dot != -1 {
				attrPart = candidate[:dot]
			}
			if isValidAttrName(attrPart) {
				schema = s[:lastColon]
				remainder = candidate
			} else {
				remainder = s
			}
		} else {
			remainder = s
		}
	} else {
		remainder = s
	}

	var attrName, subAttr string
	if dot := strings.Index(remainder, "."); dot != -1 {
		attrName = remainder[:dot]
		subAttr = remainder[dot+1:]
		if attrName == "" || subAttr == "" {
			return AttributePath{}, ErrBadPath
		}
		if !isValidAttrName(attrName) {
			return AttributePath{}, ErrBadPath
		}
		if !isValidAttrName(subAttr) {
			return AttributePath{}, ErrBadPath
		}
	} else {
		attrName = remainder
		if !isValidAttrName(attrName) {
			return AttributePath{}, ErrBadPath
		}
	}

	return AttributePath{
		Schema:        schema,
		AttributeName: attrName,
		SubAttribute:  subAttr,
	}, nil
}

func isValidAttrName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, ch := range name {
		if i == 0 {
			if !isAlpha(ch) {
				return false
			}
		} else {
			if !isNameChar(ch) {
				return false
			}
		}
	}
	return true
}

func isAlpha(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isNameChar(ch rune) bool {
	return isAlpha(ch) || isDigit(ch) || ch == '-' || ch == '_'
}

type PatchPath struct {
	Attribute    AttributePath
	ValueFilter  FilterExpression
	SubAttribute string
}

func ParsePatchPath(s string) (*PatchPath, error) {
	if s == "" {
		return nil, ErrBadPath
	}

	bracketOpen := strings.Index(s, "[")
	if bracketOpen == -1 {

		p, err := ParsePath(s)
		if err != nil {
			return nil, err
		}
		return &PatchPath{Attribute: p}, nil
	}

	attrStr := s[:bracketOpen]
	attr, err := ParsePath(attrStr)
	if err != nil {
		return nil, err
	}

	bracketClose := strings.LastIndex(s, "]")
	if bracketClose == -1 || bracketClose < bracketOpen {
		return nil, ErrBadPath
	}

	filterStr := s[bracketOpen+1 : bracketClose]
	filter, err := ParseFilter(filterStr)
	if err != nil {
		return nil, err
	}

	rest := s[bracketClose+1:]
	var subAttr string
	if rest != "" {
		if !strings.HasPrefix(rest, ".") {
			return nil, ErrBadPath
		}
		subAttr = rest[1:]
		if !isValidAttrName(subAttr) {
			return nil, ErrBadPath
		}
	}

	return &PatchPath{
		Attribute:    attr,
		ValueFilter:  filter,
		SubAttribute: subAttr,
	}, nil
}
