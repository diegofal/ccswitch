package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Claude Code's files carry members we do not know (MCP logins, future keys)
// and Claude Code rewrites them itself. Re-encoding a document would reorder
// keys and re-escape strings, so patches splice the new values into the
// original bytes and leave everything else exactly as it was.

// member is one top-level member of a JSON object, as byte offsets into it.
type member struct {
	key                        string
	keyStart, valStart, valEnd int
}

// object is the layout of a JSON object document.
type object struct {
	open, close int // offsets of '{' and '}'
	members     []member
}

func scanObject(doc []byte) (object, error) {
	if !json.Valid(doc) {
		return object{}, errors.New("not valid JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(doc))
	tok, err := dec.Token()
	if err != nil {
		return object{}, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return object{}, errors.New("top level is not a JSON object")
	}
	obj := object{open: int(dec.InputOffset()) - 1}
	for dec.More() {
		keyStart := skipSeparators(doc, int(dec.InputOffset()))
		tok, err := dec.Token()
		if err != nil {
			return object{}, err
		}
		key, _ := tok.(string) // json.Valid guarantees object keys are strings
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return object{}, err
		}
		end := int(dec.InputOffset())
		obj.members = append(obj.members, member{key: key, keyStart: keyStart, valStart: end - len(raw), valEnd: end})
	}
	if _, err := dec.Token(); err != nil {
		return object{}, err
	}
	obj.close = int(dec.InputOffset()) - 1
	return obj, nil
}

func skipSeparators(doc []byte, i int) int {
	for i < len(doc) && strings.IndexByte(" \t\r\n,", doc[i]) >= 0 {
		i++
	}
	return i
}

// style is how an object lays out its members: what goes before each key
// ("\n  " when indented, "" when compact) and between key and value.
type style struct{ before, colon string }

func (o object) style(doc []byte) style {
	if len(o.members) == 0 {
		return style{before: "\n  ", colon: ": "}
	}
	m := o.members[0]
	head := doc[m.keyStart:m.valStart] // `"key"` + spaces + ':' + spaces
	colon := bytes.LastIndexByte(head, ':')
	quote := bytes.LastIndexByte(head[:colon], '"')
	return style{before: string(doc[o.open+1 : m.keyStart]), colon: string(head[quote+1:])}
}

// format lays out a value for this style: indented two spaces per level under
// the key's own indentation (as Claude Code writes its files), or compact.
func (s style) format(v []byte) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, v); err != nil {
		return nil, err
	}
	nl := strings.LastIndexByte(s.before, '\n')
	if nl < 0 {
		return compact.Bytes(), nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), s.before[nl+1:], "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// field is a member to set: its key and its new raw JSON value.
type field struct {
	key string
	val []byte
}

// setMembers returns doc with every member named in fields set to its value.
// Members present (all occurrences) get the new value in place; absent ones
// are appended in the order given. Every other byte of doc is kept.
func setMembers(doc []byte, fields []field) ([]byte, error) {
	obj, err := scanObject(doc)
	if err != nil {
		return nil, err
	}
	st := obj.style(doc)
	vals := make(map[string][]byte, len(fields))
	for _, f := range fields {
		v, err := st.format(f.val)
		if err != nil {
			return nil, fmt.Errorf("value of %s: %w", f.key, err)
		}
		vals[f.key] = v
	}

	var out bytes.Buffer
	pos := 0
	seen := map[string]bool{}
	for _, m := range obj.members {
		v, ok := vals[m.key]
		if !ok {
			continue
		}
		seen[m.key] = true
		out.Write(doc[pos:m.valStart])
		out.Write(v)
		pos = m.valEnd
	}

	var appended bytes.Buffer
	for _, f := range fields {
		if seen[f.key] {
			continue
		}
		if appended.Len() > 0 || len(obj.members) > 0 {
			appended.WriteByte(',')
		}
		key, _ := json.Marshal(f.key) // a Go string always marshals
		appended.WriteString(st.before)
		appended.Write(key)
		appended.WriteString(st.colon)
		appended.Write(vals[f.key])
	}
	if appended.Len() == 0 {
		out.Write(doc[pos:])
		return out.Bytes(), nil
	}
	if len(obj.members) == 0 {
		out.Write(doc[pos : obj.open+1])
		out.Write(appended.Bytes())
		out.WriteString(closingIndent(st.before))
		out.Write(doc[obj.close:])
		return out.Bytes(), nil
	}
	last := obj.members[len(obj.members)-1].valEnd
	out.Write(doc[pos:last])
	out.Write(appended.Bytes())
	out.Write(doc[last:])
	return out.Bytes(), nil
}

// closingIndent is what goes before '}' of an object whose members are
// preceded by before: one indent level less.
func closingIndent(before string) string {
	return strings.TrimSuffix(before, "  ")
}

// isObject reports whether v is a JSON object.
func isObject(v json.RawMessage) bool {
	v = bytes.TrimSpace(v)
	return len(v) > 0 && v[0] == '{' && json.Valid(v)
}
