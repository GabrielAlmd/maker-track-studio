import { Suspense } from 'react';
import { Navigate, useLocation, useParams } from 'react-router-dom';
import { useFeatures } from '../../../contexts/featureState';
import { LoadingState } from '../../../components/ui/PageState';
import SalesOrder from './SalesOrder';

export function LegacySalesRoute({ create = false }) {
  const { id } = useParams();
  const { search } = useLocation();
  const suffix = id ? `/${encodeURIComponent(id)}` : create ? '/new' : '';
  return <Navigate to={`/business/orders${suffix}${search}`} replace />;
}

export function OrderRoute() {
  const { id } = useParams();
  return <SalesOrder key={id ?? 'new'} />;
}

export function BusinessRoute({ children }) {
  const { features, loading } = useFeatures();
  if (loading) return <LoadingState />;
  if (!features?.sales_orders) return <Navigate to="/dashboard" replace />;
  return <Suspense fallback={<LoadingState />}>{children}</Suspense>;
}
