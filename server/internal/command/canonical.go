package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
)

// canonicalizeArgs sorts object keys at every depth, preserves array order,
// and writes json.Number lexemes directly. This intentionally differs from a
// map[string]any/json.Marshal round trip, which would lose number precision.
func canonicalizeArgs(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := canonicalValue(decoder)
	if err != nil {
		return nil, err
	}
	if value.kind != '{' {
		return nil, errors.New("args must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("args must contain one JSON value")
		}
		return nil, err
	}
	return value.bytes, nil
}

type canonicalJSON struct {
	kind  byte
	bytes []byte
}

func canonicalValue(decoder *json.Decoder) (canonicalJSON, error) {
	token, err := decoder.Token()
	if err != nil {
		return canonicalJSON{}, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			type member struct {
				key   string
				value []byte
			}
			members := make([]member, 0)
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return canonicalJSON{}, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return canonicalJSON{}, errors.New("object key must be a string")
				}
				if _, exists := seen[key]; exists {
					return canonicalJSON{}, errors.New("duplicate object key")
				}
				seen[key] = struct{}{}
				child, err := canonicalValue(decoder)
				if err != nil {
					return canonicalJSON{}, err
				}
				members = append(members, member{key: key, value: child.bytes})
			}
			if _, err := decoder.Token(); err != nil {
				return canonicalJSON{}, err
			}
			sort.Slice(members, func(i, j int) bool { return members[i].key < members[j].key })
			var result bytes.Buffer
			result.WriteByte('{')
			for i, member := range members {
				if i > 0 {
					result.WriteByte(',')
				}
				key, _ := json.Marshal(member.key)
				result.Write(key)
				result.WriteByte(':')
				result.Write(member.value)
			}
			result.WriteByte('}')
			return canonicalJSON{kind: '{', bytes: result.Bytes()}, nil
		case '[':
			var values [][]byte
			for decoder.More() {
				child, err := canonicalValue(decoder)
				if err != nil {
					return canonicalJSON{}, err
				}
				values = append(values, child.bytes)
			}
			if _, err := decoder.Token(); err != nil {
				return canonicalJSON{}, err
			}
			var result bytes.Buffer
			result.WriteByte('[')
			for i, value := range values {
				if i > 0 {
					result.WriteByte(',')
				}
				result.Write(value)
			}
			result.WriteByte(']')
			return canonicalJSON{kind: '[', bytes: result.Bytes()}, nil
		default:
			return canonicalJSON{}, errors.New("invalid JSON delimiter")
		}
	case string:
		encoded, _ := json.Marshal(value)
		return canonicalJSON{kind: 's', bytes: encoded}, nil
	case json.Number:
		return canonicalJSON{kind: 'n', bytes: []byte(value.String())}, nil
	case bool:
		if value {
			return canonicalJSON{kind: 'b', bytes: []byte("true")}, nil
		}
		return canonicalJSON{kind: 'b', bytes: []byte("false")}, nil
	case nil:
		return canonicalJSON{kind: '0', bytes: []byte("null")}, nil
	default:
		return canonicalJSON{}, errors.New("unsupported JSON value")
	}
}
