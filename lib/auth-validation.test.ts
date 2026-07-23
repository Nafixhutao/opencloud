import assert from 'node:assert/strict';
import test from 'node:test';

import { getAuthCallbackError } from './auth-errors.ts';
import {
  changePasswordSchema,
  forgotPasswordSchema,
  loginSchema,
  profileSchema,
  registerSchema,
  resetPasswordSchema,
} from './auth-validation.ts';

test('login validation trims a valid email and requires a password', () => {
  const parsed = loginSchema.parse({
    email: '  user@example.com  ',
    password: 'secret',
  });

  assert.equal(parsed.email, 'user@example.com');
  assert.equal(loginSchema.safeParse({ email: 'not-an-email', password: '' }).success, false);
});

test('registration enforces password length and confirmation', () => {
  const valid = registerSchema.safeParse({
    name: '  Cevra User  ',
    email: 'user@example.com',
    password: 'eightchars',
    confirmPassword: 'eightchars',
  });
  const shortPassword = registerSchema.safeParse({
    name: 'Cevra User',
    email: 'user@example.com',
    password: 'short',
    confirmPassword: 'short',
  });
  const mismatch = registerSchema.safeParse({
    name: 'Cevra User',
    email: 'user@example.com',
    password: 'eightchars',
    confirmPassword: 'different',
  });

  assert.equal(valid.success, true);
  if (valid.success) {
    assert.equal(valid.data.name, 'Cevra User');
  }
  assert.equal(shortPassword.success, false);
  assert.equal(mismatch.success, false);
});

test('registration keeps the server-aligned name and password ceilings', () => {
  const overlongName = registerSchema.safeParse({
    name: 'x'.repeat(101),
    email: 'user@example.com',
    password: 'eightchars',
    confirmPassword: 'eightchars',
  });
  const overlongPassword = registerSchema.safeParse({
    name: 'Cevra User',
    email: 'user@example.com',
    password: 'x'.repeat(129),
    confirmPassword: 'x'.repeat(129),
  });

  assert.equal(overlongName.success, false);
  assert.equal(overlongPassword.success, false);
});

test('auth callback errors map known codes and hide unknown internals', () => {
  assert.equal(
    getAuthCallbackError('access_denied', 'login'),
    'Sign-in was cancelled. Choose another method or try again.',
  );
  assert.equal(
    getAuthCallbackError(['invalid_state'], 'login'),
    'That sign-in request expired. Start again from this page.',
  );
  assert.equal(
    getAuthCallbackError('provider_stack_trace', 'register'),
    'Sign-up could not be completed. Try again or use email.',
  );
  assert.equal(getAuthCallbackError(undefined, 'login'), null);
});

test('password reset and profile schemas align with server bounds', () => {
  assert.equal(forgotPasswordSchema.safeParse({ email: 'a@b.co' }).success, true);
  assert.equal(
    resetPasswordSchema.safeParse({
      password: 'eightchars',
      confirmPassword: 'eightchars',
    }).success,
    true,
  );
  assert.equal(
    changePasswordSchema.safeParse({
      currentPassword: 'old',
      newPassword: 'eightchars',
      confirmPassword: 'eightchars',
    }).success,
    true,
  );
  assert.equal(profileSchema.safeParse({ name: 'Workspace' }).success, true);
});
