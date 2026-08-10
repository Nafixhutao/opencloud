import Link from 'next/link';

import { getSession } from '@/lib/session';
import { apiJSON } from '@/lib/api';
import { ObjectBrowser } from '@/components/storage/object-browser';
import { Button } from '@/components/ui/button';
import { ArrowLeftIcon } from 'lucide-react';
import type { BucketEnvelope } from '@/lib/storage';

type PageProps = { params: Promise<{ projectId: string; bucketId: string }> };

export default async function BucketDetailPage({ params }: PageProps) {
  const session = await getSession();
  if (!session) return null;

  const { projectId, bucketId } = await params;
  const envelope = await apiJSON<BucketEnvelope>(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}`,
  ).catch(() => null);

  if (!envelope?.data) {
    return (
      <div className="py-8 text-center">
        <p className="text-muted-foreground">Bucket not found.</p>
        <Link href={`/projects/${projectId}/storage`}>
          <Button variant="outline" size="sm" className="mt-4">
            <ArrowLeftIcon data-icon="inline-start" />
            Back to Buckets
          </Button>
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <Link href={`/projects/${projectId}/storage`}>
        <Button variant="ghost" size="sm">
          <ArrowLeftIcon data-icon="inline-start" />
          Back to Buckets
        </Button>
      </Link>
      <ObjectBrowser projectId={projectId} bucketId={bucketId} bucketName={envelope.data.name} />
    </div>
  );
}
