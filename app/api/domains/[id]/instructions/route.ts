import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(
    `/api/v1/domains/${id}/instructions`,
    { method: 'GET' },
    'Could not load verification instructions.',
  );
}