import path from 'path';
import { readFileSync } from 'node:fs';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { lingui } from '@lingui/vite-plugin';
import { defineConfig } from 'vite';

import runtimeErrorOverlay from '@replit/vite-plugin-runtime-error-modal';

const rawPort = process.env.PORT;

if (!rawPort) {
  throw new Error(
    'PORT environment variable is required but was not provided.',
  );
}

const port = Number(rawPort);

if (Number.isNaN(port) || port <= 0) {
  throw new Error(`Invalid PORT value: "${rawPort}"`);
}

const basePath = process.env.BASE_PATH;

if (!basePath) {
  throw new Error(
    'BASE_PATH environment variable is required but was not provided.',
  );
}

const localeRegistry = JSON.parse(
  readFileSync(path.resolve(import.meta.dirname, 'src/lib/locale-registry.json'), 'utf8'),
) as {
  defaultLocale: string;
  locales: Record<string, { direction: 'ltr' | 'rtl' }>;
};
const localeBootstrapManifest = {
  defaultLocale: localeRegistry.defaultLocale,
  locales: localeRegistry.locales,
};

const localeBootstrapManifestPlugin = {
  name: 'askolo-locale-bootstrap-manifest',
  transformIndexHtml() {
    const manifest = JSON.stringify(localeBootstrapManifest).replace(/</g, '\\u003c');
    return [{
      tag: 'script',
      children: `window.__ASKOLO_LOCALE_BOOTSTRAP__ = ${manifest};`,
      injectTo: 'head-prepend' as const,
    }];
  },
};

export default defineConfig({
  base: basePath,
  plugins: [
    localeBootstrapManifestPlugin,
    lingui({ failOnMissing: 'resolved' }),
    react({
      babel: {
        plugins: ['@lingui/babel-plugin-lingui-macro'],
      },
    }),
    tailwindcss({ optimize: true }),
    runtimeErrorOverlay(),
    ...(process.env.NODE_ENV !== 'production' &&
    process.env.REPL_ID !== undefined
      ? [
          await import('@replit/vite-plugin-cartographer').then((m) =>
            m.cartographer({
              root: path.resolve(import.meta.dirname, '..'),
            }),
          ),
          await import('@replit/vite-plugin-dev-banner').then((m) =>
            m.devBanner(),
          ),
        ]
      : []),
  ],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, 'src'),
      '@assets': path.resolve(
        import.meta.dirname,
        '..',
        '..',
        'attached_assets',
      ),
    },
    dedupe: ['react', 'react-dom'],
  },
  root: path.resolve(import.meta.dirname),
  build: {
    outDir: path.resolve(import.meta.dirname, 'dist/public'),
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Avoid dozens of sub-kilobyte requests while preserving route-level
        // lazy loading for larger feature pages.
        experimentalMinChunkSize: 8 * 1024,
      },
    },
  },
  server: {
    port,
    strictPort: true,
    host: '0.0.0.0',
    allowedHosts: true,
    fs: {
      strict: true,
    },
  },
  preview: {
    port,
    host: '0.0.0.0',
    allowedHosts: true,
  },
});
