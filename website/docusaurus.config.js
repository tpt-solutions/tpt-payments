// @ts-check
// Integrator documentation for TPT Pay. Build with `npm run build` in this directory.
//
// The site URL is not decided yet (see todo.md, section 3), so the placeholder below is used. Set
// it before publishing. Broken links fail the build, which is the documentation link check.

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'TPT Pay',
  tagline: 'Open-source payments API. Self-hostable. Pre-release.',
  favicon: 'img/favicon.ico',
  url: 'https://docs.example.invalid',
  baseUrl: '/',
  organizationName: 'tpt-solutions',
  projectName: 'tpt-payments',
  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },
  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: require.resolve('./sidebars.js'),
          routeBasePath: '/',
          editUrl: 'https://github.com/tpt-solutions/tpt-payments/tree/master/website/',
        },
        blog: false,
      }),
    ],
  ],
  themeConfig: {
    navbar: {
      title: 'TPT Pay',
      items: [
        { type: 'docSidebar', sidebarId: 'integratorSidebar', position: 'left', label: 'Docs' },
        { href: 'https://github.com/tpt-solutions/tpt-payments', label: 'GitHub', position: 'right' },
      ],
    },
    footer: {
      style: 'dark',
      copyright: 'Dual licensed MIT / Apache-2.0.',
    },
    prism: {
      additionalLanguages: ['bash', 'json'],
    },
  },
};

module.exports = config;
