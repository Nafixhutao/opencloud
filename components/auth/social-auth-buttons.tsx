'use client';

import { Github } from 'lucide-react';
import { useState } from 'react';

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

function GoogleMark() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="size-4" fill="currentColor">
      <path d="M21.35 12.2c0-.74-.06-1.29-.2-1.86H12v3.32h5.37a4.58 4.58 0 0 1-1.99 3v2.15h3.22c1.88-1.74 2.75-4.3 2.75-6.61Z" />
      <path d="M12 21.7c2.69 0 4.94-.89 6.6-2.89l-3.22-2.16c-.89.6-2.03.96-3.38.96-2.59 0-4.79-1.75-5.58-4.1H3.1v2.23A9.97 9.97 0 0 0 12 21.7Z" />
      <path d="M6.42 13.51a6 6 0 0 1 0-3.82V7.46H3.1a9.97 9.97 0 0 0 0 8.28l3.32-2.23Z" />
      <path d="M12 5.59c1.46 0 2.77.5 3.8 1.48l2.86-2.86A9.59 9.59 0 0 0 3.1 7.46l3.32 2.23c.79-2.35 2.99-4.1 5.58-4.1Z" />
    </svg>
  );
}

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
    <div className="mt-6">
      <div className="space-y-2.5">
        {providers.map((provider) => (
          <Button
            key={provider}
            type="button"
            variant="outline"
            disabled={pendingProvider !== null}
            aria-busy={pendingProvider === provider}
            onClick={() => continueWith(provider)}
            className="h-11 w-full rounded-[0.55rem] border-[oklch(0.955_0.006_90/0.18)] bg-[oklch(0.15_0.008_260)] text-sm font-semibold text-[oklch(0.94_0.007_90)] shadow-none hover:border-[oklch(0.955_0.006_90/0.3)] hover:bg-[oklch(0.2_0.009_260)] hover:text-[oklch(0.98_0.006_90)] focus-visible:ring-[oklch(0.82_0.02_20)]"
          >
            {provider === 'google' ? <GoogleMark /> : <Github className="size-4" />}
            {pendingProvider === provider
              ? 'Continuing…'
              : 'Continue with ' + providerLabels[provider]}
          </Button>
        ))}
      </div>

      <div className="mt-6 flex items-center gap-3" aria-hidden="true">
        <span className="h-px flex-1 bg-[oklch(0.955_0.006_90/0.13)]" />
        <span className="text-[0.65rem] font-medium text-[oklch(0.58_0.012_260)]">OR</span>
        <span className="h-px flex-1 bg-[oklch(0.955_0.006_90/0.13)]" />
      </div>
    </div>
  );
}
