import { ContentCard } from "@/components/common/content-card";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";

export function SettingsPage() {
  return (
    <>
      <PageHeader
        title="系统设置"
        description="第一版保留设置入口，后续按业务系统扩展具体配置项。"
      />

      <ContentCard title="基础设置">
        <div className="grid gap-4 md:grid-cols-3">
          {[
            ["系统名称", "智慧综合管理平台"],
            ["认证方式", "模拟登录"],
            ["权限模型", "登录态守卫"],
          ].map(([label, value]) => (
            <div key={label} className="rounded-lg border border-border p-4">
              <div className="text-sm text-text-tertiary">{label}</div>
              <div className="mt-2 font-medium text-text-primary">{value}</div>
            </div>
          ))}
        </div>
        <div className="mt-4">
          <StatusTag tone="info">待扩展</StatusTag>
        </div>
      </ContentCard>
    </>
  );
}
