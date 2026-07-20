export type AuthMode = 'login' | 'register';

const callbackMessages: Record<string, string> = {
  access_denied: 'Sign-in was cancelled. Choose another method or try again.',
  invalid_state: 'That sign-in request expired. Start again from this page.',
  email_unverified: 'Use email and password first, then connect that provider later.',
  social: 'Social sign-in could not be completed. Try again or use email.',
};

export function getAuthCallbackError(
  value: string | string[] | undefined,
  mode: AuthMode,
) {
  const code = Array.isArray(value) ? value[0] : value;
  if (!code) {
    return null;
  }

  return (
    callbackMessages[code] ??
    (mode === 'login'
      ? 'Sign-in could not be completed. Try again or use email.'
      : 'Sign-up could not be completed. Try again or use email.')
  );
}
