export interface ScopeItem {
  title: string
  body: string
}

/**
 * The "read this before you deploy it" section. TionHarness ships with no auth
 * and a wildcard CORS policy on purpose -- that is a local-first design choice,
 * not an oversight, and the site has to say so plainly.
 */
export const notThis: ScopeItem[] = [
  {
    title: 'Not a hosted SaaS',
    body: 'There is no account, no tenant, no billing. You run the binary; your data never leaves the machine it runs on.',
  },
  {
    title: 'No authentication layer',
    body: 'The HTTP API has no auth and CORS is wide open. Bind it to loopback or a Tailscale address. Never expose it to the public internet.',
  },
  {
    title: 'Not a sandbox',
    body: 'File and shell tools reach the whole filesystem by design. The permission mode -- auto, ask, read-only -- is the only guardrail, so pick it deliberately.',
  },
  {
    title: 'Not a model provider',
    body: 'It orchestrates models you already have access to. Bring your own claude-cli login or an API key for one of the supported providers.',
  },
]

export const securityNote =
  'TionHarness is built to run beside you, on hardware you control. That is why it trusts its caller completely -- and why the caller has to be you.'
