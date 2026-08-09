import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const body = await request.json().catch(() => null);
  if (!body) {
    return Response.json(
      { error: { code: 'BAD_REQUEST', message: 'Missing request body' } },
      { status: 400 },
    );
  }
  return proxyAPI(
    `/api/v1/databases/${id}/console/execute`,
    {
      method: 'POST',
      cache: 'no-store',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
    'Could not execute the query.',
  );
}
