'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { Eye, EyeOff } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { SocialAuthButtons } from '@/components/auth/social-auth-buttons';
import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
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

const inputClassName =
  'h-11 rounded-[0.55rem] border-[oklch(0.955_0.006_90/0.17)] bg-[oklch(0.16_0.008_260)] px-3.5 text-sm text-[oklch(0.955_0.006_90)] shadow-none placeholder:text-[oklch(0.59_0.012_260)] focus-visible:border-[oklch(0.82_0.02_20)] focus-visible:ring-[oklch(0.72_0.03_20/0.28)]';

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
      <h1 className="text-xl leading-tight font-semibold tracking-[-0.035em] text-[oklch(0.955_0.006_90)]">
        Log in to your account.
      </h1>

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
        <FieldGroup className="gap-3">
          {formError && (
            <div
              role="alert"
              className="rounded-[0.55rem] border border-[oklch(0.66_0.16_25/0.35)] bg-[oklch(0.27_0.07_25/0.22)] px-3 py-2.5 text-sm leading-5 text-[oklch(0.82_0.1_25)]"
            >
              {formError}
            </div>
          )}

          <Field data-invalid={!!errors.email}>
            <FieldLabel htmlFor="email" className="sr-only">
              Email address
            </FieldLabel>
            <Input
              id="email"
              type="email"
              placeholder="Enter your email address"
              className={inputClassName}
              autoComplete="email"
              aria-invalid={!!errors.email}
              aria-describedby={errors.email ? 'login-email-error' : undefined}
              {...form.register('email')}
            />
            <FieldError
              id="login-email-error"
              className="text-[oklch(0.82_0.1_25)]"
              errors={[errors.email]}
            />
          </Field>

          <Field data-invalid={!!errors.password}>
            <FieldLabel htmlFor="password" className="sr-only">
              Password
            </FieldLabel>
            <div className="relative">
              <Input
                id="password"
                type={showPassword ? 'text' : 'password'}
                placeholder="Enter your password"
                className={inputClassName + ' pr-11'}
                autoComplete="current-password"
                aria-invalid={!!errors.password}
                aria-describedby={errors.password ? 'login-password-error' : undefined}
                {...form.register('password')}
              />
              <button
                type="button"
                className="absolute inset-y-0 right-0 flex size-11 items-center justify-center rounded-[0.55rem] text-[oklch(0.62_0.012_260)] outline-none transition-colors hover:text-[oklch(0.955_0.006_90)] focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[oklch(0.82_0.02_20)]"
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
                onClick={() => setShowPassword((visible) => !visible)}
              >
                {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            <FieldError
              id="login-password-error"
              className="text-[oklch(0.82_0.1_25)]"
              errors={[errors.password]}
            />
          </Field>

          <Button
            type="submit"
            disabled={isSubmitting}
            className="mt-0.5 h-11 w-full rounded-[0.55rem] bg-[oklch(0.93_0.007_90)] text-sm font-semibold text-[oklch(0.15_0.008_260)] shadow-none hover:bg-[oklch(0.86_0.01_90)] focus-visible:ring-[oklch(0.82_0.02_20)]"
          >
            {isSubmitting ? 'Logging in…' : 'Log in with email'}
          </Button>
        </FieldGroup>
      </form>

      <p className="mt-6 text-xs leading-5 text-[oklch(0.65_0.012_260)]">
        New to OpenCloud?{' '}
        <Link
          href="/register"
          className="font-medium text-[oklch(0.9_0.008_90)] underline underline-offset-4 outline-none hover:text-[oklch(0.98_0.006_90)] focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-[oklch(0.82_0.02_20)]"
        >
          Create account
        </Link>
      </p>
    </div>
  );
}
