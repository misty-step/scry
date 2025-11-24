import { NextResponse } from 'next/server';
import { createHealthSnapshot, HEALTH_RESPONSE_HEADERS } from '@/lib/health';
import { createContextLogger } from '@/lib/logger';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';
export const revalidate = 0;

const healthLogger = createContextLogger('system');

export async function GET() {
  try {
    const healthStatus = createHealthSnapshot();

    healthLogger.debug(
      {
        event: 'health_check',
        status: 'healthy',
        uptime: healthStatus.uptime,
        memory: healthStatus.memory,
      },
      'Health check completed'
    );

    return NextResponse.json(healthStatus, {
      status: 200,
      headers: { ...HEALTH_RESPONSE_HEADERS },
    });
  } catch (error) {
    healthLogger.error(
      {
        event: 'health_check',
        status: 'error',
        error: error instanceof Error ? error.message : String(error),
      },
      'Health check failed'
    );

    return NextResponse.json(
      {
        status: 'unhealthy',
        timestamp: new Date().toISOString(),
        error: 'Health check failed',
      },
      {
        status: 503,
        headers: { ...HEALTH_RESPONSE_HEADERS },
      }
    );
  }
}

export async function HEAD() {
  try {
    const snapshot = createHealthSnapshot();

    return new NextResponse(null, {
      status: 200,
      headers: {
        ...HEALTH_RESPONSE_HEADERS,
        'X-Health-Status': snapshot.status,
        'X-Health-Timestamp': snapshot.timestamp,
      },
    });
  } catch {
    return new NextResponse(null, {
      status: 503,
      headers: {
        ...HEALTH_RESPONSE_HEADERS,
        'X-Health-Status': 'unhealthy',
      },
    });
  }
}
