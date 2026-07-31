import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

const { socialSignInMock } = vi.hoisted(() => ({ socialSignInMock: vi.fn() }));

vi.mock('@/lib/auth-client', () => ({
  authClient: { signIn: { social: socialSignInMock } },
}));

import { SocialAuthButtons } from '@/components/auth/social-auth-buttons';

afterEach(() => {
  socialSignInMock.mockReset();
});

describe('SocialAuthButtons', () => {
  it('preserves the validated re-authentication return path', async () => {
    socialSignInMock.mockResolvedValue({ error: null });
    render(
      <SocialAuthButtons
        providers={['google']}
        callbackURL="/sites/site-id?page=2"
        errorCallbackURL="/login?error=social&next=%2Fsites%2Fsite-id%3Fpage%3D2"
        onError={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));

    await waitFor(() =>
      expect(socialSignInMock).toHaveBeenCalledWith({
        provider: 'google',
        callbackURL: '/sites/site-id?page=2',
        newUserCallbackURL: '/sites/site-id?page=2',
        errorCallbackURL: '/login?error=social&next=%2Fsites%2Fsite-id%3Fpage%3D2',
      }),
    );
  });
});
