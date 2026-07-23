import type { Metadata } from 'next';

import { ForgotPasswordForm } from '@/components/auth/forgot-password-form';

// oxlint-disable-next-line react/only-export-components -- Next.js page metadata.
export const metadata: Metadata = {
  title: 'Forgot password',
  description: 'Request a one-time password reset link.',
};

export default function ForgotPasswordPage() {
  return <ForgotPasswordForm />;
}
