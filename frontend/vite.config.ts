import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Unocss from 'unocss/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'
import svgLoader from 'vite-svg-loader'
import AutoImport from 'unplugin-auto-import/vite'
import { VitePWA } from 'vite-plugin-pwa'
// import basicSsl from '@vitejs/plugin-basic-ssl'

// 读取 package.json 中的版本号
import packageJson from './package.json'

// https://vite.dev/config/
export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(packageJson.version),
  },
  build: {
    outDir: 'dist',
    // 生成 sourcemap（包含第三方库）
    sourcemap: true,
    rollupOptions: {
      output: {
        // 不忽略任何文件，让所有 sourcemap 都可见
        sourcemapIgnoreList: () => false,
        advancedChunks: {
          groups: [
            {
              test: /node_modules\/(?:vue|vue-router|@vueuse\/core|pinia)/,
              name: 'common',
              priority: 1,
            },
            {
              test: /node_modules\/(?:naive-ui)/,
              name: 'ui',
              priority: 2,
            },
          ],
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  plugins: [
    vue(),
    svgLoader(),
    Unocss(),
    // basicSsl(), // HTTPS 支持
    AutoImport({
      imports: ['vue', '@vueuse/core', 'vue-router', 'pinia'],
      dts: 'src/auto-imports.d.ts',
      eslintrc: {
        enabled: true,
      },
      vueTemplate: true,
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: true,
    }),
    VitePWA({
      injectRegister: 'script',
      registerType: 'autoUpdate',
      // strategies: 'injectManifest',
      workbox: {
        globPatterns: ['**/*.{js,css,html,ico,png,svg,jpg,jpeg}'],
        navigateFallbackDenylist: [/.*\/api\/v\d+\/system\/logging.*/],
        disableDevLogs: true,
      },
      injectManifest: {
        rollupFormat: 'iife',
      },
      devOptions: {
        enabled: true,
        type: 'module',
      },
      manifest: {
        name: 'Watch Docker',
        short_name: 'Watch Docker',
        start_url: '/',
        display: 'standalone',
        id: '/',
        screenshots: [
          {
            src: '/bg.png',
            sizes: '822x1408',
            type: 'image/png',
          },
        ],
        icons: [
          {
            src: '/64x64.png',
            sizes: '64x64',
            type: 'image/png',
            purpose: 'any',
          },
          {
            src: '/192x192.png',
            sizes: '192x192',
            type: 'image/png',
            purpose: 'any',
          },
          {
            src: '/512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'any',
          },
        ],
        theme_color: '#28243D',
        background_color: '#28243D',
      },
    }),
  ],
  server: {
    host: '::',
    port: 5173,
    // HTTPS 由 @vitejs/plugin-basic-ssl 插件自动启用
    headers: {
      'Access-Control-Allow-Origin': '*',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization',
      Authorization: 'Basic YWRtaW46Q2lkc2ljLXNpc2phZC1yeXptdTE=',
    },
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true, // 启用 WebSocket 代理
        // 如果后端也是 HTTPS，请改为 https://localhost:8080
      },
    },
  },
})
