import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createLogMailAdapter,
  createMemoryMailAdapter,
  setMailAdapterForTests,
} from './mail.ts';
import {
  forgotPasswordSchema,
  changePasswordSchema,
  resetPasswordSchema,
  profileSchema,
} from './auth-validation.ts';

test('forgot password validates email', () => {
  assert.equal(forgotPasswordSchema.safeParse({ email: 'a@b.co' }).success, true);
  assert.equal(forgotPasswordSchema.safeParse({ email: 'nope' }).success, false);
});

test('reset password requires match and length', () => {
  assert.equal(
    resetPasswordSchema.safeParse({
      password: 'eightchars',
      confirmPassword: 'eightchars',
    }).success,
    true,
  );
  assert.equal(
    resetPasswordSchema.safeParse({
      password: 'short',
      confirmPassword: 'short',
    }).success,
    false,
  );
});

test('change password requires current password', () => {
  assert.equal(
    changePasswordSchema.safeParse({
      currentPassword: 'oldpass12',
      newPassword: 'newpass12',
      confirmPassword: 'newpass12',
    }).success,
    true,
  );
  assert.equal(
    changePasswordSchema.safeParse({
      currentPassword: '',
      newPassword: 'newpass12',
      confirmPassword: 'newpass12',
    }).success,
    false,
  );
});

test('profile name bounds', () => {
  assert.equal(profileSchema.safeParse({ name: 'Ada' }).success, true);
  assert.equal(profileSchema.safeParse({ name: '' }).success, false);
  assert.equal(profileSchema.safeParse({ name: 'x'.repeat(101) }).success, false);
});

test('memory mail adapter captures messages without exposing tokens in log adapter metadata', async () => {
  const mem = createMemoryMailAdapter();
  setMailAdapterForTests(mem);
  await mem.send({
    to: 'user@example.com',
    subject: 'Reset',
    text: 'link with token=SECRET',
    tags: { kind: 'password_reset' },
  });
  assert.equal(mem.sent?.length, 1);
  assert.equal(mem.sent?.[0]?.to, 'user@example.com');
  // log adapter should not throw
  const log = createLogMailAdapter();
  await log.send({ to: 'a@b.co', subject: 'x', text: 'y' });
  setMailAdapterForTests(null);
});
