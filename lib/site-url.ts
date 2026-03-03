const DEFAULT_SITE_URL = 'https://scry.study';

function parseSiteUrl(value: string): URL | null {
  const trimmed = value.trim();
  if (!trimmed) return null;

  try {
    return new URL(trimmed);
  } catch {
    try {
      return new URL(`https://${trimmed}`);
    } catch {
      return null;
    }
  }
}

export function getSiteUrl(): string {
  const parsed = parseSiteUrl(process.env.NEXT_PUBLIC_APP_URL ?? '');
  return (parsed ?? new URL(DEFAULT_SITE_URL)).origin;
}

export function getSiteUrlObject(): URL {
  return new URL(getSiteUrl());
}
