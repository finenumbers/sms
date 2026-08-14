import { useEffect, useRef } from "react";

export function InfiniteSentinel({
  onVisible,
  disabled,
}: {
  onVisible: () => void;
  disabled?: boolean;
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
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting) {
        onVisible();
      }
    });
    io.observe(el);
    return () => io.disconnect();
  }, [onVisible, disabled]);
  return <div ref={ref} className="h-8" aria-hidden />;
}
