/**
 * Configurable mail adapter for auth emails (password reset, etc.).
 *
 * Production SMTP is optional. When MAIL_PROVIDER is unset or "log", messages
 * are captured in memory (tests) or logged without secrets (dev). Never log
 * reset tokens or full URLs containing tokens in production logs.
 */

export type MailMessage = {
  to: string;
  subject: string;
  text: string;
  /** Optional HTML body */
  html?: string;
  /** Non-secret metadata for tests (never includes tokens in production logs) */
  tags?: Record<string, string>;
};

export type MailAdapter = {
  send(message: MailMessage): Promise<void>;
  /** Test helper: captured messages (memory adapter only). */
  readonly sent?: MailMessage[];
};

export type MailProvider = 'log' | 'memory' | 'smtp' | 'console';

function redactForLog(message: MailMessage): Record<string, string> {
  return {
    to: message.to,
    subject: message.subject,
    // Never include body/url/token in logs.
    body_chars: String(message.text.length),
  };
}

export function createMemoryMailAdapter(): MailAdapter {
  const sent: MailMessage[] = [];
  return {
    sent,
    async send(message) {
      sent.push(message);
    },
  };
}

export function createLogMailAdapter(): MailAdapter {
  return {
    async send(message) {
      // Structured, secret-free log line for local/dev.
      console.info('[mail:log]', JSON.stringify(redactForLog(message)));
    },
  };
}

export function createSMTPMailAdapter(opts: {
  host: string;
  port: number;
  user?: string;
  pass?: string;
  from: string;
  secure?: boolean;
}): MailAdapter {
  // Lazy nodemailer-free SMTP via raw fetch is not available; use dynamic import
  // only when configured. For MVP without a dependency, we surface a clear error
  // if SMTP is selected but undeliverable — operators set MAIL_PROVIDER=log until
  // a transport is wired. This keeps package.json lean (YAGNI).
  const from = opts.from;
  return {
    async send(message) {
      // Minimal SMTP via Node net would be fragile; document that production
      // email requires configuring an external provider. Fail loudly so we never
      // silently drop password-reset mail.
      const err = new Error(
        'SMTP mail adapter is not bundled yet. Set MAIL_PROVIDER=log or memory, ' +
          'or integrate a provider (Resend/SES) before enabling production reset email. ' +
          `Configured host=${opts.host} from=${from}`,
      );
      console.error('[mail:smtp]', err.message, redactForLog(message));
      throw err;
    },
  };
}

let singleton: MailAdapter | null = null;

export function getMailAdapter(): MailAdapter {
  if (singleton) {
    return singleton;
  }
  const provider = (process.env.MAIL_PROVIDER ?? 'log').toLowerCase() as MailProvider;
  switch (provider) {
    case 'memory':
      singleton = createMemoryMailAdapter();
      break;
    case 'smtp': {
      const host = process.env.SMTP_HOST ?? '';
      const port = Number(process.env.SMTP_PORT ?? '587');
      const from = process.env.MAIL_FROM ?? 'noreply@localhost';
      if (!host) {
        console.warn('[mail] MAIL_PROVIDER=smtp but SMTP_HOST unset; falling back to log');
        singleton = createLogMailAdapter();
      } else {
        singleton = createSMTPMailAdapter({
          host,
          port,
          user: process.env.SMTP_USER,
          pass: process.env.SMTP_PASS,
          from,
          secure: process.env.SMTP_SECURE === 'true',
        });
      }
      break;
    }
    case 'console':
    case 'log':
    default:
      singleton = createLogMailAdapter();
      break;
  }
  return singleton;
}

/** Test-only: replace the process-wide adapter. */
export function setMailAdapterForTests(adapter: MailAdapter | null): void {
  singleton = adapter;
}
