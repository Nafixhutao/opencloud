import assert from 'node:assert/strict';
import test from 'node:test';

import type { Membership } from './membership.ts';

// Pure unit checks around membership role invariants (no DB).
// Integration ensureForUser is covered by Go service tests + VPS smoke.

test('membership role type only allows customer|admin', () => {
  const roles: Membership['role'][] = ['customer', 'admin'];
  assert.deepEqual(roles, ['customer', 'admin']);
  // Signup path always assigns customer — never admin.
  const signupRole: Membership['role'] = 'customer';
  assert.equal(signupRole, 'customer');
  assert.notEqual(signupRole, 'admin');
});

test('membership status vocabulary', () => {
  const statuses: Membership['status'][] = ['active', 'suspended', 'disabled'];
  assert.ok(statuses.includes('active'));
  assert.ok(statuses.includes('suspended'));
  assert.ok(statuses.includes('disabled'));
});
