import { Link } from "react-router-dom";
import { ArrowLeft, FileQuestion } from "lucide-react";

export function NotFoundPage() {
  return (
    <div className="flex min-h-[calc(100vh-112px)] items-center justify-center">
      <div className="max-w-md text-center">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-xl bg-slate-100 text-text-tertiary">
          <FileQuestion className="h-7 w-7" aria-hidden />
        </div>
        <h1 className="mt-5 text-2xl font-semibold leading-8">页面不存在</h1>
        <p className="mt-2 text-sm text-text-tertiary">
          当前访问的页面不存在，可能已被移动或删除。
        </p>
        <Link
          to="/dashboard"
          className="mt-5 inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-primary bg-primary px-4 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden />
          返回首页
        </Link>
      </div>
    </div>
  );
}
