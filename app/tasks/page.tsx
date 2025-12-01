import type { Metadata } from 'next';
import { TasksClient } from './_components/tasks-client';

export const metadata: Metadata = {
  title: 'Background Tasks | Scry',
  description: 'Monitor AI generation jobs running in the background.',
};

export default function TasksPage() {
  return <TasksClient />;
}
