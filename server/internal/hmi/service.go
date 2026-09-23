package hmi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/command"
	"github.com/EziosWJ/edge-platform/server/internal/datapoint"
	"github.com/EziosWJ/edge-platform/server/internal/device"
	"github.com/google/uuid"
)

type DataPointReader interface {
	Page(context.Context, datapoint.PageQuery) (datapoint.Page, error)
}
type DeviceReader interface {
	Detail(context.Context, string) (device.Device, error)
}
type Service struct {
	store   Store
	points  DataPointReader
	devices DeviceReader
}

func NewService(store Store, points DataPointReader, devices DeviceReader) (*Service, error) {
	if store == nil || points == nil || devices == nil {
		return nil, ErrInvalid
	}
	return &Service{store: store, points: points, devices: devices}, nil
}

func (s *Service) Page(ctx context.Context, q PageQuery) (PageList, error) {
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 500 || strings.TrimSpace(q.Name) != q.Name || len(q.Name) > 128 {
		return PageList{}, ErrInvalid
	}
	return s.store.Page(ctx, q)
}
func (s *Service) Detail(ctx context.Context, id string) (Page, error) {
	if !validPageID(id) {
		return Page{}, ErrInvalid
	}
	return s.store.Detail(ctx, id)
}
func (s *Service) Create(ctx context.Context, m audit.Metadata, in CreateInput) (Page, error) {
	if err := validMetadata(in.Name, in.Description); err != nil {
		return Page{}, err
	}
	if in.Document == "" {
		in.Document = JSONDocument(`{"schema":"hmi-page/v1","canvas":{"width":1920,"height":1080},"nodes":[]}`)
	}
	canonical, _, err := canonicalDocument(in.Document, false)
	if err != nil {
		return Page{}, err
	}
	in.Document = canonical
	return s.store.Create(ctx, in, audit.Event{Action: "hmi.page.create", Resource: "hmi_page", Summary: "创建 HMI 页面", Metadata: m})
}
func (s *Service) UpdateMetadata(ctx context.Context, m audit.Metadata, id string, in MetadataInput) (Page, error) {
	if !validPageID(id) || validMetadata(in.Name, in.Description) != nil {
		return Page{}, ErrInvalid
	}
	return s.store.UpdateMetadata(ctx, id, in, audit.Event{Action: "hmi.page.update", Resource: "hmi_page", Summary: "更新 HMI 页面 metadata", Metadata: m})
}
func (s *Service) SaveDraft(ctx context.Context, m audit.Metadata, id string, in DraftInput) (Page, error) {
	if !validPageID(id) || in.ExpectedDraftRevision < 1 {
		return Page{}, ErrInvalid
	}
	canonical, _, err := canonicalDocument(in.Document, false)
	if err != nil {
		return Page{}, err
	}
	in.Document = canonical
	return s.store.SaveDraft(ctx, id, in, audit.Event{Action: "hmi.draft.save", Resource: "hmi_page", Summary: "保存 HMI 草稿", Metadata: m})
}
func (s *Service) Publish(ctx context.Context, m audit.Metadata, id string, in PublishInput) (Version, error) {
	if !validPageID(id) || in.ExpectedDraftRevision < 1 || in.ActorID <= 0 {
		return Version{}, ErrInvalid
	}
	// A retry for a revision already published is safe to return directly: that
	// immutable document already passed strict validation at its first publish.
	if existing, err := s.store.PublishedForRevision(ctx, id, in.ExpectedDraftRevision); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Version{}, err
	}
	page, err := s.store.Detail(ctx, id)
	if err != nil {
		return Version{}, err
	}
	if page.DraftRevision != in.ExpectedDraftRevision {
		return Version{}, ErrConflict
	}
	canonical, doc, err := canonicalDocument(page.DraftDocument, true)
	if err != nil {
		return Version{}, err
	}
	if err := s.validateBindings(ctx, doc); err != nil {
		return Version{}, err
	}
	page.DraftDocument = canonical
	return s.store.Publish(ctx, id, in, audit.Event{Action: "hmi.page.publish", Resource: "hmi_page", Summary: "发布 HMI 页面", Metadata: m})
}
func (s *Service) Runtime(ctx context.Context, id string) (RuntimeBootstrap, error) {
	if !validPageID(id) {
		return RuntimeBootstrap{}, ErrInvalid
	}
	p, v, err := s.store.Runtime(ctx, id)
	if err != nil {
		return RuntimeBootstrap{}, err
	}
	_, doc, err := canonicalDocument(v.Document, true)
	if err != nil {
		return RuntimeBootstrap{}, ErrInvalidBinding
	}
	unique := map[string]DataPointMetadata{}
	for _, n := range doc.Nodes {
		for slot, raw := range n.Bindings {
			if expectedSlots(n.Type)[slot] == "" {
				continue
			}
			var ref datapointBinding
			if json.Unmarshal(raw, &ref) != nil || ref.Kind != "datapoint" {
				continue
			}
			key := ref.DeviceID + "\x00" + ref.PointKey
			if _, ok := unique[key]; ok {
				continue
			}
			p, e := s.findPoint(ctx, ref)
			if e != nil {
				return RuntimeBootstrap{}, e
			}
			unique[key] = pointMetadata(p)
		}
	}
	points := make([]DataPointMetadata, 0, len(unique))
	for _, p := range unique {
		points = append(points, p)
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].DeviceID == points[j].DeviceID {
			return points[i].PointKey < points[j].PointKey
		}
		return points[i].DeviceID < points[j].DeviceID
	})
	return RuntimeBootstrap{Page: PageMetadata{PageID: p.PageID, Name: p.Name, Description: p.Description}, Version: v, DataPoints: points}, nil
}

func validMetadata(name, description string) error {
	if strings.TrimSpace(name) == "" || len(name) > 128 || len(description) > 2048 || strings.TrimSpace(name) != name || strings.IndexByte(name, 0) >= 0 || strings.IndexByte(description, 0) >= 0 {
		return ErrInvalid
	}
	return nil
}
func validPageID(id string) bool { return strings.TrimSpace(id) == id && uuid.Validate(id) == nil }

type datapointBinding struct {
	Kind     string `json:"kind"`
	DeviceID string `json:"deviceId"`
	PointKey string `json:"pointKey"`
}
type commandBinding struct {
	Kind         string          `json:"kind"`
	DeviceID     string          `json:"deviceId"`
	Name         string          `json:"name"`
	Args         json.RawMessage `json:"args"`
	TTLSeconds   int             `json:"ttlSeconds"`
	Confirmation *struct {
		Required *bool  `json:"required"`
		Message  string `json:"message"`
	} `json:"confirmation"`
}

func (s *Service) validateBindings(ctx context.Context, doc Document) error {
	uniquePoints := map[string]bool{}
	track := func(point datapoint.DataPoint) error {
		uniquePoints[point.DeviceID+"\x00"+point.PointKey] = true
		if len(uniquePoints) > MaxBindings {
			return ErrInvalidBinding
		}
		return nil
	}
	for _, n := range doc.Nodes {
		switch n.Type {
		case "value-display", "indicator", "gauge":
			point, err := s.resolvePoint(ctx, n.Bindings["value"], expectedSlots(n.Type)["value"])
			if err != nil {
				return err
			}
			if err := track(point); err != nil {
				return err
			}
		case "button":
			if err := s.validateCommand(ctx, n.Bindings["command"]); err != nil {
				return err
			}
		case "switch":
			p, err := s.resolvePoint(ctx, n.Bindings["state"], "BOOLEAN")
			if err != nil {
				return err
			}
			if err := track(p); err != nil {
				return err
			}
			a, _, err := s.decodeCommand(n.Bindings["onCommand"])
			if err != nil {
				return err
			}
			if a.DeviceID != p.DeviceID {
				return ErrInvalidBinding
			}
			if err = s.checkDevice(ctx, a.DeviceID); err != nil {
				return err
			}
			a, _, err = s.decodeCommand(n.Bindings["offCommand"])
			if err != nil {
				return err
			}
			if a.DeviceID != p.DeviceID {
				return ErrInvalidBinding
			}
			if err = s.checkDevice(ctx, a.DeviceID); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Service) resolvePoint(ctx context.Context, raw json.RawMessage, types string) (datapoint.DataPoint, error) {
	var b datapointBinding
	if decodeStrict(raw, &b) != nil || b.Kind != "datapoint" || b.DeviceID == "" || b.PointKey == "" {
		return datapoint.DataPoint{}, ErrInvalidBinding
	}
	p, err := s.findPoint(ctx, b)
	if err != nil {
		return datapoint.DataPoint{}, err
	}
	compatible := types == string(p.ValueType) || types == "NUMBER|BOOLEAN" && (p.ValueType == datapoint.ValueTypeNumber || p.ValueType == datapoint.ValueTypeBoolean)
	if !compatible {
		return datapoint.DataPoint{}, ErrInvalidBinding
	}
	return p, nil
}
func (s *Service) findPoint(ctx context.Context, b datapointBinding) (datapoint.DataPoint, error) {
	// DataPoint IDs are stable but HMI stores the public deviceId+pointKey identity.
	page, err := s.points.Page(ctx, datapoint.PageQuery{Page: 1, PageSize: 500, DeviceID: b.DeviceID, PointKey: b.PointKey})
	if err != nil {
		return datapoint.DataPoint{}, err
	}
	if len(page.Records) != 1 {
		return datapoint.DataPoint{}, ErrInvalidBinding
	}
	p := page.Records[0]
	if p.DeviceID != b.DeviceID || p.PointKey != b.PointKey {
		return datapoint.DataPoint{}, ErrInvalidBinding
	}
	return p, nil
}
func pointMetadata(p datapoint.DataPoint) DataPointMetadata {
	return DataPointMetadata{DataPointID: p.DataPointID, DeviceID: p.DeviceID, PointKey: p.PointKey, Name: p.Name, ValueType: string(p.ValueType), Unit: p.Unit, Precision: p.Precision, Enabled: p.Enabled}
}

func (s *Service) decodeCommand(raw json.RawMessage) (commandBinding, []byte, error) {
	var b commandBinding
	if decodeStrict(raw, &b) != nil || b.Kind != "command" || b.DeviceID == "" || strings.TrimSpace(b.Name) == "" || len(b.Name) > 256 || b.TTLSeconds < command.MinTTLSeconds || b.TTLSeconds > command.MaxTTLSeconds {
		return b, nil, ErrInvalidBinding
	}
	if len(b.Args) == 0 || !json.Valid(b.Args) {
		return b, nil, ErrInvalidBinding
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(b.Args, &args) != nil || args == nil {
		return b, nil, ErrInvalidBinding
	}
	if b.Confirmation != nil {
		if b.Confirmation.Required == nil || len(b.Confirmation.Message) > 256 || strings.IndexByte(b.Confirmation.Message, 0) >= 0 {
			return b, nil, ErrInvalidBinding
		}
	}
	payload, err := json.Marshal(struct {
		Schema    string          `json:"schema"`
		CommandID string          `json:"commandId"`
		DeviceID  string          `json:"deviceId"`
		Name      string          `json:"name"`
		Args      json.RawMessage `json:"args"`
		IssuedAt  string          `json:"issuedAt"`
		ExpiresAt string          `json:"expiresAt"`
	}{"device-command/v1", "00000000-0000-0000-0000-000000000000", b.DeviceID, b.Name, b.Args, "2000-01-01T00:00:00Z", "2000-01-01T00:00:30Z"})
	if err != nil || len(payload) > command.MaxPayloadBytes {
		return b, nil, ErrInvalidBinding
	}
	return b, payload, nil
}
func (s *Service) validateCommand(ctx context.Context, raw json.RawMessage) error {
	b, _, err := s.decodeCommand(raw)
	if err != nil {
		return err
	}
	return s.checkDevice(ctx, b.DeviceID)
}
func (s *Service) checkDevice(ctx context.Context, id string) error {
	if _, err := s.devices.Detail(ctx, id); err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return ErrInvalidBinding
		}
		return err
	}
	return nil
}
func decodeStrict(raw []byte, v any) error {
	if len(raw) == 0 {
		return ErrInvalidBinding
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var x any
	if err := d.Decode(&x); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing JSON")
		}
		return err
	}
	return nil
}
