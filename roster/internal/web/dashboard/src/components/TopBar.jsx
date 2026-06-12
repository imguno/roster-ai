import React, { useState, useRef, useEffect } from 'react'
import * as api from '../api'
import './TopBar.css'

const tabs = [
  { id: 'graph', label: '◉ Graph' },
  { id: 'routing', label: '↔ Routing' },
  { id: 'resources', label: '◈ Resources' },
  { id: 'runs', label: '▤ Runs' },
]

function NewTaskModal({ desks, groups, onClose }) {
  const [desc, setDesc] = useState('')
  const [target, setTarget] = useState('org')
  const [targetId, setTargetId] = useState('')
  const [sending, setSending] = useState(false)
  const [sent, setSent] = useState(false)
  const modalRef = useRef(null)
  const inputRef = useRef(null)

  useEffect(() => { inputRef.current?.focus() }, [])
  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])
  useEffect(() => {
    const handler = (e) => { if (modalRef.current && !modalRef.current.contains(e.target)) onClose() }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  const deskIds = Object.keys(desks).sort()
  const groupIds = Object.keys(groups).sort()

  const [error, setError] = useState(null)

  const handleSubmit = async () => {
    if (!desc.trim() || sending || (target !== 'org' && !targetId)) return
    setSending(true)
    setError(null)
    let eventType = 'task.created'
    if (target === 'team' && targetId) eventType = `task.${targetId}`
    else if (target === 'desk' && targetId) eventType = `task.${targetId}`
    try {
      await api.emitEvent(eventType, desc.trim(), 'human')
      setSent(true)
      setTimeout(onClose, 800)
    } catch (err) {
      setError(err.message || 'Failed to send task')
      setSending(false)
    }
  }

  return (
    <div className="modal-overlay">
      <div ref={modalRef} className="new-task-modal">
        <div className="ntm-header">
          <span className="ntm-title">New Task</span>
          <button className="pop-close" onClick={onClose}>×</button>
        </div>
        <div className="ntm-body">
          <textarea
            ref={inputRef}
            className="ntm-input"
            placeholder="Describe the task…"
            value={desc} onChange={e => setDesc(e.target.value)}
            rows={3}
            onKeyDown={e => { if (e.key === 'Enter' && e.metaKey) handleSubmit() }}
          />
          <div className="ntm-target-row">
            <label className="ntm-label">Target</label>
            <select className="ntm-select" value={target} onChange={e => { setTarget(e.target.value); setTargetId('') }}>
              <option value="org">Org-wide</option>
              <option value="team">Team</option>
              <option value="desk">Desk</option>
            </select>
            {target === 'team' && (
              <select className="ntm-select" value={targetId} onChange={e => setTargetId(e.target.value)}>
                <option value="">Select team…</option>
                {groupIds.map(id => <option key={id} value={id}>{groups[id]?.name || id}</option>)}
              </select>
            )}
            {target === 'desk' && (
              <select className="ntm-select" value={targetId} onChange={e => setTargetId(e.target.value)}>
                <option value="">Select desk…</option>
                {deskIds.map(id => <option key={id} value={id}>{desks[id]?.name || id}</option>)}
              </select>
            )}
          </div>
          {error && <div className="ntm-error">{error}</div>}
          <button className={`ntm-submit ${sent ? 'ntm-sent' : ''}`} onClick={handleSubmit} disabled={!desc.trim() || sending || (target !== 'org' && !targetId)}>
            {sent ? '✓ Sent' : sending ? 'Sending…' : '→ Create Task'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function TopBar({ org, desks, groups, events, view, setView, totalCost }) {
  const [showNewTask, setShowNewTask] = useState(false)
  const active = Object.values(desks).length
  const groupCount = Object.keys(groups).length
  const evCount = events.length

  return (
    <div className="topbar">
      <div className="logo"><span className="dot" /> Roster</div>
      <span className="org-name">{org?.name || 'Loading…'}</span>
      <div className="nav-tabs">
        {tabs.map(t => (
          <button key={t.id}
            className={`nav-tab ${view === t.id ? 'active' : ''}`}
            onClick={() => setView(t.id)}
          >{t.label}</button>
        ))}
      </div>
      <div className="spacer" />
      <button className="new-task-btn" onClick={() => setShowNewTask(true)}>+ New Task</button>
      <div className="stat">Desks <span className="val">{active}</span></div>
      <div className="stat">Groups <span className="val">{groupCount}</span></div>
      <div className="stat">Events <span className="val">{evCount > 99 ? '99+' : evCount}</span></div>
      {totalCost > 0 && <div className="stat">Cost <span className="val">${totalCost.toFixed(4)}</span></div>}
      {showNewTask && <NewTaskModal desks={desks} groups={groups} onClose={() => setShowNewTask(false)} />}
    </div>
  )
}
