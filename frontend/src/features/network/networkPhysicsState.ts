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

export function positionCoordinates(position: NetworkPosition): Pick<NetworkPosition, 'x' | 'y'> {
  return { x: position.x, y: position.y }
}
