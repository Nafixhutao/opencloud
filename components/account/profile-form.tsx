'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { useForm } from 'react-hook-form';

import { Button } from '@/components/ui/button';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { authClient } from '@/lib/auth-client';
import { changePasswordSchema, profileSchema, type ChangePasswordValues, type ProfileValues } from '@/lib/auth-validation';

type ProfileFormProps = {
  initialName: string;
  email: string;
  accountId: string;
  role: string;
  accountStatus: string;
};

export function ProfileForm({
  initialName,
  email,
  accountId,
  role,
  accountStatus,
}: ProfileFormProps) {
  const router = useRouter();
  const [profileMsg, setProfileMsg] = useState<string | null>(null);
  const [profileErr, setProfileErr] = useState<string | null>(null);
  const [pwMsg, setPwMsg] = useState<string | null>(null);
  const [pwErr, setPwErr] = useState<string | null>(null);

  const profileForm = useForm<ProfileValues>({
    resolver: zodResolver(profileSchema),
    defaultValues: { name: initialName },
  });

  const passwordForm = useForm<ChangePasswordValues>({
    resolver: zodResolver(changePasswordSchema),
    defaultValues: { currentPassword: '', newPassword: '', confirmPassword: '' },
  });

  async function onProfile(values: ProfileValues) {
    setProfileErr(null);
    setProfileMsg(null);
    try {
      const res = await fetch('/api/account/profile', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: values.name }),
      });
      const body = (await res.json().catch(() => null)) as
        | { error?: { message?: string } }
        | null;
      if (!res.ok) {
        setProfileErr(body?.error?.message ?? 'Could not update profile.');
        return;
      }
      setProfileMsg('Profile updated.');
      router.refresh();
    } catch {
      setProfileErr('Could not reach the control plane.');
    }
  }

  async function onPassword(values: ChangePasswordValues) {
    setPwErr(null);
    setPwMsg(null);
    try {
      const { error } = await authClient.changePassword({
        currentPassword: values.currentPassword,
        newPassword: values.newPassword,
        revokeOtherSessions: true,
      });
      if (error) {
        setPwErr(error.message ?? 'Could not change password.');
        return;
      }
      setPwMsg('Password changed. Other sessions were signed out.');
      passwordForm.reset();
    } catch {
      setPwErr('Could not reach Cevra. Try again.');
    }
  }

  return (
    <div className="flex flex-col gap-12">
      <section className="flex flex-col gap-4">
        <div>
          <p className="label-meta text-muted-foreground">Identity</p>
          <h2 className="heading-section mt-1">Workspace Profile</h2>
        </div>
        <dl className="grid gap-3 rounded-lg border border-border bg-card p-5 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-muted-foreground">Email</dt>
            <dd className="mt-1 font-medium">{email}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Role</dt>
            <dd className="mt-1 font-medium capitalize">{role}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Account ID</dt>
            <dd className="mt-1 truncate font-mono text-xs">{accountId}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Account Status</dt>
            <dd className="mt-1 font-medium capitalize">{accountStatus}</dd>
          </div>
        </dl>

        <form onSubmit={profileForm.handleSubmit(onProfile)} noValidate className="max-w-md">
          <FieldGroup className="gap-4">
            {profileErr ? <FieldError>{profileErr}</FieldError> : null}
            {profileMsg ? (
              <p role="status" className="text-sm text-success">
                {profileMsg}
              </p>
            ) : null}
            <Field data-invalid={Boolean(profileForm.formState.errors.name)}>
              <FieldLabel htmlFor="profile-name">Display Name</FieldLabel>
              <Input
                id="profile-name"
                autoComplete="name"
                {...profileForm.register('name')}
              />
              <FieldError errors={[profileForm.formState.errors.name]} />
            </Field>
            <Button
              type="submit"
              disabled={profileForm.formState.isSubmitting}
              aria-busy={profileForm.formState.isSubmitting}
            >
              {profileForm.formState.isSubmitting ? 'Saving…' : 'Save Profile'}
            </Button>
          </FieldGroup>
        </form>
      </section>

      <section className="flex max-w-md flex-col gap-4">
        <div>
          <p className="label-meta text-muted-foreground">Security</p>
          <h2 className="heading-section mt-1">Change Password</h2>
        </div>
        <form onSubmit={passwordForm.handleSubmit(onPassword)} noValidate>
          <FieldGroup className="gap-4">
            {pwErr ? <FieldError>{pwErr}</FieldError> : null}
            {pwMsg ? (
              <p role="status" className="text-sm text-success">
                {pwMsg}
              </p>
            ) : null}
            <Field data-invalid={Boolean(passwordForm.formState.errors.currentPassword)}>
              <FieldLabel htmlFor="current-password">Current Password</FieldLabel>
              <Input
                id="current-password"
                type="password"
                autoComplete="current-password"
                {...passwordForm.register('currentPassword')}
              />
              <FieldError errors={[passwordForm.formState.errors.currentPassword]} />
            </Field>
            <Field data-invalid={Boolean(passwordForm.formState.errors.newPassword)}>
              <FieldLabel htmlFor="new-password">New Password</FieldLabel>
              <Input
                id="new-password"
                type="password"
                autoComplete="new-password"
                {...passwordForm.register('newPassword')}
              />
              <FieldError errors={[passwordForm.formState.errors.newPassword]} />
            </Field>
            <Field data-invalid={Boolean(passwordForm.formState.errors.confirmPassword)}>
              <FieldLabel htmlFor="confirm-new-password">Confirm New Password</FieldLabel>
              <Input
                id="confirm-new-password"
                type="password"
                autoComplete="new-password"
                {...passwordForm.register('confirmPassword')}
              />
              <FieldError errors={[passwordForm.formState.errors.confirmPassword]} />
            </Field>
            <Button
              type="submit"
              disabled={passwordForm.formState.isSubmitting}
              aria-busy={passwordForm.formState.isSubmitting}
            >
              {passwordForm.formState.isSubmitting ? 'Updating…' : 'Update Password'}
            </Button>
          </FieldGroup>
        </form>
      </section>
    </div>
  );
}
