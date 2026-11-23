// tpt-payments/frontend/app/developers/webhook-tester/page.tsx
import type { Metadata } from 'next';
import WebhookTesterClient from './_client/WebhookTesterClient';

export const metadata: Metadata = {
  title: 'Webhook Tester - TPT Pay Developer Portal',
  description: 'Send test events and inspect webhook delivery in real-time',
};

export default function WebhookTesterPage() {
  return <WebhookTesterClient />;
}
