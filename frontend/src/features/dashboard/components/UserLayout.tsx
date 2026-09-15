import { Link, useLocation, useNavigate } from "react-router-dom";
import type { ReactNode } from "react";
import { Bell, Handshake, LayoutDashboard, LogOut, Search, Settings2, User2 } from "lucide-react";
import { getCurrentUser, clearAuthStorage } from "../../../shared/auth/currentUser";
import type { UserRole } from "../../../shared/auth/currentUser";
import { logout as logoutApi } from "../../auth/api";

type NavItem = { label: string; to: string; icon: ReactNode };

const umkmNav: NavItem[] = [
  { label: "Dashboard", to: "/umkm", icon: <LayoutDashboard size={18} /> },
  { label: "Kelola Informasi", to: "/umkm/profile", icon: <User2 size={18} /> },
  { label: "Pengajuan Kemitraan", to: "/umkm/partnerships", icon: <Handshake size={18} /> },
  { label: "Pengaturan", to: "/umkm/settings", icon: <Settings2 size={18} /> },
];

const mitraNav: NavItem[] = [
  { label: "Dashboard", to: "/mitra", icon: <LayoutDashboard size={18} /> },
  { label: "Kemitraan", to: "/mitra/partnerships", icon: <Handshake size={18} /> },
  { label: "Kelola Informasi", to: "/mitra/profile", icon: <User2 size={18} /> },
];

type Props = {
  children: ReactNode;
  role: UserRole;
  title?: string;
  subtitle?: string;
};

export default function UserLayout({ children, role, title, subtitle }: Props) {
  const location = useLocation();
  const navigate = useNavigate();
  const user = getCurrentUser();
  const navItems = role === "UMKM" ? umkmNav : mitraNav;
  const brandColor = role === "UMKM" ? "#1f45b6" : "#1d4ed8";

  async function logout() {
    try {
      await logoutApi();
    } catch {
      // Local logout should still happen even if backend logout fails.
    } finally {
      clearAuthStorage();
      navigate("/", { replace: true });
    }
  }

  return (
    <div className="user-layout">
      {/* SIDEBAR */}
      <aside className="user-sidebar" style={{ background: brandColor }}>
        {/* Logo */}
        <div className="sidebar-logo">
          <img src="/tumbuh.png" alt="UMKM Tumbuh" className="sidebar-logo-img" />
        </div>

        {/* Nav */}
        <nav className="sidebar-nav">
          {navItems.map((item) => {
            const isActive = location.pathname === item.to;
            return (
              <Link
                key={item.to}
                to={item.to}
                className={`sidebar-nav-item ${isActive ? "active" : ""}`}
              >
                <span className="sidebar-nav-icon">{item.icon}</span>
                <span className="sidebar-nav-label">{item.label}</span>
              </Link>
            );
          })}
        </nav>

        {/* Logout */}
        <div className="sidebar-footer">
          <button className="sidebar-logout" onClick={logout}>
            <LogOut size={16} /> Keluar
          </button>
        </div>
      </aside>

      {/* MAIN CONTENT */}
      <div className="user-main">
        {/* Top bar */}
        <header className="user-topbar">
          {role === "MITRA" ? (
            <div className="topbar-mitra-brand">
              <div className="topbar-mitra-title">Monitoring Perkembangan Usaha</div>
            </div>
          ) : (
            <div className="topbar-search">
              <Search size={16} className="topbar-search-icon" />
              <input
                type="text"
                placeholder="Cari laporan atau produk..."
                className="topbar-search-input"
              />
            </div>
          )}

          {/* Right side */}
          <div className="topbar-right">
            <button className="topbar-notif" aria-label="Notifikasi">
              <Bell size={18} />
            </button>
            <div className="topbar-divider" />
            <div className="topbar-user">
              <div className="topbar-user-info">
                <span className="topbar-user-name">{user?.full_name ?? "Pengguna"}</span>
                <span className="topbar-user-role">
                  {role === "UMKM" ? "Owner" : "Mitra"}
                </span>
              </div>
              <div className="topbar-avatar" onClick={logout} title="Logout">
                {user?.full_name?.[0]?.toUpperCase() ?? "U"}
              </div>
            </div>
          </div>
        </header>

        {/* Page title area */}
        {(title || subtitle) && (
          <div className="user-page-header">
            {title && <h1 className="user-page-title">{title}</h1>}
            {subtitle && <p className="user-page-subtitle">{subtitle}</p>}
          </div>
        )}

        {/* Content */}
        <div className="user-content">
          {children}
        </div>
      </div>
    </div>
  );
}
