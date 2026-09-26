package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	Create(context.Context, Command, Delivery, audit.Event) (Command, error)
	Page(context.Context, CommandPageQuery) (CommandPage, error)
	Detail(context.Context, string) (Command, error)
}

type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)
var _ DeliveryStore = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create uses INSERT ... ON CONFLICT DO NOTHING as the database convergence
// point. A duplicate command is read in the same transaction after the
// conflict; the delivery and audit are only inserted on the winner path.
func (r *Repository) Create(ctx context.Context, candidate Command, delivery Delivery, event audit.Event) (Command, error) {
	var result Command
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		insert := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "command_id"}},
			DoNothing: true,
		}).Create(&candidate)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			if err := tx.Where("command_id = ?", candidate.CommandID).Take(&result).Error; err != nil {
				return err
			}
			if result.RequestedBy != candidate.RequestedBy || result.RequestHash != candidate.RequestHash {
				return ErrConflict
			}
			return nil
		}
		if err := tx.Create(&delivery).Error; err != nil {
			return err
		}
		if err := audit.RecordOn(ctx, tx, event); err != nil {
			return err
		}
		result = candidate
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Command{}, ErrConflict
	}
	return result, err
}

func (r *Repository) Detail(ctx context.Context, commandID string) (Command, error) {
	var result Command
	err := r.db.WithContext(ctx).Where("command_id = ?", commandID).Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Command{}, ErrNotFound
	}
	return result, err
}

func (r *Repository) Page(ctx context.Context, query CommandPageQuery) (CommandPage, error) {
	var result CommandPage
	db := r.db.WithContext(ctx).Model(&Command{})
	if query.CommandID != "" {
		db = db.Where("command_id = ?", query.CommandID)
	}
	if query.DeviceID != "" {
		db = db.Where("device_id = ?", query.DeviceID)
	}
	if query.Name != "" {
		db = db.Where("name = ?", query.Name)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.RequestedBy != nil {
		db = db.Where("requested_by = ?", *query.RequestedBy)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var records []Command
	if err := db.Order("issued_at DESC, command_id ASC").
		Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&records).Error; err != nil {
		return result, err
	}
	result.Records = make([]CommandView, 0, len(records))
	for _, record := range records {
		result.Records = append(result.Records, record.View())
	}
	result.Page, result.PageSize = query.Page, query.PageSize
	return result, nil
}

func (r *Repository) NextPendingDelivery(ctx context.Context, now time.Time) (Delivery, error) {
	var result Delivery
	err := r.db.WithContext(ctx).Model(&Delivery{}).
		Joins("JOIN command ON command.command_id = command_delivery.command_id").
		Where("command.status = ? AND command.expires_at > ? AND command_delivery.expires_at > ? AND (command_delivery.next_attempt_at IS NULL OR command_delivery.next_attempt_at <= ?)", CommandStatusPending, now.UTC(), now.UTC(), now.UTC()).
		Order("command_delivery.expires_at ASC, command_delivery.command_id ASC").Take(&result).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Delivery{}, ErrNoPendingDelivery
	}
	return result, err
}

func (r *Repository) RecordDeliveryAttempt(ctx context.Context, commandID string, at time.Time, publishErr error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var delivery Delivery
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id = ?", commandID).Take(&delivery).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A result may have won the permitted in-flight race and removed the
			// delivery before transport facts were recorded.
			return nil
		}
		if err != nil {
			return err
		}
		attempt := delivery.AttemptCount + 1
		nextAttempt := at.UTC().Add(deliveryBackoff(attempt))
		updates := map[string]any{
			"attempt_count":   attempt,
			"last_attempt_at": at.UTC(),
			"last_error":      nil,
			"next_attempt_at": nextAttempt,
		}
		if publishErr != nil {
			message := publishErr.Error()
			if len(message) > 256 {
				message = message[:256]
			}
			updates["last_error"] = message
		}
		return tx.Model(&delivery).Updates(updates).Error
	})
}

func deliveryBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Second
	case 2:
		return 2 * time.Second
	case 3:
		return 4 * time.Second
	case 4:
		return 8 * time.Second
	default:
		return 10 * time.Second
	}
}

// ExpirePendingDeliveries atomically removes deliveries whose frozen command
// TTL has elapsed. It deliberately leaves the Command in PENDING: a valid
// late result remains projectable through the result-ingress transaction.
func (r *Repository) ExpirePendingDeliveries(ctx context.Context, now time.Time) (int, error) {
	expired := 0
	for {
		removed, err := r.expireOnePendingDelivery(ctx, now.UTC())
		if err != nil {
			return expired, err
		}
		if !removed {
			return expired, nil
		}
		expired++
	}
}

func (r *Repository) expireOnePendingDelivery(ctx context.Context, now time.Time) (bool, error) {
	removed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidate Delivery
		err := tx.Model(&Delivery{}).
			Joins("JOIN command ON command.command_id = command_delivery.command_id").
			Where("command.status = ? AND (command.expires_at <= ? OR command_delivery.expires_at <= ?)", CommandStatusPending, now, now).
			Order("command.expires_at ASC, command.command_id ASC").Take(&candidate).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var current Command
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id = ?", candidate.CommandID).Take(&current).Error; err != nil {
			return err
		}
		if current.Status != CommandStatusPending || (current.ExpiresAt.After(now) && candidate.ExpiresAt.After(now)) {
			return nil
		}
		if err := tx.Model(&current).Updates(map[string]any{"delivery_expired_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Where("command_id = ?", current.CommandID).Delete(&Delivery{}).Error; err != nil {
			return err
		}
		removed = true
		return nil
	})
	return removed, err
}

func (r *Repository) ProjectResult(ctx context.Context, observation ResultObservation) error {
	_, err := r.ProjectResultDecision(ctx, observation)
	return err
}

// ProjectResultDecision applies a result while holding the Command row lock.
// It returns a bounded disposition so ingress can distinguish an idempotent
// duplicate from a first-terminal-wins conflict without exposing payloads.
func (r *Repository) ProjectResultDecision(ctx context.Context, observation ResultObservation) (ResultProjection, error) {
	decision := ResultProjection{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Command
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("command_id = ?", observation.CommandID).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnknownCommand
		}
		if err != nil {
			return err
		}
		if current.EdgeID != observation.EdgeID || current.SourceDeviceID != observation.SourceDeviceID {
			return ErrResultRouteMismatch
		}
		if current.Name != observation.Name {
			return ErrResultNameMismatch
		}
		disposition, shouldApply := resultDecision(current, observation)
		decision.Disposition = disposition
		if !shouldApply {
			return nil
		}
		edgeReceivedAt := current.EdgeReceivedAt
		if edgeReceivedAt == nil {
			edgeReceivedAt = utcResultTime(&observation.EdgeReceivedAt)
		}
		updates := map[string]any{
			"status":             observation.Status,
			"edge_received_at":   edgeReceivedAt,
			"started_at":         utcResultTime(observation.StartedAt),
			"completed_at":       utcResultTime(observation.CompletedAt),
			"result_received_at": observation.CloudReceivedAt.UTC(),
			"result":             observation.Result,
			"error_type":         observation.ErrorType,
			"error_message":      observation.ErrorMessage,
		}
		if err := tx.Model(&current).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("command_id = ?", observation.CommandID).Delete(&Delivery{}).Error
	})
	return decision, err
}

func utcResultTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	// PostgreSQL timestamptz stores microseconds. Normalize before persistence
	// and semantic comparison so an identical Edge instant survives a round trip.
	instant := value.UTC().Truncate(time.Microsecond)
	return &instant
}

func resultDecision(current Command, observation ResultObservation) (ResultDisposition, bool) {
	if current.Status == CommandStatusPending {
		return ResultApplied, true
	}
	if current.Status == CommandStatusAccepted {
		if observation.Status == CommandStatusAccepted {
			if resultSemanticallyEqual(current, observation) {
				return ResultDuplicate, false
			}
			return ResultConflict, false
		}
		if !sameInstant(current.EdgeReceivedAt, &observation.EdgeReceivedAt) {
			return ResultConflict, false
		}
		return ResultApplied, true
	}
	if observation.Status == CommandStatusAccepted {
		return ResultLateAccepted, false
	}
	if current.Status == observation.Status && resultSemanticallyEqual(current, observation) {
		return ResultDuplicate, false
	}
	return ResultConflict, false
}

func resultSemanticallyEqual(current Command, observation ResultObservation) bool {
	return sameInstant(current.EdgeReceivedAt, &observation.EdgeReceivedAt) &&
		sameInstant(current.StartedAt, observation.StartedAt) &&
		sameInstant(current.CompletedAt, observation.CompletedAt) &&
		sameOptionalString(current.ErrorType, observation.ErrorType) &&
		sameOptionalString(current.ErrorMessage, observation.ErrorMessage) &&
		jsonSemanticEqual(current.Result, observation.Result)
}

func sameInstant(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UTC().Truncate(time.Microsecond).Equal(right.UTC().Truncate(time.Microsecond))
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func jsonSemanticEqual(left, right JSONDocument) bool {
	leftValue, ok := decodeJSONValue(left)
	if !ok {
		return false
	}
	rightValue, ok := decodeJSONValue(right)
	if !ok {
		return false
	}
	return semanticJSONValueEqual(leftValue, rightValue)
}

func decodeJSONValue(value JSONDocument) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return decoded, true
}

func semanticJSONValueEqual(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case json.Number:
		rightValue, ok := right.(json.Number)
		return ok && compareJSONNumbers(leftValue, rightValue)
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for i := range leftValue {
			if !semanticJSONValueEqual(leftValue[i], rightValue[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			other, ok := rightValue[key]
			if !ok || !semanticJSONValueEqual(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func compareJSONNumbers(left, right json.Number) bool {
	leftRat, ok := decimalJSONNumber(left.String())
	if !ok {
		return left == right
	}
	rightRat, ok := decimalJSONNumber(right.String())
	if !ok {
		return left == right
	}
	return leftRat.Cmp(rightRat) == 0
}

func decimalJSONNumber(value string) (*big.Rat, bool) {
	parts := strings.SplitN(strings.ToLower(value), "e", 2)
	mantissa := parts[0]
	exponent := 0
	if len(parts) == 2 {
		var err error
		exponent, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, false
		}
	}
	sign := ""
	if strings.HasPrefix(mantissa, "-") || strings.HasPrefix(mantissa, "+") {
		sign, mantissa = mantissa[:1], mantissa[1:]
	}
	decimalParts := strings.SplitN(mantissa, ".", 2)
	whole, fraction := decimalParts[0], ""
	if len(decimalParts) == 2 {
		fraction = decimalParts[1]
	}
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		digits = "0"
	}
	coefficient, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, false
	}
	if sign == "-" {
		coefficient.Neg(coefficient)
	}
	scale := len(fraction) - exponent
	if scale <= 0 {
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil))
		return new(big.Rat).SetInt(coefficient), true
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return new(big.Rat).SetFrac(coefficient, denominator), true
}
