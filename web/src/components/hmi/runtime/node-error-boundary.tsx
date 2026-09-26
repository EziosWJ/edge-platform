import { Component, type ErrorInfo, type ReactNode } from "react";

type NodeErrorBoundaryProps = {
  children: ReactNode;
  nodeId: string;
};

type NodeErrorBoundaryState = {
  failed: boolean;
};

export class HmiRuntimeNodeErrorBoundary extends Component<
  NodeErrorBoundaryProps,
  NodeErrorBoundaryState
> {
  state: NodeErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): NodeErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`HMI runtime node ${this.props.nodeId} render failed`, error, info.componentStack);
  }

  render() {
    if (this.state.failed) {
      return (
        <div className="flex h-full w-full items-center justify-center rounded border border-error/40 bg-error/5 p-2 text-center text-xs text-error" role="alert">
          组件渲染失败
        </div>
      );
    }
    return this.props.children;
  }
}
