import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ id: string; sessionId: string }> };

export async function DELETE(_request: Request, context: RouteContext) {
  const { id, sessionId } = await context.params;
  return proxyAPI(
    `/api/v1/databases/${id}/console/sessions/${sessionId}`,
    { method: 'DELETE', cache: 'no-store' },
    'Could not revoke the console session.',
  );
}
