'use client';

import { useEffect } from 'react';
import Error from 'next/error';
import * as Sentry from '@sentry/nextjs';

export default function GlobalError({ error }: { error: Error & { digest?: string } }) {
  useEffect(() => {
    Sentry.captureException(error);
  }, [error]);

  return (
    <html>
      <body>
        {/* This is the default Next.js error component */}
        <Error statusCode={500} />
      </body>
    </html>
  );
}
