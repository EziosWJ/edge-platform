package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/google/uuid"
)

type PermissionChecker interface {
	HasPermission(context.Context, int64, string) (bool, error)
}

type Service struct {
	store   Store
	devices interface {
		Detail(context.Context, string) (device.Device, error)
	}
	prefix string
	now    func() time.Time
}

func NewService(store Store, devices interface {
	Detail(context.Context, string) (device.Device, error)
}, prefix string) (*Service, error) {
	if store == nil {
		return nil, errors.New("command store is required")
	}
	if devices == nil {
		return nil, errors.New("command device resolver is required")
	}
	prefix = strings.Trim(prefix, "/")
	if prefix == "" || strings.ContainsAny(prefix, "+#") {
		return nil, errors.New("command topic prefix is invalid")
	}
	return &Service{store: store, devices: devices, prefix: prefix, now: time.Now}, nil
}

func (s *Service) Create(ctx context.Context, actorID int64, input CreateInput, event audit.Event) (CommandView, error) {
	if actorID <= 0 || !validCommandID(input.CommandID) || strings.TrimSpace(input.DeviceID) == "" || strings.TrimSpace(input.Name) == "" || len(input.Name) > 256 || len(input.Args) == 0 {
		return CommandView{}, ErrInvalid
	}
	ttl := DefaultTTLSeconds
	if input.TTLSeconds != nil {
		ttl = *input.TTLSeconds
	}
	if ttl < MinTTLSeconds || ttl > MaxTTLSeconds {
		return CommandView{}, ErrInvalid
	}
	canonicalArgs, err := canonicalizeArgs(input.Args)
	if err != nil {
		return CommandView{}, ErrInvalid
	}
	hash, err := requestHash(actorID, input.DeviceID, input.Name, canonicalArgs, ttl)
	if err != nil {
		return CommandView{}, err
	}
	// A retry must return the already frozen route and payload even if the
	// current Device projection has changed or disappeared since creation.
	if existing, err := s.store.Detail(ctx, input.CommandID); err == nil {
		if existing.RequestedBy != actorID || existing.RequestHash != hash {
			return CommandView{}, ErrConflict
		}
		return existing.View(), nil
	} else if !errors.Is(err, ErrNotFound) {
		return CommandView{}, err
	}
	resolved, err := s.devices.Detail(ctx, input.DeviceID)
	if err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return CommandView{}, ErrNotFound
		}
		return CommandView{}, err
	}
	if strings.TrimSpace(resolved.EdgeID) == "" || strings.TrimSpace(resolved.SourceDeviceID) == "" {
		return CommandView{}, ErrInvalid
	}
	issuedAt := s.now().UTC()
	expiresAt := issuedAt.Add(time.Duration(ttl) * time.Second)
	payload, topic, err := buildPayload(s.prefix, resolved.EdgeID, resolved.SourceDeviceID, input.CommandID, input.Name, canonicalArgs, issuedAt, expiresAt)
	if err != nil {
		return CommandView{}, err
	}
	if len(payload) > MaxPayloadBytes {
		return CommandView{}, ErrPayloadSize
	}
	candidate := Command{
		CommandID: input.CommandID, DeviceID: input.DeviceID, EdgeID: resolved.EdgeID,
		SourceDeviceID: resolved.SourceDeviceID, Topic: topic, Name: input.Name,
		Args: JSONDocument(canonicalArgs), RequestedBy: actorID, RequestHash: hash,
		IssuedAt: issuedAt, ExpiresAt: expiresAt, Status: CommandStatusPending,
	}
	delivery := Delivery{CommandID: input.CommandID, Topic: topic, Payload: payload, ExpiresAt: expiresAt}
	event.Resource = "command"
	created, err := s.store.Create(ctx, candidate, delivery, event)
	if err != nil {
		return CommandView{}, err
	}
	return created.View(), nil
}

func (s *Service) Page(ctx context.Context, query CommandPageQuery) (CommandPage, error) {
	if err := validateCommandPageQuery(query); err != nil {
		return CommandPage{}, err
	}
	page, err := s.store.Page(ctx, query)
	if page.Records == nil {
		page.Records = []CommandView{}
	}
	page.Page, page.PageSize = query.Page, query.PageSize
	return page, err
}

func (s *Service) Detail(ctx context.Context, commandID string) (CommandView, error) {
	if !validCommandID(strings.TrimSpace(commandID)) {
		return CommandView{}, ErrNotFound
	}
	command, err := s.store.Detail(ctx, commandID)
	if err != nil {
		return CommandView{}, err
	}
	return command.View(), nil
}

func validateCommandPageQuery(query CommandPageQuery) error {
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 500 {
		return ErrInvalid
	}
	if query.Status != nil && !validStatusForQuery(*query.Status) {
		return ErrInvalid
	}
	if query.RequestedBy != nil && *query.RequestedBy <= 0 {
		return ErrInvalid
	}
	return nil
}

// ResultStore is deliberately narrower than Store: result ingress only needs
// the atomic projection operation and does not expose delivery execution.
type ResultStore interface {
	ProjectResult(context.Context, ResultObservation) error
}

type ClassifiedResultStore interface {
	ProjectResultDecision(context.Context, ResultObservation) (ResultProjection, error)
}

// ProjectResult validates the domain-independent result observation before it
// reaches the repository transaction. Permanent contract/identity failures
// are safe to ACK; repository/transaction failures are returned unchanged so
// the MQTT runtime can request redelivery.
func (s *Service) ProjectResult(ctx context.Context, observation ResultObservation) error {
	_, err := s.ProjectResultDecision(ctx, observation)
	return err
}

func (s *Service) ProjectResultDecision(ctx context.Context, observation ResultObservation) (ResultProjection, error) {
	if !validCommandID(observation.CommandID) || strings.TrimSpace(observation.Name) == "" ||
		!validResultStatus(observation.Status) || strings.TrimSpace(observation.EdgeID) == "" ||
		strings.TrimSpace(observation.SourceDeviceID) == "" || observation.EdgeReceivedAt.IsZero() ||
		observation.CloudReceivedAt.IsZero() || len(observation.Result) == 0 || !json.Valid([]byte(observation.Result)) {
		return ResultProjection{}, ErrInvalidResult
	}
	if observation.StartedAt != nil && observation.StartedAt.IsZero() || observation.CompletedAt != nil && observation.CompletedAt.IsZero() {
		return ResultProjection{}, ErrInvalidResult
	}
	if projector, ok := s.store.(ClassifiedResultStore); ok {
		return projector.ProjectResultDecision(ctx, observation)
	}
	projector, ok := s.store.(ResultStore)
	if !ok {
		return ResultProjection{}, errors.New("command result projector is not configured")
	}
	if err := projector.ProjectResult(ctx, observation); err != nil {
		return ResultProjection{}, err
	}
	return ResultProjection{Disposition: ResultApplied}, nil
}

func validCommandID(value string) bool { return uuid.Validate(value) == nil }

func requestHash(actorID int64, deviceID, name string, args []byte, ttl int) (string, error) {
	value, err := json.Marshal(struct {
		ActorID int64           `json:"actorId"`
		Device  string          `json:"deviceId"`
		Name    string          `json:"name"`
		Args    json.RawMessage `json:"args"`
		TTL     int             `json:"ttlSeconds"`
	}{actorID, deviceID, name, args, ttl})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:]), nil
}

func buildPayload(prefix, edgeID, sourceDeviceID, commandID, name string, args []byte, issuedAt, expiresAt time.Time) ([]byte, string, error) {
	if !validTopicPart(prefix) || !validTopicPart(edgeID) || !validTopicPart(sourceDeviceID) {
		return nil, "", ErrInvalid
	}
	topic := fmt.Sprintf("%s/%s/device/%s/command", prefix, edgeID, sourceDeviceID)
	payload, err := json.Marshal(struct {
		Schema    string          `json:"schema"`
		CommandID string          `json:"commandId"`
		DeviceID  string          `json:"deviceId"`
		Name      string          `json:"name"`
		Args      json.RawMessage `json:"args"`
		IssuedAt  string          `json:"issuedAt"`
		ExpiresAt string          `json:"expiresAt"`
	}{"device-command/v1", commandID, sourceDeviceID, name, args, issuedAt.UTC().Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return nil, "", err
	}
	return payload, topic, nil
}

func validTopicPart(value string) bool {
	return value != "" && !strings.ContainsAny(value, "/+#\x00")
}
