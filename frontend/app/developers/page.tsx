// tpt-payments/frontend/app/developers/page.tsx
import { Metadata } from 'next';
import Link from 'next/link';

export const metadata: Metadata = {
  title: 'Developer Portal - TPT Pay',
  description: 'API reference, guides, and tools for integrating with TPT Pay',
};

export default function DeveloperPortalPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Developer Portal</h1>
          <p className="mt-4 text-lg text-gray-600">
            Everything you need to integrate with TPT Pay&apos;s payment platform.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {/* API Reference */}
          <Link href="/developers/api-reference" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">API Reference</h2>
              </div>
              <p className="text-gray-600">
                Searchable API documentation generated from our OpenAPI specification.
              </p>
            </div>
          </Link>

          {/* Webhook Events */}
          <Link href="/developers/webhooks" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">Webhook Events</h2>
              </div>
              <p className="text-gray-600">
                Complete catalog of webhook event types with example payloads.
              </p>
            </div>
          </Link>

          {/* API Explorer */}
          <Link href="/developers/explorer" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">API Explorer</h2>
              </div>
              <p className="text-gray-600">
                Try API calls directly in your browser with test API keys.
              </p>
            </div>
          </Link>

          {/* SDK Documentation */}
          <Link href="/developers/sdks" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">SDKs & Code Samples</h2>
              </div>
              <p className="text-gray-600">
                JavaScript, Python, and curl examples for all API endpoints.
              </p>
            </div>
          </Link>

          {/* Sandbox Environment */}
          <Link href="/developers/sandbox" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-indigo-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">Sandbox Environment</h2>
              </div>
              <p className="text-gray-600">
                Isolated test mode for safe integration testing with no real charges.
              </p>
            </div>
          </Link>

          {/* Webhook Testing Tool */}
          <Link href="/developers/webhook-tester" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">Webhook Tester</h2>
              </div>
              <p className="text-gray-600">
                Send test events and inspect webhook delivery in real-time.
              </p>
            </div>
          </Link>

          {/* Changelog */}
          <Link href="/developers/changelog" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-teal-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">Changelog</h2>
              </div>
              <p className="text-gray-600">
                API version history and platform updates.
              </p>
            </div>
          </Link>

          {/* Status Page */}
          <Link href="/developers/status" className="block">
            <div className="bg-white rounded-lg shadow p-6 hover:shadow-lg transition-shadow">
              <div className="flex items-center mb-4">
                <svg className="w-8 h-8 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5.636 18.364a9 9 0 010-12.728M18.364 5.636a9 9 0 010 12.728M11.354 7.05a3 3 0 00-4.242 4.243M3 12h2m14 0h2M5.636 5.636l1.414 1.414m9.9 9.9l1.414 1.414M12 3v2m0 14v2" />
                </svg>
                <h2 className="ml-3 text-xl font-semibold text-gray-900">Status Page</h2>
              </div>
              <p className="text-gray-600">
                Real-time uptime status and incident history.
              </p>
            </div>
          </Link>
        </div>

        {/* Quick Start Guide */}
        <div className="mt-12 bg-white rounded-lg shadow p-8">
          <h2 className="text-2xl font-bold text-gray-900 mb-6">Quick Start</h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <div className="border rounded-lg p-4">
              <div className="flex items-center mb-3">
                <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm">1</span>
                <h3 className="ml-3 font-semibold text-gray-900">Get API Keys</h3>
              </div>
              <p className="text-sm text-gray-600">
                Create your account and generate test and live API keys from the dashboard.
              </p>
            </div>
            <div className="border rounded-lg p-4">
              <div className="flex items-center mb-3">
                <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm">2</span>
                <h3 className="ml-3 font-semibold text-gray-900">Install SDK</h3>
              </div>
              <p className="text-sm text-gray-600">
                Install <code className="text-xs bg-gray-100 px-1 py-0.5 rounded">@tpt/pay</code> for JavaScript or <code className="text-xs bg-gray-100 px-1 py-0.5 rounded">tpt-pay</code> for Python.
              </p>
            </div>
            <div className="border rounded-lg p-4">
              <div className="flex items-center mb-3">
                <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm">3</span>
                <h3 className="ml-3 font-semibold text-gray-900">Make Your First API Call</h3>
              </div>
              <p className="text-sm text-gray-600">
                Create a payment intent using our API or SDK and confirm it in sandbox mode.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}