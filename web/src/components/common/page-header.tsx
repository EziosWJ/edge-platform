import type { ReactNode } from "react";

type PageHeaderProps = {
  title: string;
  description?: string;
  actions?: ReactNode;
};

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <div className="mb-space-6 flex flex-col gap-space-4 md:flex-row md:items-start md:justify-between">
      <div>
        <h1 className="text-page-title font-semibold text-text-primary">
          {title}
        </h1>
        {description && (
          <p className="mt-space-1 text-sm text-text-tertiary">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-space-2">{actions}</div>}
    </div>
  );
}
