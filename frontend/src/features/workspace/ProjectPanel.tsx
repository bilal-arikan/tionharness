import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitBranch, FolderGit2, Check, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import type { GitInfo } from '@/types'
import { FolderPickerButton } from '@/shared/components/FolderPickerButton'
import { Button } from '@/shared/components'
import { Field, inputCls } from '@/features/settings/primitives'

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
  const { t } = useTranslation('workspace')
  const [info, setInfo] = useState<GitInfo | null>(null)
  // A chosen path is probed from the first paint, so it starts loading.
  const [loading, setLoading] = useState(() => path.trim() !== '')
  // Editable git settings (seeded from info when it loads).
  const [remote, setRemote] = useState('')
  const [userName, setUserName] = useState('')
  const [userEmail, setUserEmail] = useState('')
  const [saving, setSaving] = useState(false)

  // run probes a path and lands the result through callbacks only (so the
  // effect may call it); load is the button entry point that also re-arms the
  // spinner. A blank path clears the git state instead of probing.
  const run = useCallback(
    (p: string) =>
      api
        .gitInfo(p)
        .then((g) => {
          setInfo(g)
          setRemote(g.remote)
          setUserName(g.userName)
          setUserEmail(g.userEmail)
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setLoading(false)),
    [onError],
  )
  const load = useCallback(
    (p: string) => {
      if (!p.trim()) {
        setInfo(null)
        return
      }
      setLoading(true)
      void run(p)
    },
    [run],
  )

  useKeyedReset(path, () => {
    if (path.trim()) setLoading(true)
    else setInfo(null)
  })
  useEffect(() => {
    if (!path.trim()) return
    void run(path)
  }, [path, run])

  // createDir: only set from the "folder does not exist" branch, so a typo in an
  // otherwise valid path cannot silently create a stray directory.
  const doInit = async (createDir = false) => {
    setSaving(true)
    try {
      const g = await api.gitInit(path, createDir)
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
      <Field label={t('project.directory.label')} hint={t('project.directory.hint')}>
        <div className="flex items-center gap-2">
          <input
            value={path}
            onChange={(e) => onSelectPath(e.target.value)}
            placeholder={t('project.directory.placeholder')}
            className={`${inputCls} flex-1`}
          />
          <FolderPickerButton
            startPath={path || ''}
            onSelect={onSelectPath}
            title={t('project.directory.select')}
          />
        </div>
      </Field>

      {/* Git section — only meaningful once a path is set */}
      <div className="border-t border-[var(--color-border)] pt-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <FolderGit2 size={15} className="text-[var(--color-accent)]" /> {t('project.gitTitle')}
          </h3>
          {path && (
            <button
              onClick={() => load(path)}
              disabled={loading}
              title={t('actions.refresh')}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40"
            >
              <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />{' '}
              {t('actions.refresh')}
            </button>
          )}
        </div>

        {!path ? (
          <p className="text-xs text-[var(--color-text-dim)]">{t('project.selectFirst')}</p>
        ) : loading && !info ? (
          <p className="text-xs text-[var(--color-text-dim)]">{t('actions.loading')}</p>
        ) : info && !info.gitInstalled ? (
          // Checked before the path branches: no git binary means no git action on
          // ANY path, so offering `git init` here could only fail. Say the real cause.
          <p className="text-xs text-[var(--color-danger)]">{t('project.gitMissing')}</p>
        ) : info && !info.exists ? (
          <div className="space-y-3">
            <p className="text-xs text-[var(--color-danger)]">{t('project.directoryMissing')}</p>
            <Button onClick={() => doInit(true)} disabled={saving} size="lg">
              <GitBranch size={14} /> {t('project.createAndInit')}
            </Button>
          </div>
        ) : info && !info.isGitRepo ? (
          <div className="space-y-3">
            <p className="text-xs text-[var(--color-text-dim)]">{t('project.notRepository')}</p>
            <Button onClick={() => doInit()} disabled={saving} size="lg">
              <GitBranch size={14} /> {t('project.init')}
            </Button>
          </div>
        ) : info && info.isGitRepo ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2 text-sm">
              <span className="flex items-center gap-1 rounded-md bg-[var(--color-surface-2)] px-2 py-1 text-xs">
                <GitBranch size={12} /> {info.branch || t('project.noBranch')}
              </span>
              <span className="text-xs text-[var(--color-text-dim)]">
                {t('project.repository')}
              </span>
            </div>

            <Field label={t('project.remote.label')} hint={t('project.remote.hint')}>
              <input
                value={remote}
                onChange={(e) => setRemote(e.target.value)}
                placeholder="https://github.com/user/repo.git"
                className={inputCls}
              />
            </Field>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="user.name" hint={t('project.userNameHint')}>
                <input
                  value={userName}
                  onChange={(e) => setUserName(e.target.value)}
                  className={inputCls}
                />
              </Field>
              <Field label="user.email" hint={t('project.userEmailHint')}>
                <input
                  value={userEmail}
                  onChange={(e) => setUserEmail(e.target.value)}
                  className={inputCls}
                />
              </Field>
            </div>
            <Button onClick={saveGit} disabled={saving} size="lg">
              <Check size={14} /> {t('project.saveGit')}
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  )
}
