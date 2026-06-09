import React, { useEffect, useState, useRef } from 'react'
import * as api from '../api'
import './DetailPanel.css'

export default function DetailPanel({ selected, desks, groups, deskStates, onClose }) {
  const [artifact, setArtifact] = useState(null)
  const [profile, setProfile] = useState(null)
  const [session, setSession] = useState(null)
  const [sessionTab, setSessionTab] = useState(false)

  useEffect(() => {
    setArtifact(null); setProfile(null); setSession(null); setSessionTab(false)
    if (!selected) return
    if (selected.type === 'desk') {
      api.fetchDeskArtifact(selected.id).then(setArtifact).catch(() => {})
      api.fetchDeskProfile(selected.id).then(setProfile).catch(() => {})
      api.fetchDeskSession(selected.id).then(setSession).catch(() => {})
    }
  }, [selected?.id, selected?.type])

  if (!selected) {
    return (
      <div className="detail-zone">
        <div className="dz-header"><span>Details</span></div>
        <div className="dz-body dz-empty">Select a node</div>
      </div>
    )
  }

  const id = selected.id
  if (selected.type === 'system') {
    const eventName = id.startsWith('sys:') ? id.slice(4) : id
    return (
      <div className="detail-zone">
        <div className="dz-header">
          <span>⚡ {eventName}</span>
          <button className="dz-close" onClick={onClose}>×</button>
        </div>
        <div className="dz-body">
          <Field label="Type" value="System Event" />
          <EmitSection events={[eventName]} sourceId="dashboard" />
        </div>
      </div>
    )
  }

  if (selected.type === 'group') {
    const g = groups[id] || {}
    return (
      <div className="detail-zone">
        <div className="dz-header">
          <span>⬡ {g.name || id}</span>
          <button className="dz-close" onClick={onClose}>×</button>
        </div>
        <div className="dz-body">
          {g.description && <Field label="Description" value={g.description} />}
          <Field label="Dispatch" value={g.dispatch || 'sequential'} />
          {g.lead && <Field label="Lead" value={`${g.lead.desk} (${g.lead.position || 'both'})`} />}
          {g.desks?.length > 0 && <Field label="Members" value={g.desks.join(', ')} />}
          <EmitSection emit={g.emit} subscribe={g.subscribe} sourceId={id} />
          {g.cron && <Field label="Cron" value={g.cron} />}
        </div>
      </div>
    )
  }

  const desk = desks[id] || {}
  const state = deskStates[id] || { status: 'idle' }
  const statusColors = { working: 'var(--cyan)', done: 'var(--green)', error: 'var(--red)', human: 'var(--amber)', idle: 'var(--text3)' }

  return (
    <div className="detail-zone">
      <div className="dz-header">
        <span>▸ {id}</span>
        <button className="dz-close" onClick={onClose}>×</button>
      </div>
      <div className="dz-body">
        <Field label="Status" value={
          <span style={{ color: statusColors[state.status], fontWeight: 600, textTransform: 'uppercase' }}>{state.status}</span>
        } />
        {desk.description && <Field label="Description" value={desk.description} />}
        <Field label="Executor" value={`${desk.executor?.type || '—'}${desk.executor?.params?.command ? ' — ' + desk.executor.params.command : ''}`} />
        {state.runID && <Field label="Run ID" value={<code style={{ fontSize: 10 }}>{state.runID}</code>} />}
        {state.durationMs && <Field label="Duration" value={fmtMs(state.durationMs)} />}
        {state.error && <Field label="Error" value={<span style={{ color: 'var(--red)' }}>{state.error}</span>} />}
        <EmitSection emit={desk.emit} subscribe={desk.subscribe} sourceId={id} />
        {profile && (
          <>
            <Field label="Runs" value={profile.total_runs} />
            <Field label="Success" value={`${(profile.success_rate * 100).toFixed(0)}%`} />
            {profile.estimated_cost > 0 && <Field label="Cost" value={`$${profile.estimated_cost.toFixed(4)}`} />}
          </>
        )}
        {artifact && <Field label="Artifact" value={<pre className="art-box">{artifact}</pre>} />}
        {session && session.length > 0 && (
          <div className="dz-field">
            <label style={{ cursor: 'pointer' }} onClick={() => setSessionTab(v => !v)}>
              Session ({session.length}) {sessionTab ? '▲' : '▼'}
            </label>
            {sessionTab && (
              <div className="session-entries">
                {session.map((e, i) => (
                  <div key={i} className={`session-entry session-${e.role}`}>
                    <span className="session-role">{e.role}</span>
                    <span className="session-at">{new Date(e.at).toLocaleTimeString()}</span>
                    <pre className="session-content">{e.content.length > 500 ? e.content.slice(0, 500) + '…' : e.content}</pre>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function EmitSection({ emit, subscribe, events: legacyEvents, sourceId }) {
  const [expanded, setExpanded] = useState(null)
  const [payload, setPayload] = useState('')
  const [firing, setFiring] = useState(null)
  const [done, setDone] = useState(null)

  // Build deduplicated event list with origin tags
  const allEvents = []
  const seen = new Set()
  for (const ev of (emit || [])) { allEvents.push({ name: ev, origin: 'emit' }); seen.add(ev) }
  for (const ev of (subscribe || [])) { if (!seen.has(ev)) { allEvents.push({ name: ev, origin: 'sub' }); seen.add(ev) } }
  for (const ev of (legacyEvents || [])) { if (!seen.has(ev)) { allEvents.push({ name: ev, origin: 'emit' }); seen.add(ev) } }

  if (allEvents.length === 0) return null

  const handleEmit = (ev) => {
    if (firing) return
    setFiring(ev)
    api.emitEvent(ev, expanded === ev ? payload : undefined, sourceId).then(() => {
      setDone(ev)
      setTimeout(() => { setFiring(null); setDone(null) }, 1500)
    }).catch(() => { setFiring(null) })
  }

  return (
    <div className="dz-field">
      <label>Events</label>
      <div className="emit-list">
        {allEvents.map(({ name, origin }) => (
          <div key={name} className="emit-item">
            <div className="emit-row">
              <button
                className={`emit-btn ${origin === 'sub' ? 'emit-btn-sub' : ''} ${done === name ? 'emit-done' : ''}`}
                onClick={() => handleEmit(name)}
                disabled={!!firing}
              >
                {firing === name ? '…' : done === name ? '✓' : origin === 'sub' ? '⇠' : '⇢'} {name}
              </button>
              <button
                className="emit-expand"
                onClick={() => { setExpanded(expanded === name ? null : name); setPayload('') }}
                title="Add payload"
              >
                {expanded === name ? '−' : '+'}
              </button>
            </div>
            {expanded === name && (
              <textarea
                className="emit-payload"
                placeholder="payload (optional)"
                value={payload}
                onChange={e => setPayload(e.target.value)}
                rows={2}
              />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function Field({ label, value }) {
  return (
    <div className="dz-field">
      <label>{label}</label>
      <div className="dz-val">{value}</div>
    </div>
  )
}

function fmtMs(ms) {
  const s = Math.floor(ms / 1000)
  return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + (s % 60) + 's'
}
