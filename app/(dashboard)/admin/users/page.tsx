import type { Metadata } from 'next';
import { redirect } from 'next/navigation';

import { AdminUsersTable, type AdminUserRow } from '@/components/admin/users-table';
import { apiFetch } from '@/lib/api';
import { getSession } from '@/lib/session';
import { memberships } from '@/lib/membership';

// oxlint-disable-next-line react/only-export-components -- Next.js page metadata.
export const metadata: Metadata = {
  title: 'Admin · Users',
  description: 'Manage platform users and roles.',
};

type AdminUsersPageProps = {
  searchParams: Promise<{ page?: string }>;
};

export default async function AdminUsersPage({ searchParams }: AdminUsersPageProps) {
  const session = await getSession();
  if (!session) {
    redirect('/login');
  }

  // Frontend gate (defense in depth) — backend RequireRole is authoritative.
  const membership = await memberships.getByUserId(session.user.id);
  if (!membership || membership.role !== 'admin') {
    redirect('/dashboard');
  }

  const params = await searchParams;
  const page = Math.max(1, Number(params.page ?? '1') || 1);
  const perPage = 25;

  let users: AdminUserRow[] = [];
  let total = 0;
  let loadError: string | null = null;

  try {
    const res = await apiFetch(`/api/v1/admin/users?page=${page}&per_page=${perPage}`);
    if (res.status === 401) {
      redirect('/login');
    }
    if (res.status === 403) {
      redirect('/dashboard');
    }
    if (!res.ok) {
      loadError = 'Could not load users from the API.';
    } else {
      const body = (await res.json()) as {
        data: AdminUserRow[];
        meta: { page: number; per_page: number; total: number };
      };
      users = body.data ?? [];
      total = body.meta?.total ?? 0;
    }
  } catch {
    loadError = 'API unreachable. Admin user management requires the Go API.';
  }

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="flex flex-col gap-2">
        <p className="label-meta text-muted-foreground">Administration</p>
        <h1 className="heading-page">Users</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Role and status changes are audited. You cannot disable yourself or remove the
          last active admin.
        </p>
      </header>

      {loadError ? (
        <div role="alert" className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm">
          {loadError}
        </div>
      ) : (
        <AdminUsersTable
          users={users}
          page={page}
          perPage={perPage}
          total={total}
          currentUserId={session.user.id}
        />
      )}
    </main>
  );
}
