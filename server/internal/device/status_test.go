package device

import (
	"testing"
	"time"
)

func TestParseSnapshotRequiresCompleteTypedFieldsAndAllowsExtensions(t *testing.T) {
	snapshot, err := ParseSnapshot([]byte(`{
        "status":"DEGRADED",
        "lastAttemptAt":"2026-09-21T02:03:04.123456789+08:00",
        "lastSuccessAt":null,
        "error":"partial read",
        "extension":{"attempt":3}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CommunicationStatus != StatusDegraded || snapshot.LastSuccessAt != nil || snapshot.CommunicationError == nil || *snapshot.CommunicationError != "partial read" {
		t.Fatalf("parsed snapshot = %+v", snapshot)
	}
	wantAttempt := time.Date(2026, 9, 20, 18, 3, 4, 123456789, time.UTC)
	if snapshot.LastAttemptAt == nil || !snapshot.LastAttemptAt.Equal(wantAttempt) {
		t.Fatalf("lastAttemptAt = %v, want UTC %v", snapshot.LastAttemptAt, wantAttempt)
	}

	for _, payload := range []string{
		`{}`,
		`{"status":"UNKNOWN","lastAttemptAt":null,"lastSuccessAt":null,"error":null}`,
		`{"status":"ONLINE","lastAttemptAt":"bad","lastSuccessAt":null,"error":null}`,
		`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":1,"error":null}`,
		`{"status":"ONLINE","lastAttemptAt":null,"lastSuccessAt":null,"error":1}`,
		`{"status":"ONLINE","lastAttemptAt":null,"error":null}`,
	} {
		if _, err := ParseSnapshot([]byte(payload)); err != ErrInvalid {
			t.Errorf("ParseSnapshot(%s) error = %v, want ErrInvalid", payload, err)
		}
	}
	emptyError, err := ParseSnapshot([]byte(`{"status":"INITIAL","lastAttemptAt":null,"lastSuccessAt":null,"error":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if emptyError.CommunicationError != nil {
		t.Fatalf("empty error = %v, want nil", *emptyError.CommunicationError)
	}
}
