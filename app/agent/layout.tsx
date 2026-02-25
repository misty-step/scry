import type { ReactNode } from 'react';
import type { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Agent Review | Scry',
  description: 'Practice due concepts with Willow and stream feedback in real time.',
};

export default function AgentLayout({ children }: { children: ReactNode }) {
  return children;
}
