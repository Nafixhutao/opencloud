'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { AtSign, Eye, EyeOff, KeyRound, LoaderCircle } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { SocialAuthButtons } from '@/components/auth/social-auth-buttons';
import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group';
import { authClient } from '@/lib/auth-client';
import type { SocialProvider } from '@/lib/social-providers';

const loginSchema = z.object({
  email: z.string().trim().email('Enter a valid email address'),
  password: z.string().min(1, 'Password is required'),
});

type LoginValues = z.infer<typeof loginSchema>;

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
  const [showPassword, setShowPassword] = useState(false);
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
        setFormError(error.message ?? 'Could not log in. Check your email and password.');
        return;
      }
      router.push('/dashboard');
      router.refresh();
    } catch {
      setFormError('Could not reach OpenCloud. Check your connection and try again.');
    }
  }

  const { errors, isSubmitting } = form.formState;

  return (
    <div>
      <div className="space-y-2">
        <h1 className="text-3xl leading-tight font-semibold tracking-[-0.04em]">Welcome back</h1>
        <p className="max-w-sm text-sm leading-6 text-muted-foreground">
          Log in to manage your sites, domains, DNS, and certificates.
        </p>
      </div>

      <SocialAuthButtons
        providers={enabledSocialProviders}
        errorCallbackURL="/login?error=social"
        onError={setFormError}
      />

      <form
        id="login-form"
        className="mt-6"
        onSubmit={form.handleSubmit(onSubmit)}
        noValidate
      >
        <FieldGroup className="gap-4">
          {formError && (
            <div
              role="alert"
              className="rounded-lg border border-destructive/40 bg-destructive/10 px-3.5 py-3 text-sm leading-5 text-destructive"
            >
              {formError}
            </div>
          )}

          <Field data-invalid={!!errors.email}>
            <FieldLabel htmlFor="email">Email address</FieldLabel>
            <InputGroup className="h-11 bg-card/40">
              <InputGroupAddon align="inline-start">
                <AtSign />
              </InputGroupAddon>
              <InputGroupInput
                id="email"
                type="email"
                placeholder="you@example.com"
                className="h-11 text-sm"
                autoComplete="email"
                aria-invalid={!!errors.email}
                aria-describedby={errors.email ? 'login-email-error' : undefined}
                {...form.register('email')}
              />
            </InputGroup>
            <FieldError
              id="login-email-error"
              className="text-destructive"
              errors={[errors.email]}
            />
          </Field>

          <Field data-invalid={!!errors.password}>
            <FieldLabel htmlFor="password">Password</FieldLabel>
            <InputGroup className="h-11 bg-card/40">
              <InputGroupAddon align="inline-start">
                <KeyRound />
              </InputGroupAddon>
              <InputGroupInput
                id="password"
                type={showPassword ? 'text' : 'password'}
                placeholder="Enter your password"
                className="h-11 text-sm"
                autoComplete="current-password"
                aria-invalid={!!errors.password}
                aria-describedby={errors.password ? 'login-password-error' : undefined}
                {...form.register('password')}
              />
              <InputGroupAddon align="inline-end">
                <InputGroupButton
                 type="button"
                  size="icon-xs"
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                  aria-pressed={showPassword}
                  onClick={() => setShowPassword((visible) => !visible)}
              >
                  {showPassword ? <EyeOff /> : <Eye />}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
            <FieldError
              id="login-password-error"
              className="text-destructive"
              errors={[errors.password]}
            />
          </Field>

          <Button
            type="submit"
            disabled={isSubmitting}
            className="mt-1 h-11 w-full font-semibold"
          >
            {isSubmitting && <LoaderCircle className="animate-spin" />}
            {isSubmitting ? 'Logging in…' : 'Log in with email'}
          </Button>
        </FieldGroup>
      </form>

      <p className="mt-7 text-center text-sm leading-5 text-muted-foreground">
        New to OpenCloud?{' '}
        <Link
          href="/register"
          className="font-medium text-foreground underline underline-offset-4 outline-none hover:text-primary focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-ring"
        >
          Create account
        </Link>
      </p>
    </div>
  );
}
