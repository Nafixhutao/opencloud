import { Cloud, Globe2, LockKeyhole, Network, PanelsTopLeft } from 'lucide-react';
import Link from 'next/link';
import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

import { getSession } from '@/lib/session';

const capabilities = [
  { label: 'Sites', icon: PanelsTopLeft },
  { label: 'Domains', icon: Globe2 },
  { label: 'DNS', icon: Network },
  { label: 'SSL', icon: LockKeyhole },
];

export default async function AuthLayout({ children }: Readonly<{ children: ReactNode }>) {
  const session = await getSession();
  if (session) {
    redirect('/dashboard');
  }

  return (
    <main className="min-h-svh bg-[oklch(0.115_0.008_260)] text-[oklch(0.955_0.006_90)]">
      <div aria-hidden="true" className="h-1.5 bg-[oklch(0.43_0.045_20)]" />
      <div className="grid min-h-[calc(100svh-0.375rem)] lg:grid-cols-[57fr_43fr]">
        <section
          aria-label="OpenCloud account access"
          className="flex min-h-[calc(100svh-0.375rem)] items-center justify-center overflow-y-auto px-6 py-10 sm:px-10"
        >
          <div className="w-full max-w-[20rem]">
            <Link
              href="/"
              aria-label="OpenCloud home"
              className="inline-flex items-center gap-2 text-sm font-semibold tracking-[-0.02em] text-[oklch(0.955_0.006_90)] outline-none focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-[oklch(0.78_0.025_20)] focus-visible:ring-offset-4 focus-visible:ring-offset-[oklch(0.115_0.008_260)]"
            >
              <Cloud className="size-5" strokeWidth={2} />
              OpenCloud
            </Link>
            <div className="mt-8">{children}</div>
          </div>
        </section>

        <aside
          aria-label="About OpenCloud"
          className="relative hidden min-h-[calc(100svh-0.375rem)] grid-rows-[minmax(7rem,0.72fr)_minmax(14rem,1.3fr)_minmax(5rem,0.52fr)_minmax(7rem,0.66fr)] border-l border-[oklch(0.955_0.006_90/0.1)] lg:grid"
        >
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 left-[4.5%] border-l border-dashed border-[oklch(0.955_0.006_90/0.14)]"
          />
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 left-[38%] border-l border-dashed border-[oklch(0.955_0.006_90/0.11)]"
          />
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 left-[76%] border-l border-dashed border-[oklch(0.955_0.006_90/0.11)]"
          />

          <div className="border-b border-[oklch(0.955_0.006_90/0.1)]" />

          <section className="relative flex flex-col justify-center border-b border-[oklch(0.955_0.006_90/0.1)] py-8 pr-10 pl-[calc(4.5%+1.5rem)] xl:pr-16">
            <div className="flex items-center gap-2 text-base font-semibold tracking-[-0.02em]">
              <Cloud className="size-5" strokeWidth={2} />
              OpenCloud
            </div>
            <h2 className="mt-7 max-w-[25rem] text-2xl leading-[1.15] font-semibold tracking-[-0.035em] xl:text-[1.75rem]">
              Hosting operations, without the control-panel clutter.
            </h2>
            <p className="mt-4 max-w-[28rem] text-sm leading-6 text-[oklch(0.72_0.01_260)]">
              Run websites, domains, DNS, and certificates from one focused workspace.
            </p>
          </section>

          <div className="border-b border-[oklch(0.955_0.006_90/0.1)]" />

          <section className="relative flex flex-col justify-center py-5 pr-8 pl-[calc(4.5%+1.5rem)] xl:pr-12">
            <p className="text-xs font-medium text-[oklch(0.68_0.012_260)]">One workspace for</p>
            <div className="mt-4 grid grid-cols-4 gap-3">
              {capabilities.map(({ label, icon: Icon }) => (
                <div
                  key={label}
                  className="flex items-center gap-1.5 text-xs font-medium text-[oklch(0.9_0.006_90)] xl:text-sm"
                >
                  <Icon className="size-3.5" strokeWidth={2} />
                  {label}
                </div>
              ))}
            </div>
          </section>
        </aside>
      </div>
    </main>
  );
}
