import { defineConfig } from 'unocss'
import transformerDirectives from '@unocss/transformer-directives'
import presetWind4 from '@unocss/preset-wind4'

export default defineConfig({
  content: {
    pipeline: {
      include: ['src/**/*.{vue,js,ts,jsx,tsx}'],
    },
  },
  presets: [presetWind4()],
  transformers: [
    transformerDirectives({
      enforce: 'pre',
    }),
  ],
})
