// Package datapoint owns the Cloud DataPoint configuration and CurrentValue
// projection. Raw collector fields are confined to the mapping adapter.
package datapoint

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
)

type ValueType string

const (
	ValueTypeNumber  ValueType = "NUMBER"
	ValueTypeBoolean ValueType = "BOOLEAN"
)

type Quality string

const (
	QualityNoData Quality = "NO_DATA"
	QualityGood   Quality = "GOOD"
	QualityBad    Quality = "BAD"
)

type Encoding string

const (
	EncodingUINT16     Encoding = "UINT16"
	EncodingINT16      Encoding = "INT16"
	EncodingUINT32     Encoding = "UINT32"
	EncodingINT32      Encoding = "INT32"
	EncodingFLOAT32    Encoding = "FLOAT32"
	EncodingBooleanBit Encoding = "BOOLEAN_BIT"
)

type ByteOrder string

const (
	ByteOrderBigEndian    ByteOrder = "BIG_ENDIAN"
	ByteOrderLittleEndian ByteOrder = "LITTLE_ENDIAN"
)

type WordOrder string

const (
	WordOrderHighLow WordOrder = "HIGH_LOW"
	WordOrderLowHigh WordOrder = "LOW_HIGH"
)

var (
	ErrNotFound      = errors.New("数据不存在")
	ErrInvalid       = errors.New("参数错误")
	ErrConflict      = errors.New("数据已存在")
	ErrImmutable     = errors.New("pointKey 和 valueType 创建后不可修改")
	ErrUnknownDevice = errors.New("来源 Device 不存在")
)

var pointKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type DataPoint struct {
	DataPointID      string        `gorm:"column:data_point_id;primaryKey" json:"dataPointId"`
	DeviceID         string        `gorm:"column:device_id" json:"deviceId"`
	PointKey         string        `gorm:"column:point_key" json:"pointKey"`
	Name             string        `gorm:"column:name" json:"name"`
	ValueType        ValueType     `gorm:"column:value_type" json:"valueType"`
	Unit             *string       `gorm:"column:unit" json:"unit"`
	Precision        *int          `gorm:"column:precision" json:"precision"`
	Enabled          bool          `gorm:"column:enabled" json:"enabled"`
	ConfigGeneration int64         `gorm:"column:config_generation" json:"-"`
	EffectiveAt      time.Time     `gorm:"column:effective_at" json:"-"`
	Mapping          SourceMapping `gorm:"-" json:"mapping"`
	Current          CurrentValue  `gorm:"-" json:"currentValue"`
}

func (DataPoint) TableName() string { return "data_point" }

type SourceMapping struct {
	DataPointID  string     `gorm:"column:data_point_id;primaryKey" json:"dataPointId,omitempty"`
	SourceType   string     `gorm:"column:source_type" json:"sourceType"`
	FunctionCode int        `gorm:"column:function_code" json:"functionCode"`
	Address      int        `gorm:"column:address" json:"address"`
	Encoding     Encoding   `gorm:"column:encoding" json:"encoding"`
	WordOrder    *WordOrder `gorm:"column:word_order" json:"wordOrder"`
	ByteOrder    ByteOrder  `gorm:"column:byte_order" json:"byteOrder"`
	BitIndex     *int       `gorm:"column:bit_index" json:"bitIndex"`
	Scale        float64    `gorm:"column:scale" json:"scale"`
	Offset       float64    `gorm:"column:offset" json:"offset"`
}

func (SourceMapping) TableName() string { return "source_mapping" }

type CurrentValue struct {
	DataPointID     string     `gorm:"column:data_point_id;primaryKey" json:"dataPointId,omitempty"`
	ValueType       ValueType  `gorm:"column:value_type" json:"-"`
	NumberValue     *float64   `gorm:"column:number_value" json:"-"`
	BooleanValue    *bool      `gorm:"column:boolean_value" json:"-"`
	Quality         Quality    `gorm:"column:quality" json:"quality"`
	SourceTimestamp *time.Time `gorm:"column:source_timestamp" json:"sourceTimestamp"`
	ObservedAt      *time.Time `gorm:"column:observed_at" json:"observedAt"`
	Revision        int64      `gorm:"column:revision" json:"revision"`
}

func (CurrentValue) TableName() string { return "current_value" }

func (c CurrentValue) Value() any {
	if c.ValueType == ValueTypeNumber && c.NumberValue != nil {
		return *c.NumberValue
	}
	if c.ValueType == ValueTypeBoolean && c.BooleanValue != nil {
		return *c.BooleanValue
	}
	return nil
}

type SemanticValue struct {
	Value           any
	Quality         Quality
	SourceTimestamp *time.Time
}

type RawRegister struct {
	Address *int    `json:"address"`
	Value   *uint16 `json:"value"`
}
type RawBlock struct {
	FunctionCode  int           `json:"functionCode"`
	Valid         bool          `json:"valid"`
	LastSuccessAt *time.Time    `json:"lastSuccessAt"`
	Registers     []RawRegister `json:"registers"`
}
type RawSnapshot struct {
	CommunicationStatus string     `json:"communicationStatus"`
	Blocks              []RawBlock `json:"blocks"`
}
type RawObservation struct {
	EdgeID         string
	SourceDeviceID string
	ReceivedAt     time.Time
	Snapshot       RawSnapshot
}

type PageQuery struct {
	Page      int
	PageSize  int
	DeviceID  string
	PointKey  string
	ValueType *ValueType
	Enabled   *bool
	Quality   *Quality
}
type Page struct {
	Records  []DataPoint `json:"records"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

type CreateInput struct {
	DeviceID, PointKey, Name string
	ValueType                ValueType
	Unit                     *string
	Precision                *int
	Enabled                  bool
	Mapping                  SourceMapping
}
type UpdateInput struct {
	Name      string
	Unit      *string
	Precision *int
	Mapping   SourceMapping
}

func validValueType(v ValueType) bool { return v == ValueTypeNumber || v == ValueTypeBoolean }
func validQuality(v Quality) bool     { return v == QualityNoData || v == QualityGood || v == QualityBad }
func validEncoding(v Encoding) bool {
	switch v {
	case EncodingUINT16, EncodingINT16, EncodingUINT32, EncodingINT32, EncodingFLOAT32, EncodingBooleanBit:
		return true
	}
	return false
}
func is32(v Encoding) bool      { return v == EncodingUINT32 || v == EncodingINT32 || v == EncodingFLOAT32 }
func isNumeric(v Encoding) bool { return v != EncodingBooleanBit }

func ValidateMapping(valueType ValueType, m SourceMapping) error {
	if !validValueType(valueType) || m.SourceType != "MODBUS_REGISTER" || (m.FunctionCode != 3 && m.FunctionCode != 4) || m.Address < 0 || m.Address > 65535 || !validEncoding(m.Encoding) || (m.ByteOrder != ByteOrderBigEndian && m.ByteOrder != ByteOrderLittleEndian) || math.IsNaN(m.Scale) || math.IsInf(m.Scale, 0) || math.IsNaN(m.Offset) || math.IsInf(m.Offset, 0) {
		return ErrInvalid
	}
	if valueType == ValueTypeNumber && !isNumeric(m.Encoding) || valueType == ValueTypeBoolean && m.Encoding != EncodingBooleanBit {
		return ErrInvalid
	}
	if m.Encoding == EncodingBooleanBit {
		if m.BitIndex == nil || *m.BitIndex < 0 || *m.BitIndex > 15 || m.WordOrder != nil {
			return ErrInvalid
		}
	} else if m.BitIndex != nil {
		return ErrInvalid
	} else if is32(m.Encoding) {
		if m.Address > 65534 || m.WordOrder == nil || (*m.WordOrder != WordOrderHighLow && *m.WordOrder != WordOrderLowHigh) {
			return ErrInvalid
		}
	} else if m.WordOrder != nil {
		return ErrInvalid
	}
	return nil
}

func ValidatePointKey(value string) bool { return pointKeyPattern.MatchString(value) }
func ValidateCreate(in CreateInput) error {
	if strings.TrimSpace(in.DeviceID) == "" || !ValidatePointKey(in.PointKey) || strings.TrimSpace(in.Name) == "" || in.Precision != nil && (*in.Precision < 0 || *in.Precision > 12) {
		return ErrInvalid
	}
	return ValidateMapping(in.ValueType, in.Mapping)
}

func sameMapping(a, b SourceMapping) bool {
	return a.SourceType == b.SourceType && a.FunctionCode == b.FunctionCode && a.Address == b.Address && a.Encoding == b.Encoding && a.ByteOrder == b.ByteOrder && a.Scale == b.Scale && a.Offset == b.Offset && ptrEqualWord(a.WordOrder, b.WordOrder) && ptrEqualInt(a.BitIndex, b.BitIndex)
}
func ptrEqualInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func ptrEqualWord(a, b *WordOrder) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func mappingChanged(a, b SourceMapping) bool { return !sameMapping(a, b) }

func project(mapping SourceMapping, raw RawSnapshot) SemanticValue {
	bad := SemanticValue{Quality: QualityBad}
	for _, block := range raw.Blocks {
		if block.FunctionCode != mapping.FunctionCode {
			continue
		}
		if !blockContains(block, mapping.Address) {
			continue
		}
		if !block.Valid || block.LastSuccessAt == nil {
			return bad
		}
		first, ok := registerAt(block, mapping.Address)
		if !ok || first.Value == nil {
			return bad
		}
		if mapping.Encoding == EncodingBooleanBit {
			return SemanticValue{Value: ((*first.Value >> uint(*mapping.BitIndex)) & 1) == 1, Quality: QualityGood, SourceTimestamp: cloneTime(block.LastSuccessAt)}
		}
		var decoded float64
		switch mapping.Encoding {
		case EncodingUINT16:
			decoded = float64(order16(*first.Value, mapping.ByteOrder))
		case EncodingINT16:
			decoded = float64(int16(order16(*first.Value, mapping.ByteOrder)))
		case EncodingUINT32, EncodingINT32, EncodingFLOAT32:
			second, ok := registerAt(block, mapping.Address+1)
			if !ok || second.Value == nil {
				return bad
			}
			firstWord, secondWord := order16(*first.Value, mapping.ByteOrder), order16(*second.Value, mapping.ByteOrder)
			if *mapping.WordOrder == WordOrderLowHigh {
				firstWord, secondWord = secondWord, firstWord
			}
			bits := uint32(firstWord)<<16 | uint32(secondWord)
			switch mapping.Encoding {
			case EncodingUINT32:
				decoded = float64(bits)
			case EncodingINT32:
				decoded = float64(int32(bits))
			case EncodingFLOAT32:
				decoded = float64(math.Float32frombits(bits))
			}
		}
		value := decoded*mapping.Scale + mapping.Offset
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return bad
		}
		return SemanticValue{Value: value, Quality: QualityGood, SourceTimestamp: cloneTime(block.LastSuccessAt)}
	}
	return bad
}

func blockContains(block RawBlock, address int) bool {
	for _, r := range block.Registers {
		if r.Address != nil && *r.Address == address {
			return true
		}
	}
	return false
}
func registerAt(block RawBlock, address int) (RawRegister, bool) {
	for _, r := range block.Registers {
		if r.Address != nil && *r.Address == address {
			return r, true
		}
	}
	return RawRegister{}, false
}
func order16(v uint16, order ByteOrder) uint16 {
	if order == ByteOrderLittleEndian {
		return (v << 8) | (v >> 8)
	}
	return v
}
func cloneTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	x := v.UTC()
	return &x
}
