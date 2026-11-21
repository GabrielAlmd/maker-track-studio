import { Link } from 'react-router-dom';

export default function KpiCard({ label, value, tone, icon, to, note }) {
  const Icon = icon;
  return <Link to={to} className="dashboard-kpi-card" data-tone={tone}>
    <div className="dashboard-kpi-heading">
      <h2 className="dashboard-kpi-label">{label}</h2>
      <span className="dashboard-kpi-icon" aria-hidden="true"><Icon className="h-4 w-4" /></span>
    </div>
    <p className="dashboard-kpi-value">{value}</p>
    {note && <p className="dashboard-kpi-note">{note}</p>}
  </Link>;
}
