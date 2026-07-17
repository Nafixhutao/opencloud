import { ArrowLeft, Check, ShieldCheck } from 'lucide-react';
import Link from 'next/link';
import type { ReactNode } from 'react';

import { FloatingPaths } from '@/components/floating-paths';
import { Logo } from '@/components/logo';

const capabilities = ['Sites', 'Domains', 'DNS', 'SSL'];

export function AuthPage({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <main className="dark relative grid min-h-svh overflow-x-clip bg-background text-foreground lg:grid-cols-[minmax(0,0.92fr)_minmax(30rem,1.08fr)]">
      <aside className="relative hidden min-h-svh overflow-hidden border-r border-border bg-secondary/20 p-10 lg:flex lg:flex-col xl:p-14">
        <div
          aria-hidden="true"
          className="absolute inset-0 bg-linear-to-b from-transparent via-background/5 to-background/80"
        />
        <div className="absolute inset-0 opacity-70">
          <FloatingPaths position={1} />
          <FloatingPaths position={-1} />
        </div>

        <Link
          href="/"
          aria-label="OpenCloud home"
          className="relative z-10 mr-auto rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-4 focus-visible:ring-offset-background"
        >
          <Logo />
        </Link>

        <div className="relative z-10 mt-auto max-w-xl">
          <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-border bg-background/40 px-3 py-1.5 text-xs font-medium text-muted-foreground backdrop-blur-sm">
            <ShieldCheck className="size-3.5 text-foreground" />
            Secure account access
          </div>
          <h2 className="max-w-lg text-4xl leading-[1.08] font-semibold tracking-[-0.045em] xl:text-5xl">
            Your hosting operation, finally in one place.
          </h2>
          <p className="mt-5 max-w-lg text-base leading-7 text-muted-foreground">
            Manage the services behind every website from one focused OpenCloud workspace.
          </p>
          <ul className="mt-8 flex flex-wrap gap-x-5 gap-y-3" aria-label="OpenCloud capabilities">
            {capabilities.map((capability) => (
              <li key={capability} className="flex items-center gap-2 text-sm font-medium">
                <span className="flex size-5 items-center justify-center rounded-full border border-border bg-background/50">
                  <Check className="size-3" />
                </span>
                {capability}
              </li>
            ))}
          </ul>
        </div>
      </aside>

      <section className="relative flex min-h-svh items-center justify-center px-5 py-24 sm:px-8 lg:px-12">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 -z-0 overflow-hidden opacity-70"
        >
          <div className="absolute top-0 right-0 h-[52rem] w-[32rem] -translate-y-1/2 rounded-full bg-[radial-gradient(circle,var(--color-muted)_0%,transparent_68%)] blur-3xl" />
        </div>

        <Link
          href="/"
          className="absolute top-6 right-5 z-10 inline-flex h-9 items-center gap-2 rounded-lg px-3 text-sm font-medium text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring sm:right-8 lg:left-8 lg:right-auto"
        >
          <ArrowLeft className="size-4" />
          Home
        </Link>

        <div className="relative z-10 w-full max-w-md">
          <Logo className="mb-10 lg:hidden" />
          {children}
        </div>
      </section>
    </main>
  );
}
