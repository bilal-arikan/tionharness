import { describe, expect, it } from 'vitest'
import {
  MAX_HISTORY_ENTRIES,
  MAX_MODEL_BYTES,
  MAX_POINTS_PER_STROKE,
  MAX_STROKES,
  MAX_TOTAL_POINTS,
  POINT_MODEL_BYTES,
  STROKE_MODEL_BYTES,
} from './imageAnnotatorLimits'
import {
  appendPoint,
  beginStroke,
  clear,
  createDrawingModel,
  drawingModelBytes,
  finalizeStroke,
  modelBytes,
  redo,
  snapshot,
  snapshotsEqual,
  undo,
  type Stroke,
} from './imageAnnotatorModel'

const point = { x: 1, y: 2, pressure: 0.5 }
const stroke = (id: number, points = 1): Stroke => ({
  id,
  color: '#000',
  width: 2,
  points: Array.from({ length: points }, () => point),
})

describe('drawing model', () => {
  it('finalizes, undoes to baseline, redoes, and clears with monotonic revisions', () => {
    const baseline = snapshot(createDrawingModel())
    let model = beginStroke(createDrawingModel(), point, '#000', 2).model
    model = finalizeStroke(model).model
    expect(model.revision).toBe(1)
    model = undo(model)
    expect(model.revision).toBe(2)
    expect(snapshotsEqual(snapshot(model), baseline)).toBe(true)
    model = redo(model)
    expect(model.revision).toBe(3)
    model = clear(model)
    expect(model.revision).toBe(4)
  })

  it('accepts stroke limit-1 and limit, then finalizes at limit+1', () => {
    let model = createDrawingModel()
    model = beginStroke(model, point, '#000', 2).model
    model = {
      ...model,
      active: {
        ...model.active!,
        points: Array.from({ length: MAX_POINTS_PER_STROKE - 1 }, () => point),
      },
    }
    expect(appendPoint(model, point).model.active?.points).toHaveLength(MAX_POINTS_PER_STROKE)
    const result = appendPoint(appendPoint(model, point).model, point)
    expect(result.warning).toBe('STROKE_POINT_LIMIT')
    expect(result.model.active).toBeNull()
  })

  it('rejects the 501st stroke', () => {
    const strokes = Array.from({ length: MAX_STROKES }, (_, i) => stroke(i))
    expect(
      beginStroke(createDrawingModel(strokes.slice(0, -1)), point, '#000', 2).warning,
    ).toBeUndefined()
    expect(beginStroke(createDrawingModel(strokes), point, '#000', 2).warning).toBe(
      'DRAWING_STROKE_LIMIT',
    )
  })

  it('enforces total point and deterministic model-byte limits', () => {
    const atPointLimit = createDrawingModel([stroke(1, MAX_TOTAL_POINTS)])
    expect(beginStroke(atPointLimit, point, '#000', 2).warning).toBe('DRAWING_POINT_LIMIT')
    expect(modelBytes([stroke(1, 2)])).toBe(STROKE_MODEL_BYTES + 2 * POINT_MODEL_BYTES)
    const deltaPointCount = 150_000
    const historyStroke = stroke(1, deltaPointCount)
    const atMemoryLimit = {
      ...createDrawingModel(),
      undo: Array.from({ length: 10 }, (_, i) => ({ strokes: i % 2 ? [historyStroke] : [] })),
    }
    expect(drawingModelBytes(atMemoryLimit)).toBeGreaterThan(MAX_MODEL_BYTES)
    expect(beginStroke(atMemoryLimit, point, '#000', 2).warning).toBe('DRAWING_MEMORY_LIMIT')
  })

  it('evicts undo history FIFO at 100 entries without removing live strokes', () => {
    let model = createDrawingModel()
    for (let i = 0; i < MAX_HISTORY_ENTRIES + 1; i++) {
      model = beginStroke(model, { ...point, x: i }, '#000', 2).model
      model = finalizeStroke(model).model
    }
    expect(model.undo).toHaveLength(MAX_HISTORY_ENTRIES)
    expect(model.strokes).toHaveLength(MAX_HISTORY_ENTRIES + 1)
    for (let i = 0; i < MAX_HISTORY_ENTRIES; i++) model = undo(model)
    expect(model.strokes).toHaveLength(1)
  })
})
