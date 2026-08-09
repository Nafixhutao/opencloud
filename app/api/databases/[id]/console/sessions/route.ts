import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const body = await request.json().catch(() => null);
  return proxyAPI(
    `/api/v1/databases/${id}/console/sessions`,
    {
      method: 'POST',
      cache: 'no-store',
      headers: { 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    },
    'Could not start a console session.',
  );
}
