// ESLint flat config for the merchant dashboard. Next 16 removed `next lint`, so the lint script
// runs ESLint directly with the Next.js rules (Core Web Vitals and TypeScript).
import { createRequire } from "node:module"

const require = createRequire(import.meta.url)
const nextConfig = require("eslint-config-next/core-web-vitals")

export default [
  { ignores: [".next/**", "node_modules/**", "next-env.d.ts", "public/**"] },
  ...(Array.isArray(nextConfig) ? nextConfig : [nextConfig]),
]
