'use client';

import { useState } from 'react';

import { Button } from '@/components/ui/button';
import { FieldSeparator } from '@/components/ui/field';
import { authClient } from '@/lib/auth-client';
import type { SocialProvider } from '@/lib/social-providers';

type SocialAuthButtonsProps = {
  providers: readonly SocialProvider[];
  callbackURL: string;
  errorCallbackURL: string;
  onError: (message: string | null) => void;
};

const providerLabels: Record<SocialProvider, string> = {
  google: 'Google',
  github: 'GitHub',
};

function GoogleMark() {
  return (
    <svg data-icon="inline-start" aria-hidden="true" viewBox="0 0 24 24" fill="currentColor">
      <path d="M21.35 12.2c0-.74-.06-1.29-.2-1.86H12v3.32h5.37a4.58 4.58 0 0 1-1.99 3v2.15h3.22c1.88-1.74 2.75-4.3 2.75-6.61Z" />
      <path d="M12 21.7c2.69 0 4.94-.89 6.6-2.89l-3.22-2.16c-.89.6-2.03.96-3.38.96-2.59 0-4.79-1.75-5.58-4.1H3.1v2.23A9.97 9.97 0 0 0 12 21.7Z" />
      <path d="M6.42 13.51a6 6 0 0 1 0-3.82V7.46H3.1a9.97 9.97 0 0 0 0 8.28l3.32-2.23Z" />
      <path d="M12 5.59c1.46 0 2.77.5 3.8 1.48l2.86-2.86A9.59 9.59 0 0 0 3.1 7.46l3.32 2.23c.79-2.35 2.99-4.1 5.58-4.1Z" />
    </svg>
  );
}

function GithubMark() {
  return (
    <svg data-icon="inline-start" aria-hidden="true" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 .7A11.5 11.5 0 0 0 8.36 23.1c.58.1.79-.25.79-.56v-2.23c-3.23.7-3.91-1.37-3.91-1.37-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.71.08-.71 1.17.08 1.78 1.2 1.78 1.2 1.04 1.77 2.72 1.26 3.38.96.1-.75.4-1.26.74-1.55-2.58-.29-5.29-1.29-5.29-5.68 0-1.26.45-2.28 1.19-3.08-.12-.29-.52-1.47.11-3.04 0 0 .97-.31 3.16 1.18a10.9 10.9 0 0 1 5.75 0c2.19-1.49 3.15-1.18 3.15-1.18.64 1.57.24 2.75.12 3.04.74.8 1.19 1.82 1.19 3.08 0 4.4-2.72 5.38-5.3 5.67.42.36.79 1.07.79 2.16v3.24c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .7Z" />
    </svg>
  );
}

export function SocialAuthButtons({
  providers,
  callbackURL,
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
        callbackURL,
        newUserCallbackURL: callbackURL,
        errorCallbackURL,
      });

      if (error) {
        onError(error.message ?? 'Social sign-in could not be started. Try again.');
        setPendingProvider(null);
      }
    } catch {
      onError('Cevra could not be reached. Check your connection and try again.');
      setPendingProvider(null);
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-2.5">
        {providers.map((provider) => (
          <Button
            key={provider}
            type="button"
            variant="outline"
            className="w-full"
            disabled={pendingProvider !== null}
            aria-busy={pendingProvider === provider}
            onClick={() => continueWith(provider)}
          >
            {provider === 'google' ? (
              <GoogleMark />
            ) : (
              <GithubMark />
            )}
            {pendingProvider === provider
              ? `Connecting to ${providerLabels[provider]}…`
              : `Continue with ${providerLabels[provider]}`}
          </Button>
        ))}
      </div>

      <FieldSeparator>Or Continue with Email</FieldSeparator>
    </div>
  );
}
