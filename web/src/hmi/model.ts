import type { DataPointValueType } from "@/types/datapoint";

export type HmiNodeType =
  | "text"
  | "shape"
  | "value-display"
  | "indicator"
  | "gauge"
  | "button"
  | "switch";

export type HmiCanvasSize = { width: number; height: number };

export const HMI_CANVAS_PRESETS: readonly HmiCanvasSize[] = [
  { width: 1920, height: 1080 },
  { width: 1366, height: 768 },
  { width: 1280, height: 720 },
];

export const DEFAULT_HMI_CANVAS = HMI_CANVAS_PRESETS[0];

export type HmiNodeGeometry = {
  nodeId: string;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: 0 | 90 | 180 | 270;
  zIndex: number;
};

export type TextNodeProps = {
  text: string;
  color: string;
  fontSize: number;
  fontWeight: "normal" | "medium" | "bold";
  align: "left" | "center" | "right";
};

export type ShapeNodeProps = {
  shape: "rectangle" | "ellipse" | "line";
  fill: string;
  stroke: string;
  strokeWidth: number;
};

export type ValueDisplayNodeProps = {
  label: string;
  precision: number | null;
  showUnit: boolean;
  color: string;
};

export type IndicatorNodeProps = {
  label: string;
  trueLabel: string;
  falseLabel: string;
  trueColor: string;
  falseColor: string;
};

export type GaugeNodeProps = {
  label: string;
  min: number;
  max: number;
  precision: number | null;
  showUnit: boolean;
};

export type ButtonNodeProps = {
  label: string;
};

export type SwitchNodeProps = {
  label: string;
  onLabel: string;
  offLabel: string;
};

export type HmiNodeProps =
  | TextNodeProps
  | ShapeNodeProps
  | ValueDisplayNodeProps
  | IndicatorNodeProps
  | GaugeNodeProps
  | ButtonNodeProps
  | SwitchNodeProps;

export type DataPointBinding = {
  kind: "datapoint";
  deviceId: string;
  pointKey: string;
};

export type CommandBinding = {
  kind: "command";
  deviceId: string;
  name: string;
  args: Record<string, unknown>;
  ttlSeconds: number;
  confirmation: { required: boolean; message?: string };
};

export type HmiBinding = DataPointBinding | CommandBinding;
export type HmiBindings = Record<string, HmiBinding>;

type HmiPropsByType = {
  text: TextNodeProps;
  shape: ShapeNodeProps;
  "value-display": ValueDisplayNodeProps;
  indicator: IndicatorNodeProps;
  gauge: GaugeNodeProps;
  button: ButtonNodeProps;
  switch: SwitchNodeProps;
};

export type HmiNode = {
  [T in HmiNodeType]: HmiNodeGeometry & { type: T; props: HmiPropsByType[T]; bindings: HmiBindings };
}[HmiNodeType];

export type HmiDocument = {
  schema: "hmi-page/v1";
  canvas: HmiCanvasSize;
  nodes: HmiNode[];
};

export const COMPONENT_DEFAULTS: Record<HmiNodeType, HmiNodeProps> = {
  text: {
    text: "文本",
    color: "#1f2937",
    fontSize: 18,
    fontWeight: "normal",
    align: "left",
  },
  shape: {
    shape: "rectangle",
    fill: "#e5e7eb",
    stroke: "#6b7280",
    strokeWidth: 1,
  },
  "value-display": { label: "数据点", precision: null, showUnit: true, color: "#1f2937" },
  indicator: {
    label: "状态",
    trueLabel: "运行",
    falseLabel: "停止",
    trueColor: "#16a34a",
    falseColor: "#9ca3af",
  },
  gauge: { label: "仪表", min: 0, max: 100, precision: null, showUnit: true },
  button: { label: "执行" },
  switch: { label: "开关", onLabel: "开", offLabel: "关" },
};

const DEFAULT_GEOMETRY: Omit<HmiNodeGeometry, "nodeId" | "x" | "y"> = {
  width: 180,
  height: 64,
  rotation: 0,
  zIndex: 1,
};

export function createHmiNode(
  type: HmiNodeType,
  position: { x: number; y: number } = { x: 80, y: 80 },
): HmiNode {
  const nodeId = globalThis.crypto?.randomUUID?.() ?? createFallbackId();
  return {
    ...DEFAULT_GEOMETRY,
    nodeId,
    ...position,
    type,
    props: structuredClone(COMPONENT_DEFAULTS[type]),
    bindings: {},
  } as HmiNode;
}

export function cloneHmiNode(node: HmiNode, offset = 24): HmiNode {
  return {
    ...structuredClone(node),
    nodeId: globalThis.crypto?.randomUUID?.() ?? createFallbackId(),
    x: node.x + offset,
    y: node.y + offset,
    zIndex: node.zIndex + 1,
  };
}

function createFallbackId() {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (character) => {
    const random = Math.floor(Math.random() * 16);
    return (character === "x" ? random : (random & 0x3) | 0x8).toString(16);
  });
}

export function isBindingCompatible(
  slot: string,
  valueType: DataPointValueType,
) {
  if (slot === "value" && (valueType === "NUMBER" || valueType === "BOOLEAN")) return true;
  if (slot === "state" && valueType === "BOOLEAN") return true;
  return false;
}
