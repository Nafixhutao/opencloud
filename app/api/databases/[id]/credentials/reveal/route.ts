import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const response = await proxyAPI(
    `/api/v1/databases/${id}/credentials/reveal`,
    { method: 'POST', cache: 'no-store' },
    'Could not reveal database credentials.',
  );
  response.headers.set('Cache-Control', 'no-store, private');
  response.headers.set('Pragma', 'no-cache');
  return response;
}
