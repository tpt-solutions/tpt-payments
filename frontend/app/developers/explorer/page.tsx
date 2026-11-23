// tpt-payments/frontend/app/developers/explorer/page.tsx
import type { Metadata } from 'next';
import ExplorerClient from './_client/ExplorerClient';

export const metadata: Metadata = {
  title: 'API Explorer - TPT Pay Developer Portal',
  description: 'Try API calls in-browser with test keys',
};

export default function APIExplorerPage() {
  return <ExplorerClient />;
}
