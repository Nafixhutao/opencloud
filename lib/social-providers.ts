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

const resolvedSocialProviders = resolveSocialProviders(process.env);

export const enabledSocialProviders = resolvedSocialProviders.enabledProviders;
export const socialProviders = resolvedSocialProviders.credentials;
