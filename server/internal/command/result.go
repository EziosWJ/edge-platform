package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

const (
	maxResultErrorType    = 100
	maxResultErrorMessage = 4096
)

// DecodeResult decodes only the Collector command-result data object. The
// outer MQTT envelope and topic identity are owned by the MQTT parser; this
// function intentionally keeps arbitrary result JSON as raw bytes.
func DecodeResult(data []byte) (ResultObservation, error) {
	if len(data) == 0 || len(data) > MaxPayloadBytes || !json.Valid(data) {
		return ResultObservation{}, ErrInvalidResult
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return ResultObservation{}, ErrInvalidResult
	}
	commandID, err := requiredString(fields, "commandId")
	if err != nil || !validCommandID(commandID) {
		return ResultObservation{}, ErrInvalidResult
	}
	name, err := requiredString(fields, "name")
	if err != nil || strings.TrimSpace(name) == "" || len(name) > 256 {
		return ResultObservation{}, ErrInvalidResult
	}
	status, err := requiredString(fields, "status")
	if err != nil || !validResultStatus(status) {
		return ResultObservation{}, ErrInvalidResult
	}
	receivedAt, err := requiredInstant(fields, "receivedAt")
	if err != nil {
		return ResultObservation{}, ErrInvalidResult
	}
	startedAt, err := nullableInstantField(fields, "startedAt")
	if err != nil {
		return ResultObservation{}, ErrInvalidResult
	}
	completedAt, err := nullableInstantField(fields, "completedAt")
	if err != nil {
		return ResultObservation{}, ErrInvalidResult
	}
	result, ok := fields["result"]
	if !ok || len(result) == 0 || !json.Valid(result) {
		return ResultObservation{}, ErrInvalidResult
	}
	errorType, errorMessage, err := decodeResultError(fields)
	if err != nil {
		return ResultObservation{}, ErrInvalidResult
	}
	return ResultObservation{
		CommandID:      commandID,
		Name:           name,
		Status:         status,
		EdgeReceivedAt: receivedAt,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		Result:         JSONDocument(string(result)),
		ErrorType:      errorType,
		ErrorMessage:   errorMessage,
	}, nil
}

func requiredString(fields map[string]json.RawMessage, key string) (string, error) {
	raw, ok := fields[key]
	if !ok || len(raw) == 0 {
		return "", ErrInvalidResult
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" {
		return "", ErrInvalidResult
	}
	return value, nil
}

func requiredInstant(fields map[string]json.RawMessage, key string) (time.Time, error) {
	value, err := requiredString(fields, key)
	if err != nil {
		return time.Time{}, err
	}
	instant, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return instant.UTC(), nil
}

func nullableInstantField(fields map[string]json.RawMessage, key string) (*time.Time, error) {
	raw, ok := fields[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !ok {
			return nil, ErrInvalidResult
		}
		return nil, nil
	}
	instant, err := requiredInstant(fields, key)
	if err != nil {
		return nil, err
	}
	return &instant, nil
}

func decodeResultError(fields map[string]json.RawMessage) (*string, *string, error) {
	raw, ok := fields["error"]
	if !ok {
		return nil, nil, ErrInvalidResult
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil, nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, nil, ErrInvalidResult
	}
	errorType, err := requiredString(value, "type")
	if err != nil || len(errorType) > maxResultErrorType {
		return nil, nil, ErrInvalidResult
	}
	errorMessage, err := requiredString(value, "message")
	if err != nil || len(errorMessage) > maxResultErrorMessage {
		return nil, nil, ErrInvalidResult
	}
	return &errorType, &errorMessage, nil
}
