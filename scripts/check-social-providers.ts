import assert from 'node:assert/strict';

import { resolveSocialProviders } from '../lib/social-providers';

const empty = resolveSocialProviders({});
assert.deepEqual(empty.enabledProviders, []);
assert.deepEqual(empty.credentials, {});

const partial = resolveSocialProviders({
  GOOGLE_CLIENT_ID: 'google-id',
  GOOGLE_CLIENT_SECRET: 'change-me',
  GITHUB_CLIENT_ID: 'github-id',
});
assert.deepEqual(partial.enabledProviders, []);

const googleOnly = resolveSocialProviders({
  GOOGLE_CLIENT_ID: '  google-id  ',
  GOOGLE_CLIENT_SECRET: '  google-secret  ',
});
assert.deepEqual(googleOnly.enabledProviders, ['google']);
assert.equal(googleOnly.credentials.google?.clientId, 'google-id');
assert.equal(googleOnly.credentials.google?.clientSecret, 'google-secret');

const both = resolveSocialProviders({
  GOOGLE_CLIENT_ID: 'google-id',
  GOOGLE_CLIENT_SECRET: 'google-secret',
  GITHUB_CLIENT_ID: 'github-id',
  GITHUB_CLIENT_SECRET: 'github-secret',
});
assert.deepEqual(both.enabledProviders, ['google', 'github']);

console.log('Social provider configuration checks passed.');
