import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(
    `/api/v1/domains/${id}/challenge`,
    { method: 'POST' },
    'Could not rotate the ownership challenge.',
  );
}
