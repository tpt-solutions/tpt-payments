// tpt-payments/frontend/app/developers/api-reference/page.tsx
import { Metadata } from 'next';
import { notFound } from 'next/navigation';

export const metadata: Metadata = {
  title: 'API Reference - TPT Pay Developer Portal',
  description: 'Searchable API documentation generated from OpenAPI specification',
};

interface OpenAPISpec {
  openapi: string;
  info: {
    title: string;
    description: string;
    version: string;
  };
  paths: Record<string, any>;
  components: {
    schemas: Record<string, any>;
  };
}

async function getOpenAPISpec(): Promise<OpenAPISpec | null> {
  try {
    const baseUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
    const res = await fetch(`${baseUrl}/openapi.json`, { next: { revalidate: 3600 } });
    if (!res.ok) return null;
    return res.json();
  } catch {
    return null;
  }
}

function MethodBadge({ method }: { method: string }) {
  const colors: Record<string, string> = {
    get: 'bg-green-100 text-green-800',
    post: 'bg-blue-100 text-blue-800',
    put: 'bg-yellow-100 text-yellow-800',
    delete: 'bg-red-100 text-red-800',
    patch: 'bg-purple-100 text-purple-800',
  };
  return (
    <span className={`inline-block px-2 py-1 text-xs font-bold rounded ${colors[method.toLowerCase()] || 'bg-gray-100 text-gray-800'}`}>
      {method.toUpperCase()}
    </span>
  );
}

export default async function APIReferencePage() {
  const spec = await getOpenAPISpec();

  if (!spec) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-gray-900 mb-4">API Specification Not Available</h1>
          <p className="text-gray-600">Unable to load the OpenAPI specification. Please ensure the API server is running.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-gray-900">API Reference</h1>
          <p className="mt-2 text-lg text-gray-600">
            {spec.info.description} (Version: {spec.info.version})
          </p>
        </div>

        {/* Quick Links */}
        <div className="mb-8 bg-white rounded-lg shadow p-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">Quick Links</h2>
          <div className="flex flex-wrap gap-2">
            {Object.keys(spec.paths).map((path) => (
              <a
                key={path}
                href={`#${path.replace(/[\/{}]/g, '_')}`}
                className="px-3 py-1 bg-blue-50 text-blue-700 rounded-full text-sm hover:bg-blue-100"
              >
                {path}
              </a>
            ))}
          </div>
        </div>

        {/* Endpoints */}
        <div className="space-y-6">
          {Object.entries(spec.paths).map(([path, methods]) => (
            <div key={path} id={path.replace(/[\/{}]/g, '_')} className="bg-white rounded-lg shadow overflow-hidden">
              <div className="px-6 py-4 bg-gray-50 border-b">
                <h3 className="text-lg font-mono font-semibold text-gray-900">{path}</h3>
              </div>
              <div className="divide-y">
                {Object.entries(methods as Record<string, any>).map(([method, operation]: [string, any]) => (
                  <div key={method} className="p-6">
                    <div className="flex items-center gap-3 mb-4">
                      <MethodBadge method={method} />
                      <h4 className="text-xl font-semibold text-gray-900">{operation.summary || 'No summary'}</h4>
                    </div>
                    {operation.description && (
                      <p className="text-gray-600 mb-4">{operation.description}</p>
                    )}
                    {operation.operationId && (
                      <p className="text-sm text-gray-500 mb-2">
                        <span className="font-medium">Operation ID:</span> {operation.operationId}
                      </p>
                    )}

                    {/* Parameters */}
                    {operation.parameters && operation.parameters.length > 0 && (
                      <div className="mb-4">
                        <h5 className="text-sm font-semibold text-gray-700 mb-2">Parameters</h5>
                        <div className="overflow-x-auto">
                          <table className="min-w-full divide-y divide-gray-200">
                            <thead className="bg-gray-50">
                              <tr>
                                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
                                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">In</th>
                                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Required</th>
                                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-200">
                              {operation.parameters.map((param: any, idx: number) => (
                                <tr key={idx}>
                                  <td className="px-4 py-2 text-sm font-mono text-gray-900">{param.name}</td>
                                  <td className="px-4 py-2 text-sm text-gray-600">{param.in}</td>
                                  <td className="px-4 py-2 text-sm text-gray-600">{param.required ? 'Yes' : 'No'}</td>
                                  <td className="px-4 py-2 text-sm text-gray-600">{param.schema?.type || 'N/A'}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      </div>
                    )}

                    {/* Responses */}
                    {operation.responses && (
                      <div>
                        <h5 className="text-sm font-semibold text-gray-700 mb-2">Responses</h5>
                        <div className="space-y-2">
                          {Object.entries(operation.responses).map(([status, response]: [string, any]) => (
                            <div key={status} className="border rounded p-3">
                              <span className={`inline-block px-2 py-1 text-xs font-bold rounded ${
                                status.startsWith('2') ? 'bg-green-100 text-green-800' :
                                status.startsWith('4') ? 'bg-red-100 text-red-800' :
                                status.startsWith('5') ? 'bg-orange-100 text-orange-800' :
                                'bg-gray-100 text-gray-800'
                              }`}>
                                {status}
                              </span>
                              <span className="ml-2 text-sm text-gray-600">{response.description}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* Schemas */}
        {spec.components?.schemas && (
          <div className="mt-12">
            <h2 className="text-2xl font-bold text-gray-900 mb-6">Schemas</h2>
            <div className="space-y-6">
              {Object.entries(spec.components.schemas).map(([name, schema]: [string, any]) => (
                <div key={name} className="bg-white rounded-lg shadow p-6">
                  <h3 className="text-xl font-bold text-gray-900 mb-4">{name}</h3>
                  {schema.description && (
                    <p className="text-gray-600 mb-4">{schema.description}</p>
                  )}
                  {schema.properties && (
                    <div className="overflow-x-auto">
                      <table className="min-w-full divide-y divide-gray-200">
                        <thead className="bg-gray-50">
                          <tr>
                            <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Field</th>
                            <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                            <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Required</th>
                            <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Description</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-200">
                          {Object.entries(schema.properties).map(([field, props]: [string, any]) => (
                            <tr key={field}>
                              <td className="px-4 py-2 text-sm font-mono text-gray-900">{field}</td>
                              <td className="px-4 py-2 text-sm text-gray-600">{props.type || props.$ref || 'N/A'}</td>
                              <td className="px-4 py-2 text-sm text-gray-600">
                                {schema.required?.includes(field) ? 'Yes' : 'No'}
                              </td>
                              <td className="px-4 py-2 text-sm text-gray-600">{props.description || ''}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}