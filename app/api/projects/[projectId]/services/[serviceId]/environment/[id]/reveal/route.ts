import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; serviceId: string; id: string }> };

// The Go handler sets no-store cache headers on revealed secrets; proxyAPI
// forwards the cache-control allowlist so they survive the BFF hop.
export async function POST(_request: Request, context: RouteContext) {
  const { projectId, serviceId, id } = await context.params;
  return proxyAPI(
    `/api/v1/projects/${projectId}/services/${serviceId}/environment/${id}/reveal`,
    { method: 'POST' },
    'Could not reveal the secret.',
  );
}
