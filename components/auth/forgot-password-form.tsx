'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import Link from 'next/link';
import { useState } from 'react';
import { useForm } from 'react-hook-form';

import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { authClient } from '@/lib/auth-client';
import {
  forgotPasswordSchema,
  type ForgotPasswordValues,
} from '@/lib/auth-validation';

export function ForgotPasswordForm() {
  const [formError, setFormError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const form = useForm<ForgotPasswordValues>({
    resolver: zodResolver(forgotPasswordSchema),
    defaultValues: { email: '' },
  });

  async function onSubmit(values: ForgotPasswordValues) {
    setFormError(null);
    setSuccess(false);
    try {
      const { error } = await authClient.requestPasswordReset({
        email: values.email,
        redirectTo: `${window.location.origin}/reset-password`,
      });
      if (error) {
        setFormError(error.message ?? 'Could not start password reset. Try again.');
        return;
      }
      // Always show the same success copy (enumeration-safe).
      setSuccess(true);
    } catch {
      setFormError('Cevra could not be reached. Check your connection and try again.');
    }
  }

  const { errors, isSubmitting } = form.formState;

  return (
    <div className="flex flex-col gap-8">
      <header className="flex flex-col gap-3">
        <p className="label-meta text-muted-foreground">Account Recovery</p>
        <h1 className="heading-auth max-w-[14ch] text-balance">Reset Password</h1>
        <p className="max-w-md text-sm leading-6 text-muted-foreground">
          Enter the email for your workspace. If an account exists, we send a one-time
          reset link. Production email delivery depends on a configured mail provider.
        </p>
      </header>

      {success ? (
        <div
          role="status"
          className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm leading-6"
        >
          If that email is registered, a reset link is on its way. The link expires in one
          hour and can be used only once.
        </div>
      ) : (
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <FieldGroup className="gap-4">
            {formError ? <FieldError>{formError}</FieldError> : null}
            <Field data-invalid={Boolean(errors.email)}>
              <FieldLabel htmlFor="forgot-email">Email Address</FieldLabel>
              <Input
                id="forgot-email"
                type="email"
                autoComplete="email"
                spellCheck={false}
                placeholder="you@example.com"
                aria-invalid={Boolean(errors.email)}
                aria-describedby={errors.email ? 'forgot-email-error' : undefined}
                {...form.register('email')}
              />
              <FieldError id="forgot-email-error" errors={[errors.email]} />
            </Field>
            <Button type="submit" className="w-full" disabled={isSubmitting} aria-busy={isSubmitting}>
              {isSubmitting ? 'Sending…' : 'Send Reset Link'}
            </Button>
          </FieldGroup>
        </form>
      )}

      <p className="text-sm text-muted-foreground">
        Remembered it?{' '}
        <Link className="text-link-signal hover:underline" href="/login">
          Sign In
        </Link>
      </p>
    </div>
  );
}
