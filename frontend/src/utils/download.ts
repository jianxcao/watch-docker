export function buildImageDownloadUrl(imageId: string, token: string, now = Date.now()): string {
  return `/api/v1/images/${encodeURIComponent(imageId)}/download?token=${encodeURIComponent(token)}&_t=${now}`
}

export function triggerBrowserDownload(url: string): void {
  const link = document.createElement('a')
  link.href = url
  link.download = ''
  link.style.display = 'none'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}
