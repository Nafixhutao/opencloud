import { getSession } from '@/lib/session';
import { apiJSON } from '@/lib/api';
import { BucketManager } from '@/components/storage/bucket-manager';
import type { BucketListEnvelope } from '@/lib/storage';

type PageProps = { params: Promise<{ projectId: string }> };

export default async function StoragePage({ params }: PageProps) {
  const session = await getSession();
  if (!session) return null;

  const { projectId } = await params;
  const envelope = await apiJSON<BucketListEnvelope>(
    `/api/v1/projects/${projectId}/storage/buckets?page=1&per_page=25`,
  ).catch((err) => {
    console.error('Failed to load storage buckets:', err);
    return null;
  });

  if (!envelope) {
    return (
      <div className="py-8 text-center">
        <p className="text-sm text-destructive">Could not load storage buckets.</p>
      </div>
    );
  }

  return <BucketManager projectId={projectId} initialData={envelope.data} />;
}
