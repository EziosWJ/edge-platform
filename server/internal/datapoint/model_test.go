package datapoint

import (
	"math"
	"testing"
	"time"
)

func mapping(valueType ValueType, encoding Encoding) SourceMapping {
	word := WordOrderHighLow
	return SourceMapping{SourceType: "MODBUS_REGISTER", FunctionCode: 3, Address: 100, Encoding: encoding, ByteOrder: ByteOrderBigEndian, WordOrder: &word, Scale: 1, Offset: 0}
}

func TestValidateMappingCompatibilityAndKeys(t *testing.T) {
	if !ValidatePointKey("current_a") || ValidatePointKey("Current-A") || ValidatePointKey("1x") {
		t.Fatal("point key validation failed")
	}
	if err := ValidateMapping(ValueTypeNumber, mapping(ValueTypeNumber, EncodingUINT32)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMapping(ValueTypeBoolean, mapping(ValueTypeBoolean, EncodingBooleanBit)); err == nil {
		t.Fatal("boolean without bit must fail")
	}
	bit := 3
	m := mapping(ValueTypeBoolean, EncodingBooleanBit)
	m.WordOrder = nil
	m.BitIndex = &bit
	if err := ValidateMapping(ValueTypeBoolean, m); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMapping(ValueTypeNumber, mapping(ValueTypeNumber, EncodingBooleanBit)); err == nil {
		t.Fatal("numeric boolean mapping must fail")
	}
}

func TestDecodeIntegerFloatBooleanAndTransform(t *testing.T) {
	success := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	first, second := 0x4148, 0x0000 // 12.5f
	value1, value2 := uint16(first), uint16(second)
	raw := RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: true, LastSuccessAt: &success, Registers: []RawRegister{{Address: intPtr(100), Value: &value1}, {Address: intPtr(101), Value: &value2}, {Address: intPtr(102), Value: uint16Ptr(0x8001)}}}}}
	f := mapping(ValueTypeNumber, EncodingFLOAT32)
	got := project(f, raw)
	if got.Quality != QualityGood || got.Value.(float64) != 12.5 {
		t.Fatalf("float = %+v", got)
	}
	i := mapping(ValueTypeNumber, EncodingINT16)
	i.Address = 102
	i.WordOrder = nil
	got = project(i, raw)
	if got.Value.(float64) != -32767 {
		t.Fatalf("int16 = %+v", got)
	}
	b := mapping(ValueTypeBoolean, EncodingBooleanBit)
	b.WordOrder = nil
	bit := 15
	b.BitIndex = &bit
	b.Address = 102
	got = project(b, raw)
	if got.Value != true {
		t.Fatalf("bit = %+v", got)
	}
	f.Scale = 2
	f.Offset = 1
	got = project(f, raw)
	if got.Value.(float64) != 26 {
		t.Fatalf("transform = %+v", got)
	}
	word := WordOrderLowHigh
	u := mapping(ValueTypeNumber, EncodingUINT32)
	u.WordOrder = &word
	u.Address = 100
	lowHigh := RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: true, LastSuccessAt: &success, Registers: []RawRegister{{Address: intPtr(100), Value: uint16Ptr(2)}, {Address: intPtr(101), Value: uint16Ptr(1)}}}}}
	got = project(u, lowHigh)
	if got.Value.(float64) != float64(uint32(1)<<16|2) {
		t.Fatalf("low-high word order = %+v", got)
	}
	u.ByteOrder = ByteOrderLittleEndian
	got = project(u, RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: true, LastSuccessAt: &success, Registers: []RawRegister{{Address: intPtr(100), Value: uint16Ptr(0x0201)}, {Address: intPtr(101), Value: uint16Ptr(0x0403)}}}}})
	if got.Quality != QualityGood {
		t.Fatalf("little-endian decode = %+v", got)
	}
}

func TestDecodeBadAndRawValidation(t *testing.T) {
	if _, err := ParseRawSnapshot([]byte(`{"communicationStatus":"ONLINE","blocks": [{"functionCode":3,"valid":true,"lastSuccessAt":null,"lastAttemptAt":null,"registers":[{"address":1,"value":1},{"address":1,"value":2}]}]}`)); err == nil {
		t.Fatal("duplicate register must invalidate complete raw")
	}
	m := mapping(ValueTypeNumber, EncodingUINT16)
	raw := RawSnapshot{CommunicationStatus: "ONLINE", Blocks: []RawBlock{{FunctionCode: 3, Valid: false, Registers: []RawRegister{{Address: intPtr(100), Value: uint16Ptr(1)}}}}}
	got := project(m, raw)
	if got.Quality != QualityBad {
		t.Fatal(got)
	}
	if math.IsNaN(math.NaN()) == false {
		t.Fatal("math sanity")
	}
}

func intPtr(v int) *int          { return &v }
func uint16Ptr(v uint16) *uint16 { return &v }
