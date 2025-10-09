import type { ContainerStatus } from '@/common/types'
import mitt from 'mitt'
type Events = {
  containers: ContainerStatus[]
}
const emitter = mitt<Events>()

export default emitter
