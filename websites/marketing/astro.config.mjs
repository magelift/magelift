import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://magelift.dev',
  trailingSlash: 'always',
  build: {
    // Docs are generated into public/docs by the site build script.
    assets: '_assets',
  },
});
