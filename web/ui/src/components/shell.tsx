import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import logo from "../assets/logo.png";

export type NavItem = { to: string; label: string; disabled?: boolean } | { separator: true };

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
  user?: ReactNode;
  aside?: ReactNode;
  onLogout: () => void;
  children: ReactNode;
}) {
  const admin = variant === "admin";
  return (
    <div className="grid h-screen grid-cols-[13rem_minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-zinc-50">
      <div className="flex flex-col justify-end border-b border-white/20 px-3 pt-3" style={{ backgroundColor: "#212124" }}>
        <img src={logo} alt="fine numbers" className="mx-auto mb-3 block w-[85%]" />
      </div>
      <header
        className={`flex items-center justify-between px-4 ${
          admin ? "bg-[#FBE95F] text-black" : "border-b border-zinc-200 bg-white"
        }`}
      >
        <div className={`text-sm ${admin ? "font-semibold text-black" : "text-black"}`}>{user}</div>
        <div className="flex items-center gap-3 text-sm">{aside}</div>
      </header>
      <nav className="flex min-h-0 flex-col overflow-hidden px-3 pb-3" style={{ backgroundColor: "#212124" }}>
        <div className="min-h-0 flex-1 overflow-y-auto pt-3">
          {nav.map((item, i) =>
            "separator" in item ? (
              <div key={`sep-${i}`} className="my-2 border-t border-white/20" />
            ) : (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) =>
                  item.disabled
                    ? `mb-1 block rounded-md px-3 py-2 text-sm font-bold text-white/40 ${
                        isActive ? "bg-white/10" : "hover:bg-white/5"
                      }`
                    : `mb-1 block rounded-md px-3 py-2 text-sm font-bold ${
                        isActive ? "bg-[#FBE95F] text-black" : "text-white hover:bg-[#5A543B]"
                      }`
                }
              >
                <span className="block">{item.label}</span>
                {item.disabled ? <span className="mt-0.5 block text-xs font-normal text-white/35">нет тарифа</span> : null}
              </NavLink>
            ),
          )}
        </div>
        <div className="mt-3 shrink-0">
          <div className="px-3 py-2 text-xs text-white/60">Версия {appVersion}</div>
          <div className="mb-1 border-t border-white/20" />
          <button
            type="button"
            onClick={onLogout}
            className="block w-full rounded-md bg-[#FBE95F] px-3 py-2 text-left text-sm font-bold text-black hover:bg-[#F0DC4A]"
          >
            Выйти
          </button>
        </div>
      </nav>
      <main className="min-h-0 overflow-y-auto p-6">{children}</main>
    </div>
  );
}

export function LoginLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-50 p-4">
      <div className="w-full max-w-sm rounded-lg border border-zinc-200 bg-white p-6 shadow-sm">
        <img src={logo} alt="fine numbers" className="mx-auto mb-5 block w-[80%]" />
        <h1 className="mb-4 text-lg font-semibold">{title}</h1>
        {children}
      </div>
    </div>
  );
}
