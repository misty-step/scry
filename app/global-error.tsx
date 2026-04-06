'use client';

import { useEffect } from 'react';
import Error from 'next/error';
import { captureRuntimeException } from '@/lib/error-monitoring';

export default function GlobalError({ error }: { error: Error & { digest?: string } }) {
  useEffect(() => {
    void captureRuntimeException(error, {
      context: {
        digest: error.digest,
        route: typeof window !== 'undefined' ? window.location.pathname : 'unknown',
      },
    });
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
