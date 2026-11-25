'use client';

import { useState } from 'react';

// The endpoints the explorer can send. They match the server-side allowlist in
// app/api/tpt/explorer/route.ts; an endpoint the core does not serve is not listed.
type Endpoint = {
  method: 'GET' | 'POST';
  path: string; // as documented, with /v1
  summary: string;
  idParam?: string; // the payment intent id this endpoint needs
  body?: Record<string, unknown>;
};

const apiEndpoints: Endpoint[] = [
  {
    method: 'POST',
    path: '/v1/payment_intents',
    summary: 'Create a payment intent',
    body: { amount: 2000, currency: 'usd', metadata: { order_id: '12345' } },
  },
  { method: 'GET', path: '/v1/payment_intents/:id', summary: 'Retrieve a payment intent', idParam: 'pi_' },
  {
    method: 'POST',
    path: '/v1/payment_intents/:id/confirm',
    summary: 'Attach a payment method and authorize',
    idParam: 'pi_',
    body: { payment_method: 'pm_card_visa' },
  },
  { method: 'POST', path: '/v1/payment_intents/:id/capture', summary: 'Capture an authorized payment', idParam: 'pi_' },
  {
    method: 'POST',
    path: '/v1/refunds',
    summary: 'Refund a payment',
    body: { payment_intent: 'pi_...', amount: 500 },
  },
];

const methodColors: Record<string, string> = {
  GET: 'bg-green-100 text-green-800',
  POST: 'bg-blue-100 text-blue-800',
  DELETE: 'bg-red-100 text-red-800',
};

interface Reply {
  status: number;
  body: unknown;
}

export default function ExplorerClient() {
  const [selected, setSelected] = useState<Endpoint>(apiEndpoints[0]);
  const [apiKey, setApiKey] = useState('');
  const [id, setId] = useState('');
  const [bodyText, setBodyText] = useState(JSON.stringify(apiEndpoints[0].body ?? {}, null, 2));
  const [reply, setReply] = useState<Reply | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const needsId = Boolean(selected.idParam);
  const resolvedPath = selected.path.replace(':id', id || '<id>');

  const choose = (endpoint: Endpoint) => {
    setSelected(endpoint);
    setBodyText(JSON.stringify(endpoint.body ?? {}, null, 2));
    setReply(null);
    setError(null);
  };

  const send = async () => {
    setError(null);
    setReply(null);
    if (needsId && !id.trim()) {
      setError('Enter a payment intent id (it starts with pi_).');
      return;
    }
    let body: unknown = undefined;
    if (selected.method === 'POST') {
      try {
        body = JSON.parse(bodyText || '{}');
      } catch {
        setError('The request body is not valid JSON.');
        return;
      }
    }
    setLoading(true);
    try {
      const res = await fetch('/api/tpt/explorer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          method: selected.method,
          path: needsId ? selected.path.replace(':id', id.trim()) : selected.path,
          body,
          key: apiKey.trim(),
        }),
      });
      const json = await res.json().catch(() => ({ error: 'the server returned no JSON' }));
      if (!res.ok) {
        setError(typeof json.error === 'string' ? json.error : `request failed (${res.status})`);
      } else {
        setReply({ status: res.status, body: json });
      }
    } catch {
      setError('Could not reach the explorer. Try again.');
    } finally {
      setLoading(false);
    }
  };

  const keyForCurl = '$TPT_SECRET_KEY'; // the key is never printed, even the one typed here
  const curl = `curl -X ${selected.method} https://api.tpt-payments.com${resolvedPath} \\
  -H "Authorization: Bearer ${keyForCurl}" \\
  -H "Content-Type: application/json"${selected.method === 'POST' ? ` \\\n  -d '${bodyText.replace(/\s+/g, ' ').trim()}'` : ''}`;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Interactive API Explorer</h1>
          <p className="mt-4 text-lg text-gray-600">
            Send real requests to the sandbox with a test key. Live keys are refused.
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-1">
            <div className="bg-white rounded-lg shadow">
              <div className="px-4 py-3 bg-gray-50 border-b">
                <h2 className="text-lg font-semibold text-gray-900">Endpoints</h2>
              </div>
              <div className="divide-y">
                {apiEndpoints.map((endpoint) => (
                  <button
                    key={`${endpoint.method} ${endpoint.path}`}
                    onClick={() => choose(endpoint)}
                    className={`w-full text-left px-4 py-3 hover:bg-gray-50 ${selected === endpoint ? 'bg-blue-50' : ''}`}
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

          <div className="lg:col-span-2">
            <div className="bg-white rounded-lg shadow">
              <div className="px-6 py-4 bg-gray-50 border-b">
                <div className="flex items-center gap-3">
                  <span className={`px-3 py-1 text-sm font-bold rounded ${methodColors[selected.method]}`}>{selected.method}</span>
                  <code className="text-lg font-mono text-gray-900">{resolvedPath}</code>
                </div>
                <p className="mt-2 text-sm text-gray-600">{selected.summary}</p>
              </div>

              <div className="p-6 space-y-6">
                <div>
                  <label htmlFor="explorer-key" className="block text-sm font-medium text-gray-700 mb-2">
                    Test API key
                  </label>
                  <input
                    id="explorer-key"
                    type="password"
                    autoComplete="off"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-sm"
                    placeholder="sk_test_..."
                  />
                  <p className="mt-1 text-xs text-gray-500">
                    The key is sent for this request only. It is not stored or logged.
                  </p>
                </div>

                {needsId && (
                  <div>
                    <label htmlFor="explorer-id" className="block text-sm font-medium text-gray-700 mb-2">
                      Payment intent id
                    </label>
                    <input
                      id="explorer-id"
                      value={id}
                      onChange={(e) => setId(e.target.value)}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-sm"
                      placeholder="pi_..."
                    />
                  </div>
                )}

                {selected.method === 'POST' && (
                  <div>
                    <label htmlFor="explorer-body" className="block text-sm font-medium text-gray-700 mb-2">
                      Request body (JSON)
                    </label>
                    <textarea
                      id="explorer-body"
                      value={bodyText}
                      onChange={(e) => setBodyText(e.target.value)}
                      rows={6}
                      className="w-full px-3 py-2 border border-gray-300 rounded-md font-mono text-sm"
                    />
                  </div>
                )}

                <div>
                  <button
                    onClick={send}
                    disabled={loading || !apiKey.trim()}
                    className="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {loading ? 'Sending…' : 'Send request'}
                  </button>
                </div>

                {error && (
                  <div role="alert" className="rounded-md border border-red-300 bg-red-50 p-3 text-sm text-red-700">
                    {error}
                  </div>
                )}

                {reply && (
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">
                      Response (HTTP {reply.status})
                    </label>
                    <div className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto">
                      <pre className="text-sm">{JSON.stringify(reply.body, null, 2)}</pre>
                    </div>
                  </div>
                )}
              </div>
            </div>

            <div className="mt-6 bg-white rounded-lg shadow p-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-4">cURL example</h3>
              <pre className="bg-gray-900 text-gray-100 p-4 rounded-md overflow-x-auto text-sm">{curl}</pre>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
