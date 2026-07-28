import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(`/api/v1/databases/${id}`, { method: 'GET' }, 'Could not load database.');
}

export async function DELETE(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(
    `/api/v1/databases/${id}`,
    { method: 'DELETE' },
    'Could not queue database deletion.',
  );
}
