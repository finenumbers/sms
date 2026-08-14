import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import logo from "../assets/logo.png";

export type NavItem = { to: string; label: string };

const appVersion = String(import.meta.env.VITE_APP_VERSION ?? "dev").replace(/^v/i, "");

export function Shell({
  variant = "client",
  nav,
  user,
  aside,
  onLogout,
  children,
}: {
  variant?: "client" | "admin";
  nav: NavItem[];
  user?: string;
  aside?: ReactNode;
  onLogout: () => void;
  children: ReactNode;
}) {
  const admin = variant === "admin";
  return (
    <div className="flex h-screen overflow-hidden bg-zinc-50">
      <nav className="flex h-screen w-52 shrink-0 flex-col overflow-hidden p-3" style={{ backgroundColor: "#212124" }}>
        <img src={logo} alt="fine numbers" className="mb-3 w-full" />
        <div className="mb-3 border-t border-white/20" />
        <div className="min-h-0 flex-1 overflow-y-auto">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) =>
                `mb-1 block rounded-md px-3 py-2 text-sm font-bold ${
                  isActive ? "bg-[#FBE95F] text-black" : "text-white hover:bg-[#5A543B]"
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </div>
        <div className="mt-3 shrink-0">
          <div className="px-3 py-2 text-xs text-white/60">Версия {appVersion}</div>
          <div className="mb-1 border-t border-white/20" />
          <button
            type="button"
            onClick={onLogout}
            className="block w-full rounded-md px-3 py-2 text-left text-sm font-bold text-white hover:bg-[#5A543B]"
          >
            Выйти
          </button>
        </div>
      </nav>
      <div className="flex min-w-0 flex-1 flex-col">
        <header
          className={`sticky top-0 z-10 flex shrink-0 items-center justify-between px-4 py-3 ${
            admin ? "bg-[#FBE95F] text-black" : "border-b border-zinc-200 bg-white"
          }`}
        >
          <div className={`text-sm ${admin ? "font-semibold text-black" : "font-semibold text-zinc-600"}`}>{user}</div>
          <div className={`flex items-center gap-3 text-sm ${admin ? "text-black" : "text-zinc-600"}`}>{aside}</div>
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
