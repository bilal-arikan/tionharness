import {
  MAX_HISTORY_ENTRIES,
  MAX_MODEL_BYTES,
  MAX_POINTS_PER_STROKE,
  MAX_STROKES,
  MAX_TOTAL_POINTS,
  POINT_MODEL_BYTES,
  STROKE_MODEL_BYTES,
  type ImageAnnotatorErrorCode,
} from './imageAnnotatorLimits'

export interface StrokePoint {
  x: number
  y: number
  pressure: number
}
export interface Stroke {
  id: number
  color: string
  width: number
  points: StrokePoint[]
}
export interface DrawingSnapshot {
  strokes: Stroke[]
}
export interface DrawingModel {
  strokes: Stroke[]
  undo: DrawingSnapshot[]
  redo: DrawingSnapshot[]
  revision: number
  nextStrokeId: number
  active: Stroke | null
}

export type ModelResult = { model: DrawingModel; warning?: ImageAnnotatorErrorCode }

const copyStrokes = (strokes: Stroke[]): Stroke[] =>
  strokes.map((stroke) => ({ ...stroke, points: stroke.points.map((point) => ({ ...point })) }))

export const snapshot = (model: DrawingModel): DrawingSnapshot => ({
  strokes: copyStrokes(model.strokes),
})
export const snapshotsEqual = (a: DrawingSnapshot, b: DrawingSnapshot): boolean =>
  JSON.stringify(a) === JSON.stringify(b)
export const totalPoints = (strokes: Stroke[]): number =>
  strokes.reduce((sum, stroke) => sum + stroke.points.length, 0)
export const modelBytes = (strokes: Stroke[]): number =>
  strokes.length * STROKE_MODEL_BYTES + totalPoints(strokes) * POINT_MODEL_BYTES

function deltaBytes(a: DrawingSnapshot, b: DrawingSnapshot): number {
  const aById = new Map(a.strokes.map((stroke) => [stroke.id, stroke]))
  const bById = new Map(b.strokes.map((stroke) => [stroke.id, stroke]))
  const ids = new Set([...aById.keys(), ...bById.keys()])
  let bytes = 0
  for (const id of ids) {
    const before = aById.get(id)
    const after = bById.get(id)
    if (JSON.stringify(before) !== JSON.stringify(after))
      bytes += Math.max(modelBytes(before ? [before] : []), modelBytes(after ? [after] : []))
  }
  return bytes
}

export function drawingModelBytes(model: DrawingModel): number {
  const current = snapshot(model)
  let bytes = modelBytes(model.strokes)
  let cursor = current
  for (let i = model.undo.length - 1; i >= 0; i--) {
    bytes += deltaBytes(cursor, model.undo[i])
    cursor = model.undo[i]
  }
  cursor = current
  for (const entry of model.redo) {
    bytes += deltaBytes(cursor, entry)
    cursor = entry
  }
  if (model.active) bytes += modelBytes([model.active])
  return bytes
}

function finalizedActiveBytes(model: DrawingModel, additionalPoints = 0): number {
  if (!model.active) return drawingModelBytes(model)
  const activeBytes =
    STROKE_MODEL_BYTES + (model.active.points.length + additionalPoints) * POINT_MODEL_BYTES
  // Active stroke becomes both live model data and one undo delta when finalized.
  return drawingModelBytes(model) + activeBytes + additionalPoints * POINT_MODEL_BYTES
}

export function createDrawingModel(strokes: Stroke[] = []): DrawingModel {
  return {
    strokes: copyStrokes(strokes),
    undo: [],
    redo: [],
    revision: 0,
    nextStrokeId: 1,
    active: null,
  }
}

function pushUndo(model: DrawingModel): {
  undo: DrawingSnapshot[]
  warning?: ImageAnnotatorErrorCode
} {
  const undo = [...model.undo, snapshot(model)]
  if (undo.length <= MAX_HISTORY_ENTRIES) return { undo }
  return { undo: undo.slice(-MAX_HISTORY_ENTRIES), warning: 'HISTORY_LIMIT' }
}

export function beginStroke(
  model: DrawingModel,
  point: StrokePoint,
  color: string,
  width: number,
): ModelResult {
  if (model.active) return { model }
  if (model.strokes.length >= MAX_STROKES) return { model, warning: 'DRAWING_STROKE_LIMIT' }
  if (totalPoints(model.strokes) >= MAX_TOTAL_POINTS)
    return { model, warning: 'DRAWING_POINT_LIMIT' }
  if (drawingModelBytes(model) + 2 * (STROKE_MODEL_BYTES + POINT_MODEL_BYTES) > MAX_MODEL_BYTES)
    return { model, warning: 'DRAWING_MEMORY_LIMIT' }
  return { model: { ...model, active: { id: model.nextStrokeId, color, width, points: [point] } } }
}

export function appendPoint(model: DrawingModel, point: StrokePoint): ModelResult {
  if (!model.active) return { model }
  if (model.active.points.length >= MAX_POINTS_PER_STROKE)
    return finalizeStroke(model, 'STROKE_POINT_LIMIT')
  if (totalPoints(model.strokes) + model.active.points.length >= MAX_TOTAL_POINTS)
    return finalizeStroke(model, 'DRAWING_POINT_LIMIT')
  if (finalizedActiveBytes(model, 1) > MAX_MODEL_BYTES)
    return finalizeStroke(model, 'DRAWING_MEMORY_LIMIT')
  return {
    model: { ...model, active: { ...model.active, points: [...model.active.points, point] } },
  }
}

export function finalizeStroke(
  model: DrawingModel,
  warning?: ImageAnnotatorErrorCode,
): ModelResult {
  if (!model.active) return { model, warning }
  const history = pushUndo(model)
  return {
    model: {
      ...model,
      strokes: [...model.strokes, model.active],
      active: null,
      undo: history.undo,
      redo: [],
      revision: model.revision + 1,
      nextStrokeId: model.nextStrokeId + 1,
    },
    warning: warning ?? history.warning,
  }
}

export function undo(model: DrawingModel): DrawingModel {
  if (!model.undo.length || model.active) return model
  const previous = model.undo[model.undo.length - 1]
  return {
    ...model,
    strokes: copyStrokes(previous.strokes),
    undo: model.undo.slice(0, -1),
    redo: [snapshot(model), ...model.redo],
    revision: model.revision + 1,
  }
}

export function redo(model: DrawingModel): DrawingModel {
  if (!model.redo.length || model.active) return model
  const next = model.redo[0]
  const history = pushUndo(model)
  return {
    ...model,
    strokes: copyStrokes(next.strokes),
    undo: history.undo,
    redo: model.redo.slice(1),
    revision: model.revision + 1,
  }
}

export function clear(model: DrawingModel): DrawingModel {
  if (!model.strokes.length || model.active) return model
  const history = pushUndo(model)
  return { ...model, strokes: [], undo: history.undo, redo: [], revision: model.revision + 1 }
}
