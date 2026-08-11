import { ArrowLeft } from 'lucide-react';
import Link from 'next/link';

import { DomainManager } from '@/components/domains/domain-manager';
import { Badge } from '@/components/ui/badge';
import { apiJSON } from '@/lib/api';
import type { DomainsEnvelope } from '@/lib/domains';
import type { Site } from '@/lib/sites';

type SiteDomainPageProps = {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ page?: string }>;
};

export default async function SiteDomainPage({ params, searchParams }: SiteDomainPageProps) {
  const { id } = await params;
  const rawPage = Number((await searchParams).page ?? '1');
  const page = Number.isInteger(rawPage) && rawPage > 0 ? rawPage : 1;
  const domainPath = page === 1
    ? `/api/v1/sites/${id}/domains`
    : `/api/v1/sites/${id}/domains?page=${page}&per_page=25`;
  const [siteEnvelope, domains] = await Promise.all([
    apiJSON<{ data: Site }>(`/api/v1/sites/${id}`),
    apiJSON<DomainsEnvelope>(domainPath),
  ]);
  const site = siteEnvelope.data;

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-3xl">
        <Link
          href="/sites"
          className="inline-flex items-center gap-1.5 text-sm text-link-signal hover:underline"
        >
          <ArrowLeft aria-hidden="true" className="size-4" />
          Back to sites
        </Link>
        <p className="label-meta mt-8 text-muted-foreground">Site domains</p>
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <h1 className="heading-page break-all">{site.domain}</h1>
          <Badge variant="outline">Primary hostname</Badge>
        </div>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Prove ownership with DNS, publish exact-host routing, and monitor HTTPS for
          every customer hostname attached to this site.
        </p>
      </header>
      <DomainManager site={site} initialData={domains} />
    </main>
  );
}
