import { Box, CheckCircle2, GitBranch, ShieldCheck } from 'lucide-react';

import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

const deploySteps = [
  {
    title: 'Repository Connected',
    detail: 'main · commit 8f13a2c',
    icon: GitBranch,
  },
  {
    title: 'Runtime Prepared',
    detail: 'Node.js 22 · Singapore',
    icon: Box,
  },
  {
    title: 'Route Secured',
    detail: 'TLS issued · edge ready',
    icon: ShieldCheck,
  },
];

export function AuthSpotlight() {
  return (
    <aside
      className="flex min-h-[34rem] items-center border-t border-border bg-secondary p-6 sm:p-10 lg:min-h-svh lg:border-l lg:border-t-0 lg:p-12"
      aria-label="Cevra deployment workflow preview"
    >
      <div className="mx-auto flex w-full max-w-[32rem] flex-col gap-8">
        <header className="flex max-w-md flex-col gap-3">
          <p className="label-meta text-muted-foreground">Deployment Workflow</p>
          <h2 className="heading-section text-balance">
            Every resource in one traceable path.
          </h2>
          <p className="text-sm leading-6 text-muted-foreground">
            Connect source, prepare the runtime, and secure the route without losing the
            thread between them.
          </p>
        </header>

        <Card className="[--card-spacing:--spacing(5)]">
          <CardHeader>
            <CardTitle>sample-api</CardTitle>
            <CardDescription>Production deployment</CardDescription>
            <CardAction className="flex items-center gap-2 text-sm text-success">
              <CheckCircle2 className="size-4" aria-hidden="true" />
              Ready
            </CardAction>
          </CardHeader>
          <CardContent>
            <ol className="relative flex flex-col before:absolute before:bottom-8 before:left-[1.0625rem] before:top-8 before:w-px before:bg-foreground/20">
              {deploySteps.map(({ title, detail, icon: Icon }) => (
                <li key={title} className="relative flex gap-4 py-4 first:pt-0 last:pb-0">
                  <span className="relative flex size-9 shrink-0 items-center justify-center rounded-full border border-foreground/20 bg-background text-success">
                    <Icon className="size-4" aria-hidden="true" />
                  </span>
                  <div className="flex min-w-0 flex-col gap-1 pt-1.5">
                    <p className="text-sm font-medium">{title}</p>
                    <p className="label-meta truncate text-muted-foreground">{detail}</p>
                  </div>
                </li>
              ))}
            </ol>
          </CardContent>
          <CardFooter className="justify-between gap-4">
            <span className="label-meta text-muted-foreground">Build 00:42</span>
            <span className="label-meta text-success">Healthy</span>
          </CardFooter>
        </Card>

        <dl className="grid grid-cols-3 gap-4">
          <div>
            <dt className="label-meta text-muted-foreground">Regions</dt>
            <dd className="mt-1 text-sm font-medium tabular-nums">1</dd>
          </div>
          <div>
            <dt className="label-meta text-muted-foreground">Services</dt>
            <dd className="mt-1 text-sm font-medium tabular-nums">3</dd>
          </div>
          <div>
            <dt className="label-meta text-muted-foreground">TLS</dt>
            <dd className="mt-1 text-sm font-medium">Active</dd>
          </div>
        </dl>
      </div>
    </aside>
  );
}
