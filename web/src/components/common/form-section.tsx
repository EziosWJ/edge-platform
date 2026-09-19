import type { PropsWithChildren } from "react";
import { ContentCard } from "@/components/common/content-card";

type FormSectionProps = PropsWithChildren<{
  title: string;
  description?: string;
}>;

export function FormSection({ title, description, children }: FormSectionProps) {
  return (
    <ContentCard title={title} description={description}>
      <div className="grid gap-space-4 md:grid-cols-2">{children}</div>
    </ContentCard>
  );
}
