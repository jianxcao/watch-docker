import type { ContainerStatus, ImageInfo } from './types'

export interface ParsedImageRef {
  repository: string
  tag: string
  digest: string
}

/**
 * 解析镜像引用，正确区分 registry 端口号 (:5000/repo) 与标签 (:tag)
 */
export function parseImageRef(raw: string): ParsedImageRef {
  const input = (raw || '').trim()
  if (!input) {
    return { repository: '', tag: '', digest: '' }
  }

  let namePart = input
  let digest = ''
  const atIndex = input.indexOf('@')
  if (atIndex >= 0) {
    namePart = input.slice(0, atIndex)
    digest = input.slice(atIndex + 1)
  }

  const lastSlash = namePart.lastIndexOf('/')
  const lastColon = namePart.lastIndexOf(':')
  if (lastColon > lastSlash) {
    return {
      repository: namePart.slice(0, lastColon),
      tag: namePart.slice(lastColon + 1),
      digest,
    }
  }

  return {
    repository: namePart,
    tag: '',
    digest,
  }
}

/**
 * 规范化镜像引用，处理 docker.io/library 前缀与默认 :latest 标签（仅在无 tag 且无 digest 时补全）
 */
export function normalizeImageRef(raw: string): string {
  const { repository, tag, digest } = parseImageRef(raw)
  if (!repository) {
    return ''
  }

  let repo = repository
  if (repo.startsWith('docker.io/')) {
    repo = repo.slice('docker.io/'.length)
  } else if (repo.startsWith('index.docker.io/')) {
    repo = repo.slice('index.docker.io/'.length)
  }
  if (repo.startsWith('library/') && repo.slice('library/'.length).indexOf('/') === -1) {
    repo = repo.slice('library/'.length)
  }

  if (tag) {
    return `${repo}:${tag}`
  }
  if (digest) {
    return `${repo}@${digest}`
  }
  return `${repo}:latest`
}

/**
 * 精确判断容器是否使用了指定镜像：
 * 1) 优先精确比对 imageID；
 * 2) 其次比对规范化后的 repo:tag 或 repo@digest（不会将不同版本标签误判为使用中）。
 */
export function doesContainerUseImage(
  container: Pick<ContainerStatus, 'image' | 'imageID'>,
  image: Pick<ImageInfo, 'id' | 'repoTags' | 'repoDigests'>,
): boolean {
  if (container.imageID && image.id && container.imageID === image.id) {
    return true
  }

  const rawContainerImage = (container.image || '').trim()
  if (!rawContainerImage) {
    return false
  }

  if (image.id && (rawContainerImage === image.id || image.id === `sha256:${rawContainerImage}`)) {
    return true
  }

  const { digest: containerDigest } = parseImageRef(rawContainerImage)
  if (containerDigest && image.repoDigests?.some((rd) => rd.endsWith(`@${containerDigest}`))) {
    return true
  }

  const validTags = (image.repoTags || []).filter((tag) => tag && tag !== '<none>:<none>')
  if (validTags.length === 0) {
    return false
  }

  const normalizedContainer = normalizeImageRef(rawContainerImage)
  return validTags.some((tag) => normalizeImageRef(tag) === normalizedContainer)
}
