import type { ReactNode } from "react";
import type { DataPointRecord, DataPointValueType } from "@/types/datapoint";
import type { DataPointCurrentValue } from "@/types/datapoint";
import type {
  CommandBinding,
  GaugeNodeProps,
  HmiBinding,
  HmiNode,
  HmiNodeType,
} from "@/hmi/model";

export type HmiCommandFeedback = {
  status: "idle" | "submitting" | "pending" | "accepted" | "succeeded" | "failed";
  message?: string;
};

export type HmiComponentRendererProps = {
  node: HmiNode;
  mode: "editor" | "runtime";
  value?: DataPointCurrentValue;
  metadata?: Pick<DataPointRecord, "name" | "valueType" | "unit" | "precision">;
  onCommand?: (binding: CommandBinding) => Promise<void>;
  commandState?: HmiCommandFeedback;
};

export type HmiBindingSlot = {
  name: string;
  kind: HmiBinding["kind"];
  valueTypes?: DataPointValueType[];
  required: boolean;
};

export type HmiComponentDefinition = {
  type: HmiNodeType;
  label: string;
  category: "static" | "display" | "control";
  bindingSlots: HmiBindingSlot[];
  requiredProps: string[];
  defaultWidth: number;
  defaultHeight: number;
};

export const HMI_COMPONENTS: Record<HmiNodeType, HmiComponentDefinition> = {
  text: { type: "text", label: "文本", category: "static", bindingSlots: [], requiredProps: ["text"], defaultWidth: 180, defaultHeight: 48 },
  shape: { type: "shape", label: "形状", category: "static", bindingSlots: [], requiredProps: ["shape"], defaultWidth: 160, defaultHeight: 100 },
  "value-display": { type: "value-display", label: "数值显示", category: "display", bindingSlots: [{ name: "value", kind: "datapoint", valueTypes: ["NUMBER", "BOOLEAN"], required: true }], requiredProps: ["label", "showUnit"], defaultWidth: 220, defaultHeight: 72 },
  indicator: { type: "indicator", label: "状态指示", category: "display", bindingSlots: [{ name: "value", kind: "datapoint", valueTypes: ["BOOLEAN"], required: true }], requiredProps: ["trueLabel", "falseLabel"], defaultWidth: 180, defaultHeight: 72 },
  gauge: { type: "gauge", label: "仪表", category: "display", bindingSlots: [{ name: "value", kind: "datapoint", valueTypes: ["NUMBER"], required: true }], requiredProps: ["min", "max"], defaultWidth: 220, defaultHeight: 180 },
  button: { type: "button", label: "按钮", category: "control", bindingSlots: [{ name: "command", kind: "command", required: true }], requiredProps: ["label"], defaultWidth: 160, defaultHeight: 52 },
  switch: { type: "switch", label: "开关", category: "control", bindingSlots: [{ name: "state", kind: "datapoint", valueTypes: ["BOOLEAN"], required: true }, { name: "onCommand", kind: "command", required: true }, { name: "offCommand", kind: "command", required: true }], requiredProps: ["onLabel", "offLabel"], defaultWidth: 200, defaultHeight: 64 },
};

export const HMI_STATIC_COLORS = ["#1f2937", "#ffffff", "#6b7280", "#9ca3af", "#e5e7eb", "#1677ff", "#16a34a", "#d97706", "#dc2626", "#0891b2"] as const;

function safeColor(color: string, fallback = "#1f2937") {
  return (HMI_STATIC_COLORS as readonly string[]).includes(color) ? color : fallback;
}

function pointText(value?: DataPointCurrentValue, metadata?: HmiComponentRendererProps["metadata"], precisionOverride?: number | null) {
  if (!value || value.quality === "NO_DATA" || value.value === null) return "--";
  if (typeof value.value === "boolean") return value.value ? "是" : "否";
  const precision = precisionOverride ?? metadata?.precision ?? 2;
  return value.value.toLocaleString(undefined, { maximumFractionDigits: Math.max(0, Math.min(6, precision)) });
}

function QualityMark({ value }: { value?: DataPointCurrentValue }) {
  if (!value || value.quality === "NO_DATA") return <span className="text-text-tertiary">无数据</span>;
  if (value.quality === "BAD") return <span className="text-error">数据无效</span>;
  return null;
}

function commandBinding(node: HmiNode, slot: string): CommandBinding | undefined {
  const binding = node.bindings[slot];
  return binding?.kind === "command" ? binding : undefined;
}

function WidgetFrame({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`h-full w-full overflow-hidden rounded-md border border-border bg-surface p-3 text-sm text-text-primary ${className}`}>{children}</div>;
}

export function HmiComponentRenderer({ node, mode, value, metadata, onCommand, commandState }: HmiComponentRendererProps) {
  const displayOnly = mode === "editor";
  switch (node.type) {
    case "text":
      return <div className="flex h-full w-full items-center overflow-hidden whitespace-pre-wrap" style={{ color: safeColor(node.props.color), fontSize: Math.max(10, Math.min(72, node.props.fontSize)), fontWeight: node.props.fontWeight, textAlign: node.props.align }}>{node.props.text}</div>;
    case "shape": {
      const common = { backgroundColor: safeColor(node.props.fill, "#e5e7eb"), borderColor: safeColor(node.props.stroke, "#6b7280"), borderWidth: Math.max(0, Math.min(12, node.props.strokeWidth)), borderStyle: "solid" };
      if (node.props.shape === "line") return <div className="flex h-full w-full items-center"><div className="w-full" style={{ borderTopWidth: Math.max(1, common.borderWidth), borderTopStyle: "solid", borderColor: common.borderColor }} /></div>;
      return <div className="h-full w-full" style={{ ...common, borderRadius: node.props.shape === "ellipse" ? "9999px" : "4px" }} />;
    }
    case "value-display":
      return <WidgetFrame><div className="text-xs text-text-tertiary">{node.props.label || metadata?.name || "数据点"}</div><div className="mt-1 text-2xl font-semibold tabular-nums" style={{ color: safeColor(node.props.color) }}>{pointText(value, metadata, node.props.precision)}<span className="ml-1 text-sm font-normal">{node.props.showUnit ? metadata?.unit ?? "" : ""}</span></div><div className="mt-1"><QualityMark value={value} /></div></WidgetFrame>;
    case "indicator": {
      const goodValue = typeof value?.value === "boolean" ? value.value : false;
      const stateColor = safeColor(goodValue ? node.props.trueColor : node.props.falseColor, goodValue ? "#16a34a" : "#9ca3af");
      return <WidgetFrame><div className="text-xs text-text-tertiary">{node.props.label || metadata?.name || "状态"}</div><div className="mt-2 flex items-center gap-2"><span className="h-3 w-3 rounded-full" style={{ backgroundColor: value?.quality === "NO_DATA" || !value ? "#9ca3af" : stateColor }} /><span className="font-medium">{value?.quality === "NO_DATA" || !value ? "--" : (goodValue ? node.props.trueLabel : node.props.falseLabel)}</span></div><div className="mt-1"><QualityMark value={value} /></div></WidgetFrame>;
    }
    case "gauge": {
      const props = node.props as GaugeNodeProps;
      const number = value?.quality !== "NO_DATA" && typeof value?.value === "number" ? value.value : null;
      const min = Number.isFinite(props.min) ? props.min : 0;
      const max = Number.isFinite(props.max) && props.max > min ? props.max : min + 1;
      const percent = number === null ? 0 : Math.max(0, Math.min(100, ((number - min) / (max - min)) * 100));
      return <WidgetFrame className="flex flex-col justify-between"><div className="text-xs text-text-tertiary">{props.label || metadata?.name || "仪表"}</div><div className="py-2"><div className="h-3 overflow-hidden rounded-full bg-neutral-background"><div className="h-full rounded-full bg-primary" style={{ width: `${percent}%` }} /></div><div className="mt-2 flex justify-between text-xs text-text-tertiary"><span>{min}</span><span>{max}</span></div></div><div className="text-center text-lg font-semibold tabular-nums">{number === null ? "--" : pointText(value, metadata, props.precision)}{props.showUnit ? metadata?.unit ?? "" : ""}<div className="text-xs font-normal"><QualityMark value={value} /></div></div></WidgetFrame>;
    }
    case "button": {
      const binding = commandBinding(node, "command");
      const statusText: Record<HmiCommandFeedback["status"], string> = { idle: "", submitting: "提交中", pending: "等待设备", accepted: "已接收", succeeded: "执行成功", failed: "执行失败" };
      const stateText = commandState?.message ?? (commandState ? statusText[commandState.status] : "");
      return <button type="button" disabled={displayOnly || !onCommand || !binding || commandState?.status === "submitting" || commandState?.status === "pending" || commandState?.status === "accepted"} onClick={() => binding && void onCommand?.(binding)} className="h-full w-full rounded-md border border-primary bg-primary px-3 text-sm font-medium text-white disabled:cursor-default disabled:opacity-80">{node.props.label}{stateText && mode === "runtime" ? <span className="ml-2 text-xs">{stateText}</span> : null}</button>;
    }
    case "switch": {
      const lastKnown = typeof value?.value === "boolean" ? value.value : null;
      const current = value?.quality === "GOOD" ? lastKnown : null;
      const binding = current === null ? undefined : commandBinding(node, current ? "offCommand" : "onCommand");
      const commandMessage = commandState?.message ?? (commandState?.status === "failed" ? "命令不可执行" : "");
      return <WidgetFrame><div className="flex items-center justify-between"><span>{node.props.label}</span><button type="button" disabled={displayOnly || current === null || !onCommand || !binding || commandState?.status === "submitting" || commandState?.status === "pending" || commandState?.status === "accepted"} onClick={() => binding && void onCommand?.(binding)} aria-pressed={lastKnown === true} className={`rounded-full px-3 py-1 text-xs text-white disabled:opacity-60 ${lastKnown ? "bg-success" : "bg-neutral-500"}`}>{lastKnown === null ? "--" : lastKnown ? node.props.onLabel : node.props.offLabel}</button></div><div className="mt-1 flex justify-between text-xs"><QualityMark value={value} /><span className={commandState?.status === "failed" ? "text-error" : "text-text-tertiary"}>{mode === "runtime" ? commandMessage : ""}</span></div></WidgetFrame>;
    }
  }
}

export function validateHmiNodeForPublish(node: HmiNode) {
  const definition = HMI_COMPONENTS[node.type];
  const errors: string[] = [];
  if (!definition) return ["组件类型不受支持"];
  for (const prop of definition.requiredProps) {
    const value = (node.props as unknown as Record<string, unknown>)[prop];
    if (value === undefined || value === null || value === "") errors.push(`缺少属性 ${prop}`);
  }
  for (const slot of definition.bindingSlots) {
    const binding = node.bindings[slot.name];
    if (!binding && slot.required) errors.push(`缺少绑定 ${slot.name}`);
    if (binding && binding.kind !== slot.kind) errors.push(`绑定 ${slot.name} 类型不匹配`);
  }
  if (node.type === "gauge" && node.props.min >= node.props.max) errors.push("仪表最大值必须大于最小值");
  return errors;
}

export function areBindingTypesCompatible(componentType: HmiNodeType, slotName: string, type: DataPointValueType) {
  const slot = HMI_COMPONENTS[componentType].bindingSlots.find((candidate) => candidate.name === slotName);
  return slot?.valueTypes?.includes(type) ?? false;
}
