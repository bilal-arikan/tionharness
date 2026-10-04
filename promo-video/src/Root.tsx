import { Composition } from 'remotion'
import { Promo } from './Promo'
import { FPS, TOTAL_FRAMES } from './theme'
import { FlowsOverview, OVERVIEW_FRAMES } from './flows/Overview'
import { EVOLUTION_FRAMES, FlowsEvolution } from './flows/Evolution'
import { FlowsJev, JEV_FRAMES } from './flows/Jev'

const formats = [
  { id: 'Horizontal', width: 1920, height: 1080 },
  { id: 'Vertical', width: 1080, height: 1920 },
  { id: 'Square', width: 1080, height: 1080 },
]

export const RemotionRoot: React.FC = () => (
  <>
    {formats.map((f) => (
      <Composition
        key={f.id}
        id={f.id}
        component={Promo}
        durationInFrames={TOTAL_FRAMES}
        fps={FPS}
        width={f.width}
        height={f.height}
      />
    ))}
    <Composition id="FlowsOverview" component={FlowsOverview} durationInFrames={OVERVIEW_FRAMES} fps={FPS} width={1080} height={1920} />
    <Composition id="FlowsEvolution" component={FlowsEvolution} durationInFrames={EVOLUTION_FRAMES} fps={FPS} width={1080} height={1920} />
    <Composition id="FlowsJev" component={FlowsJev} durationInFrames={JEV_FRAMES} fps={FPS} width={1080} height={1920} />
  </>
)
