import type { Network } from 'vis-network'
import type { NetworkPosition, NetworkPositions } from './networkLayoutStorage'

interface Velocity {
  x: number
  y: number
}

interface PhysicsNetwork {
  physics: {
    physicsBody: {
      velocities: Record<string, Velocity>
    }
  }
}

const SETTLED_WAKE_SPEED = 3

function physicsVelocities(network: Network): Record<string, Velocity> {
  const physics = (network as Network & Partial<PhysicsNetwork>).physics
  if (!physics?.physicsBody?.velocities) {
    throw new Error('vis-network physics velocity state is unavailable')
  }
  return physics.physicsBody.velocities
}

function assertFiniteVelocity(id: string, velocity: Velocity): void {
  if (!Number.isFinite(velocity.x) || !Number.isFinite(velocity.y)) {
    throw new Error(`vis-network returned an invalid velocity for node ${id}`)
  }
}

export function captureNetworkPositions(
  network: Network,
  nodeIds: string[],
  includeVelocity: boolean,
): NetworkPositions {
  const positions = network.getPositions(nodeIds) as NetworkPositions
  if (!includeVelocity) return positions

  const velocities = physicsVelocities(network)
  return Object.fromEntries(
    nodeIds.map((id) => {
      const position = positions[id]
      if (!position) throw new Error(`vis-network returned no position for node ${id}`)
      const velocity = velocities[id]
      if (!velocity) return [id, position]
      assertFiniteVelocity(id, velocity)
      return [id, { ...position, vx: velocity.x, vy: velocity.y }]
    }),
  )
}

export function restoreNetworkVelocities(network: Network, positions: NetworkPositions): void {
  const velocities = physicsVelocities(network)
  for (const [id, position] of Object.entries(positions)) {
    if (position.vx === undefined || position.vy === undefined) continue
    const velocity: Velocity = { x: position.vx, y: position.vy }
    assertFiniteVelocity(id, velocity)
    velocities[id] = velocity
  }
}

export function wakeSettledNetwork(network: Network, positions: NetworkPositions): void {
  const velocities = physicsVelocities(network)
  const entries = Object.entries(positions)
  const alreadyMoving = entries.some(([id]) => {
    const velocity = velocities[id]
    return velocity && (Math.abs(velocity.x) > 0.001 || Math.abs(velocity.y) > 0.001)
  })
  if (alreadyMoving || entries.length === 0) return

  const center = entries.reduce(
    (sum, [, position]) => ({ x: sum.x + position.x, y: sum.y + position.y }),
    { x: 0, y: 0 },
  )
  center.x /= entries.length
  center.y /= entries.length

  entries.forEach(([id, position], index) => {
    let dx = position.x - center.x
    let dy = position.y - center.y
    let distance = Math.hypot(dx, dy)
    if (distance < 0.001) {
      const angle = (index / entries.length) * Math.PI * 2
      dx = Math.cos(angle)
      dy = Math.sin(angle)
      distance = 1
    }
    velocities[id] = {
      x: (-dy / distance) * SETTLED_WAKE_SPEED,
      y: (dx / distance) * SETTLED_WAKE_SPEED,
    }
  })
}

export function positionCoordinates(position: NetworkPosition): Pick<NetworkPosition, 'x' | 'y'> {
  return { x: position.x, y: position.y }
}
