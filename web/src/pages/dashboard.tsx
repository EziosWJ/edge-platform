import { Cloud, Database, MonitorCog, RadioTower } from "lucide-react";
import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";

const foundations = [
  {
    title: "平台基础能力",
    description: "认证、RBAC、系统配置、文件、审计与通知已完成脚手架迁移。",
    status: "已具备",
    tone: "success" as const,
    icon: Cloud,
  },
  {
    title: "MQTT 接入",
    description: "下一阶段接入 Edge Collector MQTT v1 contract，由 Server 统一订阅和发布。",
    status: "待实现",
    tone: "neutral" as const,
    icon: RadioTower,
  },
  {
    title: "设备语义",
    description: "将建立 Edge、Device、DataPoint 和 CurrentValue，避免业务直接依赖寄存器与 Topic。",
    status: "待实现",
    tone: "neutral" as const,
    icon: Database,
  },
  {
    title: "HMI 组态",
    description: "在数据点和 Command 闭环稳定后实施，编辑器基础设施采用 AntV X6。",
    status: "待实现",
    tone: "neutral" as const,
    icon: MonitorCog,
  },
];

export function DashboardPage() {
  return (
    <>
      <PageHeader
        title="Edge Platform"
        description="云端工业 IoT / HMI 平台工程基线"
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {foundations.map((item) => {
          const Icon = item.icon;
          return (
            <ContentCard key={item.title} bodyClassName="p-4">
              <div className="flex items-start justify-between gap-3">
                <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-slate-50 text-primary">
                  <Icon className="h-5 w-5" aria-hidden />
                </span>
                <StatusTag tone={item.tone}>{item.status}</StatusTag>
              </div>
              <div className="mt-4 font-medium text-text-primary">{item.title}</div>
              <p className="mt-2 text-sm leading-6 text-text-secondary">
                {item.description}
              </p>
            </ContentCard>
          );
        })}
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-2">
        <ContentCard title="Edge / Cloud 边界">
          <div className="space-y-3 text-sm leading-6 text-text-secondary">
            <p>Edge Collector 负责现场协议、轮询、Starlark 与最终控制执行。</p>
            <p>Edge Platform 负责设备语义、实时状态、历史、Command 与 HMI。</p>
            <p>两者通过 MQTT v1 contract 通信；正式 Web 页面不直接连接 MQTT Broker。</p>
          </div>
        </ContentCard>

        <ContentCard title="下一阶段">
          <div className="rounded-lg border border-border bg-slate-50 px-4 py-4 text-sm leading-7 text-text-secondary">
            MQTT ingest → Edge → Device → DataPoint → CurrentValue → WebSocket → Command
          </div>
        </ContentCard>
      </div>
    </>
  );
}
