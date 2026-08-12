import { getSession } from '@/lib/session';
import { apiJSON } from '@/lib/api';
import { BucketManager } from '@/components/storage/bucket-manager';
import type { BucketListEnvelope } from '@/lib/storage';

type PageProps = { params: Promise<{ projectId: string }> };

export default async function StoragePage({ params }: PageProps) {
  const [session, { projectId }] = await Promise.all([getSession(), params]);
  if (!session) return null;

  const envelope = await apiJSON<BucketListEnvelope>(
    `/api/v1/projects/${projectId}/storage/buckets?page=1&per_page=25`,
  ).catch(() => null);

  if (!envelope) {
    return (
      <main
        id="dashboard-content"
        className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
      >
        <div className="flex flex-col items-start gap-3 rounded-lg border p-5">
          <p role="alert" className="text-sm text-destructive">Could not load storage buckets.</p>
        </div>
      </main>
    );
  }

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-2xl">
        <p className="label-meta text-muted-foreground">Object Storage</p>
        <h1 className="heading-page mt-2">Buckets</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Create and manage object storage buckets for this project. Upload, download,
          and organize files with tenant-scoped access controls.
        </p>
      </header>
      <BucketManager projectId={projectId} initialData={envelope.data} />
    </main>
  );
}
