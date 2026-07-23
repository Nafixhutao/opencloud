import nodemailer from 'nodemailer';
import type SMTPTransport from 'nodemailer/lib/smtp-transport';

export type MailMessage = {
  to: string;
  subject: string;
  text: string;
  html?: string;
  /** Non-secret metadata for test assertions; never serialized into logs. */
  tags?: Record<string, string>;
};

export type MailAdapter = {
  send(message: MailMessage): Promise<void>;
  readonly sent?: MailMessage[];
};

export type SMTPConfig = {
  host: string;
  port: number;
  user?: string;
  pass?: string;
  from: string;
  secure: boolean;
};

type MailEnvironment = Readonly<Record<string, string | undefined>>;
type MailTransport = {
  sendMail(message: {
    from: string;
    to: string;
    subject: string;
    text: string;
    html?: string;
  }): Promise<unknown>;
};

function redactedMetadata(message: MailMessage): Record<string, string> {
  return {
    to: message.to,
    subject: message.subject,
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
      console.info('[mail:log]', JSON.stringify(redactedMetadata(message)));
    },
  };
}

export function readSMTPConfig(env: MailEnvironment = process.env): SMTPConfig {
  const host = env.SMTP_HOST?.trim() ?? '';
  const from = env.MAIL_FROM?.trim() ?? '';
  const port = Number(env.SMTP_PORT ?? '587');
  const secureValue = (env.SMTP_SECURE ?? 'false').toLowerCase();
  const user = env.SMTP_USER?.trim() || undefined;
  const pass = env.SMTP_PASS || undefined;

  if (!host || !from) {
    throw new Error('SMTP_HOST and MAIL_FROM are required when MAIL_PROVIDER=smtp');
  }
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('SMTP_PORT must be an integer between 1 and 65535');
  }
  if (secureValue !== 'true' && secureValue !== 'false') {
    throw new Error('SMTP_SECURE must be true or false');
  }
  if (Boolean(user) !== Boolean(pass)) {
    throw new Error('SMTP_USER and SMTP_PASS must be configured together');
  }

  return {
    host,
    port,
    user,
    pass,
    from,
    secure: secureValue === 'true',
  };
}

export function createSMTPMailAdapter(
  config: SMTPConfig,
  transport: MailTransport = nodemailer.createTransport({
    host: config.host,
    port: config.port,
    secure: config.secure,
    requireTLS: !config.secure,
    auth:
      config.user && config.pass
        ? {
            user: config.user,
            pass: config.pass,
          }
        : undefined,
    connectionTimeout: 10_000,
    greetingTimeout: 10_000,
    socketTimeout: 20_000,
    tls: {
      minVersion: 'TLSv1.2',
      servername: config.host,
      rejectUnauthorized: true,
    },
  } satisfies SMTPTransport.Options),
): MailAdapter {
  return {
    async send(message) {
      await transport.sendMail({
        from: config.from,
        to: message.to,
        subject: message.subject,
        text: message.text,
        html: message.html,
      });
    },
  };
}

let singleton: MailAdapter | null = null;

export function getMailAdapter(env: MailEnvironment = process.env): MailAdapter {
  if (singleton) {
    return singleton;
  }

  const provider = (env.MAIL_PROVIDER ?? 'log').toLowerCase();
  const production = (env.ENV?.trim() || env.NODE_ENV) === 'production';
  if (production && provider !== 'smtp') {
    throw new Error('Production requires MAIL_PROVIDER=smtp with complete credentials');
  }

  switch (provider) {
    case 'memory':
      if (production) {
        throw new Error('MAIL_PROVIDER=memory is forbidden in production');
      }
      singleton = createMemoryMailAdapter();
      break;
    case 'log':
      if (production) {
        throw new Error('MAIL_PROVIDER=log is forbidden in production');
      }
      singleton = createLogMailAdapter();
      break;
    case 'smtp': {
      const config = readSMTPConfig(env);
      if (production && (!config.user || !config.pass)) {
        throw new Error('Production SMTP requires SMTP_USER and SMTP_PASS');
      }
      singleton = createSMTPMailAdapter(config);
      break;
    }
    default:
      throw new Error(`Unsupported MAIL_PROVIDER: ${provider}`);
  }
  return singleton;
}

/** Test-only: replace or clear the process-wide adapter. */
export function setMailAdapterForTests(adapter: MailAdapter | null): void {
  singleton = adapter;
}
