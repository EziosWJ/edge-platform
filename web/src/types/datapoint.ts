import type { ApiPageRequest } from "@/types/api";

export type DataPointValueType = "NUMBER" | "BOOLEAN";
export type DataPointQuality = "NO_DATA" | "GOOD" | "BAD";
export type DataPointEncoding = "UINT16" | "INT16" | "UINT32" | "INT32" | "FLOAT32" | "BOOLEAN_BIT";
export type DataPointMapping = {
  sourceType: "MODBUS_REGISTER";
  functionCode: 3 | 4;
  address: number;
  encoding: DataPointEncoding;
  wordOrder: "HIGH_LOW" | "LOW_HIGH" | null;
  byteOrder: "BIG_ENDIAN" | "LITTLE_ENDIAN";
  bitIndex: number | null;
  scale: number;
  offset: number;
};
export type DataPointCurrentValue = {
  value: number | boolean | null;
  quality: DataPointQuality;
  sourceTimestamp: string | null;
  observedAt: string | null;
  revision: number;
};
export type DataPointRecord = {
  dataPointId: string;
  deviceId: string;
  pointKey: string;
  name: string;
  valueType: DataPointValueType;
  unit: string | null;
  precision: number | null;
  enabled: boolean;
  currentValue: DataPointCurrentValue;
  mapping?: DataPointMapping;
};
export type DataPointPageQuery = ApiPageRequest & {
  deviceId?: string;
  pointKey?: string;
  valueType?: DataPointValueType;
  enabled?: boolean;
  quality?: DataPointQuality;
};
