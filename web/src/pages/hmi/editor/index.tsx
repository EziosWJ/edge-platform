import { Graph } from "@antv/x6";
import { Copy, Layers2, Plus, Redo2, RotateCw, Save, Trash2, Undo2, Upload } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { PageHeader } from "@/components/common/page-header";
import { EmptyState } from "@/components/common/empty-state";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/components/common/toast-store";
import { getErrorMessage, isApiError } from "@/lib/api-error";
import { hasPermission } from "@/lib/permission";
import { HMI_COMPONENTS, HMI_STATIC_COLORS } from "@/hmi/registry";
import { cloneHmiNode, createHmiNode, HMI_CANVAS_PRESETS } from "@/hmi/model";
import type { CommandBinding, DataPointBinding, HmiCanvasSize, HmiDocument, HmiNode, HmiNodeType } from "@/hmi/model";
import { getDataPointPage } from "@/api/datapoint";
import { getDevicePage } from "@/api/device";
import type { DataPointRecord } from "@/types/datapoint";
import type { DeviceRecord } from "@/types/device";
import { getHmiPage, publishHmiPage, saveHmiDraft, type HmiPageDetail } from "@/hmi/editor/api";
import { canonicalNodeToX6, documentToX6Graph, nodeToCanonical, x6GraphToCanonical } from "@/hmi/editor/x6-adapter";

const EMPTY_DOCUMENT: HmiDocument = { schema: "hmi-page/v1", canvas: { width: 1920, height: 1080 }, nodes: [] };
const SNAPSHOT_LIMIT = 100;
const COMPONENT_ORDER: HmiNodeType[] = ["text", "shape", "value-display", "indicator", "gauge", "button", "switch"];

function fingerprint(document: HmiDocument) { return JSON.stringify(document); }
function maxZIndex(nodes: HmiNode[]) { return nodes.reduce((maximum, node) => Math.max(maximum, node.zIndex), 0); }

export function HmiEditorPage() {
  const { pageId = "" } = useParams();
  const navigate = useNavigate();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const graphRef = useRef<Graph | null>(null);
  const documentRef = useRef<HmiDocument>(EMPTY_DOCUMENT);
  const historyPastRef = useRef<HmiDocument[]>([]);
  const historyFutureRef = useRef<HmiDocument[]>([]);
  const savedFingerprintRef = useRef(fingerprint(EMPTY_DOCUMENT));
  const applyingDocumentRef = useRef(false);
  const clipboardRef = useRef<HmiNode | null>(null);
  const [page, setPage] = useState<HmiPageDetail | null>(null);
  const [document, setDocument] = useState<HmiDocument>(EMPTY_DOCUMENT);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [error, setError] = useState("");
  const [dirty, setDirty] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [scale, setScale] = useState(1);
  const [devices, setDevices] = useState<DeviceRecord[]>([]);
  const [deviceLoading, setDeviceLoading] = useState(false);
  const [dataPoints, setDataPoints] = useState<DataPointRecord[]>([]);
  const [activeDeviceId, setActiveDeviceId] = useState("");
  const canPublish = hasPermission("hmi:publish");

  const selectedNode = useMemo(() => document.nodes.find((node) => node.nodeId === selectedId) ?? null, [document.nodes, selectedId]);

  const setCurrentDocument = useCallback((next: HmiDocument, updateHistory: boolean) => {
    const previous = documentRef.current;
    if (fingerprint(previous) === fingerprint(next)) return;
    if (updateHistory) {
      historyPastRef.current = [...historyPastRef.current.slice(-(SNAPSHOT_LIMIT - 1)), structuredClone(previous)];
      historyFutureRef.current = [];
    }
    documentRef.current = next;
    setDocument(next);
    setDirty(fingerprint(next) !== savedFingerprintRef.current);
  }, []);

  const applyDocumentToGraph = useCallback((next: HmiDocument) => {
    if (!graphRef.current) return;
    applyingDocumentRef.current = true;
    documentToX6Graph(graphRef.current, next);
    window.requestAnimationFrame(() => { applyingDocumentRef.current = false; });
  }, []);

  const reload = useCallback(async () => {
    if (!pageId) return;
    setLoading(true);
    setError("");
    try {
      const detail = await getHmiPage(pageId);
      if (detail.draftDocument?.schema !== "hmi-page/v1") throw new Error("服务器返回了不支持的 HMI 文档版本");
      const initial = structuredClone(detail.draftDocument);
      setPage(detail);
      documentRef.current = initial;
      setDocument(initial);
      savedFingerprintRef.current = fingerprint(initial);
      setDirty(false);
      historyPastRef.current = [];
      historyFutureRef.current = [];
      applyDocumentToGraph(initial);
    } catch (loadError) {
      setError(getErrorMessage(loadError, "页面草稿加载失败"));
    } finally {
      setLoading(false);
    }
  }, [applyDocumentToGraph, pageId]);

  useEffect(() => { void reload(); }, [reload]);

  useEffect(() => {
    if (!containerRef.current) return;
    const graph = new Graph({
      container: containerRef.current,
      width: documentRef.current.canvas.width,
      height: documentRef.current.canvas.height,
      grid: { visible: true, size: 10, type: "mesh", args: { color: "#e5e7eb", thickness: 1 } },
      background: { color: "#ffffff" },
      interacting: { nodeMovable: true, edgeMovable: false },
      panning: { enabled: false },
      mousewheel: { enabled: false },
    });
    graphRef.current = graph;
    const commitGraph = () => {
      if (applyingDocumentRef.current || !graphRef.current) return;
      const next = x6GraphToCanonical(graphRef.current, documentRef.current.canvas);
      setCurrentDocument(next, true);
    };
    ["node:added", "node:removed", "node:change:position", "node:change:size", "node:change:angle", "node:change:zIndex", "node:change:data"].forEach((eventName) => graph.on(eventName, commitGraph));
    graph.on("node:click", ({ node }) => {
      setSelectedId(node.id);
      for (const cell of graph.getNodes()) {
        const data = cell.getData() ?? {};
        const selected = cell.id === node.id;
        if (data.selected !== selected) cell.setData({ ...data, selected });
      }
    });
    return () => { graph.dispose(); graphRef.current = null; };
  }, [setCurrentDocument]);

  useEffect(() => {
    if (!viewportRef.current) return;
    const target = viewportRef.current;
    const observer = new ResizeObserver(([entry]) => {
      const availableWidth = Math.max(1, entry.contentRect.width - 48);
      const availableHeight = Math.max(1, entry.contentRect.height - 48);
      setScale(Math.min(availableWidth / document.canvas.width, availableHeight / document.canvas.height, 1));
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, [document.canvas.width, document.canvas.height]);

  useEffect(() => {
    let active = true;
    setDeviceLoading(true);
    void getDevicePage({ page: 1, pageSize: 100 }).then((result) => { if (active) setDevices(result.records); }).catch(() => { if (active) setDevices([]); }).finally(() => { if (active) setDeviceLoading(false); });
    return () => { active = false; };
  }, []);

  const loadDataPoints = useCallback(async (deviceId: string) => {
    setActiveDeviceId(deviceId);
    if (!deviceId) { setDataPoints([]); return; }
    try {
      const result = await getDataPointPage({ deviceId, page: 1, pageSize: 100 });
      setDataPoints(result.records);
    } catch { setDataPoints([]); }
  }, []);

  const reloadDataPointForBinding = useCallback((binding?: DataPointBinding) => {
    if (!binding) return;
    setActiveDeviceId(binding.deviceId);
    void getDataPointPage({ deviceId: binding.deviceId, page: 1, pageSize: 100 }).then((result) => setDataPoints(result.records)).catch(() => setDataPoints([]));
  }, []);

  useEffect(() => {
    const graph = graphRef.current;
    if (!graph) return;
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable=true]")) return;
      const mod = event.metaKey || event.ctrlKey;
      if (mod && event.key.toLowerCase() === "c") {
        const node = graph.getCellById(selectedId ?? "");
        if (node?.isNode()) clipboardRef.current = nodeToCanonical(node);
        event.preventDefault();
      } else if (mod && event.key.toLowerCase() === "v") {
        const source = clipboardRef.current;
        if (source) { const clone = cloneHmiNode(source); clone.zIndex = maxZIndex(documentRef.current.nodes) + 1; graph.addNode(canonicalNodeToX6(clone)); }
        event.preventDefault();
      } else if (mod && event.key.toLowerCase() === "z") {
        event.preventDefault();
        if (event.shiftKey) redo(); else undo();
      } else if (mod && event.key.toLowerCase() === "y") {
        event.preventDefault(); redo();
      } else if (event.key === "Delete" || event.key === "Backspace") {
        const selected = graph.getCellById(selectedId ?? "");
        if (selected) { graph.removeCell(selected); setSelectedId(null); event.preventDefault(); }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [selectedId]);

  function selectNode(id: string) {
    const graph = graphRef.current;
    if (!graph) return;
    setSelectedId(id);
    for (const cell of graph.getNodes()) {
      const data = cell.getData() ?? {};
      const selected = cell.id === id;
      if (data.selected !== selected) cell.setData({ ...data, selected });
    }
  }

  function undo() {
    const previous = historyPastRef.current.pop();
    if (!previous) return;
    historyFutureRef.current.push(structuredClone(documentRef.current));
    documentRef.current = previous;
    setDocument(previous);
    setDirty(fingerprint(previous) !== savedFingerprintRef.current);
    applyDocumentToGraph(previous);
  }

  function redo() {
    const next = historyFutureRef.current.pop();
    if (!next) return;
    historyPastRef.current.push(structuredClone(documentRef.current));
    documentRef.current = next;
    setDocument(next);
    setDirty(fingerprint(next) !== savedFingerprintRef.current);
    applyDocumentToGraph(next);
  }

  function addNode(type: HmiNodeType) {
    const graph = graphRef.current;
    if (!graph) return;
    const node = createHmiNode(type, { x: 80 + documentRef.current.nodes.length * 14, y: 80 + documentRef.current.nodes.length * 14 });
    node.zIndex = maxZIndex(documentRef.current.nodes) + 1;
    graph.addNode(canonicalNodeToX6(node));
    selectNode(node.nodeId);
  }

  function duplicateSelected() {
    const graph = graphRef.current;
    if (!graph) return;
    const node = graph?.getSelectedCells().find((cell) => cell.isNode());
    if (!node?.isNode()) return;
    const clone = cloneHmiNode(nodeToCanonical(node));
    clone.zIndex = maxZIndex(documentRef.current.nodes) + 1;
    graph.addNode(canonicalNodeToX6(clone));
    selectNode(clone.nodeId);
  }

  function patchSelected(mutator: (node: HmiNode) => HmiNode) {
    const graph = graphRef.current;
    const cell = graph?.getCellById(selectedId ?? "");
    if (!cell?.isNode()) return;
    const next = mutator(nodeToCanonical(cell));
    cell.setData({ type: next.type, props: next.props, bindings: next.bindings });
  }

  function patchGeometry(key: "width" | "height", value: number) {
    if (!selectedId || !Number.isFinite(value)) return;
    const graph = graphRef.current;
    const node = graph?.getCellById(selectedId);
    if (node?.isNode()) {
      const current = node.getSize();
      const nextValue = Math.max(20, Math.min(document.canvas[key], value));
      node.setSize({ ...current, [key]: nextValue });
    }
  }

  function rotateSelected() {
    const node = graphRef.current?.getCellById(selectedId ?? "");
    if (node?.isNode()) node.rotate((node.getAngle() + 90) % 360, { absolute: true });
  }

  function setSelectedRotation(rotation: HmiNode["rotation"]) {
    const node = graphRef.current?.getCellById(selectedId ?? "");
    if (node?.isNode()) node.rotate(rotation, { absolute: true });
  }

  function changeZOrder(to: "front" | "back") {
    const graph = graphRef.current;
    const node = graph?.getCellById(selectedId ?? "");
    if (!node?.isNode() || !graph) return;
    node.setZIndex(to === "front" ? maxZIndex(graph.getNodes().map(nodeToCanonical)) + 1 : 0);
  }

  function changeCanvas(sizeIndex: number) {
    const canvas = HMI_CANVAS_PRESETS[sizeIndex];
    if (!canvas) return;
    const next = { ...documentRef.current, canvas: { ...canvas } };
    setCurrentDocument(next, true);
    graphRef.current?.resize(canvas.width, canvas.height);
  }

  async function saveDraft() {
    if (!page || !dirty) return;
    setSaving(true);
    try {
      const canonical = x6GraphToCanonical(graphRef.current!, documentRef.current.canvas);
      const result = await saveHmiDraft(page.pageId, { expectedDraftRevision: page.draftRevision, document: canonical });
      setPage(result);
      documentRef.current = canonical;
      setDocument(canonical);
      savedFingerprintRef.current = fingerprint(canonical);
      setDirty(false);
      toast.success(`草稿已保存，修订号 ${result.draftRevision}`);
    } catch (saveError) {
      if (isApiError(saveError) && saveError.status === 409) toast.error("草稿版本冲突：其他编辑会话已保存更新，请重新加载后再编辑");
      else toast.error(getErrorMessage(saveError, "草稿保存失败"));
    } finally { setSaving(false); }
  }

  async function publish() {
    if (!page || dirty || !canPublish) return;
    const errors = validateForPublish(documentRef.current);
    if (errors.length) { toast.error({ title: "页面尚不能发布", description: errors.slice(0, 3).join("；") }); return; }
    setPublishing(true);
    try {
      const result = await publishHmiPage(page.pageId, { expectedDraftRevision: page.draftRevision });
      setPage((current) => current ? { ...current, publishedVersionId: result.publishedVersionId } : current);
      toast.success(`已发布页面版本 ${result.versionNo}`);
    } catch (publishError) {
      toast.error(getErrorMessage(publishError, "页面发布失败"));
    } finally { setPublishing(false); }
  }

  const currentDataPointBinding = selectedNode?.bindings.value?.kind === "datapoint" ? selectedNode.bindings.value : selectedNode?.bindings.state?.kind === "datapoint" ? selectedNode.bindings.state : undefined;
  useEffect(() => { reloadDataPointForBinding(currentDataPointBinding); }, [currentDataPointBinding?.deviceId, reloadDataPointForBinding]);

  if (loading) return <div className="p-card text-sm text-text-secondary">正在加载 HMI 页面…</div>;
  if (error || !page) return <EmptyState title="HMI 页面无法打开" description={error || "找不到指定页面。"} actionText="返回页面列表" onAction={() => navigate("/hmi/pages")} />;

  return <div className="flex min-h-[calc(100vh-120px)] flex-col gap-space-4">
    <PageHeader title={page.name} description={`修订 ${page.draftRevision} · ${dirty ? "有未保存更改" : "草稿已同步"}`} actions={<><Button variant="secondary" onClick={() => navigate("/hmi/pages")} disabled={dirty}>返回列表</Button><Button variant="secondary" onClick={undo} disabled={historyPastRef.current.length === 0} aria-label="撤销"><Undo2 className="h-4 w-4" aria-hidden />撤销</Button><Button variant="secondary" onClick={redo} disabled={historyFutureRef.current.length === 0} aria-label="重做"><Redo2 className="h-4 w-4" aria-hidden />重做</Button><Button variant="primary" onClick={() => void saveDraft()} disabled={!dirty || saving}><Save className="h-4 w-4" aria-hidden />{saving ? "保存中" : "保存草稿"}</Button><Button variant="secondary" onClick={() => void publish()} disabled={dirty || publishing || !canPublish}><Upload className="h-4 w-4" aria-hidden />{publishing ? "发布中" : "发布"}</Button></>} />
    <div className="flex min-h-0 flex-1 overflow-hidden rounded-admin border border-border bg-surface">
      <aside className="w-52 shrink-0 border-r border-border p-3">
        <div className="mb-2 text-xs font-semibold text-text-secondary">添加组件</div>
        <div className="grid grid-cols-2 gap-2">{COMPONENT_ORDER.map((type) => <Button key={type} variant="secondary" size="sm" className="justify-start" onClick={() => addNode(type)}><Plus className="h-3.5 w-3.5" aria-hidden />{HMI_COMPONENTS[type].label}</Button>)}</div>
        <Field label="画布尺寸" htmlFor="hmi-canvas-size"><Select id="hmi-canvas-size" value={String(HMI_CANVAS_PRESETS.findIndex((size) => size.width === document.canvas.width && size.height === document.canvas.height))} onChange={(event) => changeCanvas(Number(event.target.value))}><option value="0">1920 × 1080</option><option value="1">1366 × 768</option><option value="2">1280 × 720</option></Select></Field>
        <p className="mt-2 text-xs text-text-tertiary">固定逻辑画布。编辑器按比例缩放显示；节点坐标以画布像素保存。</p>
      </aside>
      <section ref={viewportRef} className="flex min-w-0 flex-1 items-center justify-center overflow-auto bg-neutral-background p-6" aria-label="HMI 编辑画布">
        <div className="shrink-0 bg-white shadow-admin" style={{ width: document.canvas.width * scale, height: document.canvas.height * scale }}><div ref={containerRef} style={{ width: document.canvas.width, height: document.canvas.height, transform: `scale(${scale})`, transformOrigin: "top left" }} /></div>
      </section>
      <aside className="w-72 shrink-0 overflow-y-auto border-l border-border p-3">
        <Inspector node={selectedNode} canvas={document.canvas} devices={devices} dataPoints={dataPoints} activeDeviceId={activeDeviceId} deviceLoading={deviceLoading} onLoadDevice={loadDataPoints} onPatch={patchSelected} onGeometry={patchGeometry} onRotate={rotateSelected} onRotation={setSelectedRotation} onZOrder={changeZOrder} onDuplicate={duplicateSelected} onDelete={() => { const node = graphRef.current?.getCellById(selectedId ?? ""); if (node) graphRef.current?.removeCell(node); }} />
      </aside>
    </div>
  </div>;
}

function validateForPublish(document: HmiDocument) {
  const errors: string[] = [];
  if (!HMI_CANVAS_PRESETS.some((size) => size.width === document.canvas.width && size.height === document.canvas.height)) errors.push("画布尺寸不受支持");
  if (document.nodes.length > 500) errors.push("节点数量不能超过 500");
  if (new TextEncoder().encode(JSON.stringify(document)).byteLength > 2 * 1024 * 1024) errors.push("HMI 文档超过 2 MiB 限制");
  const colorAllowed = (color: string) => (HMI_STATIC_COLORS as readonly string[]).includes(color);
  const ids = new Set<string>();
  let dataPointBindingCount = 0;
  for (const node of document.nodes) {
    const definition = HMI_COMPONENTS[node.type];
    if (ids.has(node.nodeId)) errors.push("节点 ID 必须唯一");
    ids.add(node.nodeId);
    if (![node.x, node.y, node.width, node.height, node.zIndex].every(Number.isFinite) || node.x < 0 || node.y < 0 || node.width < 20 || node.height < 20 || node.x + node.width > document.canvas.width || node.y + node.height > document.canvas.height || ![0, 90, 180, 270].includes(node.rotation)) errors.push("节点位置、尺寸、层级或角度无效");
    for (const slot of definition.bindingSlots) if (slot.required && !node.bindings[slot.name]) errors.push(`${definition.label}缺少 ${slot.name} 绑定`);
    for (const slot of definition.bindingSlots) {
      const binding = node.bindings[slot.name];
      if (binding && binding.kind !== slot.kind) errors.push(`${definition.label}的 ${slot.name} 绑定类型无效`);
    }
    if (node.type === "text" && (node.props.text.trim() === "" || node.props.fontSize < 10 || node.props.fontSize > 72 || !colorAllowed(node.props.color))) errors.push("文本组件属性无效");
    if (node.type === "shape" && (!colorAllowed(node.props.fill) || !colorAllowed(node.props.stroke) || node.props.strokeWidth < 0 || node.props.strokeWidth > 12)) errors.push("形状组件颜色或描边无效");
    if (node.type === "value-display" && (!node.props.label.trim() || !colorAllowed(node.props.color) || (node.props.precision !== null && (node.props.precision < 0 || node.props.precision > 6)))) errors.push("数值显示组件属性无效");
    if (node.type === "indicator" && (!node.props.label.trim() || !node.props.trueLabel.trim() || !node.props.falseLabel.trim() || !colorAllowed(node.props.trueColor) || !colorAllowed(node.props.falseColor))) errors.push("状态指示组件属性无效");
    if (node.type === "gauge" && node.props.max <= node.props.min) errors.push("仪表最大值必须大于最小值");
    if (node.type === "gauge" && !node.props.label.trim()) errors.push("仪表组件标签不能为空");
    if (node.type === "button" && !node.props.label.trim()) errors.push("按钮组件标签不能为空");
    if (node.type === "switch" && (!node.props.label.trim() || !node.props.onLabel.trim() || !node.props.offLabel.trim())) errors.push("开关组件文本不能为空");
    for (const binding of Object.values(node.bindings)) {
      if (binding.kind === "datapoint") dataPointBindingCount += 1;
      if (binding.kind === "command" && (!binding.deviceId.trim() || !binding.name.trim() || binding.ttlSeconds < 1 || binding.ttlSeconds > 300 || !binding.args || typeof binding.args !== "object" || Array.isArray(binding.args))) errors.push("Command 绑定参数无效（TTL 需为 1 至 300 秒）");
    }
    if (node.type === "switch") {
      const state = node.bindings.state;
      const on = node.bindings.onCommand;
      const off = node.bindings.offCommand;
      if (state?.kind === "datapoint" && on?.kind === "command" && state.deviceId !== on.deviceId) errors.push("开关状态与开命令必须绑定同一设备");
      if (state?.kind === "datapoint" && off?.kind === "command" && state.deviceId !== off.deviceId) errors.push("开关状态与关命令必须绑定同一设备");
    }
  }
  if (dataPointBindingCount > 1000) errors.push("DataPoint 绑定不能超过 1000 个");
  return [...new Set(errors)];
}

function Inspector({ node, canvas, devices, dataPoints, activeDeviceId, deviceLoading, onLoadDevice, onPatch, onGeometry, onRotate, onRotation, onZOrder, onDuplicate, onDelete }: {
  node: HmiNode | null;
  canvas: HmiCanvasSize;
  devices: DeviceRecord[];
  dataPoints: DataPointRecord[];
  activeDeviceId: string;
  deviceLoading: boolean;
  onLoadDevice: (deviceId: string) => void;
  onPatch: (mutator: (node: HmiNode) => HmiNode) => void;
  onGeometry: (key: "width" | "height", value: number) => void;
  onRotate: () => void;
  onRotation: (rotation: HmiNode["rotation"]) => void;
  onZOrder: (to: "front" | "back") => void;
  onDuplicate: () => void;
  onDelete: () => void;
}) {
  if (!node) return <div className="space-y-2"><h2 className="text-sm font-semibold">属性</h2><p className="text-xs text-text-tertiary">选择画布中的组件后编辑属性、尺寸和绑定。</p></div>;
  const setProp = (key: string, value: unknown) => onPatch((current) => ({ ...current, props: { ...current.props, [key]: value } } as HmiNode));
  const setBinding = (slot: string, binding?: DataPointBinding | CommandBinding) => onPatch((current) => {
    const bindings = { ...current.bindings };
    if (binding) bindings[slot] = binding; else delete bindings[slot];
    return { ...current, bindings } as HmiNode;
  });
  const dataPointSlot = node.type === "switch" ? "state" : "value";
  const selectedDataPoint = node.bindings[dataPointSlot]?.kind === "datapoint" ? node.bindings[dataPointSlot] as DataPointBinding : undefined;
  const currentCommand = (slot: string) => node.bindings[slot]?.kind === "command" ? node.bindings[slot] as CommandBinding : undefined;
  const renderDataPointBinding = (slot: string, allowed: DataPointRecord["valueType"][]) => <DataPointBindingEditor slot={slot} binding={node.bindings[slot]?.kind === "datapoint" ? node.bindings[slot] as DataPointBinding : undefined} devices={devices} dataPoints={dataPoints.filter((point) => allowed.includes(point.valueType))} activeDeviceId={activeDeviceId} deviceLoading={deviceLoading} onLoadDevice={onLoadDevice} onChange={setBinding} />;
  const renderCommand = (slot: string, fixedDeviceId?: string) => <CommandBindingEditor slot={slot} binding={currentCommand(slot)} deviceId={fixedDeviceId ?? selectedDataPoint?.deviceId} devices={devices} onChange={setBinding} />;
  return <div className="space-y-3">
    <div className="flex items-center justify-between"><h2 className="text-sm font-semibold">{HMI_COMPONENTS[node.type].label}属性</h2><div className="flex"><Button size="icon" variant="ghost" aria-label="复制组件" onClick={onDuplicate}><Copy className="h-4 w-4" aria-hidden /></Button><Button size="icon" variant="ghost" aria-label="置于顶层" onClick={() => onZOrder("front")}><Layers2 className="h-4 w-4" aria-hidden /></Button><Button size="icon" variant="ghost" aria-label="旋转 90 度" onClick={onRotate}><RotateCw className="h-4 w-4" aria-hidden /></Button><Button size="icon" variant="ghost" aria-label="删除组件" onClick={onDelete}><Trash2 className="h-4 w-4" aria-hidden /></Button></div></div>
    <div className="grid grid-cols-2 gap-2"><Field label="宽度"><Input type="number" min={20} max={canvas.width} value={Math.round(node.width)} onChange={(event) => onGeometry("width", Number(event.target.value))} /></Field><Field label="高度"><Input type="number" min={20} max={canvas.height} value={Math.round(node.height)} onChange={(event) => onGeometry("height", Number(event.target.value))} /></Field></div>
    <Field label="旋转角度"><Select value={String(node.rotation)} onChange={(event) => onRotation(Number(event.target.value) as HmiNode["rotation"])}><option value="0">0°</option><option value="90">90°</option><option value="180">180°</option><option value="270">270°</option></Select></Field>
    <Field label="层级"><div className="flex gap-2"><Button size="sm" variant="secondary" onClick={() => onZOrder("back")}>置于底层</Button><Button size="sm" variant="secondary" onClick={() => onZOrder("front")}>置于顶层</Button></div></Field>
    {node.type === "text" && <><Field label="文本"><Textarea rows={3} maxLength={1000} value={node.props.text} onChange={(event) => setProp("text", event.target.value)} /></Field><Field label="字号"><Input type="number" min={10} max={72} value={node.props.fontSize} onChange={(event) => setProp("fontSize", Number(event.target.value))} /></Field><Field label="字重"><Select value={node.props.fontWeight} onChange={(event) => setProp("fontWeight", event.target.value)}><option value="normal">常规</option><option value="medium">中等</option><option value="bold">粗体</option></Select></Field><Field label="文字颜色"><ColorSelect value={node.props.color} onChange={(value) => setProp("color", value)} /></Field><Field label="对齐"><Select value={node.props.align} onChange={(event) => setProp("align", event.target.value)}><option value="left">左对齐</option><option value="center">居中</option><option value="right">右对齐</option></Select></Field></>}
    {node.type === "shape" && <><Field label="形状"><Select value={node.props.shape} onChange={(event) => setProp("shape", event.target.value)}><option value="rectangle">矩形</option><option value="ellipse">椭圆</option><option value="line">直线</option></Select></Field><Field label="填充颜色"><ColorSelect value={node.props.fill} onChange={(value) => setProp("fill", value)} /></Field><Field label="描边颜色"><ColorSelect value={node.props.stroke} onChange={(value) => setProp("stroke", value)} /></Field><Field label="描边宽度"><Input type="number" min={0} max={12} value={node.props.strokeWidth} onChange={(event) => setProp("strokeWidth", Number(event.target.value))} /></Field></>}
    {(node.type === "value-display" || node.type === "indicator" || node.type === "gauge" || node.type === "button" || node.type === "switch") && <Field label="组件标签"><Input value={node.props.label} maxLength={100} onChange={(event) => setProp("label", event.target.value)} /></Field>}
    {(node.type === "value-display" || node.type === "gauge") && <><Field label="精度覆盖"><Input type="number" min={0} max={6} placeholder="使用数据点精度" value={node.props.precision ?? ""} onChange={(event) => setProp("precision", event.target.value === "" ? null : Number(event.target.value))} /></Field><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={node.props.showUnit} onChange={(event) => setProp("showUnit", event.target.checked)} />显示数据点单位</label></>}
    {node.type === "value-display" && <Field label="数值颜色"><ColorSelect value={node.props.color} onChange={(value) => setProp("color", value)} /></Field>}
    {node.type === "indicator" && <><Field label="真值文本"><Input value={node.props.trueLabel} onChange={(event) => setProp("trueLabel", event.target.value)} /></Field><Field label="假值文本"><Input value={node.props.falseLabel} onChange={(event) => setProp("falseLabel", event.target.value)} /></Field><Field label="真值颜色"><ColorSelect value={node.props.trueColor} onChange={(value) => setProp("trueColor", value)} /></Field><Field label="假值颜色"><ColorSelect value={node.props.falseColor} onChange={(value) => setProp("falseColor", value)} /></Field></>}
    {node.type === "gauge" && <div className="grid grid-cols-2 gap-2"><Field label="最小值"><Input type="number" value={node.props.min} onChange={(event) => setProp("min", Number(event.target.value))} /></Field><Field label="最大值"><Input type="number" value={node.props.max} onChange={(event) => setProp("max", Number(event.target.value))} /></Field></div>}
    {node.type === "switch" && <><Field label="开状态文本"><Input value={node.props.onLabel} onChange={(event) => setProp("onLabel", event.target.value)} /></Field><Field label="关状态文本"><Input value={node.props.offLabel} onChange={(event) => setProp("offLabel", event.target.value)} /></Field></>}
    {node.type === "value-display" && renderDataPointBinding("value", ["NUMBER", "BOOLEAN"])}
    {node.type === "indicator" && renderDataPointBinding("value", ["BOOLEAN"])}
    {node.type === "gauge" && renderDataPointBinding("value", ["NUMBER"])}
    {node.type === "switch" && <>{renderDataPointBinding("state", ["BOOLEAN"])}{renderCommand("onCommand", selectedDataPoint?.deviceId)}{renderCommand("offCommand", selectedDataPoint?.deviceId)}</>}
    {node.type === "button" && renderCommand("command")}
  </div>;
}

function ColorSelect({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return <Select value={value} onChange={(event) => onChange(event.target.value)}>{HMI_STATIC_COLORS.map((color) => <option key={color} value={color}>{color}</option>)}</Select>;
}

function DataPointBindingEditor({ slot, binding, devices, dataPoints, activeDeviceId, deviceLoading, onLoadDevice, onChange }: {
  slot: string;
  binding?: DataPointBinding;
  devices: DeviceRecord[];
  dataPoints: DataPointRecord[];
  activeDeviceId: string;
  deviceLoading: boolean;
  onLoadDevice: (deviceId: string) => void;
  onChange: (slot: string, binding?: DataPointBinding) => void;
}) {
  return <div className="space-y-2 rounded-control border border-border p-2">
    <div className="text-xs font-medium text-text-secondary">数据点绑定 · {slot}</div>
    <Select aria-label={`${slot}绑定设备`} value={binding?.deviceId ?? activeDeviceId} onChange={(event) => { const deviceId = event.target.value; onLoadDevice(deviceId); onChange(slot, undefined); }}><option value="">选择 Cloud Device</option>{devices.map((device) => <option key={device.deviceId} value={device.deviceId}>{device.deviceId}</option>)}</Select>
    <Select aria-label={`${slot}绑定数据点`} value={binding?.pointKey ?? ""} disabled={!activeDeviceId || deviceLoading} onChange={(event) => { const point = dataPoints.find((item) => item.pointKey === event.target.value); if (point) onChange(slot, { kind: "datapoint", deviceId: point.deviceId, pointKey: point.pointKey }); }}><option value="">{deviceLoading ? "读取数据点…" : "选择语义数据点"}</option>{dataPoints.map((point) => <option key={point.dataPointId} value={point.pointKey}>{point.name} · {point.pointKey} · {point.valueType}</option>)}</Select>
    {binding && <div className="break-all text-[11px] text-text-tertiary">{binding.deviceId} / {binding.pointKey}</div>}
  </div>;
}

function CommandBindingEditor({ slot, binding, deviceId, devices, onChange }: {
  slot: string;
  binding?: CommandBinding;
  deviceId?: string;
  devices: DeviceRecord[];
  onChange: (slot: string, binding?: CommandBinding) => void;
}) {
  const [argsText, setArgsText] = useState(() => JSON.stringify(binding?.args ?? {}, null, 2));
  const [argsError, setArgsError] = useState("");
  useEffect(() => { setArgsText(JSON.stringify(binding?.args ?? {}, null, 2)); }, [binding?.deviceId, binding?.name, binding?.ttlSeconds]);
  const current: CommandBinding = binding ?? { kind: "command", deviceId: deviceId ?? "", name: "", args: {}, ttlSeconds: 30, confirmation: { required: true, message: "确认执行该操作？" } };
  const update = (patch: Partial<CommandBinding>) => onChange(slot, { ...current, ...patch, deviceId: deviceId ?? patch.deviceId ?? current.deviceId });
  return <div className="space-y-2 rounded-control border border-border p-2">
    <div className="text-xs font-medium text-text-secondary">Command 绑定 · {slot}</div>
    <Field label="目标设备"><Select value={deviceId ?? current.deviceId} disabled={Boolean(deviceId)} onChange={(event) => update({ deviceId: event.target.value })}><option value="">选择 Cloud Device</option>{devices.map((device) => <option key={device.deviceId} value={device.deviceId}>{device.deviceId}</option>)}</Select></Field>
    <Field label="Command 名称"><Input maxLength={100} value={current.name} onChange={(event) => update({ name: event.target.value })} /></Field>
    <Field label="静态 Args JSON 对象" error={argsError} help="仅保存固定 JSON 对象，不支持表达式。"><Textarea rows={4} value={argsText} onChange={(event) => { setArgsText(event.target.value); try { const parsed: unknown = JSON.parse(event.target.value); if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) { update({ args: parsed as Record<string, unknown> }); setArgsError(""); } else setArgsError("Args 必须是 JSON 对象。"); } catch { setArgsError("请输入有效的 JSON 对象。"); } }} /></Field>
    <Field label="TTL 秒" help="M6 支持 1 至 300 秒。"><Input type="number" min={1} max={300} value={current.ttlSeconds} onChange={(event) => update({ ttlSeconds: Number(event.target.value) })} /></Field>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={current.confirmation.required} onChange={(event) => update({ confirmation: { ...current.confirmation, required: event.target.checked } })} />执行前确认</label>
    {current.confirmation.required && <Field label="确认提示"><Input maxLength={200} value={current.confirmation.message ?? ""} onChange={(event) => update({ confirmation: { ...current.confirmation, message: event.target.value } })} /></Field>}
  </div>;
}
