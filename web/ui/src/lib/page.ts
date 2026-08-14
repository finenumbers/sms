export const PAGE_SIZE = 50;
export const INFINITE_PAGE_SIZE = 100;

export function withPage(path: string, offset: number, extra: Record<string, string> = {}, limit = PAGE_SIZE): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(extra)) {
    if (v) {
      p.set(k, v);
    }
  }
  p.set("limit", String(limit));
  p.set("offset", String(offset));
  return `${path}?${p.toString()}`;
}
