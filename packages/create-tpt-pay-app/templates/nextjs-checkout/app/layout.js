export const metadata = { title: 'Checkout example', description: 'A TPT Pay checkout with Next.js' };

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body style={{ fontFamily: 'system-ui, sans-serif', margin: '2rem', maxWidth: '36rem' }}>{children}</body>
    </html>
  );
}
