import { proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; bucketId: string }> };

export async function GET(request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  const searchParams = new URL(request.url).searchParams;
  const qs = searchParams.toString();
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/presigned-get${qs ? `?${qs}` : ''}`,
    { method: 'GET' },
    'Could not generate presigned URL.',
  );
}
