import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createLogMailAdapter,
  createMemoryMailAdapter,
  createSMTPMailAdapter,
  getMailAdapter,
  readSMTPConfig,
  setMailAdapterForTests,
} from './mail.ts';

test.afterEach(() => setMailAdapterForTests(null));

test('memory adapter captures the complete message for auth integration tests', async () => {
  const adapter = createMemoryMailAdapter();
  await adapter.send({
    to: 'user@example.com',
    subject: 'Reset',
    text: 'one-time link',
    tags: { kind: 'password_reset' },
  });
  assert.equal(adapter.sent?.length, 1);
  assert.equal(adapter.sent?.[0]?.tags?.kind, 'password_reset');
});

test('log adapter emits metadata without body, token, or URL', async () => {
  const lines: string[] = [];
  const original = console.info;
  console.info = (...args: unknown[]) => lines.push(args.join(' '));
  try {
    await createLogMailAdapter().send({
      to: 'user@example.com',
      subject: 'Reset',
      text: 'https://example.test/reset?token=TOP_SECRET',
    });
  } finally {
    console.info = original;
  }
  assert.equal(lines.length, 1);
  assert.equal(lines[0].includes('TOP_SECRET'), false);
  assert.equal(lines[0].includes('https://'), false);
  assert.match(lines[0], /body_chars/);
});

test('smtp adapter uses TLS-hardened config and sends through the transport', async () => {
  const delivered: unknown[] = [];
  const transport = {
    async sendMail(message: unknown) {
      delivered.push(message);
      return {};
    },
  };
  const config = readSMTPConfig({
    SMTP_HOST: 'smtp.example.com',
    SMTP_PORT: '587',
    SMTP_USER: 'mailer',
    SMTP_PASS: 'external-secret',
    SMTP_SECURE: 'false',
    MAIL_FROM: 'OpenCloud <noreply@example.com>',
  });
  const adapter = createSMTPMailAdapter(config, transport);
  await adapter.send({
    to: 'user@example.com',
    subject: 'Verify',
    text: 'verification body',
  });
  assert.equal(delivered.length, 1);
  assert.deepEqual(config, {
    host: 'smtp.example.com',
    port: 587,
    user: 'mailer',
    pass: 'external-secret',
    from: 'OpenCloud <noreply@example.com>',
    secure: false,
  });
});

test('production rejects non-delivery adapters and incomplete SMTP credentials', () => {
  assert.throws(
    () => getMailAdapter({ ENV: 'production', MAIL_PROVIDER: 'log' }),
    /Production requires MAIL_PROVIDER=smtp/,
  );
  assert.throws(
    () =>
      getMailAdapter({
        ENV: 'production',
        MAIL_PROVIDER: 'smtp',
        SMTP_HOST: 'smtp.example.com',
        SMTP_PORT: '587',
        SMTP_SECURE: 'false',
        MAIL_FROM: 'noreply@example.com',
      }),
    /requires SMTP_USER and SMTP_PASS/,
  );
});
