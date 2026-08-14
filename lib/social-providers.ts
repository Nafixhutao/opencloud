export type SocialProvider = 'google' | 'github';

type OAuthCredentials = {
  clientId: string;
  clientSecret: string;
};

type SocialProviderResolution = {
  enabledProviders: SocialProvider[];
  credentials: Partial<Record<SocialProvider, OAuthCredentials>>;
};

function readCredential(value: string | undefined) {
  const normalized = value?.trim();
  if (!normalized || normalized.toLowerCase().startsWith('change-me')) {
    return undefined;
  }
  return normalized;
}

function credentialPair(clientId: string | undefined, clientSecret: string | undefined) {
  const normalizedId = readCredential(clientId);
  const normalizedSecret = readCredential(clientSecret);
  return normalizedId && normalizedSecret
    ? { clientId: normalizedId, clientSecret: normalizedSecret }
    : undefined;
}

export function resolveSocialProviders(
  environment: Record<string, string | undefined>,
): SocialProviderResolution {
  const google = credentialPair(
    environment.GOOGLE_CLIENT_ID,
    environment.GOOGLE_CLIENT_SECRET,
  );
  const github = credentialPair(
    environment.GITHUB_CLIENT_ID,
    environment.GITHUB_CLIENT_SECRET,
  );

  return {
    enabledProviders: [
      ...(google ? (['google'] as const) : []),
      ...(github ? (['github'] as const) : []),
    ],
    credentials: {
      ...(google ? { google } : {}),
      ...(github ? { github } : {}),
    },
  };
}

/**
 * Lazy exports: evaluate <code>process.env</code> on first property access
 * (request time), not at module load (build time) — mirroring the lazy pattern
 * in <code>auth.ts</code>. OAuth credentials injected at container start or
 * rotated without a rebuild would otherwise stay invisible forever.
 */
export const enabledSocialProviders: readonly SocialProvider[] = new Proxy(
  [] as SocialProvider[],
  {
    get(_target, prop, receiver) {
      return Reflect.get(resolveSocialProviders(process.env).enabledProviders, prop, receiver);
    },
  },
) as readonly SocialProvider[];

export const socialProviders: Partial<Record<SocialProvider, OAuthCredentials>> = new Proxy(
  {} as Partial<Record<SocialProvider, OAuthCredentials>>,
  {
    get(_target, prop, receiver) {
      return Reflect.get(resolveSocialProviders(process.env).credentials, prop, receiver);
    },
  },
) as Partial<Record<SocialProvider, OAuthCredentials>>;
