import { ConceptsClient } from '@/app/concepts/_components/concepts-client';

export const metadata = {
  title: 'Library | Scry',
  description: 'Browse and manage your knowledge library',
};

export default function LibraryPage() {
  return <ConceptsClient />;
}
