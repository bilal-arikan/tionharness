// Per-tool icons. Every built-in tool gets its own lucide icon so it reads the
// same in the chat activity cards and on the Settings → Tools screen. lucide
// icons inherit currentColor, so they re-theme with the palette (unlike the
// emoji glyphs used previously). Keys are the lowercased base tool name (any MCP
// "server__tool" prefix stripped — see toolBase in ./tools).
import {
  // file system
  FileText, FilePlus2, FilePen, FolderTree, FolderSearch, TextSearch, SquareTerminal,
  // web
  Globe,
  // search / view
  MessagesSquare, ScanSearch, Search, Focus,
  // agents / sessions
  UserPlus, UserCog, UserMinus, Users, Bot, MessageSquarePlus, Send, ArrowLeftRight,
  Inbox, FolderCog, Pencil,
  // artifacts
  PackageOpen, PackagePlus, PackageCheck, PackageMinus, Package,
  // config
  FileCog, Save, Files,
  // ask / confirm / notify
  MessageCircleQuestion, ShieldQuestion, Bell,
  // debug / logs
  Bug, ScrollText,
  // flows
  Workflow, PencilRuler, ListTree, GitBranch, Play,
  // hooks
  Webhook,
  // mcp
  Plug, PlugZap, Power, Unplug,
  // schedule
  AlarmClock, CalendarPlus, CalendarCog, CalendarX, CalendarClock,
  // secrets
  KeyRound,
  // settings
  Settings, SlidersHorizontal,
  // skills
  Sparkles, WandSparkles, Wand2, Download,
  // tasks / todo
  ListTodo, ListPlus, SquarePen, Move, ListChecks,
  // wake
  AlarmClockPlus,
  // workspaces
  Boxes, FolderPlus, FolderPen, FolderX,
  // activate
  Zap, ZapOff,
  // generic
  Trash2, Puzzle, Wrench,
  type LucideIcon,
} from 'lucide-react'

// Exact per-tool map (base name, lowercase).
const TOOL_ICONS: Record<string, LucideIcon> = {
  // file system
  read: FileText,
  write: FilePlus2,
  edit: FilePen,
  ls: FolderTree,
  glob: FolderSearch,
  grep: TextSearch,
  bash: SquareTerminal,
  terminal: SquareTerminal,

  // web
  webfetch: Globe,
  http_get: Globe,
  http_request: Globe,

  // search / view
  conversation_search: MessagesSquare,
  skill_search: ScanSearch,
  tool_search: Search,
  focus_view: Focus,

  // agents
  create_agent: UserPlus,
  update_agent: UserCog,
  delete_agent: UserMinus,
  list_agents: Users,
  run_subagent: Bot,
  spawn_session: MessageSquarePlus,
  send_message: Send,
  handoff_session: ArrowLeftRight,

  // artifacts
  read_artifact: PackageOpen,
  create_artifact: PackagePlus,
  update_artifact: PackageCheck,
  delete_artifact: PackageMinus,
  list_artifacts: Package,

  // config
  read_config: FileCog,
  write_config: Save,
  list_config: Files,

  // ask / confirm / notify
  ask_user: MessageCircleQuestion,
  request_confirmation: ShieldQuestion,
  notify: Bell,

  // debug / logs
  read_session_debug: Bug,
  read_logs: ScrollText,

  // flows
  create_flow: Workflow,
  update_flow: PencilRuler,
  delete_flow: Trash2,
  list_flows: ListTree,
  get_flow: GitBranch,
  run_flow: Play,

  // hooks
  list_hooks: Webhook,
  create_hook: Webhook,
  delete_hook: Trash2,

  // mcp
  list_mcp_servers: Plug,
  create_mcp_server: PlugZap,
  toggle_mcp_server: Power,
  delete_mcp_server: Unplug,

  // schedule
  run_schedule: AlarmClock,
  create_schedule: CalendarPlus,
  update_schedule: CalendarCog,
  delete_schedule: CalendarX,
  list_schedules: CalendarClock,

  // secrets
  secret: KeyRound,

  // session edit
  update_session: Pencil,
  shell_manage: FolderCog,
  list_sessions: Inbox,

  // settings
  get_settings: Settings,
  update_settings: SlidersHorizontal,

  // skills
  use_skill: Sparkles,
  create_skill: WandSparkles,
  update_skill: Wand2,
  delete_skill: Trash2,
  import_skill: Download,

  // tasks
  list_tasks: ListTodo,
  create_task: ListPlus,
  update_task: SquarePen,
  move_task: Move,
  delete_task: Trash2,

  // todo
  todo_write: ListChecks,

  // wake
  schedule_wake: AlarmClockPlus,

  // workspaces
  list_workspaces: Boxes,
  create_workspace: FolderPlus,
  rename_workspace: FolderPen,
  delete_workspace: FolderX,

  // tool activation
  activate_tools: Zap,
  deactivate_tools: ZapOff,
}

// toolIcon resolves a (possibly namespaced) tool name to its lucide icon. Exact
// built-in match wins; MCP tools fall back to a puzzle piece; everything else to
// a generic wrench.
export function toolIcon(name: string): LucideIcon {
  const i = name.lastIndexOf('__')
  const base = (i >= 0 ? name.slice(i + 2) : name).toLowerCase()
  if (TOOL_ICONS[base]) return TOOL_ICONS[base]
  if (name.startsWith('mcp__') || name.includes('__')) return Puzzle
  return Wrench
}
