import { z } from 'zod';

export const createSiteFormSchema = z.object({
  domain: z
    .string()
    .trim()
    .min(3, 'Enter a complete domain.')
    .max(253, 'Domain must be 253 characters or fewer.')
    .transform((value) => value.toLowerCase().replace(/\.$/, ''))
    .refine(
      (value) =>
        value.includes('.') &&
        value.split('.').every(
          (label) =>
            label.length > 0 &&
            label.length <= 63 &&
            /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(label),
        ),
      'Enter a valid ASCII domain, for example site.example.com.',
    ),
});

export const createSiteSchema = createSiteFormSchema.extend({
  template: z.literal('static').optional().default('static'),
});

export type CreateSiteValues = z.infer<typeof createSiteFormSchema>;
