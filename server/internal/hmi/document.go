package hmi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"

	"github.com/google/uuid"
)

var canvasSizes = map[[2]int]bool{{1920, 1080}: true, {1366, 768}: true, {1280, 720}: true}
var staticColors = set("#1f2937", "#ffffff", "#6b7280", "#9ca3af", "#e5e7eb", "#1677ff", "#16a34a", "#d97706", "#dc2626", "#0891b2")

func canonicalDocument(raw JSONDocument, strict bool) (JSONDocument, Document, error) {
	if len(raw) == 0 || len(raw) > MaxDocumentBytes {
		return "", Document{}, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return "", Document{}, ErrInvalid
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", Document{}, ErrInvalid
	}
	if doc.Schema != SchemaV1 || !canvasSizes[[2]int{doc.Canvas.Width, doc.Canvas.Height}] || len(doc.Nodes) > MaxNodes {
		return "", Document{}, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range doc.Nodes {
		n := &doc.Nodes[i]
		if uuid.Validate(n.NodeID) != nil || seen[n.NodeID] || !finite(n.X) || !finite(n.Y) || !finite(n.Width) || !finite(n.Height) || n.Width <= 0 || n.Height <= 0 || n.X < 0 || n.Y < 0 || n.X+n.Width > float64(doc.Canvas.Width) || n.Y+n.Height > float64(doc.Canvas.Height) || !validRotation(n.Rotation) || n.Type == "" || n.Props == nil || n.Bindings == nil {
			return "", Document{}, ErrInvalid
		}
		seen[n.NodeID] = true
		if err := validateNode(*n, strict); err != nil {
			return "", Document{}, err
		}
	}
	encoded, err := json.Marshal(doc)
	if err != nil || len(encoded) > MaxDocumentBytes {
		return "", Document{}, ErrInvalid
	}
	return JSONDocument(encoded), doc, nil
}

func validateNode(n Node, strict bool) error {
	propKeys := map[string]map[string]bool{
		"text":          set("text", "color", "fontSize", "fontWeight", "align"),
		"shape":         set("shape", "fill", "stroke", "strokeWidth"),
		"value-display": set("label", "color", "precision", "showUnit"),
		"indicator":     set("label", "trueLabel", "falseLabel", "trueColor", "falseColor"),
		"gauge":         set("min", "max", "precision", "showUnit", "label"),
		"button":        set("label"),
		"switch":        set("label", "onLabel", "offLabel"),
	}
	allowed, ok := propKeys[n.Type]
	if !ok {
		return ErrInvalid
	}
	for key, value := range n.Props {
		if !allowed[key] || !json.Valid(value) {
			return ErrInvalid
		}
	}
	for key, value := range n.Bindings {
		if !json.Valid(value) {
			return ErrInvalid
		}
		slotType, ok := expectedSlots(n.Type)[key]
		if !ok {
			return ErrInvalid
		}
		if err := validateBindingShape(value, slotType); err != nil {
			return err
		}
	}
	if err := validateProps(n.Type, n.Props, strict); err != nil {
		return err
	}
	if strict {
		for key := range expectedSlots(n.Type) {
			if bindingRequired(n.Type, key) && len(n.Bindings[key]) == 0 {
				return ErrInvalidBinding
			}
		}
	}
	return nil
}

func validateBindingShape(raw json.RawMessage, slotType string) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return ErrInvalid
	}
	kindRaw := fields["kind"]
	if len(kindRaw) == 0 {
		return nil
	} // Drafts may contain an incomplete binding.
	var kind string
	if json.Unmarshal(kindRaw, &kind) != nil {
		return ErrInvalid
	}
	if slotType == "COMMAND" {
		if kind != "command" {
			return ErrInvalid
		}
		for key := range fields {
			if !set("kind", "deviceId", "name", "args", "ttlSeconds", "confirmation")[key] {
				return ErrInvalid
			}
		}
		for _, key := range []string{"deviceId", "name"} {
			if v := fields[key]; len(v) > 0 {
				var s string
				if json.Unmarshal(v, &s) != nil {
					return ErrInvalid
				}
			}
		}
		if v := fields["args"]; len(v) > 0 {
			var args map[string]json.RawMessage
			if json.Unmarshal(v, &args) != nil || args == nil {
				return ErrInvalid
			}
		}
		if v := fields["ttlSeconds"]; len(v) > 0 {
			var ttl int
			if json.Unmarshal(v, &ttl) != nil {
				return ErrInvalid
			}
		}
		return nil
	}
	if kind != "datapoint" {
		return ErrInvalid
	}
	for key := range fields {
		if !set("kind", "deviceId", "pointKey")[key] {
			return ErrInvalid
		}
	}
	for _, key := range []string{"deviceId", "pointKey"} {
		if v := fields[key]; len(v) > 0 {
			var s string
			if json.Unmarshal(v, &s) != nil {
				return ErrInvalid
			}
		}
	}
	return nil
}

func expectedSlots(kind string) map[string]string {
	switch kind {
	case "value-display":
		return map[string]string{"value": "NUMBER|BOOLEAN"}
	case "indicator":
		return map[string]string{"value": "BOOLEAN"}
	case "gauge":
		return map[string]string{"value": "NUMBER"}
	case "button":
		return map[string]string{"command": "COMMAND"}
	case "switch":
		return map[string]string{"state": "BOOLEAN", "onCommand": "COMMAND", "offCommand": "COMMAND"}
	default:
		return map[string]string{}
	}
}
func bindingRequired(kind, slot string) bool {
	return kind == "button" && slot == "command" || kind == "switch"
}
func set(keys ...string) map[string]bool {
	out := map[string]bool{}
	for _, k := range keys {
		out[k] = true
	}
	return out
}
func finite(v float64) bool    { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validRotation(v int) bool { return v == 0 || v == 90 || v == 180 || v == 270 }

func validateProps(kind string, props map[string]json.RawMessage, strict bool) error {
	stringKeys := map[string]bool{"text": true, "color": true, "align": true, "shape": true, "fill": true, "stroke": true, "label": true, "trueLabel": true, "falseLabel": true, "trueColor": true, "falseColor": true, "onLabel": true, "offLabel": true, "fontWeight": true}
	for key, raw := range props {
		if stringKeys[key] {
			var v string
			maxLength := 256
			if key == "text" {
				maxLength = 1000
			}
			if json.Unmarshal(raw, &v) != nil || len(v) > maxLength || strings.IndexByte(v, 0) >= 0 {
				return ErrInvalid
			}
			if key == "color" || key == "fill" || key == "stroke" || key == "trueColor" || key == "falseColor" {
				if !validColor(v) {
					return ErrInvalid
				}
			}
			if key == "align" && v != "left" && v != "center" && v != "right" {
				return ErrInvalid
			}
			if key == "fontWeight" && v != "normal" && v != "medium" && v != "bold" {
				return ErrInvalid
			}
			continue
		}
		if key == "showUnit" {
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return ErrInvalid
			}
			continue
		}
		if key == "fontSize" || key == "strokeWidth" || key == "precision" || key == "min" || key == "max" {
			if key == "precision" && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var v float64
			if json.Unmarshal(raw, &v) != nil || !finite(v) {
				return ErrInvalid
			}
			if key == "precision" && (v < 0 || v > 12 || math.Trunc(v) != v) || key == "fontSize" && (v < 8 || v > 128) || key == "strokeWidth" && (v < 0 || v > 32) {
				return ErrInvalid
			}
		}
	}
	if v := props["shape"]; len(v) > 0 {
		var s string
		_ = json.Unmarshal(v, &s)
		if s != "rectangle" && s != "ellipse" && s != "line" {
			return ErrInvalid
		}
	}
	if strict {
		required := map[string][]string{
			"text":          {"text"},
			"shape":         {"shape"},
			"value-display": {"label", "showUnit"},
			"indicator":     {"trueLabel", "falseLabel", "trueColor", "falseColor"},
			"gauge":         {"label", "min", "max", "showUnit"},
			"button":        {"label"},
			"switch":        {"label", "onLabel", "offLabel"},
		}
		for _, key := range required[kind] {
			raw, ok := props[key]
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return ErrInvalid
			}
			if stringKeys[key] {
				var value string
				if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
					return ErrInvalid
				}
			}
		}
	}
	if kind == "gauge" && len(props["min"]) > 0 && len(props["max"]) > 0 {
		var min, max float64
		_ = json.Unmarshal(props["min"], &min)
		_ = json.Unmarshal(props["max"], &max)
		if max <= min {
			return ErrInvalid
		}
	}
	return nil
}

func validColor(v string) bool {
	return staticColors[v]
}
