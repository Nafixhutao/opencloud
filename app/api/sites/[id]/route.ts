import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(`/api/v1/sites/${id}`, { method: 'GET' }, 'Could not load site.');
}

export async function DELETE(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(
    `/api/v1/sites/${id}`,
    { method: 'DELETE' },
    'Could not queue site deletion.',
  );
}
