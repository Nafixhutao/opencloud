import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(
    `/api/v1/sites/${id}/suspend`,
    { method: 'POST' },
    'Could not queue site suspension.',
  );
}
