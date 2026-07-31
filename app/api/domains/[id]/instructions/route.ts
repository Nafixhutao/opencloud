import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const response = await proxyAPI(
    `/api/v1/domains/${id}/instructions`,
    { method: 'GET' },
    'Could not load DNS instructions.',
  );
  response.headers.set('Cache-Control', 'private, no-store');
  response.headers.set('Pragma', 'no-cache');
  return response;
}
