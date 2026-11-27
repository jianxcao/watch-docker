export interface ImagesViewMountDeps {
  fetchImages: () => Promise<void>
  fetchContainers: () => Promise<void>
  hasContainerWork: () => boolean
  onImagesError?: (message: string) => void
}

/**
 * 镜像页挂载初始化：每次进入独立触发镜像列表刷新（由 store 负责并发复用），并在需要时并行按需加载容器列表。
 */
export async function loadImagesViewOnMount(deps: ImagesViewMountDeps): Promise<void> {
  const imageTask = deps.fetchImages().catch((error: unknown) => {
    const msg = error instanceof Error ? error.message : '未知错误'
    deps.onImagesError?.(msg || '未知错误')
  })

  const containerTask = deps.hasContainerWork()
    ? deps.fetchContainers().catch((error: unknown) => {
        console.error('获取容器列表失败:', error)
      })
    : Promise.resolve()

  await Promise.all([imageTask, containerTask])
}
