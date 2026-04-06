const EMAIL_REDACTION_PATTERN =
  /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?<!\[EMAIL_REDACTED\])/g;

export function redactEmails(value: string): string {
  return value.replace(EMAIL_REDACTION_PATTERN, '[EMAIL_REDACTED]');
}

export function shouldIgnoreError(error: unknown): boolean {
  if (!(error instanceof Error)) {
    return false;
  }

  const code =
    typeof (error as { code?: unknown }).code === 'string'
      ? ((error as { code?: string }).code ?? '')
      : '';
  const message = error.message.toLowerCase();

  return code === 'ECONNRESET' || message === 'aborted' || message.includes('econnreset');
}
