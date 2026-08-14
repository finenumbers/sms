import { useEffect, useRef } from "react";

export function InfiniteSentinel({
  onVisible,
  disabled,
  root,
}: {
  onVisible: () => void;
  disabled?: boolean;
  root?: Element | null;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (disabled) {
      return;
    }
    const el = ref.current;
    if (!el) {
      return;
    }
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          onVisible();
        }
      },
      { root: root ?? null },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [onVisible, disabled, root]);
  return <div ref={ref} className="h-8" aria-hidden />;
}
