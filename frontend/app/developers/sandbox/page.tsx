// tpt-payments/frontend/app/developers/sandbox/page.tsx
import { Metadata } from 'next';

export const metadata: Metadata = {
  title: 'Sandbox Environment - TPT Pay Developer Portal',
  description: 'Isolated test mode for safe integration testing with no real charges',
};

const testCards = [
  { brand: 'Visa', number: '4242424242424242', cvv: '123', response: 'Succeeds' },
  { brand: 'Visa (Debit)', number: '4000056655665556', cvv: '123', response: 'Succeeds' },
  { brand: 'Mastercard', number: '5555555555554444', cvv: '123', response: 'Succeeds' },
  { brand: 'Amex', number: '378282246310005', cvv: '1234', response: 'Succeeds' },
  { brand: 'Visa (3D Secure)', number: '4000002500003155', cvv: '123', response: 'Requires 3DS' },
  { brand: 'Visa (Decline)', number: '4000000000000069', cvv: '123', response: 'Declined' },
  { brand: 'Visa (Insufficient Funds)', number: '4000000000009995', cvv: '123', response: 'Declined' },
];

const testBankAccounts = [
  { type: 'US Checking', number: '000123456789', routing: '110000000', response: 'Succeeds' },
  { type: 'US Savings', number: '000987654321', routing: '110000000', response: 'Succeeds' },
];

export default function SandboxPage() {
  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">Sandbox Environment</h1>
          <p className="mt-4 text-lg text-gray-600">
            Isolated test mode for safe integration testing with no real charges.
          </p>
        </div>

        {/* Getting Started */}
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-6 mb-8">
          <h2 className="text-xl font-bold text-blue-900 mb-4">Getting Started with Sandbox</h2>
          <div className="space-y-4">
            <div className="flex items-start gap-3">
              <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm flex-shrink-0">1</span>
              <div>
                <h3 className="font-semibold text-blue-900">Create a Sandbox Account</h3>
                <p className="text-sm text-blue-800 mt-1">
                  Sign up for a free sandbox account at <code className="bg-blue-100 px-1 rounded">https://sandbox.tpt-payments.com</code>
                </p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm flex-shrink-0">2</span>
              <div>
                <h3 className="font-semibold text-blue-900">Get Test API Keys</h3>
                <p className="text-sm text-blue-800 mt-1">
                  Generate test API keys from the dashboard. Test keys start with <code className="bg-blue-100 px-1 rounded">sk_test_</code>
                </p>
              </div>
            </div>
            <div className="flex items-start gap-3">
              <span className="flex items-center justify-center w-8 h-8 bg-blue-600 text-white rounded-full font-bold text-sm flex-shrink-0">3</span>
              <div>
                <h3 className="font-semibold text-blue-900">Use Sandbox API Endpoint</h3>
                <p className="text-sm text-blue-800 mt-1">
                  Point your integration to <code className="bg-blue-100 px-1 rounded">https://api.tpt-payments.com/v1</code>
                </p>
              </div>
            </div>
          </div>
        </div>

        {/* Test Card Numbers */}
        <div className="bg-white rounded-lg shadow p-6 mb-8">
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Test Card Numbers</h2>
          <p className="text-gray-600 mb-6">
            Use these card numbers in the sandbox environment. No real cards will be charged.
          </p>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Brand</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Number</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">CVV</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Response</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {testCards.map((card, idx) => (
                  <tr key={idx}>
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">{card.brand}</td>
                    <td className="px-4 py-3 text-sm font-mono text-gray-600">{card.number}</td>
                    <td className="px-4 py-3 text-sm font-mono text-gray-600">{card.cvv}</td>
                    <td className="px-4 py-3 text-sm">
                      <span className={`px-2 py-1 rounded-full text-xs font-medium ${
                        card.response === 'Succeeds' ? 'bg-green-100 text-green-800' :
                        card.response === 'Declined' ? 'bg-red-100 text-red-800' :
                        'bg-yellow-100 text-yellow-800'
                      }`}>
                        {card.response}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Test Bank Accounts */}
        <div className="bg-white rounded-lg shadow p-6 mb-8">
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Test Bank Accounts</h2>
          <p className="text-gray-600 mb-6">
            Use these bank account numbers for testing ACH and payout flows.
          </p>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Account Number</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Routing Number</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Response</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {testBankAccounts.map((account, idx) => (
                  <tr key={idx}>
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">{account.type}</td>
                    <td className="px-4 py-3 text-sm font-mono text-gray-600">{account.number}</td>
                    <td className="px-4 py-3 text-sm font-mono text-gray-600">{account.routing}</td>
                    <td className="px-4 py-3 text-sm">
                      <span className="px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-800">
                        {account.response}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Sandbox Limitations */}
        <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-6">
          <h2 className="text-xl font-bold text-yellow-900 mb-4">Sandbox Limitations</h2>
          <ul className="list-disc list-inside space-y-2 text-sm text-yellow-800">
            <li>No real money is moved; all transactions are simulated.</li>
            <li>Webhooks may have delayed delivery (up to 1 minute).</li>
            <li>Some features (like direct acquiring) are not available in sandbox.</li>
            <li>API rate limits are more lenient in sandbox mode.</li>
            <li>Test data is periodically reset (every 30 days).</li>
          </ul>
        </div>
      </div>
    </div>
  );
}