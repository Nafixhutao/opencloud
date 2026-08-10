import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; bucketId: string }> };

export async function DELETE(_request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}`,
    { method: 'DELETE' },
    'Could not queue bucket deletion.',
  );
}
