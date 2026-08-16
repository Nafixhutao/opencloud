import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string }> };

/**
 * BFF proxy for the control plane's project log query endpoint. Filter
 * parameters are forwarded verbatim — the Go API owns their validation.
 */
export async function GET(request: Request, context: RouteContext) {
  const { projectId } = await context.params;
  const search = new URL(request.url).searchParams.toString();
  return proxyAPI(
    `/api/v1/projects/${projectId}/logs${search ? `?${search}` : ''}`,
    { method: 'GET' },
    'Could not load project logs.',
  );
}
