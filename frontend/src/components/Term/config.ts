import { type ITheme } from '@xterm/xterm'

// 默认暗色主题
export const defaultTheme: ITheme = {
  background: 'rgba(0, 0, 0, 0.1)',
  foreground: '#d4d4d4',
  cursor: '#d4d4d4',
  black: '#000000',
  red: '#cd3131',
  green: '#0dbc79',
  yellow: '#e5e510',
  blue: '#2472c8',
  magenta: '#bc3fbc',
  cyan: '#11a8cd',
  white: '#e5e5e5',
  brightBlack: '#666666',
  brightRed: '#f14c4c',
  brightGreen: '#23d18b',
  brightYellow: '#f5f543',
  brightBlue: '#3b8eea',
  brightMagenta: '#d670d6',
  brightCyan: '#29b8db',
  brightWhite: '#e5e5e5',
}

export function useTheme(theme: string): ITheme {
  if (theme === 'light') {
    return {
      ...defaultTheme,
      background: '#1e1e1e',
    }
  }
  return defaultTheme
}
