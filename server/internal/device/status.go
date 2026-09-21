package device

import (
	"bytes"
	"encoding/json"
	"time"
)

// ParseSnapshot validates the complete domain DeviceStatus data object. It
// intentionally permits unknown extension fields while requiring every M3
// current-state field to be present with the exact supported type.
func ParseSnapshot(data []byte) (Snapshot, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return Snapshot{}, ErrInvalid
	}

	var status string
	if !decodeRequiredString(fields, "status", &status) || !validStatus(CommunicationStatus(status)) {
		return Snapshot{}, ErrInvalid
	}
	lastAttemptAt, ok := decodeNullableTime(fields, "lastAttemptAt")
	if !ok {
		return Snapshot{}, ErrInvalid
	}
	lastSuccessAt, ok := decodeNullableTime(fields, "lastSuccessAt")
	if !ok {
		return Snapshot{}, ErrInvalid
	}
	communicationError, ok := decodeNullableString(fields, "error")
	if !ok {
		return Snapshot{}, ErrInvalid
	}

	return normalizeSnapshot(Snapshot{
		CommunicationStatus: CommunicationStatus(status),
		LastAttemptAt:       lastAttemptAt,
		LastSuccessAt:       lastSuccessAt,
		CommunicationError:  communicationError,
	}), nil
}

func decodeRequiredString(fields map[string]json.RawMessage, key string, target *string) bool {
	raw, ok := fields[key]
	if !ok || bytes.Equal(raw, []byte("null")) {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

func decodeNullableTime(fields map[string]json.RawMessage, key string) (*time.Time, bool) {
	raw, ok := fields[key]
	if !ok {
		return nil, false
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return nil, false
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil, false
	}
	value = value.UTC()
	return &value, true
}

func decodeNullableString(fields map[string]json.RawMessage, key string) (*string, bool) {
	raw, ok := fields[key]
	if !ok {
		return nil, false
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	if value == "" {
		return nil, true
	}
	return &value, true
}
