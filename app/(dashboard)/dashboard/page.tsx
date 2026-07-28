import {
  ArrowRight,
  CheckCircle2,
  Database,
  Globe2,
  HardDrive,
  KeyRound,
  Server,
  ShieldCheck,
} from 'lucide-react';
import type { Metadata } from 'next';
import { redirect } from 'next/navigation';

import { buttonVariants } from '@/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { apiJSON } from '@/lib/api';
import type { DatabasesEnvelope } from '@/lib/databases';
import { getSession } from '@/lib/session';
import type { SitesEnvelope } from '@/lib/sites';
import { cn } from '@/lib/utils';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Dashboard',
};

const setupSteps = [
  {
    title: 'Create a Site',
    description: 'Choose a runtime and prepare the first hosting service.',
    meta: 'Runtime · Region · Source',
    icon: Server,
  },
  {
    title: 'Connect a Domain',
    description: 'Point a domain to the service and keep DNS details close.',
    meta: 'Domain · DNS · Routing',
    icon: Globe2,
  },
  {
    title: 'Secure the Route',
    description: 'Issue a certificate and verify the public endpoint.',
    meta: 'TLS · Health · Edge',
    icon: ShieldCheck,
  },
];

export default async function DashboardPage() {
  const session = await getSession();
  if (!session) {
    redirect('/login');
  }

  const firstName = session.user.name.trim().split(/\s+/)[0] || 'there';
  const memberSince = new Intl.DateTimeFormat('en', {
    month: 'short',
    year: 'numeric',
  }).format(new Date(session.user.createdAt));
  const [sitesResult, databasesResult] = await Promise.allSettled([
    apiJSON<SitesEnvelope>('/api/v1/sites'),
    apiJSON<DatabasesEnvelope>('/api/v1/databases'),
  ]);
  const sites = sitesResult.status === 'fulfilled' ? sitesResult.value.data : null;
  const databases =
    databasesResult.status === 'fulfilled' ? databasesResult.value.data : null;
  const activeSites = sites?.filter((site) => site.status === 'active').length ?? 0;
  const activeDatabases =
    databases?.filter((database) => database.status === 'active').length ?? 0;
  const metrics = [
    {
      label: 'Active Sites',
      value: sites ? String(activeSites) : 'Unavailable',
      detail: sites ? `${sites.length} total workloads` : 'Control plane unavailable',
      icon: Globe2,
    },
    { label: 'Domains', value: '0', detail: 'No routes connected', icon: HardDrive },
    {
      label: 'Databases',
      value: databases ? String(databases.length) : 'Unavailable',
      detail: databases
        ? `${activeDatabases} active instances`
        : 'Control plane unavailable',
      icon: Database,
    },
    {
      label: 'Email Status',
      value: session.user.emailVerified ? 'Verified' : 'Pending',
      detail: session.user.emailVerified ? 'Identity confirmed' : 'Verification required',
      icon: KeyRound,
    },
  ];

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-16 px-6 py-12 sm:px-8 sm:py-16"
    >
      <section className="flex flex-col items-start gap-8 lg:flex-row lg:items-end lg:justify-between">
        <div className="flex max-w-3xl flex-col gap-3">
          <p className="label-meta text-muted-foreground">Workspace Overview</p>
          <h1 className="heading-page text-balance">Good to see you, {firstName}.</h1>
          <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
            Your control plane is ready. Start with one site, then connect the services
            that keep it available.
          </p>
        </div>
        <a href="#workspace" className={buttonVariants({ size: 'lg' })}>
          Open Setup Guide
          <ArrowRight data-icon="inline-end" />
        </a>
      </section>

      <section aria-labelledby="overview-heading" className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-4">
          <h2 id="overview-heading" className="heading-section">
            Resource Overview
          </h2>
          <p className="label-meta hidden text-muted-foreground sm:block">
            Live Workspace State
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {metrics.map(({ label, value, detail, icon: Icon }) => (
            <Card key={label} size="sm">
              <CardHeader>
                <CardDescription>{label}</CardDescription>
                <CardAction className="text-muted-foreground">
                  <Icon className="size-4" aria-hidden="true" />
                </CardAction>
              </CardHeader>
              <CardContent className="mt-auto flex flex-col gap-2">
                <p className="text-2xl font-semibold tracking-[-0.04em] tabular-nums">
                  {value}
                </p>
                <p className="truncate text-xs text-muted-foreground">{detail}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>

      <section
        id="workspace"
        aria-labelledby="workspace-heading"
        className="grid scroll-mt-24 gap-4 lg:grid-cols-12"
      >
        <Card className="lg:col-span-8">
          <CardHeader>
            <p className="label-meta text-info">Setup Guide</p>
            <CardTitle>
              <h2 id="workspace-heading" className="heading-section">
                Bring Your First Site Online
              </h2>
            </CardTitle>
            <CardDescription>
              Follow the resource order below so every dependency stays traceable.
            </CardDescription>
            <CardAction className="label-meta text-muted-foreground">0 of 3 Complete</CardAction>
          </CardHeader>
          <CardContent>
            <ol className="relative flex flex-col before:absolute before:bottom-8 before:left-[1.1875rem] before:top-8 before:w-px before:bg-border">
              {setupSteps.map(({ title, description, meta, icon: Icon }, index) => (
                <li key={title} className="relative flex gap-4 py-5 first:pt-0 last:pb-0">
                  <span
                    className={cn(
                      'relative flex size-10 shrink-0 items-center justify-center rounded-full border',
                      index === 0
                        ? 'border-info bg-info text-white'
                        : 'border-border bg-background text-muted-foreground'
                    )}
                  >
                    <Icon className="size-4" aria-hidden="true" />
                  </span>
                  <div className="flex min-w-0 flex-1 flex-col gap-1 pt-1">
                    <div className="flex items-center justify-between gap-4">
                      <h3 className="text-sm font-semibold">{title}</h3>
                      <span className="label-meta text-muted-foreground">
                        {String(index + 1).padStart(2, '0')}
                      </span>
                    </div>
                    <p className="text-sm leading-5 text-muted-foreground">{description}</p>
                    <p className="label-meta mt-1 text-muted-foreground">{meta}</p>
                  </div>
                </li>
              ))}
            </ol>
          </CardContent>
          <CardFooter className="gap-2 text-sm text-muted-foreground">
            <CheckCircle2 className="size-4 text-info" aria-hidden="true" />
            {sites === null
              ? 'Site state is temporarily unavailable.'
              : activeSites > 0
                ? `${activeSites} active ${activeSites === 1 ? 'site' : 'sites'} in this workspace.`
                : 'No deployments yet. Create a site to begin the workflow.'}
          </CardFooter>
        </Card>

        <Card className="lg:col-span-4">
          <CardHeader>
            <p className="label-meta text-muted-foreground">Account</p>
            <CardTitle>
              <h2 className="text-base font-semibold">Workspace Identity</h2>
            </CardTitle>
            <CardDescription>The account securing this control-plane session.</CardDescription>
          </CardHeader>
          <CardContent className="mt-auto">
            <dl className="grid grid-cols-[auto_1fr] gap-x-5 gap-y-5">
              <dt className="text-muted-foreground">Name</dt>
              <dd className="min-w-0 truncate text-right font-medium">{session.user.name}</dd>
              <dt className="text-muted-foreground">Email</dt>
              <dd className="min-w-0 truncate text-right">{session.user.email}</dd>
              <dt className="text-muted-foreground">Member Since</dt>
              <dd className="text-right tabular-nums">{memberSince}</dd>
              <dt className="text-muted-foreground">Region</dt>
              <dd className="text-right">Singapore</dd>
            </dl>
          </CardContent>
          <CardFooter className="gap-2 text-sm">
            <CheckCircle2 className="size-4 text-success" aria-hidden="true" />
            Session Active
          </CardFooter>
        </Card>
      </section>
    </main>
  );
}
