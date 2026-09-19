import { useEffect, useState, type FormEvent } from "react";
import { Boxes, Eye, EyeOff, LockKeyhole, ShieldCheck, UserRound } from "lucide-react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { Field } from "@/components/common/field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { isApiError } from "@/lib/api-error";
import { useAuthStore } from "@/store/auth-store";
import type { LoginErrors } from "@/types";

const REMEMBERED_USERNAME_KEY = "edge-platform-remembered-username";

function MobileBrand() {
  return (
    <div className="mb-8 flex items-center gap-3 min-[992px]:hidden">
      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary text-white shadow-[0_8px_24px_rgb(22_119_255_/_0.22)]">
        <Boxes className="h-5 w-5" aria-hidden />
      </div>
      <div>
        <p className="text-base font-semibold tracking-tight text-text-primary">Edge Platform</p>
        <p className="mt-0.5 text-xs text-text-tertiary">
          Industrial IoT / HMI Platform
        </p>
      </div>
    </div>
  );
}

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const login = useAuthStore((state) => state.login);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [rememberMe, setRememberMe] = useState(false);
  const [errors, setErrors] = useState<LoginErrors>({});
  const [submitting, setSubmitting] = useState(false);

  const stateFrom =
    (location.state as { from?: { pathname?: string } } | null)?.from
      ?.pathname;
  const queryRedirect = new URLSearchParams(location.search).get("redirect");
  let from = stateFrom ?? queryRedirect ?? "/dashboard";

  if (from.includes("://") || from.startsWith("//")) {
    from = "/dashboard";
  }

  useEffect(() => {
    try {
      const rememberedUsername = localStorage.getItem(REMEMBERED_USERNAME_KEY);
      if (rememberedUsername) {
        setUsername(rememberedUsername);
        setRememberMe(true);
      }
    } catch {
      // 浏览器禁用本地存储时不影响正常登录。
    }
  }, []);

  if (isAuthenticated) {
    return <Navigate to={from} replace />;
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const nextErrors: LoginErrors = {};

    if (!username.trim()) {
      nextErrors.username = "请输入用户名";
    }
    if (!password) {
      nextErrors.password = "请输入密码";
    }

    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }

    try {
      setSubmitting(true);
      await login(username.trim(), password);
      try {
        if (rememberMe) {
          localStorage.setItem(REMEMBERED_USERNAME_KEY, username.trim());
        } else {
          localStorage.removeItem(REMEMBERED_USERNAME_KEY);
        }
      } catch {
        // 本地存储不可用时仍以登录结果为准。
      }
      navigate(from, { replace: true });
    } catch (error) {
      setErrors({
        account: isApiError(error)
          ? error.message
          : "登录失败，请稍后重试",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="flex min-h-screen w-screen min-w-0 flex-col overflow-hidden bg-[#f5f8fc] min-[992px]:grid min-[992px]:grid-cols-[55%_45%]">
      <section
        className="relative hidden min-h-screen overflow-hidden bg-[#edf5ff] min-[992px]:flex min-[992px]:items-center min-[992px]:px-16 xl:px-24"
        aria-label="平台介绍"
        style={{
          background:
            "radial-gradient(circle at 20% 25%, rgba(37,99,235,.15), transparent 35%), linear-gradient(135deg, #f0f6ff 0%, #e8f1ff 100%)",
        }}
      >
        <div
          className="pointer-events-none absolute inset-0 opacity-60"
          style={{
            backgroundImage:
              "linear-gradient(rgba(37, 99, 235, 0.07) 1px, transparent 1px), linear-gradient(90deg, rgba(37, 99, 235, 0.07) 1px, transparent 1px)",
            backgroundSize: "48px 48px",
          }}
          aria-hidden
        />
        <div className="pointer-events-none absolute -right-48 top-1/2 h-[34rem] w-[34rem] -translate-y-1/2 rounded-full border border-primary/10" aria-hidden />
        <div className="pointer-events-none absolute -right-20 top-1/2 h-[22rem] w-[22rem] -translate-y-1/2 rounded-full border border-primary/10" aria-hidden />
        <div className="pointer-events-none absolute right-24 top-1/2 h-2 w-2 -translate-y-1/2 rounded-full bg-primary shadow-[0_0_0_8px_rgb(22_119_255_/_0.08),0_0_30px_rgb(22_119_255_/_0.5)]" aria-hidden />
        <div className="pointer-events-none absolute bottom-16 left-16 h-px w-32 bg-primary/20" aria-hidden />
        <div className="pointer-events-none absolute bottom-16 left-16 h-16 w-px bg-primary/20" aria-hidden />

        <div className="relative z-10 w-full max-w-[560px]">
          <div className="mb-14 flex items-center gap-3">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary text-white shadow-[0_12px_28px_rgb(22_119_255_/_0.24)]">
              <Boxes className="h-6 w-6" aria-hidden />
            </div>
            <div>
              <p className="text-xl font-semibold tracking-tight text-text-primary">
                Edge Platform
              </p>
              <p className="mt-1 text-xs tracking-wide text-text-tertiary">
                Industrial IoT / HMI Platform
              </p>
            </div>
          </div>

          <p className="mb-4 text-sm font-medium tracking-[0.12em] text-primary">
            企业级智慧管理解决方案
          </p>
          <h1 className="text-4xl font-semibold leading-[1.2] tracking-[-0.03em] text-text-primary xl:text-5xl">
            安全 · 稳定 · 高效
          </h1>
          <div className="mt-6 h-px w-12 bg-primary/70" aria-hidden />
          <p className="mt-6 max-w-md text-base leading-7 text-text-secondary">
            统一权限、业务数据与运营流程，让管理工作清晰可控。
          </p>

          <div className="mt-16 grid max-w-md grid-cols-3 gap-8 border-t border-primary/15 pt-5">
            <div>
              <p className="text-lg font-semibold text-text-primary">统一</p>
              <p className="mt-1 text-xs text-text-tertiary">权限与数据视图</p>
            </div>
            <div>
              <p className="text-lg font-semibold text-text-primary">实时</p>
              <p className="mt-1 text-xs text-text-tertiary">关键状态可追踪</p>
            </div>
            <div>
              <p className="text-lg font-semibold text-text-primary">可靠</p>
              <p className="mt-1 text-xs text-text-tertiary">操作留痕可审计</p>
            </div>
          </div>
        </div>
      </section>

      <section className="flex min-h-screen w-full items-center justify-center bg-[radial-gradient(circle_at_top_right,_rgba(219,234,254,0.6),_transparent_42%),linear-gradient(135deg,_#f8fbff_0%,_#f2f6fb_100%)] px-5 py-10 sm:px-8 min-[992px]:px-12">
        <div className="w-full max-w-[420px]">
          <div className="w-full rounded-2xl border border-slate-900/[0.06] bg-white/[0.96] p-6 shadow-[0_20px_50px_rgb(15_23_42_/_0.08),0_2px_8px_rgb(15_23_42_/_0.04)] sm:p-8 min-[992px]:px-10 min-[992px]:py-9">
            <MobileBrand />

            <div className="mb-8">
              <p className="mb-2 text-sm font-medium text-primary">欢迎回来</p>
              <h2 className="text-2xl font-semibold tracking-tight text-text-primary">欢迎登录</h2>
              <p className="mt-2 text-sm text-text-tertiary">请输入账号信息进入系统</p>
            </div>

            <form className="space-y-4" onSubmit={handleSubmit}>
              <Field
                label="用户名"
                htmlFor="username"
                required
                error={errors.username}
              >
                <div className="relative">
                  <UserRound
                    className="pointer-events-none absolute left-3.5 top-1/2 h-[18px] w-[18px] -translate-y-1/2 text-slate-400"
                    aria-hidden
                  />
                  <Input
                    id="username"
                    value={username}
                    placeholder="请输入用户名"
                    onChange={(event) => {
                      setUsername(event.target.value);
                      setErrors((current) => ({ ...current, username: undefined, account: undefined }));
                    }}
                    className="h-[46px] rounded-[10px] border-slate-200 bg-slate-50/60 pl-[42px] pr-3 focus:bg-white"
                    autoComplete="username"
                    aria-invalid={Boolean(errors.username)}
                  />
                </div>
              </Field>

              <Field
                label="密码"
                htmlFor="password"
                required
                error={errors.password}
              >
                <div className="relative">
                  <LockKeyhole
                    className="pointer-events-none absolute left-3.5 top-1/2 h-[18px] w-[18px] -translate-y-1/2 text-slate-400"
                    aria-hidden
                  />
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    placeholder="请输入密码"
                    onChange={(event) => {
                      setPassword(event.target.value);
                      setErrors((current) => ({ ...current, password: undefined, account: undefined }));
                    }}
                    className="h-[46px] rounded-[10px] border-slate-200 bg-slate-50/60 pl-[42px] pr-[42px] focus:bg-white"
                    autoComplete="current-password"
                    aria-invalid={Boolean(errors.password)}
                  />
                  <button
                    type="button"
                    className="absolute right-3.5 top-1/2 -translate-y-1/2 rounded-md p-1 text-slate-400 transition-colors hover:text-text-secondary focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary"
                    aria-label={showPassword ? "隐藏密码" : "显示密码"}
                    onClick={() => setShowPassword((value) => !value)}
                  >
                    {showPassword ? <EyeOff className="h-[18px] w-[18px]" aria-hidden /> : <Eye className="h-[18px] w-[18px]" aria-hidden />}
                  </button>
                </div>
              </Field>

              <div className="flex items-center justify-between pt-1">
                <label htmlFor="remember-me" className="inline-flex cursor-pointer items-center gap-2 text-sm text-text-secondary">
                  <Checkbox id="remember-me" checked={rememberMe} onChange={(event) => setRememberMe(event.target.checked)} />
                  记住我
                </label>
                <span className="inline-flex items-center gap-1 text-xs text-text-tertiary">
                  <ShieldCheck className="h-3.5 w-3.5 text-success" aria-hidden />
                  安全登录
                </span>
              </div>

              {errors.account && (
                <p className="rounded-xl border border-red-100 bg-red-50 px-3.5 py-3 text-sm leading-5 text-error" role="alert">
                  {errors.account}
                </p>
              )}

              <Button
                type="submit"
                variant="primary"
                size="lg"
                className="mt-2 h-[46px] w-full rounded-[10px] shadow-[0_10px_24px_rgb(22_119_255_/_0.20)] transition-[transform,box-shadow] hover:-translate-y-0.5 hover:shadow-[0_14px_28px_rgb(22_119_255_/_0.26)]"
                disabled={submitting}
                aria-busy={submitting}
              >
                {submitting ? (
                  <>
                    <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" aria-hidden />
                    登录中…
                  </>
                ) : "登录"}
              </Button>
            </form>
          </div>

          <p className="mt-6 text-center text-xs text-slate-400">© 2026 Edge Platform</p>
        </div>
      </section>
    </main>
  );
}
