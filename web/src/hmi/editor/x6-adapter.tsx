import { renderToStaticMarkup } from "react-dom/server";
import { Shape, type Graph, type Node } from "@antv/x6";
import { HmiComponentRenderer } from "@/hmi/registry";
import type { HmiDocument, HmiNode, HmiNodeType } from "@/hmi/model";

type X6NodeData = { type: HmiNodeType; props: HmiNode["props"]; bindings: HmiNode["bindings"] };
export const HMI_X6_NODE_SHAPE = "hmi-component-node";

Shape.HTML.register({
  shape: HMI_X6_NODE_SHAPE,
  width: 180,
  height: 64,
  effect: ["data"],
  html: (cell) => {
    const wrapper = document.createElement("div");
    if (!cell.isNode()) return wrapper;
    wrapper.className = `h-full w-full overflow-hidden ${cell.getData()?.selected ? "rounded outline outline-2 outline-primary" : ""}`;
    wrapper.innerHTML = renderToStaticMarkup(<HmiComponentRenderer node={nodeToCanonical(cell)} mode="editor" />);
    return wrapper;
  },
});

export function canonicalNodeToX6(node: HmiNode) {
  return {
    id: node.nodeId,
    shape: HMI_X6_NODE_SHAPE,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    angle: node.rotation,
    zIndex: node.zIndex,
    data: { type: node.type, props: structuredClone(node.props), bindings: structuredClone(node.bindings) } satisfies X6NodeData,
  };
}

export function x6GraphToCanonical(graph: Graph, canvas: HmiDocument["canvas"]): HmiDocument {
  const nodes = graph.getNodes().map((cell: Node) => {
    const data = cell.getData() as X6NodeData;
    return {
      nodeId: cell.id,
      x: cell.getPosition().x,
      y: cell.getPosition().y,
      width: cell.getSize().width,
      height: cell.getSize().height,
      rotation: normalizeRotation(cell.getAngle()),
      zIndex: cell.getZIndex(),
      type: data.type,
      props: structuredClone(data.props),
      bindings: structuredClone(data.bindings),
    } as HmiNode;
  });
  return { schema: "hmi-page/v1", canvas: { ...canvas }, nodes: nodes.sort((left, right) => left.zIndex - right.zIndex) };
}

export function documentToX6Graph(graph: Graph, document: HmiDocument) {
  graph.clearCells();
  for (const node of document.nodes) graph.addNode(canonicalNodeToX6(node));
  graph.resize(document.canvas.width, document.canvas.height);
}

export function nodeToCanonical(cell: Node): HmiNode {
  const data = cell.getData() as X6NodeData;
  return {
    nodeId: cell.id,
    x: cell.getPosition().x,
    y: cell.getPosition().y,
    width: cell.getSize().width,
    height: cell.getSize().height,
    rotation: normalizeRotation(cell.getAngle()),
    zIndex: cell.getZIndex(),
    type: data.type,
    props: structuredClone(data.props),
    bindings: structuredClone(data.bindings),
  } as HmiNode;
}

export function normalizeRotation(angle: number): HmiNode["rotation"] {
  const normalized = ((Math.round(angle / 90) * 90) % 360 + 360) % 360;
  return normalized as HmiNode["rotation"];
}
