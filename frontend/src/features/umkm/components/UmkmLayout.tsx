import { useState } from "react";
import { Bell } from "lucide-react";
import { getCurrentUser } from "../../../shared/auth/currentUser";
import Sidebar from "../../../shared/components/Sidebar";
import TransportToggle from "../../partnerships/components/TransportToggle";

type UmkmLayoutProps = {
  children: React.ReactNode;
  title?: string;
  subtitle?: string;
};

export default function UmkmLayout({ children, title, subtitle }: UmkmLayoutProps) {
  const user = getCurrentUser();
  const [collapsed, setCollapsed] = useState(false);

  const breadcrumb = user?.role === "MITRA" ? "Informasi Mitra" : "Informasi UMKM";
  const roleLabel =
    user?.role === "UMKM"
      ? "Owner UMKM"
      : user?.role === "MITRA"
        ? "Mitra"
        : user?.role ?? "User";

  return (
    <div className={`umkm-shell ${collapsed ? "sidebar-collapsed" : ""}`}>
      <Sidebar collapsed={collapsed} onToggle={() => setCollapsed((c) => !c)} />

      <section className="umkm-main">
        <header className="umkm-topbar">
          <div>
            <div className="umkm-breadcrumb">{breadcrumb}</div>
            {title ? <h1>{title}</h1> : null}
            {subtitle ? <p>{subtitle}</p> : null}
          </div>

          <div className="umkm-topbar-right">
            <TransportToggle />

            <button className="umkm-icon-btn" type="button" aria-label="Notifikasi">
              <Bell size={18} />
            </button>

            <div className="umkm-user-chip">
              <div>
                <strong>{user?.full_name ?? "User"}</strong>
                <span>{roleLabel}</span>
              </div>
              <div className="umkm-avatar">
                {user?.full_name?.[0]?.toUpperCase() ?? "U"}
              </div>
            </div>
          </div>
        </header>

        <div className="umkm-content">{children}</div>
      </section>
    </div>
  );
}
