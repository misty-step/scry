const BYTES_IN_MB = 1024 * 1024;

export interface HealthSnapshot {
  status: 'healthy';
  timestamp: string;
  uptime: number;
  memory: {
    used: number;
    total: number;
  };
  environment: string;
  version: string;
}

export const HEALTH_RESPONSE_HEADERS = {
  'Cache-Control': 'no-store, no-cache, must-revalidate',
  'Content-Type': 'application/json',
};

// Lightweight, dependency-free snapshot for external uptime checks.
export function createHealthSnapshot(): HealthSnapshot {
  const memoryUsage = process.memoryUsage();

  return {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    uptime: process.uptime(),
    memory: {
      used: Math.round(memoryUsage.heapUsed / BYTES_IN_MB),
      total: Math.round(memoryUsage.heapTotal / BYTES_IN_MB),
    },
    environment: process.env.NODE_ENV || 'unknown',
    version: process.env.npm_package_version || '0.1.0',
  };
}
