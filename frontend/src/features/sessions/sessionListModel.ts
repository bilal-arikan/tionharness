// A session header records the model from its last completed reply. While a new
// turn is running, that snapshot is necessarily stale; the bound agent's model
// is the request model for the in-flight turn. Once the turn ends, the persisted
// session model wins again because the provider may have resolved an alias to a
// more concrete model id.
export function sessionListModel(
  sessionModel: string | undefined,
  agentModel: string | undefined,
  running: boolean,
): string {
  return (running ? agentModel || sessionModel : sessionModel) ?? ''
}
