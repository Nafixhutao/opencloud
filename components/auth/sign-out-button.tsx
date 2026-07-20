'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { authClient } from '@/lib/auth-client';

export function SignOutButton() {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSignOut() {
    setError(null);
    setPending(true);
    try {
      const result = await authClient.signOut();
      if (result.error) {
        setError(result.error.message ?? 'Could not sign out. Try again.');
        return;
      }
      router.push('/login');
      router.refresh();
    } catch {
      setError('Could not reach Cevra. Check your connection and try again.');
    } finally {
      setPending(false);
    }
  }

  return (
    <div>
      {error ? (
        <p role="alert" className="sr-only">
          {error}
        </p>
      ) : null}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onSignOut}
        disabled={pending}
      >
        {pending ? 'Signing Out…' : error ? 'Try Sign Out Again' : 'Sign Out'}
      </Button>
    </div>
  );
}
