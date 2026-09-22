package datapoint

import (
	"bytes"
	"encoding/json"
	"time"
)

// ParseRawSnapshot validates the complete raw data object before projection.
// Unknown extension fields are allowed, but malformed known fields invalidate
// the whole snapshot so no point can be partially updated.
func ParseRawSnapshot(data []byte) (RawSnapshot, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return RawSnapshot{}, ErrInvalid
	}
	var status string
	if !requiredString(fields, "communicationStatus", &status) || !validCommunicationStatus(status) {
		return RawSnapshot{}, ErrInvalid
	}
	blocksRaw, ok := fields["blocks"]
	if !ok || bytes.Equal(blocksRaw, []byte("null")) {
		return RawSnapshot{}, ErrInvalid
	}
	var blocks []json.RawMessage
	if json.Unmarshal(blocksRaw, &blocks) != nil {
		return RawSnapshot{}, ErrInvalid
	}
	result := RawSnapshot{CommunicationStatus: status, Blocks: make([]RawBlock, 0, len(blocks))}
	seen := map[[2]int]struct{}{}
	for _, blockRaw := range blocks {
		var f map[string]json.RawMessage
		if json.Unmarshal(blockRaw, &f) != nil || f == nil {
			return RawSnapshot{}, ErrInvalid
		}
		var fc int
		if !requiredInt(f, "functionCode", &fc) || (fc != 3 && fc != 4) {
			return RawSnapshot{}, ErrInvalid
		}
		var valid bool
		if !requiredBool(f, "valid", &valid) {
			return RawSnapshot{}, ErrInvalid
		}
		success, ok := nullableTime(f, "lastSuccessAt")
		if !ok {
			return RawSnapshot{}, ErrInvalid
		}
		if _, ok := nullableTime(f, "lastAttemptAt"); !ok {
			return RawSnapshot{}, ErrInvalid
		}
		registersRaw, ok := f["registers"]
		if !ok || bytes.Equal(registersRaw, []byte("null")) {
			return RawSnapshot{}, ErrInvalid
		}
		var registers []json.RawMessage
		if json.Unmarshal(registersRaw, &registers) != nil {
			return RawSnapshot{}, ErrInvalid
		}
		block := RawBlock{FunctionCode: fc, Valid: valid, LastSuccessAt: success, Registers: make([]RawRegister, 0, len(registers))}
		for _, registerRaw := range registers {
			var rf map[string]json.RawMessage
			if json.Unmarshal(registerRaw, &rf) != nil || rf == nil {
				return RawSnapshot{}, ErrInvalid
			}
			address, ok := integer(rf, "address")
			if !ok || address < 0 || address > 65535 {
				return RawSnapshot{}, ErrInvalid
			}
			key := [2]int{fc, address}
			if _, exists := seen[key]; exists {
				return RawSnapshot{}, ErrInvalid
			}
			seen[key] = struct{}{}
			value, ok := nullableUint16(rf, "value")
			if !ok {
				return RawSnapshot{}, ErrInvalid
			}
			block.Registers = append(block.Registers, RawRegister{Address: &address, Value: value})
		}
		result.Blocks = append(result.Blocks, block)
	}
	return result, nil
}

func validCommunicationStatus(value string) bool {
	switch value {
	case "INITIAL", "ONLINE", "DEGRADED", "OFFLINE":
		return true
	}
	return false
}
func requiredString(f map[string]json.RawMessage, key string, out *string) bool {
	raw, ok := f[key]
	return ok && !bytes.Equal(raw, []byte("null")) && json.Unmarshal(raw, out) == nil
}
func requiredBool(f map[string]json.RawMessage, key string, out *bool) bool {
	raw, ok := f[key]
	return ok && !bytes.Equal(raw, []byte("null")) && json.Unmarshal(raw, out) == nil
}
func requiredInt(f map[string]json.RawMessage, key string, out *int) bool {
	raw, ok := f[key]
	return ok && json.Unmarshal(raw, out) == nil
}
func integer(f map[string]json.RawMessage, key string) (int, bool) {
	var n int
	raw, ok := f[key]
	if !ok || json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	return n, true
}
func nullableTime(f map[string]json.RawMessage, key string) (*time.Time, bool) {
	raw, ok := f[key]
	if !ok {
		return nil, false
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return nil, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, false
	}
	t = t.UTC()
	return &t, true
}
func nullableUint16(f map[string]json.RawMessage, key string) (*uint16, bool) {
	raw, ok := f[key]
	if !ok {
		return nil, false
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil || n < 0 || n > 65535 {
		return nil, false
	}
	v := uint16(n)
	return &v, true
}
