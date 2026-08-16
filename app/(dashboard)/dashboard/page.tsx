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
import type { ResourceOverviewEnvelope } from '@/lib/resource-overview';
import { getSession } from '@/lib/session';
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
  const overview = await apiJSON<ResourceOverviewEnvelope>('/api/v1/overview').then(
    (response) => response.data,
    () => null,
  );
  const metrics = [
    {
      label: 'Active Sites',
      value: overview ? String(overview.sites_active) : 'Unavailable',
      detail: overview
        ? `${overview.sites_total} total workloads`
        : 'Control plane unavailable',
      icon: Globe2,
    },
    { label: 'Domains', value: '0', detail: 'No routes connected', icon: HardDrive },
    {
      label: 'Databases',
      value: overview ? String(overview.databases_total) : 'Unavailable',
      detail: overview
        ? `${overview.databases_active} active instances`
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
      className="mx-auto flex w-full max-w-[1400px] scroll-mt-20 flex-col gap-16 px-6 py-12 sm:px-8 sm:py-16"
    >
      <section className="relative overflow-hidden rounded-xl border border-border bg-card/50 p-8 shadow-lg backdrop-blur-sm sm:p-12 lg:p-16">
        <div className="absolute -left-1/4 -top-1/4 h-64 w-64 rounded-full bg-primary/10 blur-3xl"></div>
        <div className="absolute -right-1/4 -bottom-1/4 h-64 w-64 rounded-full bg-foreground/5 blur-3xl"></div>
        <div className="relative z-10 flex flex-col items-start gap-8 lg:flex-row lg:items-end lg:justify-between">
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
        </div>
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
        <div className="relative z-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {metrics.map(({ label, value, detail, icon: Icon }) => (
            <Card key={label} size="sm" className="group hover:border-primary/50 transition-all duration-300 hover:shadow-md hover:translate-y-[-2px]">
              <CardHeader>
                <CardDescription>{label}</CardDescription>
                <CardAction className="text-muted-foreground group-hover:text-primary transition-colors duration-300">
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
        <Card className="lg:col-span-8 group hover:border-primary/30 transition-all duration-300">
          <CardHeader>
            <p className="label-meta text-muted-foreground">Setup Guide</p>
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
                      'relative flex size-10 shrink-0 items-center justify-center rounded-full border transition-all duration-300 group-hover:border-primary/50',
                      index === 0
                        ? 'border-border bg-foreground text-background shadow-lg'
                        : 'border-border bg-background text-muted-foreground hover:border-primary/30'
                    )}
                  >
                    <Icon className="size-4" aria-hidden="true" />
                  </span>
                  <div className="flex min-w-0 flex-1 flex-col gap-1 pt-1">
                    <div className="flex items-center justify-between gap-4">
                      <h3 className="text-sm font-semibold group-hover:text-primary transition-colors duration-300">{title}</h3>
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
            <CheckCircle2 className="size-4 text-muted-foreground" aria-hidden="true" />
            {overview === null
              ? 'Site state is temporarily unavailable.'
              : overview.sites_active > 0
                ? `${overview.sites_active} active ${overview.sites_active === 1 ? 'site' : 'sites'} in this workspace.`
                : 'No deployments yet. Create a site to begin the workflow.'}
          </CardFooter>
        </Card>

        <Card className="lg:col-span-4 group hover:border-primary/30 transition-all duration-300">
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
              <dd className="min-w-0 truncate text-right font-medium text-primary">{session.user.name}</dd>
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
