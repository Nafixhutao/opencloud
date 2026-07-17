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

const registerSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .max(100, 'Name must be at most 100 characters'),
  email: z.string().trim().email('Enter a valid email address'),
  password: z
    .string()
    .min(8, 'Password must be at least 8 characters')
    .max(128, 'Password must be at most 128 characters'),
});

type RegisterValues = z.infer<typeof registerSchema>;

type RegisterFormProps = {
  enabledSocialProviders: readonly SocialProvider[];
  initialError?: string | null;
};

const inputClassName =
  'h-11 rounded-[0.55rem] border-[oklch(0.955_0.006_90/0.17)] bg-[oklch(0.16_0.008_260)] px-3.5 text-sm text-[oklch(0.955_0.006_90)] shadow-none placeholder:text-[oklch(0.59_0.012_260)] focus-visible:border-[oklch(0.82_0.02_20)] focus-visible:ring-[oklch(0.72_0.03_20/0.28)]';

export function RegisterForm({
  enabledSocialProviders,
  initialError = null,
}: RegisterFormProps) {
  const router = useRouter();
  const [formError, setFormError] = useState<string | null>(initialError);
  const [showPassword, setShowPassword] = useState(false);
  const form = useForm<RegisterValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: { name: '', email: '', password: '' },
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
        setFormError(error.message ?? 'Could not create the account. Try again.');
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
        Create your account.
      </h1>

      <SocialAuthButtons
        providers={enabledSocialProviders}
        errorCallbackURL="/register?error=social"
        onError={setFormError}
      />

      <form
        id="register-form"
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

          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="name" className="sr-only">
              Name
            </FieldLabel>
            <Input
              id="name"
              placeholder="Enter your name"
              className={inputClassName}
              autoComplete="name"
              aria-invalid={!!errors.name}
              aria-describedby={errors.name ? 'register-name-error' : undefined}
              {...form.register('name')}
            />
            <FieldError
              id="register-name-error"
              className="text-[oklch(0.82_0.1_25)]"
              errors={[errors.name]}
            />
          </Field>

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
              aria-describedby={errors.email ? 'register-email-error' : undefined}
              {...form.register('email')}
            />
            <FieldError
              id="register-email-error"
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
                placeholder="Create a password"
                className={inputClassName + ' pr-11'}
                autoComplete="new-password"
                aria-invalid={!!errors.password}
                aria-describedby={errors.password ? 'register-password-error' : undefined}
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
              id="register-password-error"
              className="text-[oklch(0.82_0.1_25)]"
              errors={[errors.password]}
            />
          </Field>

          <Button
            type="submit"
            disabled={isSubmitting}
            className="mt-0.5 h-11 w-full rounded-[0.55rem] bg-[oklch(0.93_0.007_90)] text-sm font-semibold text-[oklch(0.15_0.008_260)] shadow-none hover:bg-[oklch(0.86_0.01_90)] focus-visible:ring-[oklch(0.82_0.02_20)]"
          >
            {isSubmitting ? 'Creating account…' : 'Create account with email'}
          </Button>
        </FieldGroup>
      </form>

      <p className="mt-6 text-xs leading-5 text-[oklch(0.65_0.012_260)]">
        Already have an account?{' '}
        <Link
          href="/login"
          className="font-medium text-[oklch(0.9_0.008_90)] underline underline-offset-4 outline-none hover:text-[oklch(0.98_0.006_90)] focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-[oklch(0.82_0.02_20)]"
        >
          Log in
        </Link>
      </p>
    </div>
  );
}
