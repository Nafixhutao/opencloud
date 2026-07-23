'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { Button } from '@/components/ui/button';

export type AdminUserRow = {
  membership_id: string;
  user_id: string;
  account_id: string;
  role: string;
  status: string;
  account_name?: string;
  created_at: string;
  updated_at: string;
};

type AdminUsersTableProps = {
  users: AdminUserRow[];
  page: number;
  perPage: number;
  total: number;
  currentUserId: string;
};

export function AdminUsersTable({
  users,
  page,
  perPage,
  total,
  currentUserId,
}: AdminUsersTableProps) {
  const router = useRouter();
  const [busyId, setBusyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / perPage));

  async function patchUser(
    membershipId: string,
    body: { role?: string; status?: string },
  ) {
    setError(null);
    setMessage(null);
    setBusyId(membershipId);
    try {
      const res = await fetch(`/api/admin/users/${membershipId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const data = (await res.json().catch(() => null)) as {
        error?: { message?: string };
      } | null;
      if (!res.ok) {
        setError(data?.error?.message ?? 'Update failed');
        return;
      }
      setMessage('User updated.');
      router.refresh();
    } catch {
      setError('Could not reach the control plane.');
    } finally {
      setBusyId(null);
    }
  }

  if (users.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center text-sm text-muted-foreground">
        No users found.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {message ? (
        <p role="status" className="text-sm text-success">
          {message}
        </p>
      ) : null}

      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full min-w-[640px] text-left text-sm">
          <thead className="border-b border-border bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
            <tr>
              <th className="px-4 py-3 font-medium">Account</th>
              <th className="px-4 py-3 font-medium">User ID</th>
              <th className="px-4 py-3 font-medium">Role</th>
              <th className="px-4 py-3 font-medium">Status</th>
              <th className="px-4 py-3 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => {
              const isSelf = u.user_id === currentUserId;
              const busy = busyId === u.membership_id;
              return (
                <tr key={u.membership_id} className="border-b border-border last:border-0">
                  <td className="px-4 py-3">
                    <div className="font-medium">{u.account_name || '—'}</div>
                    <div className="mt-0.5 font-mono text-xs text-muted-foreground">
                      {u.account_id.slice(0, 8)}…
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">
                    {u.user_id.slice(0, 12)}…
                    {isSelf ? (
                      <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase">
                        you
                      </span>
                    ) : null}
                  </td>
                  <td className="px-4 py-3 capitalize">{u.role}</td>
                  <td className="px-4 py-3 capitalize">{u.status}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      {u.role === 'customer' ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => patchUser(u.membership_id, { role: 'admin' })}
                        >
                          Make admin
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy || isSelf}
                          onClick={() => patchUser(u.membership_id, { role: 'customer' })}
                        >
                          Demote
                        </Button>
                      )}
                      {u.status === 'active' ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy || isSelf}
                          onClick={() =>
                            patchUser(u.membership_id, { status: 'suspended' })
                          }
                        >
                          Suspend
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy || isSelf}
                          onClick={() =>
                            patchUser(u.membership_id, { status: 'active' })
                          }
                        >
                          Activate
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          Page {page} of {totalPages} · {total} users
        </span>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => router.push(`/admin/users?page=${page - 1}`)}
          >
            Previous
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= totalPages}
            onClick={() => router.push(`/admin/users?page=${page + 1}`)}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
