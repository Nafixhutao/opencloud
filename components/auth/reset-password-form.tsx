'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useForm } from 'react-hook-form';

import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { authClient } from '@/lib/auth-client';
import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/lib/auth-validation';

type ResetPasswordFormProps = {
  token: string | null;
  initialError?: string | null;
};

export function ResetPasswordForm({ token, initialError = null }: ResetPasswordFormProps) {
  const router = useRouter();
  const [formError, setFormError] = useState<string | null>(initialError);
  const [success, setSuccess] = useState(false);
  const form = useForm<ResetPasswordValues>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: { password: '', confirmPassword: '' },
  });

  async function onSubmit(values: ResetPasswordValues) {
    setFormError(null);
    if (!token) {
      setFormError('This reset link is missing or invalid. Request a new one.');
      return;
    }
    try {
      const { error } = await authClient.resetPassword({
        newPassword: values.password,
        token,
      });
      if (error) {
        setFormError(error.message ?? 'Could not reset password. Request a new link.');
        return;
      }
      setSuccess(true);
      setTimeout(() => {
        router.push('/login');
        router.refresh();
      }, 1500);
    } catch {
      setFormError('Cevra could not be reached. Check your connection and try again.');
    }
  }

  const { errors, isSubmitting } = form.formState;

  if (!token) {
    return (
      <div className="flex flex-col gap-6">
        <header className="flex flex-col gap-3">
          <p className="label-meta text-muted-foreground">Account Recovery</p>
          <h1 className="heading-auth">Invalid Reset Link</h1>
          <p className="text-sm text-muted-foreground">
            This link is missing a token, expired, or already used.
          </p>
        </header>
        <Link className="text-sm text-link-signal hover:underline" href="/forgot-password">
          Request a new reset link
        </Link>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8">
      <header className="flex flex-col gap-3">
        <p className="label-meta text-muted-foreground">Account Recovery</p>
        <h1 className="heading-auth max-w-[12ch] text-balance">Choose a New Password</h1>
        <p className="max-w-md text-sm leading-6 text-muted-foreground">
          Use at least 8 characters. This one-time link expires after use.
        </p>
      </header>

      {success ? (
        <div role="status" className="rounded-md border border-border bg-muted/40 px-4 py-3 text-sm">
          Password updated. Redirecting to sign in…
        </div>
      ) : (
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <FieldGroup className="gap-4">
            {formError ? <FieldError>{formError}</FieldError> : null}
            <Field data-invalid={Boolean(errors.password)}>
              <FieldLabel htmlFor="reset-password">New Password</FieldLabel>
              <Input
                id="reset-password"
                type="password"
                autoComplete="new-password"
                placeholder="At least 8 characters"
                aria-invalid={Boolean(errors.password)}
                {...form.register('password')}
              />
              <FieldError errors={[errors.password]} />
            </Field>
            <Field data-invalid={Boolean(errors.confirmPassword)}>
              <FieldLabel htmlFor="reset-confirm">Confirm Password</FieldLabel>
              <Input
                id="reset-confirm"
                type="password"
                autoComplete="new-password"
                placeholder="Repeat your password"
                aria-invalid={Boolean(errors.confirmPassword)}
                {...form.register('confirmPassword')}
              />
              <FieldError errors={[errors.confirmPassword]} />
            </Field>
            <Button type="submit" className="w-full" disabled={isSubmitting} aria-busy={isSubmitting}>
              {isSubmitting ? 'Updating…' : 'Update Password'}
            </Button>
          </FieldGroup>
        </form>
      )}
    </div>
  );
}
