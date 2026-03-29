import { generateDownloadToken, getDownloadUrl } from '@/common/api'

/**
 * 触发浏览器下载
 */
function triggerBrowserDownload(url: string, filename: string) {
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

/**
 * 下载容器文件
 * @param containerId 容器ID
 * @param filePath 文件路径
 * @param filename 下载的文件名（不包含 .tar 后缀）
 * @returns Promise<void>
 * @throws Error 下载失败时抛出错误
 */
export async function downloadContainerFile(
  containerId: string,
  filePath: string,
  filename: string,
): Promise<void> {
  // 生成临时下载令牌（60秒有效，用后即焚）
  const res = await generateDownloadToken(containerId, filePath)
  if (res.code !== 0) {
    throw new Error(res.msg || '生成下载令牌失败')
  }

  // 构造下载 URL（支持流式下载）
  const downloadUrl = getDownloadUrl(containerId, filePath, res.data.token)

  // 触发浏览器原生下载（后端返回 tar 格式）
  triggerBrowserDownload(downloadUrl, filename + '.tar')
}
