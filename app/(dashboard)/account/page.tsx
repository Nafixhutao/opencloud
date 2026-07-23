import type { Metadata } from 'next';
import { redirect } from 'next/navigation';

import { ProfileForm } from '@/components/account/profile-form';
import { apiFetch } from '@/lib/api';
import { getSession } from '@/lib/session';

// oxlint-disable-next-line react/only-export-components -- Next.js page metadata.
export const metadata: Metadata = {
  title: 'Account',
  description: 'Manage your workspace profile and password.',
};

type MeResponse = {
  data: {
    user_id: string;
    account_id: string;
    role: string;
    status: string;
    account: { id: string; name: string; status: string };
  };
};

export default async function AccountPage() {
  const session = await getSession();
  if (!session) {
    redirect('/login');
  }

  let me: MeResponse['data'] | null = null;
  let loadError: string | null = null;
  try {
    const res = await apiFetch('/api/v1/me');
    if (res.status === 401) {
      redirect('/login');
    }
    if (!res.ok) {
      loadError = 'Could not load account details from the API.';
    } else {
      const body = (await res.json()) as MeResponse;
      me = body.data;
    }
  } catch {
    loadError = 'API unreachable. Profile editing requires the Go API.';
  }

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[800px] flex-col gap-10 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="flex flex-col gap-2">
        <p className="label-meta text-muted-foreground">Account</p>
        <h1 className="heading-page">Profile & Security</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Update your workspace display name and password. Tenant membership and role
          claims are server-managed.
        </p>
      </header>

      {loadError ? (
        <div
          role="alert"
          className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm"
        >
          {loadError} You can still change your password below using session auth.
        </div>
      ) : null}

      <ProfileForm
        initialName={me?.account.name ?? session.user.name}
        email={session.user.email}
        accountId={me?.account_id ?? '—'}
        role={me?.role ?? 'customer'}
        accountStatus={me?.account.status ?? me?.status ?? 'active'}
      />
    </main>
  );
}
