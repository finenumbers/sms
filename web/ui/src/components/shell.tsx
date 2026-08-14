import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { Button } from "./ui";

export type NavItem = { to: string; label: string };

export function Shell({
  brand,
  nav,
  user,
  aside,
  onLogout,
  children,
}: {
  brand: string;
  nav: NavItem[];
  user?: string;
  aside?: ReactNode;
  onLogout: () => void;
  children: ReactNode;
}) {
  return (
    <div className="flex h-screen overflow-hidden bg-zinc-50">
      <nav className="h-screen w-52 shrink-0 overflow-y-auto p-3" style={{ backgroundColor: "#212124" }}>
        {nav.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              `mb-1 block rounded-md px-3 py-2 text-sm font-bold text-white ${isActive ? "bg-white/10" : "hover:bg-white/10"}`
            }
          >
            {item.label}
          </NavLink>
        ))}
      </nav>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex shrink-0 items-center justify-between border-b border-zinc-200 bg-white px-4 py-3">
          <div className="font-semibold">{brand}</div>
          <div className="flex items-center gap-3 text-sm text-zinc-600">
            {aside}
            <span>{user}</span>
            <Button variant="ghost" type="button" onClick={onLogout}>
              Выйти
            </Button>
          </div>
        </header>
        <main className="min-w-0 flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}

export function LoginLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-50 p-4">
      <div className="w-full max-w-sm rounded-lg border border-zinc-200 bg-white p-6 shadow-sm">
        <h1 className="mb-4 text-lg font-semibold">{title}</h1>
        {children}
      </div>
    </div>
  );
}
