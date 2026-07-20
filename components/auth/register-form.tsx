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
import { registerSchema, type RegisterValues } from '@/lib/auth-validation';

type RegisterFormProps = {
  enabledSocialProviders: readonly SocialProvider[];
  initialError?: string | null;
};

export function RegisterForm({
  enabledSocialProviders,
  initialError = null,
}: RegisterFormProps) {
  const router = useRouter();
  const [formError, setFormError] = useState<string | null>(initialError);
  const form = useForm<RegisterValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: { name: '', email: '', password: '', confirmPassword: '' },
  });

  async function onSubmit(values: RegisterValues) {
    setFormError(null);

    try {
      const { error } = await authClient.signUp.email({
        name: values.name,
        email: values.email,
        password: values.password,
      });

      if (error) {
        setFormError(error.message ?? 'The account could not be created. Try again.');
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
        <p className="label-meta text-muted-foreground">New Workspace</p>
        <h1 className="heading-auth max-w-[12ch] text-balance">Create Your Account</h1>
        <p className="max-w-md text-sm leading-6 text-muted-foreground">
          Create one secure account for every resource that keeps your work online.
        </p>
      </header>

      <SocialAuthButtons
        providers={enabledSocialProviders}
        errorCallbackURL="/register?error=social"
        onError={setFormError}
      />

      <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
        <FieldGroup className="gap-4">
          {formError ? <FieldError>{formError}</FieldError> : null}

          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="register-name">Full Name</FieldLabel>
            <Input
              id="register-name"
              autoComplete="name"
              placeholder="Your name"
              aria-invalid={Boolean(errors.name)}
              aria-describedby={errors.name ? 'register-name-error' : undefined}
              {...form.register('name')}
            />
            <FieldError id="register-name-error" errors={[errors.name]} />
          </Field>

          <Field data-invalid={Boolean(errors.email)}>
            <FieldLabel htmlFor="register-email">Email Address</FieldLabel>
            <Input
              id="register-email"
              type="email"
              autoComplete="email"
              spellCheck={false}
              placeholder="you@example.com"
              aria-invalid={Boolean(errors.email)}
              aria-describedby={errors.email ? 'register-email-error' : undefined}
              {...form.register('email')}
            />
            <FieldError id="register-email-error" errors={[errors.email]} />
          </Field>

          <Field data-invalid={Boolean(errors.password)}>
            <FieldLabel htmlFor="register-password">Password</FieldLabel>
            <Input
              id="register-password"
              type="password"
              autoComplete="new-password"
              placeholder="At least 8 characters"
              aria-invalid={Boolean(errors.password)}
              aria-describedby={errors.password ? 'register-password-error' : undefined}
              {...form.register('password')}
            />
            <FieldError id="register-password-error" errors={[errors.password]} />
          </Field>

          <Field data-invalid={Boolean(errors.confirmPassword)}>
            <FieldLabel htmlFor="register-confirm-password">Confirm Password</FieldLabel>
            <Input
              id="register-confirm-password"
              type="password"
              autoComplete="new-password"
              placeholder="Repeat your password"
              aria-invalid={Boolean(errors.confirmPassword)}
              aria-describedby={
                errors.confirmPassword ? 'register-confirm-password-error' : undefined
              }
              {...form.register('confirmPassword')}
            />
            <FieldError
              id="register-confirm-password-error"
              errors={[errors.confirmPassword]}
            />
          </Field>

          <Button type="submit" className="w-full" disabled={isSubmitting} aria-busy={isSubmitting}>
            {isSubmitting ? 'Creating Account…' : 'Create Account'}
          </Button>
        </FieldGroup>
      </form>

      <div className="flex flex-col gap-3 text-sm text-muted-foreground">
        <p>By creating an account, you agree to Cevra&apos;s service terms and privacy policy.</p>
        <p>
          Already have an account?{' '}
          <Link className="text-link-signal hover:underline" href="/login">
            Sign In
          </Link>
        </p>
      </div>
    </div>
  );
}
