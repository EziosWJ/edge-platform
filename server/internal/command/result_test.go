package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
)

func TestDecodeResultPreservesCollectorShapeAndLosslessJSON(t *testing.T) {
	observation, err := DecodeResult([]byte(`{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"SUCCEEDED","receivedAt":"2026-09-10T04:59:00Z","startedAt":"2026-09-10T04:59:30Z","completedAt":"2026-09-10T05:00:00Z","result":{"number":90071992547409931234567890,"array":[1,"x",null]},"error":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if observation.EdgeReceivedAt.Location() != time.UTC || observation.StartedAt == nil || observation.CompletedAt == nil {
		t.Fatalf("timestamps = %+v", observation)
	}
	if string(observation.Result) != `{"number":90071992547409931234567890,"array":[1,"x",null]}` {
		t.Fatalf("result bytes = %s", observation.Result)
	}
}

func TestDecodeResultRejectsContractShape(t *testing.T) {
	base := `{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"SUCCEEDED","receivedAt":"2026-09-10T04:59:00Z","startedAt":null,"completedAt":null,"result":null,"error":null}`
	for _, payload := range []string{
		`{"commandId":"11111111-1111-4111-8111-111111111111"}`,
		`{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"BROKEN","receivedAt":"2026-09-10T04:59:00Z","startedAt":null,"completedAt":null,"result":null,"error":null}`,
		`{"commandId":"11111111-1111-4111-8111-111111111111","name":"close","status":"SUCCEEDED","receivedAt":"bad","startedAt":null,"completedAt":null,"result":null,"error":null}`,
		base[:len(base)-len(`,"error":null}`)],
	} {
		if !errors.Is(func() error { _, err := DecodeResult([]byte(payload)); return err }(), ErrInvalidResult) {
			t.Errorf("DecodeResult(%s) was accepted", payload)
		}
	}
}

func resultObservation(status string) ResultObservation {
	edgeReceived := time.Date(2026, 9, 22, 3, 0, 0, 0, time.UTC)
	cloudReceived := edgeReceived.Add(2 * time.Second)
	return ResultObservation{
		CommandID: "11111111-1111-4111-8111-111111111111", EdgeID: "edge", SourceDeviceID: "source", Name: "close", Status: status,
		EdgeReceivedAt: edgeReceived, CloudReceivedAt: cloudReceived, Result: JSONDocument(`{"ok":true,"number":90071992547409931234567890}`),
	}
}

func TestRepositoryProjectResultTransitionsAndKeepsFirstResult(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("11111111-1111-4111-8111-111111111111", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	accepted := resultObservation(CommandStatusAccepted)
	if err := repository.ProjectResult(context.Background(), accepted); err != nil {
		t.Fatal(err)
	}
	var current Command
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusAccepted || current.EdgeReceivedAt == nil || current.ResultReceivedAt == nil {
		t.Fatalf("accepted projection = %+v", current)
	}
	if !current.ResultReceivedAt.Equal(accepted.CloudReceivedAt) || !current.EdgeReceivedAt.Equal(accepted.EdgeReceivedAt) {
		t.Fatalf("clocks were mixed: edge=%v cloud=%v", current.EdgeReceivedAt, current.ResultReceivedAt)
	}
	var deliveries int64
	if err := db.Table("command_delivery").Count(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("delivery count after accepted = %d", deliveries)
	}

	final := accepted
	final.Status = CommandStatusSucceeded
	final.EdgeReceivedAt = accepted.EdgeReceivedAt
	errorType, errorMessage := "DEVICE_ERROR", "device rejected command"
	final.ErrorType, final.ErrorMessage = &errorType, &errorMessage
	final.CloudReceivedAt = accepted.CloudReceivedAt.Add(time.Second)
	if err := repository.ProjectResult(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	firstReceived := *current.ResultReceivedAt
	if current.Status != CommandStatusSucceeded || current.Result != final.Result || !firstReceived.Equal(final.CloudReceivedAt) || !current.EdgeReceivedAt.Equal(accepted.EdgeReceivedAt) {
		t.Fatalf("final projection = %+v", current)
	}
	if current.ErrorType == nil || *current.ErrorType != errorType || current.ErrorMessage == nil || *current.ErrorMessage != errorMessage {
		t.Fatalf("stable error projection = type=%v message=%v", current.ErrorType, current.ErrorMessage)
	}
	duplicate := final
	duplicate.CloudReceivedAt = final.CloudReceivedAt.Add(time.Minute)
	duplicate.Result = JSONDocument(`{"different":true}`)
	if err := repository.ProjectResult(context.Background(), duplicate); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Result != final.Result || !current.ResultReceivedAt.Equal(firstReceived) {
		t.Fatalf("duplicate overwrote first terminal result: %+v", current)
	}
}

func TestRepositoryProjectResultClassifiesDuplicateAndFirstTerminalConflict(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("55555555-5555-4555-8555-555555555555", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	accepted := resultObservation(CommandStatusSucceeded)
	accepted.CommandID = first.CommandID
	accepted.Result = JSONDocument(`{"number":90071992547409931234567890,"array":[1,2],"object":{"a":1,"b":2}}`)
	if decision, err := repository.ProjectResultDecision(context.Background(), accepted); err != nil || decision.Disposition != ResultApplied {
		t.Fatalf("first terminal decision = %+v, err=%v", decision, err)
	}
	duplicate := accepted
	duplicate.CloudReceivedAt = accepted.CloudReceivedAt.Add(time.Hour)
	duplicate.Result = JSONDocument(`{"object":{"b":2,"a":1},"array":[1,2],"number":90071992547409931234567890}`)
	if decision, err := repository.ProjectResultDecision(context.Background(), duplicate); err != nil || decision.Disposition != ResultDuplicate {
		t.Fatalf("semantic duplicate decision = %+v, err=%v", decision, err)
	}
	conflict := duplicate
	conflict.Result = JSONDocument(`{"object":{"b":2,"a":1},"array":[1,2],"number":90071992547409931234567891}`)
	if decision, err := repository.ProjectResultDecision(context.Background(), conflict); err != nil || decision.Disposition != ResultConflict {
		t.Fatalf("large-number conflict decision = %+v, err=%v", decision, err)
	}
	arrayConflict := duplicate
	arrayConflict.Result = JSONDocument(`{"object":{"a":1,"b":2},"array":[2,1],"number":90071992547409931234567890}`)
	if decision, err := repository.ProjectResultDecision(context.Background(), arrayConflict); err != nil || decision.Disposition != ResultConflict {
		t.Fatalf("array-order conflict decision = %+v, err=%v", decision, err)
	}
	conflict = accepted
	conflict.EdgeReceivedAt = accepted.EdgeReceivedAt.Add(time.Second)
	if decision, err := repository.ProjectResultDecision(context.Background(), conflict); err != nil || decision.Disposition != ResultConflict {
		t.Fatalf("source-time conflict decision = %+v, err=%v", decision, err)
	}
	conflict = accepted
	conflict.Status = CommandStatusFailed
	if decision, err := repository.ProjectResultDecision(context.Background(), conflict); err != nil || decision.Disposition != ResultConflict {
		t.Fatalf("different-terminal conflict decision = %+v, err=%v", decision, err)
	}
	var current Command
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusSucceeded || current.Result != accepted.Result {
		t.Fatalf("conflict changed first terminal fact: %+v", current)
	}
	if !current.ResultReceivedAt.Equal(accepted.CloudReceivedAt) {
		t.Fatalf("duplicate changed resultReceivedAt: %v", current.ResultReceivedAt)
	}
}

func TestRepositoryProjectResultRejectsMismatchedAcceptedReceivedAt(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("66666666-6666-4666-8666-666666666666", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	accepted := resultObservation(CommandStatusAccepted)
	accepted.CommandID = first.CommandID
	if decision, err := repository.ProjectResultDecision(context.Background(), accepted); err != nil || decision.Disposition != ResultApplied {
		t.Fatalf("accepted decision = %+v, err=%v", decision, err)
	}
	final := accepted
	final.Status = CommandStatusSucceeded
	final.EdgeReceivedAt = accepted.EdgeReceivedAt.Add(time.Minute)
	if decision, err := repository.ProjectResultDecision(context.Background(), final); err != nil || decision.Disposition != ResultConflict {
		t.Fatalf("mismatched final decision = %+v, err=%v", decision, err)
	}
	var current Command
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusAccepted || !current.EdgeReceivedAt.Equal(accepted.EdgeReceivedAt) {
		t.Fatalf("mismatched final changed accepted fact: %+v", current)
	}
}

func TestRepositoryProjectResultClassifiesLateAccepted(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("77777777-7777-4777-8777-777777777777", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	final := resultObservation(CommandStatusFailed)
	final.CommandID = first.CommandID
	if _, err := repository.ProjectResultDecision(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	late := final
	late.Status = CommandStatusAccepted
	late.CloudReceivedAt = final.CloudReceivedAt.Add(time.Minute)
	if decision, err := repository.ProjectResultDecision(context.Background(), late); err != nil || decision.Disposition != ResultLateAccepted {
		t.Fatalf("late accepted decision = %+v, err=%v", decision, err)
	}
}

func TestRepositoryProjectResultRejectsUnknownAndFrozenIdentityMismatch(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	if err := repository.ProjectResult(context.Background(), resultObservation(CommandStatusSucceeded)); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("unknown result error = %v", err)
	}
	first, delivery := testCommand("44444444-4444-4444-8444-444444444444", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	wrongRoute := resultObservation(CommandStatusSucceeded)
	wrongRoute.CommandID = first.CommandID
	wrongRoute.EdgeID = "other-edge"
	if err := repository.ProjectResult(context.Background(), wrongRoute); !errors.Is(err, ErrResultRouteMismatch) {
		t.Fatalf("route mismatch error = %v", err)
	}
	wrongName := resultObservation(CommandStatusSucceeded)
	wrongName.CommandID = first.CommandID
	wrongName.Name = "open"
	if err := repository.ProjectResult(context.Background(), wrongName); !errors.Is(err, ErrResultNameMismatch) {
		t.Fatalf("name mismatch error = %v", err)
	}
}

func TestRepositoryProjectResultFinalBeforeAcceptedAndLateAccepted(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("22222222-2222-4222-8222-222222222222", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	final := resultObservation(CommandStatusFailed)
	final.CommandID = first.CommandID
	if err := repository.ProjectResult(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	late := final
	late.Status = CommandStatusAccepted
	late.CloudReceivedAt = final.CloudReceivedAt.Add(time.Minute)
	if err := repository.ProjectResult(context.Background(), late); err != nil {
		t.Fatal(err)
	}
	var current Command
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusFailed || !current.ResultReceivedAt.Equal(final.CloudReceivedAt) {
		t.Fatalf("late accepted regressed final: %+v", current)
	}
}

func TestRepositoryProjectResultRollsBackOnDeliveryDeleteFailure(t *testing.T) {
	db := openCommandTestDB(t)
	repository := NewRepository(db)
	first, delivery := testCommand("33333333-3333-4333-8333-333333333333", "hash")
	if _, err := repository.Create(context.Background(), first, delivery, audit.Event{Action: "command.create", Metadata: audit.Metadata{ActorID: 7}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE command_delivery").Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.ProjectResult(context.Background(), resultObservation(CommandStatusSucceeded)); err == nil {
		t.Fatal("projection succeeded without delivery table")
	}
	var current Command
	if err := db.Where("command_id = ?", first.CommandID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != CommandStatusPending {
		t.Fatalf("failed transaction changed command status to %s", current.Status)
	}
}
