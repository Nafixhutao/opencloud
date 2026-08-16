import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; serviceId: string }> };

export async function GET(request: Request, context: RouteContext) {
  const { projectId, serviceId } = await context.params;
  const parsed = Math.min(Math.max(parseInt(new URL(request.url).searchParams.get('limit') ?? '50'), 1), 100);
  const query = new URLSearchParams({ limit: String(Number.isNaN(parsed) ? 50 : parsed) });
  return proxyAPI(
    `/api/v1/projects/${projectId}/services/${serviceId}/environment/audit?${query}`,
    { method: 'GET' },
    'Could not load the environment activity trail.',
  );
}
