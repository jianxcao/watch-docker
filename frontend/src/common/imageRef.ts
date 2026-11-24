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
  } else if (repo.startsWith('registry-1.docker.io/')) {
    repo = repo.slice('registry-1.docker.io/'.length)
  }
  if (repo.startsWith('library/') && repo.slice('library/'.length).indexOf('/') === -1) {
    repo = repo.slice('library/'.length)
  }

  if (digest) {
    return `${repo}@${digest}`
  }
  if (tag) {
    return `${repo}:${tag}`
  }
  return `${repo}:latest`
}

function stripSha256Prefix(id: string): string {
  const trimmed = id.trim().toLowerCase()
  return trimmed.startsWith('sha256:') ? trimmed.slice('sha256:'.length) : trimmed
}

function isMatchingImageId(a: string, b: string): boolean {
  const left = a.trim().toLowerCase()
  const right = b.trim().toLowerCase()
  if (!left || !right) {
    return false
  }
  if (left === right) {
    return true
  }
  const leftHex = stripSha256Prefix(left)
  const rightHex = stripSha256Prefix(right)
  if (leftHex === rightHex) {
    return true
  }
  const hexShortRe = /^[0-9a-f]{12,64}$/
  if (hexShortRe.test(leftHex) && hexShortRe.test(rightHex)) {
    return leftHex.length <= rightHex.length
      ? rightHex.startsWith(leftHex)
      : leftHex.startsWith(rightHex)
  }
  return false
}

/**
 * 精确判断容器是否使用了指定镜像：
 * 1) 当双方都有 imageID 时严格按 imageID 比对（支持 12-64 位短 ID 前缀，不匹配时不回退标签）；
 * 2) 若容器引用本身是 digest，则仅比对 repoDigests，不回退到标签；
 * 3) 其次比对规范化后的 repo:tag。
 */
export function doesContainerUseImage(
  container: Pick<ContainerStatus, 'image' | 'imageID'>,
  image: Pick<ImageInfo, 'id' | 'repoTags' | 'repoDigests'>,
): boolean {
  if (container.imageID && image.id) {
    return isMatchingImageId(container.imageID, image.id)
  }

  const rawContainerImage = (container.image || '').trim()
  if (!rawContainerImage) {
    return false
  }

  if (image.id && isMatchingImageId(rawContainerImage, image.id)) {
    return true
  }

  const { digest: containerDigest } = parseImageRef(rawContainerImage)
  if (containerDigest) {
    return Boolean(image.repoDigests?.some((rd) => rd.endsWith(`@${containerDigest}`)))
  }

  const validTags = (image.repoTags || []).filter((tag) => tag && tag !== '<none>:<none>')
  if (validTags.length === 0) {
    return false
  }

  const normalizedContainer = normalizeImageRef(rawContainerImage)
  return validTags.some((tag) => normalizeImageRef(tag) === normalizedContainer)
}
