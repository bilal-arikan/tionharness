import { useCallback, useEffect, useState } from 'react'
import { GitBranch, FolderGit2, Check, RefreshCw } from 'lucide-react'
import { api } from '../../api'
import type { GitInfo } from '../../types'
import { FolderPickerButton } from '../common/FolderPickerButton'
import { Field, inputCls } from '../settings/primitives'

interface Props {
  // Current project path (= the workspace's default working dir).
  path: string
  // Pick/clear the project path (persisted by the parent).
  onSelectPath: (p: string) => void
  onError: (msg: string) => void
}

// ProjectPanel is the Workspace window's "Proje" sub-page: it sets the workspace
// project path and, once a path is chosen, surfaces its git state — offering
// `git init` when it is not yet a repo, and repo-local settings (origin remote,
// commit identity) when it is.
export function ProjectPanel({ path, onSelectPath, onError }: Props) {
  const [info, setInfo] = useState<GitInfo | null>(null)
  const [loading, setLoading] = useState(false)
  // Editable git settings (seeded from info when it loads).
  const [remote, setRemote] = useState('')
  const [userName, setUserName] = useState('')
  const [userEmail, setUserEmail] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(
    (p: string) => {
      if (!p.trim()) {
        setInfo(null)
        return
      }
      setLoading(true)
      api
        .gitInfo(p)
        .then((g) => {
          setInfo(g)
          setRemote(g.remote)
          setUserName(g.userName)
          setUserEmail(g.userEmail)
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setLoading(false))
    },
    [onError],
  )

  useEffect(() => {
    load(path)
  }, [path, load])

  const doInit = async () => {
    setSaving(true)
    try {
      const g = await api.gitInit(path)
      setInfo(g)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const saveGit = async () => {
    setSaving(true)
    try {
      const g = await api.gitConfig(path, { remote, userName, userEmail })
      setInfo(g)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-5">
      {/* Project path */}
      <Field
        label="Proje dizini (path)"
        hint="Bu workspace'in proje klasörü. Yeni sohbetler bu dizinden başlar (oturum bazında değiştirilebilir). Mutlak yol."
      >
        <div className="flex items-center gap-2">
          <input
            value={path}
            onChange={(e) => onSelectPath(e.target.value)}
            placeholder="(workspace dizini)"
            className={`${inputCls} flex-1`}
          />
          <FolderPickerButton
            startPath={path || ''}
            onSelect={onSelectPath}
            title="Proje dizinini seç"
          />
        </div>
      </Field>

      {/* Git section — only meaningful once a path is set */}
      <div className="border-t border-[var(--color-border)] pt-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <FolderGit2 size={15} className="text-[var(--color-accent)]" /> Git
          </h3>
          {path && (
            <button
              onClick={() => load(path)}
              disabled={loading}
              title="Yenile"
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40"
            >
              <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Yenile
            </button>
          )}
        </div>

        {!path ? (
          <p className="text-xs text-[var(--color-text-dim)]">Önce bir proje dizini seç.</p>
        ) : loading && !info ? (
          <p className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</p>
        ) : info && !info.exists ? (
          <p className="text-xs text-[var(--color-danger)]">Dizin bulunamadı.</p>
        ) : info && !info.isGitRepo ? (
          <div className="space-y-3">
            <p className="text-xs text-[var(--color-text-dim)]">
              Bu dizin bir git deposu değil. Sürüm kontrolü için başlat:
            </p>
            <button
              onClick={doInit}
              disabled={saving}
              className="flex items-center gap-1.5 rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-40"
            >
              <GitBranch size={14} /> git init (main)
            </button>
          </div>
        ) : info && info.isGitRepo ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm">
              <span className="flex items-center gap-1 rounded-md bg-[var(--color-surface-2)] px-2 py-1 text-xs">
                <GitBranch size={12} /> {info.branch || '(dal yok)'}
              </span>
              <span className="text-xs text-[var(--color-text-dim)]">Git deposu</span>
            </div>

            <Field label="Origin remote URL" hint="Uzak depo adresi (push/pull için).">
              <input
                value={remote}
                onChange={(e) => setRemote(e.target.value)}
                placeholder="https://github.com/kullanici/repo.git"
                className={inputCls}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="user.name" hint="Bu repodaki commit yazarı adı.">
                <input value={userName} onChange={(e) => setUserName(e.target.value)} className={inputCls} />
              </Field>
              <Field label="user.email" hint="Bu repodaki commit yazarı e-postası.">
                <input value={userEmail} onChange={(e) => setUserEmail(e.target.value)} className={inputCls} />
              </Field>
            </div>
            <button
              onClick={saveGit}
              disabled={saving}
              className="flex items-center gap-1.5 rounded-lg bg-[var(--color-accent)] px-3 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-40"
            >
              <Check size={14} /> Git ayarlarını kaydet
            </button>
          </div>
        ) : null}
      </div>
    </div>
  )
}
