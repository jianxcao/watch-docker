function buildDownloadQuery(token: string, now: number): string {
  const tokenQuery = token ? `token=${encodeURIComponent(token)}&` : ''
  return `${tokenQuery}_t=${now}`
}

export function buildImageDownloadUrl(imageId: string, token: string, now = Date.now()): string {
  return `/api/v1/images/${encodeURIComponent(imageId)}/download?${buildDownloadQuery(token, now)}`
}

export function buildContainerExportUrl(
  containerId: string,
  token: string,
  now = Date.now(),
): string {
  return `/api/v1/containers/${encodeURIComponent(containerId)}/export?${buildDownloadQuery(token, now)}`
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
