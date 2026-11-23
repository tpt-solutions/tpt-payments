// tpt-payments/frontend/app/developers/explorer/page.tsx
import { Metadata } from 'next';
import { useState } from 'react';

export const metadata: Metadata = {
  title: 'API Explorer - TPT Pay Developer Portal',
  description: 'Try API calls in-browser with test keys',
};

const apiEndpoints = [
  {
    method: 'POST',
    path: '/v1/payment_intents',
    summary: 'Create a Payment Intent',
    body: {
      amount: 2000,
      currency: 'usd',
      payment_method_types: ['card'],
      metadata: { order_id: '12345' },
    },
  },
  {
    method: 'GET',
    path: '/v1/payment_intents/:id',
    summary: 'Retrieve a Payment Intent',
    params: { id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo' },
  },
  {
    method: 'POST',
    path: '/v1/payment_intents/:id/confirm',
    summary: 'Confirm a Payment Intent',
    params: { id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo' },
    body: {
      payment_method: 'pm_card_visa',
    },
  },
  {
    method: 'POST',
    path: '/v1/payment_intents/:id/capture',
    summary: 'Capture a Payment Intent',
    params: { id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo' },
  },
  {
    method: 'POST',
    path: '/v1/refunds',
    summary: 'Create a Refund',
    body: {
      payment_intent: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
      amount: 2000,
    },
  },
  {
    method: 'POST',
    path: '/v1/subscriptions',
    summary: 'Create a Subscription',
    body: {
      customer: 'cus_123456789',
      items: [{ price: 'price_abcdef123456' }],
    },
  },
];

export default function APIExplorerPage() {
  const [selectedEndpoint, setSelectedEndpoint] = useState(apiEndpoints[0]);
  const [apiKey, setApiKey] = useState('sk_test_...');
  const [response, setResponse] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const methodColors: Record<string, string> = {
    GET: 'bg-green-100 text-green-800',
    POST: 'bg-blue-100 text-blue-800',
    DELETE: 'bg-red-100 text-red-800',
  };

  const handleTryIt = async () => {
    setLoading(true);
    // Simulate API call
    setTimeout(() => {
      const mockResponse = {
        id: 'pi_3MtwB2eZvKYlo2C2eZvKYlo',
        object: 'payment_intent',
        amount: selectedEndpoint.body?.amount || 2000,
        currency: selectedEndpoint.body?.currency || 'usd',
        status: 'requires_confirmation',
        created: Math.floor(Date.now() / 1000),
      };
      setResponse(JSON.stringify(mockResponse, null, 2));
      setLoading(false);
    }, 1000);
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Interactive API Explorer</h1>
          <p className="mt-4 text-lg text-gray-600">
            Try API calls directly in your browser using your test API keys.
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Endpoint List */}
          <div className="lg:col-span-1">
            <div className="bg-white rounded-lg shadow">
              <div className="px-4 py-3 bg-gray-50 border-b">
                <h2 className="text-lg font-semibold text-gray-900">Endpoints</h2>
              </div>
              <div className="divide-y">
                {apiEndpoints.map((endpoint, idx) => (
                  <button
                    key={idx}
                    onClick={() => setSelectedEndpoint(endpoint)}
                    className={`w-full text-left px-4 py-3 hover:bg-gray-50 ${
                      selectedEndpoint === endpoint ? 'bg-blue-50' : ''
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <span className={`px-2 py-1 text-xs font-bold rounded ${methodColors[endpoint.method]}`}>
                        {endpoint.method}
                      </span>
                      <span className="text-sm font-mono text-gray-700 truncate">{endpoint.path}</span>
                    </div>
                    <p className="text-xs text-gray-500 mt-1">{endpoint.summary}</p>
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Request Panel */}
          <div className="lg:col-span-2">
            <div className="bg-white rounded-lg shadow">
              <div className="px-6 py-4 bg-gray-50 border-b">
                <div className="flex items-center gap-3">
                  <span className={`px-3 py-1 text-sm font-bold rounded ${methodColors[selectedEndpoint.method]}`}>
                    {selectedEndpoint.method}
                  </span>
                  <code className="text-lg font-mono text-gray-900">{selectedEndpoint.path}</code>
                </div>
                <p className="mt-2 text-sm text-gray-600">{selectedEndpoint.summary}</p>
              </div>

              <div className="p-6 space-y-6">
                {/* API Key Input */}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">
                    API Key
                  </label>
                  <input
                    type="text"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-sm"
                    placeholder="sk_test_..."
                  />
                </div>

                {/* Request Body */}
                {selectedEndpoint.body && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Request Body
                    </label>
                    <pre className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto text-sm">
                      {JSON.stringify(selectedEndpoint.body, null, 2)}
                    </pre>
                  </div>
                )}

                {/* Try It Button */}
                <div>
                  <button
                    onClick={handleTryIt}
                    disabled={loading}
                    className="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {loading ? 'Sending...' : 'Try It!'}
                  </button>
                </div>

                {/* Response */}
                {response && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Response
                    </label>
                    <div className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto">
                      <pre className="text-sm">{response}</pre>
                    </div>
                  </div>
                )}
              </div>
            </div>

            {/* Curl Example */}
            <div className="mt-6 bg-white rounded-lg shadow p-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-4">cURL Example</h3>
              <pre className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto text-sm">
{`curl -X ${selectedEndpoint.method} https://api-sandbox.tpt-payments.com${selectedEndpoint.path} \\
  -H "Authorization: Bearer ${apiKey}" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(selectedEndpoint.body || {})}'`}
              </pre>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}