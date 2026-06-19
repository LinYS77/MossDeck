import type { HTMLAttributes } from "react";
import { cn } from "../lib/cn";
import styles from "./GlassPanel.module.css";

type Variant = "default" | "strong" | "subtle";

export interface GlassPanelProps extends HTMLAttributes<HTMLDivElement> {
  /** Visual weight of the panel. */
  variant?: Variant;
  /** Adds stronger border + glow shadow (for the focal search, dialogs). */
  interactive?: boolean;
  /** Element to render. Defaults to a div. */
  as?: "div" | "section" | "a";
  href?: string;
  target?: string;
  rel?: string;
}

const variantClass: Record<Variant, string> = {
  default: styles.default,
  strong: styles.strong,
  subtle: styles.subtle,
};

/** Frosted-glass surface used across the app. One place to tune blur/border. */
export function GlassPanel({
  variant = "default",
  interactive = false,
  className,
  children,
  as = "div",
  href,
  target,
  rel,
  ...rest
}: GlassPanelProps) {
  const classes = cn(
    styles.panel,
    variantClass[variant],
    interactive && styles.interactive,
    className,
  );

  if (as === "a") {
    return (
      <a className={classes} href={href} target={target} rel={rel}>
        {children}
      </a>
    );
  }

  if (as === "section") {
    return (
      <section className={classes} {...rest}>
        {children}
      </section>
    );
  }

  return (
    <div className={classes} {...rest}>
      {children}
    </div>
  );
}
