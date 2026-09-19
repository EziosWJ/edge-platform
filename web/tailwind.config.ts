import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        primary: "var(--color-primary)",
        "primary-hover": "var(--color-primary-hover)",
        background: "var(--color-background)",
        surface: "var(--color-surface)",
        border: "var(--color-border)",
        "text-primary": "var(--color-text-primary)",
        "text-secondary": "var(--color-text-secondary)",
        "text-tertiary": "var(--color-text-tertiary)",
        success: "var(--color-success)",
        warning: "var(--color-warning)",
        error: "var(--color-error)",
        info: "var(--color-info)",
        "overlay-background": "var(--color-overlay-background)",
        "success-background": "var(--color-success-background)",
        "success-border": "var(--color-success-border)",
        "warning-background": "var(--color-warning-background)",
        "warning-border": "var(--color-warning-border)",
        "error-background": "var(--color-error-background)",
        "error-border": "var(--color-error-border)",
        "info-background": "var(--color-info-background)",
        "info-border": "var(--color-info-border)",
        "neutral-background": "var(--color-neutral-background)",
        "neutral-border": "var(--color-neutral-border)",
      },
      spacing: {
        "space-1": "var(--space-1)",
        "space-2": "var(--space-2)",
        "space-3": "var(--space-3)",
        "space-4": "var(--space-4)",
        "space-5": "var(--space-5)",
        "space-6": "var(--space-6)",
        "space-8": "var(--space-8)",
      },
      padding: {
        card: "var(--card-padding)",
        "card-compact": "var(--card-padding-compact)",
      },
      height: {
        "control-sm": "var(--control-height-sm)",
        "control-md": "var(--control-height-md)",
        "control-lg": "var(--control-height-lg)",
      },
      minHeight: {
        "control-textarea": "var(--control-height-textarea)",
      },
      maxWidth: {
        "modal-sm": "var(--modal-width-sm)",
        "modal-md": "var(--modal-width-md)",
        "modal-lg": "var(--modal-width-lg)",
      },
      borderRadius: {
        admin: "var(--radius-admin)",
        tight: "var(--radius-tight)",
        tag: "var(--radius-tag)",
        control: "var(--radius-control)",
        card: "var(--radius-card)",
        "card-lg": "var(--radius-card-lg)",
        pill: "var(--radius-pill)",
      },
      boxShadow: {
        admin: "var(--shadow-admin)",
        none: "var(--shadow-none)",
        subtle: "var(--shadow-subtle)",
        floating: "var(--shadow-floating)",
      },
      fontSize: {
        "page-title": [
          "var(--font-size-page-title)",
          { lineHeight: "var(--line-height-page-title)" },
        ],
        "section-title": [
          "var(--font-size-section-title)",
          { lineHeight: "var(--line-height-section-title)" },
        ],
        "card-title": [
          "var(--font-size-card-title)",
          { lineHeight: "var(--line-height-card-title)" },
        ],
        body: [
          "var(--font-size-body)",
          { lineHeight: "var(--line-height-body)" },
        ],
        "form-label": [
          "var(--font-size-form-label)",
          { lineHeight: "var(--line-height-form-label)" },
        ],
        "body-secondary": [
          "var(--font-size-body-secondary)",
          { lineHeight: "var(--line-height-body-secondary)" },
        ],
        helper: [
          "var(--font-size-helper)",
          { lineHeight: "var(--line-height-helper)" },
        ],
        "table-cell": [
          "var(--font-size-table-cell)",
          { lineHeight: "var(--line-height-table-cell)" },
        ],
      },
      fontFamily: {
        sans: [
          "Inter",
          "PingFang SC",
          "Microsoft YaHei",
          "Noto Sans SC",
          "sans-serif",
        ],
      },
    },
  },
  plugins: [],
} satisfies Config;
