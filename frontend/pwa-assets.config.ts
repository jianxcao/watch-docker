import { defineConfig, minimal2023Preset as preset } from '@vite-pwa/assets-generator/config'

export default defineConfig({
  headLinkOptions: {
    preset: '2023',
  },
  transparent: {
    sizes: [64, 180, 192, 512],
    favicons: [[512, 'favicon.ico']],
  },
  preset,
  images: ['public/logo.svg'],
})
