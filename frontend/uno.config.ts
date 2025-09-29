import { defineConfig } from 'unocss'
import transformerDirectives from '@unocss/transformer-directives'
import presetWind4 from '@unocss/preset-wind4'
console.debug('uno.config.ts')
export default defineConfig({
  presets: [presetWind4()],
  transformers: [transformerDirectives()],
})
