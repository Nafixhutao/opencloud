'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useForm } from 'react-hook-form';

import { SocialAuthButtons } from '@/components/auth/social-auth-buttons';
import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { authClient } from '@/lib/auth-client';
import type { SocialProvider } from '@/lib/social-providers';
import { loginSchema, type LoginValues } from '@/lib/auth-validation';

type LoginFormProps = {
  enabledSocialProviders: readonly SocialProvider[];
  initialError?: string | null;
};

export function LoginForm({
  enabledSocialProviders,
  initialError = null,
}: LoginFormProps) {
  const router = useRouter();
  const [formError, setFormError] = useState<string | null>(initialError);
  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: '', password: '' },
  });

  async function onSubmit(values: LoginValues) {
    setFormError(null);

    try {
      const { error } = await authClient.signIn.email({
        email: values.email,
        password: values.password,
      });

      if (error) {
        setFormError(error.message ?? 'Check your email and password, then try again.');
        return;
      }

      router.push('/dashboard');
      router.refresh();
    } catch {
      setFormError('Cevra could not be reached. Check your connection and try again.');
    }
  }

  const { errors, isSubmitting } = form.formState;

  return (
    <div className="flex flex-col gap-8">
      <header className="flex flex-col gap-3">
        <p className="label-meta text-muted-foreground">Account Access</p>
        <h1 className="heading-auth max-w-[12ch] text-balance">Sign In to Cevra</h1>
        <p className="max-w-md text-sm leading-6 text-muted-foreground">
          Manage sites, domains, databases, and certificates from one workspace.
        </p>
      </header>

      <SocialAuthButtons
        providers={enabledSocialProviders}
        errorCallbackURL="/login?error=social"
        onError={setFormError}
      />

      <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
        <FieldGroup className="gap-4">
          {formError ? <FieldError>{formError}</FieldError> : null}

          <Field data-invalid={Boolean(errors.email)}>
            <FieldLabel htmlFor="login-email">Email Address</FieldLabel>
            <Input
              id="login-email"
              type="email"
              autoComplete="email"
              spellCheck={false}
              placeholder="you@example.com"
              aria-invalid={Boolean(errors.email)}
              aria-describedby={errors.email ? 'login-email-error' : undefined}
              {...form.register('email')}
            />
            <FieldError id="login-email-error" errors={[errors.email]} />
          </Field>

          <Field data-invalid={Boolean(errors.password)}>
            <div className="flex items-center justify-between gap-3">
              <FieldLabel htmlFor="login-password">Password</FieldLabel>
              <Link
                className="text-xs text-link-signal hover:underline"
                href="/forgot-password"
              >
                Forgot password?
              </Link>
            </div>
            <Input
              id="login-password"
              type="password"
              autoComplete="current-password"
              placeholder="Enter your password"
              aria-invalid={Boolean(errors.password)}
              aria-describedby={errors.password ? 'login-password-error' : undefined}
              {...form.register('password')}
            />
            <FieldError id="login-password-error" errors={[errors.password]} />
          </Field>

          <Button type="submit" className="w-full" disabled={isSubmitting} aria-busy={isSubmitting}>
            {isSubmitting ? 'Signing In…' : 'Sign In to Dashboard'}
          </Button>
        </FieldGroup>
      </form>

      <p className="text-sm text-muted-foreground">
        New to Cevra?{' '}
        <Link className="text-link-signal hover:underline" href="/register">
          Create an Account
        </Link>
      </p>
    </div>
  );
}
