// Sidebar for the integrator docs. Order here is the reading order.
/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  integratorSidebar: [
    'intro',
    'quickstart',
    {
      type: 'category',
      label: 'Guides',
      collapsed: false,
      items: ['payments', 'refunds', 'webhooks', 'sandbox'],
    },
    {
      type: 'category',
      label: 'SDKs and tools',
      items: ['sdks'],
    },
    {
      type: 'category',
      label: 'Reference',
      items: ['api-reference'],
    },
    {
      type: 'category',
      label: 'Migrating',
      items: ['migration/stripe'],
    },
  ],
};

module.exports = sidebars;
