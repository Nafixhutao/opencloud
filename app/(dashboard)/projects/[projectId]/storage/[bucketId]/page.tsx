import Link from 'next/link';

import { getSession } from '@/lib/session';
import { apiJSON } from '@/lib/api';
import { ObjectBrowser } from '@/components/storage/object-browser';
import { Button } from '@/components/ui/button';
import { ArrowLeftIcon } from 'lucide-react';
import type { BucketEnvelope } from '@/lib/storage';

type PageProps = { params: Promise<{ projectId: string; bucketId: string }> };

export default async function BucketDetailPage({ params }: PageProps) {
  const [session, { projectId, bucketId }] = await Promise.all([getSession(), params]);
  if (!session) return null;

  const envelope = await apiJSON<BucketEnvelope>(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}`,
  ).catch(() => null);

  if (!envelope?.data) {
    return (
      <main
        id="dashboard-content"
        className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
      >
        <div className="flex flex-col items-start gap-3 rounded-lg border p-5">
          <p role="alert" className="text-sm text-muted-foreground">Bucket not found.</p>
          <Link href={`/projects/${projectId}/storage`}>
            <Button variant="outline" size="sm">
              <ArrowLeftIcon data-icon="inline-start" />
              Back to Buckets
            </Button>
          </Link>
        </div>
      </main>
    );
  }

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header>
        <Link
          href={`/projects/${projectId}/storage`}
          className="inline-flex items-center gap-1.5 text-sm text-link-signal hover:underline"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Back to buckets
        </Link>
        <p className="label-meta mt-6 text-muted-foreground">Object Storage</p>
        <h1 className="heading-page mt-2">{envelope.data.name}</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Browse, upload, and manage objects in this bucket.
        </p>
      </header>
      <ObjectBrowser projectId={projectId} bucketId={bucketId} bucketName={envelope.data.name} />
    </main>
  );
}
