'use client';

import { useState } from 'react';

import { AuthDivider } from '@/components/auth-divider';
import { GithubIcon } from '@/components/github-icon';
import { GoogleIcon } from '@/components/google-icon';
import { Button } from '@/components/ui/button';
import { authClient } from '@/lib/auth-client';
import type { SocialProvider } from '@/lib/social-providers';

type SocialAuthButtonsProps = {
  providers: readonly SocialProvider[];
  errorCallbackURL: string;
  onError: (message: string | null) => void;
};

const providerLabels: Record<SocialProvider, string> = {
  google: 'Google',
  github: 'GitHub',
};

export function SocialAuthButtons({
  providers,
  errorCallbackURL,
  onError,
}: SocialAuthButtonsProps) {
  const [pendingProvider, setPendingProvider] = useState<SocialProvider | null>(null);

  if (providers.length === 0) {
    return null;
  }

  async function continueWith(provider: SocialProvider) {
    onError(null);
    setPendingProvider(provider);

    try {
      const { error } = await authClient.signIn.social({
        provider,
        callbackURL: '/dashboard',
        newUserCallbackURL: '/dashboard',
        errorCallbackURL,
      });

      if (error) {
        onError(error.message ?? 'Social sign-in could not be started. Try again.');
        setPendingProvider(null);
      }
    } catch {
      onError('Could not reach OpenCloud. Check your connection and try again.');
      setPendingProvider(null);
    }
  }

  return (
    <div className="mt-7">
      <div className={providers.length > 1 ? 'grid gap-2 sm:grid-cols-2' : 'grid gap-2'}>
        {providers.map((provider) => (
          <Button
            key={provider}
            type="button"
            variant="outline"
            disabled={pendingProvider !== null}
            aria-busy={pendingProvider === provider}
            onClick={() => continueWith(provider)}
            className="h-11 w-full bg-card/40 font-semibold"
          >
            {provider === 'google' ? (
              <GoogleIcon aria-hidden="true" className="size-4" />
            ) : (
              <GithubIcon aria-hidden="true" className="size-4" />
            )}
            {pendingProvider === provider
              ? 'Continuing…'
              : 'Continue with ' + providerLabels[provider]}
          </Button>
        ))}
      </div>

      <AuthDivider className="mt-6">or continue with email</AuthDivider>
    </div>
  );
}
