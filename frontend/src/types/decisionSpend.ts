export interface DecisionSpendTotals {
  calls: number
  failures: number
  inputTokens: number
  outputTokens: number
  unknownCostCalls: number
  primaryCalls: number
  challengerCalls: number
  testCalls: number
  costUSD: number
  reportedCostUSD: number
  estimatedCostUSD: number
}

export interface DecisionSpendModel extends DecisionSpendTotals {
  provider: string
  model: string
}

export interface DecisionSpendReport {
  days: number
  startedAt: number
  updatedAt: number
  storageError: boolean
  totals: DecisionSpendTotals
  today: DecisionSpendTotals
  models: DecisionSpendModel[]
}
