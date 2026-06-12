import React, { useMemo, useCallback, useState, useEffect, useRef } from 'react'
import {
  ReactFlow, Background, Controls, MarkerType,
  Position, Handle, useNodesState, useEdgesState,
} from 'reactflow'
import dagre from 'dagre'
import * as api from '../api'
import 'reactflow/dist/style.css'
import './GraphView.css'

// ── Speech bubble — scrolls through output text ──
function SpeechBubble({ text }) {
  const [offset, setOffset] = useState(0)
  const len = 50

  useEffect(() => {
    if (!text || text.length <= len) return
    setOffset(0)
    const t = setInterval(() => {
      setOffset(prev => {
        const next = prev + len
        return next >= text.length ? 0 : next
      })
    }, 3000)
    return () => clearInterval(t)
  }, [text])

  if (!text) return null
  const display = text.length <= len ? text : text.slice(offset, offset + len) + (offset + len < text.length ? ' …' : '')

  return (
    <div className="speech-bubble">
      <div className="speech-text">{display}</div>
      <div className="speech-arrow" />
    </div>
  )
}

// ── Desk node ──
function DeskNode({ data }) {
  const { label, status, lastRun, bubble, retryCount } = data
  return (
    <div className={`desk-card ds-${status}`}>
      <Handle type="target" position={Position.Left} className="node-handle" />
      {(status === 'working' || status === 'done') && <SpeechBubble text={bubble || (status === 'working' ? 'thinking …' : null)} />}
      <div className="desk-header">
        <span className={`desk-dot ds-dot-${status}`} />
        <span className="desk-name">{label}</span>
        {retryCount > 0 && <span className="desk-retry" title="Retries from failure events">↻{retryCount}</span>}
        {status !== 'idle' && <span className={`desk-status dst-${status}`}>{status}</span>}
      </div>
      {lastRun && <div className="desk-meta">{lastRun}</div>}
      <Handle type="source" position={Position.Right} className="node-handle" />
    </div>
  )
}

// ── Group node ──
function GroupNode({ data }) {
  const { label, dispatch, status, deskCount, queueDepth, lastRun, cron } = data
  return (
    <div className={`group-room gr-${status}`}>
      <Handle type="target" position={Position.Left} className="node-handle" />
      <div className="group-header">
        <span className="group-name">{label}</span>
        {dispatch !== 'sequential' && <span className="group-dispatch">{dispatch}</span>}
        <span className="group-count">{deskCount}</span>
        {queueDepth > 0 && <span className="group-queue">Q:{queueDepth}</span>}
      </div>
      <div className="group-meta">
        {lastRun && <span>{lastRun}</span>}
        {cron && <span>{cron}</span>}
      </div>
      <Handle type="source" position={Position.Right} className="node-handle" />
    </div>
  )
}

// ── System event node ──
function SystemNode({ data }) {
  return (
    <div className="sys-node">
      <span className="sys-label">{data.label}</span>
      <Handle type="source" position={Position.Right} className="node-handle" />
    </div>
  )
}

// ── Resource node ──
function ResourceNode({ data }) {
  const { label, resType, actions } = data
  return (
    <div className="res-node">
      <Handle type="target" position={Position.Top} className="node-handle" />
      <div className="res-header">
        <span className="res-name">{label}</span>
        <span className="res-type">{resType}</span>
      </div>
      {actions.length > 0 && (
        <div className="res-actions">
          {actions.map(a => <span key={a} className="res-action">{a}</span>)}
        </div>
      )}
    </div>
  )
}

const nodeTypes = { groupnode: GroupNode, desknode: DeskNode, resnode: ResourceNode, sysnode: SystemNode }

// ── Status bar ──
function StatusBar({ deskStates, events }) {
  const working = Object.values(deskStates).filter(s => s.status === 'working').length
  const errors = Object.values(deskStates).filter(s => s.status === 'error').length
  const human = Object.values(deskStates).filter(s => s.status === 'human').length
  const total = Object.keys(deskStates).length
  const lastEv = events.length ? events[events.length - 1] : null
  return (
    <div className="status-bar">
      <span className="sb-item">{total - working - errors - human} idle</span>
      {working > 0 && <span className="sb-item sb-working">{working} working</span>}
      {errors > 0 && <span className="sb-item sb-error">{errors} error</span>}
      {human > 0 && <span className="sb-item sb-human">{human} waiting</span>}
      <span className="sb-spacer" />
      {lastEv && <span className="sb-last">{lastEv.type} · {lastEv.step_id || '—'}</span>}
    </div>
  )
}

// ── Draggable popover with tabs ──
function NodePopover({ node, position, desks, groups, resources, deskStates, events, onClose }) {
  const [tab, setTab] = useState('info')
  const [profile, setProfile] = useState(null)
  const [session, setSession] = useState(null)
  const [pos, setPos] = useState(position)
  const [dragging, setDragging] = useState(false)
  const dragRef = useRef({ startX: 0, startY: 0, origX: 0, origY: 0 })
  const popRef = useRef(null)
  const chatEndRef = useRef(null)
  const kind = node?.data?.nodeKind
  const id = node?.id

  useEffect(() => { setPos(position) }, [position])

  useEffect(() => {
    setProfile(null); setSession(null); setTab('info')
    if (!id || (kind !== 'desk' && kind !== 'group')) return
    if (kind === 'desk') {
      api.fetchDeskProfile(id).then(setProfile).catch(() => {})
    }
    // Merge session + logs
    Promise.all([
      api.fetchDeskSession(id).catch(() => []),
      api.fetchDeskLogs(id).catch(() => []),
    ]).then(([sess, logs]) => {
      const timeline = [
        ...(sess || []).map(e => ({ ...e, _kind: e.role === 'user' ? 'in' : 'out' })),
        ...(logs || []).map(e => ({ role: e.type, content: e.content, at: e.at, _kind: 'log' })),
      ].sort((a, b) => new Date(a.at) - new Date(b.at))
      setSession(timeline)
    })
  }, [id, kind])

  // Poll session+logs every 3s while desk is working
  const status = (deskStates[id] || {}).status
  useEffect(() => {
    if (!id || (kind !== 'desk' && kind !== 'group') || status !== 'working') return
    const poll = setInterval(() => {
      Promise.all([
        api.fetchDeskSession(id).catch(() => []),
        api.fetchDeskLogs(id).catch(() => []),
      ]).then(([sess, logs]) => {
        const timeline = [
          ...(sess || []).map(e => ({ ...e, _kind: e.role === 'user' ? 'in' : 'out' })),
          ...(logs || []).map(e => ({ role: e.type, content: e.content, at: e.at, _kind: 'log' })),
        ].sort((a, b) => new Date(a.at) - new Date(b.at))
        setSession(timeline)
      })
    }, 3000)
    return () => clearInterval(poll)
  }, [id, kind, status])

  // Auto-scroll chat
  useEffect(() => {
    if (tab === 'chat' && chatEndRef.current) chatEndRef.current.scrollIntoView({ behavior: 'smooth' })
  }, [tab, session?.length])

  // Close on outside click
  useEffect(() => {
    const handler = (e) => { if (popRef.current && !popRef.current.contains(e.target)) onClose() }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  // Drag logic
  const onDragStart = useCallback((e) => {
    if (e.target.closest('.pop-close') || e.target.closest('.pop-tab')) return
    setDragging(true)
    dragRef.current = { startX: e.clientX, startY: e.clientY, origX: pos.x, origY: pos.y }
  }, [pos])
  useEffect(() => {
    if (!dragging) return
    const onMove = (e) => {
      setPos({ x: dragRef.current.origX + (e.clientX - dragRef.current.startX), y: dragRef.current.origY + (e.clientY - dragRef.current.startY) })
    }
    const onUp = () => setDragging(false)
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp) }
  }, [dragging])

  if (!node) return null

  const state = deskStates[id] || { status: 'idle' }
  const deskName = node.data.label || id
  const desk = desks[id] || {}
  const group = groups[id] || {}
  const fmtMs = ms => { const s = Math.floor(ms / 1000); return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + (s % 60) + 's' }

  // Collect emit/subscribe
  let emitList = [], subList = []
  if (kind === 'group') { emitList = group.emit || []; subList = group.subscribe || [] }
  else if (kind === 'desk') { emitList = desk.emit || []; subList = desk.subscribe || [] }
  else if (kind === 'system') { emitList = [id.startsWith('sys:') ? id.slice(4) : id] }

  // Find member desks for a group
  const memberDesks = kind === 'group' ? Object.entries(desks).filter(([,d]) => d.parent === id).map(([did]) => did) : []

  // Find connected resources
  const connectedRes = kind === 'desk' ? (desk.resources || []) : kind === 'group' ? (group.resources || []) : []

  // Past runs for this scope
  const scopeEvents = (events || []).filter(e => e.step_id === id)
  const runMap = {}
  for (const ev of scopeEvents) {
    if (!ev.run_id) continue
    if (!runMap[ev.run_id]) runMap[ev.run_id] = { run_id: ev.run_id, status: 'running', events: [] }
    runMap[ev.run_id].events.push(ev)
    if (ev.type === 'step.completed') runMap[ev.run_id].status = 'completed'
    if (ev.type === 'step.failed') { runMap[ev.run_id].status = 'failed'; runMap[ev.run_id].error = ev.error }
    if (ev.duration_ms) runMap[ev.run_id].duration = ev.duration_ms
    if (!runMap[ev.run_id].at || ev.at < runMap[ev.run_id].at) runMap[ev.run_id].at = ev.at
  }
  const pastRuns = Object.values(runMap).sort((a, b) => (b.at || '') > (a.at || '') ? 1 : -1)

  const tabs = kind === 'system' ? ['info'] : ['info', 'chat', 'history']

  return (
    <div ref={popRef} className="node-popover" style={{ left: pos.x, top: pos.y }}>
      <div className="pop-header" onMouseDown={onDragStart}>
        <span className="pop-title">{deskName}</span>
        <span className={`pop-status-dot pop-st-${state.status}`} />
        <span className="pop-kind">{kind}</span>
        <button className="pop-close" onClick={onClose}>×</button>
      </div>

      {tabs.length > 1 && (
        <div className="pop-tabs">
          {tabs.map(t => (
            <button key={t} className={`pop-tab ${tab === t ? 'pop-tab-active' : ''}`} onClick={() => setTab(t)}>
              {t === 'info' ? 'Info' : t === 'chat' ? `Chat${session?.length ? ` (${session.length})` : ''}` : 'History'}
            </button>
          ))}
        </div>
      )}

      <div className="pop-body">
        {/* ── INFO TAB ── */}
        {tab === 'info' && kind === 'desk' && (
          <>
            <div className="pop-row"><span className="pop-label">Status</span><span className={`pop-status pop-st-${state.status}`}>{state.status}</span></div>
            <div className="pop-row"><span className="pop-label">Agent</span><span>{desk.agent?.id || '—'}</span></div>
            {desk.role && <div className="pop-row"><span className="pop-label">Role</span><span>{desk.role}</span></div>}
            {desk.goal && <div className="pop-row"><span className="pop-label">Goal</span><span>{desk.goal}</span></div>}
            {desk.parent && <div className="pop-row"><span className="pop-label">Group</span><span>{desk.parent}</span></div>}
            {desk.skills?.length > 0 && <div className="pop-row"><span className="pop-label">Skills</span><span>{desk.skills.join(', ')}</span></div>}
            {connectedRes.length > 0 && <div className="pop-row"><span className="pop-label">Resources</span><span>{connectedRes.join(', ')}</span></div>}
            {subList.length > 0 && <div className="pop-row"><span className="pop-label">Subscribe</span><span>{subList.join(', ')}</span></div>}
            {emitList.length > 0 && <div className="pop-row"><span className="pop-label">Emit</span><span className="pop-emit-text">{emitList.join(', ')}</span></div>}
            <div className="pop-row"><span className="pop-label">Executor</span><span>{desk.executor?.type || '—'}</span></div>
            {state.runID && <div className="pop-row"><span className="pop-label">Run</span><code className="pop-code">{state.runID}</code></div>}
            {state.durationMs > 0 && <div className="pop-row"><span className="pop-label">Time</span><span>{fmtMs(state.durationMs)}</span></div>}
            {state.error && <div className="pop-row"><span className="pop-label">Error</span><span className="pop-error">{state.error}</span></div>}
            {profile && (
              <>
                <div className="pop-row"><span className="pop-label">Total Runs</span><span>{profile.total_runs}</span></div>
                <div className="pop-row"><span className="pop-label">Success</span><span>{(profile.success_rate * 100).toFixed(0)}%</span></div>
                {profile.estimated_cost > 0 && <div className="pop-row"><span className="pop-label">Cost</span><span>${profile.estimated_cost.toFixed(4)}</span></div>}
              </>
            )}
            <EmitActions emit={emitList} subscribe={subList} sourceId={id} />
          </>
        )}

        {tab === 'info' && kind === 'group' && (
          <>
            <div className="pop-row"><span className="pop-label">Status</span><span className={`pop-status pop-st-${state.status}`}>{state.status}</span></div>
            {group.description && <div className="pop-row"><span className="pop-label">Desc</span><span>{group.description}</span></div>}
            <div className="pop-row"><span className="pop-label">Mode</span><span>{group.dispatch || 'sequential'}</span></div>
            {memberDesks.length > 0 && <div className="pop-row"><span className="pop-label">Desks</span><span>{memberDesks.join(', ')}</span></div>}
            {connectedRes.length > 0 && <div className="pop-row"><span className="pop-label">Resources</span><span>{connectedRes.join(', ')}</span></div>}
            {subList.length > 0 && <div className="pop-row"><span className="pop-label">Subscribe</span><span>{subList.join(', ')}</span></div>}
            {emitList.length > 0 && <div className="pop-row"><span className="pop-label">Emit</span><span className="pop-emit-text">{emitList.join(', ')}</span></div>}
            <EmitActions emit={emitList} subscribe={subList} sourceId={id} />
          </>
        )}

        {tab === 'info' && kind === 'system' && (
          <>
            <div className="pop-row"><span className="pop-label">Type</span><span>System Event</span></div>
            <EmitActions emit={emitList} subscribe={subList} sourceId={id} />
          </>
        )}

        {/* ── CHAT TAB ── */}
        {tab === 'chat' && (
          <div className="pop-chat">
            {(!session || session.length === 0) && <div className="pop-empty">No messages yet</div>}
            {session && session.map((e, i) => {
              const k = e._kind || (e.role === 'user' ? 'in' : 'out')
              const cls = k === 'log' ? 'chat-msg-log' : k === 'in' ? 'chat-msg-in' : 'chat-msg-out'
              const sender = k === 'log' ? (e.role === 'step' ? 'progress' : 'result')
                : k === 'in' ? 'Task' : deskName
              return (
                <div key={i} className={`chat-msg ${cls}`}>
                  <div><span className="chat-sender">{sender}</span><span className="chat-time">{new Date(e.at).toLocaleTimeString()}</span></div>
                  <div className="chat-text">{e.content.length > 400 ? e.content.slice(0, 400) + '...' : e.content}</div>
                </div>
              )
            })}
            <div ref={chatEndRef} />
          </div>
        )}

        {/* ── HISTORY TAB ── */}
        {tab === 'history' && (
          <div className="pop-history">
            {pastRuns.length === 0 && <div className="pop-empty">No runs yet</div>}
            {pastRuns.map((r, i) => {
              const dot = r.status === 'completed' ? 'var(--green)' : r.status === 'failed' ? 'var(--red)' : 'var(--cyan)'
              return (
                <div key={i} className="pop-run-row">
                  <span className="pop-run-dot" style={{ background: dot }} />
                  <span className="pop-run-time">{r.at ? new Date(r.at).toLocaleString() : '—'}</span>
                  <span className="pop-run-status">{r.status}</span>
                  {r.duration > 0 && <span className="pop-run-dur">{fmtMs(r.duration)}</span>}
                  {r.error && <div className="pop-run-error">{r.error}</div>}
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function EmitActions({ emit, subscribe, sourceId }) {
  const [expanded, setExpanded] = useState(null)
  const [payload, setPayload] = useState('')
  const [firing, setFiring] = useState(null)
  const [done, setDone] = useState(null)

  const allEvents = []
  const seen = new Set()
  for (const ev of (emit || [])) { allEvents.push({ name: ev, origin: 'emit' }); seen.add(ev) }
  for (const ev of (subscribe || [])) { if (!seen.has(ev)) { allEvents.push({ name: ev, origin: 'sub' }); seen.add(ev) } }
  if (!allEvents.length) return null

  const handleEmit = (ev) => {
    if (firing) return
    setFiring(ev)
    api.emitEvent(ev, expanded === ev ? payload : undefined, sourceId).then(() => {
      setDone(ev)
      setTimeout(() => { setFiring(null); setDone(null) }, 1500)
    }).catch(() => { setFiring(null) })
  }

  return (
    <div className="pop-section">
      <span className="pop-label">Events</span>
      <div className="emit-actions">
        {allEvents.map(({ name, origin }) => (
          <div key={name} className="emit-action-row">
            <button
              className={`emit-action-btn ${origin === 'sub' ? 'emit-action-sub' : ''} ${done === name ? 'emit-action-done' : ''}`}
              onClick={() => handleEmit(name)} disabled={!!firing}
            >
              {firing === name ? '…' : done === name ? '✓' : '→'} {name}
            </button>
            <button className="emit-action-expand" onClick={() => { setExpanded(expanded === name ? null : name); setPayload('') }}>
              {expanded === name ? '−' : '+'}
            </button>
            {expanded === name && (
              <textarea className="emit-action-payload" placeholder="payload" value={payload}
                onChange={e => setPayload(e.target.value)} rows={2} />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Edge popover — shows recent events of a given type ──
function EdgePopover({ edgeLabel, events, position, onClose }) {
  const popRef = useRef(null)
  useEffect(() => {
    const handler = (e) => { if (popRef.current && !popRef.current.contains(e.target)) onClose() }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  // Find events matching this edge label (event type pattern)
  const matching = (events || []).filter(ev => {
    const t = ev.type || ''
    return t === edgeLabel || t.endsWith('.' + edgeLabel)
  }).slice(-10).reverse()

  return (
    <div ref={popRef} className="edge-popover" style={{ left: position.x, top: position.y }}>
      <div className="edge-pop-header">
        <span className="edge-pop-label">{edgeLabel}</span>
        <button className="pop-close" onClick={onClose}>×</button>
      </div>
      <div className="edge-pop-body">
        {matching.length === 0 && <div className="pop-empty">No recent events</div>}
        {matching.map((ev, i) => (
          <div key={ev.at + ev.type + (ev.step_id || '') + i} className="edge-pop-row">
            <span className="edge-pop-time">{new Date(ev.at).toLocaleTimeString()}</span>
            <span className="edge-pop-type">{ev.type}</span>
            <span className="edge-pop-step">{ev.step_id || '—'}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function GraphView({ org, desks, groups, resources, deskStates, queues, selected, onSelect, events }) {
  const [popover, setPopover] = useState(null)
  const [edgePopover, setEdgePopover] = useState(null)
  const [bubbles, setBubbles] = useState({})
  const draggedPositions = useRef({})

  // Poll for bubble text — show last assistant response from session
  useEffect(() => {
    const allDeskIds = Object.keys(desks)
    if (!allDeskIds.length) return

    const fetchBubbles = async () => {
      const next = {}
      for (const id of allDeskIds) {
        const st = deskStates[id]?.status
        if (st !== 'working' && st !== 'done' && st !== 'error') continue
        try {
          const sess = await api.fetchDeskSession(id)
          if (sess?.length > 0) {
            const last = sess.findLast(e => e.role === 'assistant') || sess.findLast(e => e.role === 'result') || sess[sess.length - 1]
            next[id] = last.content || ''
          }
        } catch {}
      }
      setBubbles(next)
    }
    fetchBubbles()
    const t = setInterval(fetchBubbles, 5000)
    return () => clearInterval(t)
  }, [Object.keys(desks).join(','), Object.entries(deskStates).map(([k, v]) => k + v.status).join(',')])

  const prevEventDerived = useRef({ lastRunMap: {}, retryCountMap: {}, recentEventTypes: new Set() })
  const { lastRunMap, retryCountMap, recentEventTypes } = useMemo(() => {
    const lrm = {}, rcm = {}
    const recent = new Set()
    const now = Date.now()
    for (let i = (events || []).length - 1; i >= 0; i--) {
      const ev = events[i]
      if (ev.type === 'step.started' && ev.step_id) lrm[ev.step_id] = ev.at
      if ((ev.type || '').match(/\.(failed|rejected)$/) && ev.step_id)
        rcm[ev.step_id] = (rcm[ev.step_id] || 0) + 1
      if (ev.at && (now - new Date(ev.at).getTime()) < 10000) recent.add(ev.type)
    }
    // Return previous refs if data hasn't changed to keep downstream memos stable
    const prev = prevEventDerived.current
    const lrmKeys = Object.keys(lrm), prevLrmKeys = Object.keys(prev.lastRunMap)
    const rcmKeys = Object.keys(rcm), prevRcmKeys = Object.keys(prev.retryCountMap)
    const lrmSame = lrmKeys.length === prevLrmKeys.length && lrmKeys.every(k => lrm[k] === prev.lastRunMap[k])
    const rcmSame = rcmKeys.length === prevRcmKeys.length && rcmKeys.every(k => rcm[k] === prev.retryCountMap[k])
    const recentSame = recent.size === prev.recentEventTypes.size && [...recent].every(v => prev.recentEventTypes.has(v))
    if (lrmSame && rcmSame && recentSame) return prev
    const next = { lastRunMap: lrm, retryCountMap: rcm, recentEventTypes: recent }
    prevEventDerived.current = next
    return next
  }, [events])

  const initial = useMemo(() => {
    if (!org) return { nodes: [], edges: [] }
    return buildGraph(org, desks, groups, resources || {}, deskStates, queues, bubbles, lastRunMap, retryCountMap, recentEventTypes)
  }, [org, desks, groups, resources, deskStates, queues, bubbles, lastRunMap, retryCountMap, recentEventTypes])

  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges)

  useEffect(() => {
    setNodes(initial.nodes.map(n => ({
      ...n,
      position: draggedPositions.current[n.id] || n.position,
    })))
    setEdges(initial.edges)
  }, [initial])

  const onNodeDragStop = useCallback((_, node) => {
    draggedPositions.current[node.id] = node.position
  }, [])

  const onNodeClick = useCallback((event, node) => {
    onSelect({ type: node.data.nodeKind || 'desk', id: node.id })
    const bounds = event.currentTarget.closest('.react-flow')?.getBoundingClientRect()
    if (bounds) {
      let x = event.clientX - bounds.left + 20
      let y = event.clientY - bounds.top - 20
      // Prevent clipping: keep popover within bounds
      x = Math.min(x, bounds.width - 380)
      y = Math.min(y, bounds.height - 400)
      y = Math.max(y, 10)
      x = Math.max(x, 10)
      setPopover({ node, position: { x, y } })
    }
  }, [onSelect])

  const onEdgeClick = useCallback((event, edge) => {
    const bounds = event.currentTarget.closest('.react-flow')?.getBoundingClientRect()
    if (bounds && edge.label) {
      const x = Math.min(event.clientX - bounds.left + 10, bounds.width - 280)
      const y = Math.min(event.clientY - bounds.top - 10, bounds.height - 200)
      setEdgePopover({ label: edge.label, position: { x: Math.max(x, 10), y: Math.max(y, 10) } })
    }
  }, [])

  const onPaneClick = useCallback(() => {
    setPopover(null)
    setEdgePopover(null)
    onSelect(null)
  }, [onSelect])

  return (
    <div className="graph-wrapper">
      <StatusBar deskStates={deskStates} events={events} />
      <div className="graph-container">
        <ReactFlow
          nodes={nodes} edges={edges}
          onNodesChange={onNodesChange} onEdgesChange={onEdgesChange}
          nodeTypes={nodeTypes}
          onNodeClick={onNodeClick} onNodeDragStop={onNodeDragStop}
          onEdgeClick={onEdgeClick} onPaneClick={onPaneClick}
          fitView fitViewOptions={{ padding: 0.3 }}
          proOptions={{ hideAttribution: true }}
          minZoom={0.08} maxZoom={2.5}
          nodesDraggable nodesConnectable={false}
          defaultEdgeOptions={{ type: 'smoothstep' }}
          onlyRenderVisibleElements={false}
        >
          <Background color="#181c24" gap={28} size={1} />
          <Controls showInteractive={false} />
        </ReactFlow>
        {popover && (
          <NodePopover
            node={popover.node} position={popover.position}
            desks={desks} groups={groups} resources={resources} deskStates={deskStates} events={events}
            onClose={() => { setPopover(null); onSelect(null) }}
          />
        )}
        {edgePopover && (
          <EdgePopover
            edgeLabel={edgePopover.label} events={events}
            position={edgePopover.position}
            onClose={() => setEdgePopover(null)}
          />
        )}
      </div>
    </div>
  )
}

// ═══════════════════════════════════════════
function buildGraph(org, desks, groups, resources, deskStates, queues, bubbles, lastRunMap, retryCountMap, recentEventTypes) {
  const rawNodes = []
  const rawEdges = []
  const routing = org?.routing || []

  const emitMap = {}, subMap = {}
  for (const [id, g] of Object.entries(groups)) {
    for (const ev of (g.emit || [])) (emitMap[ev] = emitMap[ev] || []).push(id)
    for (const ev of (g.subscribe || [])) (subMap[ev] = subMap[ev] || []).push(id)
  }
  for (const [id, d] of Object.entries(desks)) {
    for (const ev of (d.emit || [])) (emitMap[ev] = emitMap[ev] || []).push(id)
    for (const ev of (d.subscribe || [])) (subMap[ev] = subMap[ev] || []).push(id)
  }

  const deskToGroup = {}
  for (const [id, d] of Object.entries(desks)) {
    if (d.parent && groups[d.parent]) deskToGroup[id] = d.parent
  }

  // System events
  const systemEvents = new Set(['hub.started'])
  for (const ev of Object.keys(subMap)) if (!emitMap[ev]) systemEvents.add(ev)
  for (const ev of systemEvents) {
    const subs = subMap[ev] || []
    if (!subs.length && ev !== 'hub.started') continue
    const sysId = 'sys:' + ev
    rawNodes.push({ id: sysId, type: 'sysnode', width: 130, height: 32, data: { label: ev, nodeKind: 'system' } })
    for (const sub of subs) rawEdges.push({ id: `${sysId}-${sub}`, source: sysId, target: sub, kind: 'system' })
  }

  // Build parent→children maps for groups
  const groupToSubGroups = {} // groupID → [subGroupIDs]
  const groupToDesks = {}     // groupID → [deskIDs]
  const groupParent = {}      // groupID → parentGroupID
  for (const [id, g] of Object.entries(groups)) {
    if (g.parent && groups[g.parent]) {
      groupParent[id] = g.parent;
      (groupToSubGroups[g.parent] = groupToSubGroups[g.parent] || []).push(id)
    }
  }
  for (const [id, d] of Object.entries(desks)) {
    if (d.parent && groups[d.parent]) {
      (groupToDesks[d.parent] = groupToDesks[d.parent] || []).push(id)
    }
  }

  // Bottom-up size calculation for groups
  const groupSize = {}
  const calcGroupSize = (gid) => {
    if (groupSize[gid]) return groupSize[gid]
    const childDesksHere = (groupToDesks[gid] || []).sort()
    const childGroups = (groupToSubGroups[gid] || []).sort()

    // Size of desks in this group
    const dCols = Math.min(Math.max(childDesksHere.length, 1), 3)
    const dRows = Math.ceil(childDesksHere.length / dCols)
    let contentW = dCols * 218
    let contentH = dRows * 72

    // Add sub-group sizes
    for (const sg of childGroups) {
      const sz = calcGroupSize(sg)
      contentW = Math.max(contentW, sz.w + 28)
      contentH += sz.h + 10
    }

    const w = Math.max(260, contentW + 28)
    const h = Math.max(100, contentH + 56)
    groupSize[gid] = { w, h }
    return { w, h }
  }
  for (const gid of Object.keys(groups)) calcGroupSize(gid)

  // Groups
  for (const [id, g] of Object.entries(groups)) {
    const allDesks = (groupToDesks[id] || [])
    const status = getGroupStatus(allDesks, deskStates)
    const sz = groupSize[id] || { w: 260, h: 100 }
    rawNodes.push({
      id, type: 'groupnode', width: sz.w, height: sz.h,
      data: {
        label: g.name || id, nodeKind: 'group',
        dispatch: g.dispatch || 'sequential', status, deskCount: allDesks.length,
        subscribe: g.subscribe, emit: g.emit,
        queueDepth: queues[id] || 0, lastRun: fmtAgo(lastRunMap[id]), cron: g.cron,
      },
    })
  }

  // Desks
  for (const [id, d] of Object.entries(desks)) {
    const status = deskStates[id]?.status || 'idle'
    rawNodes.push({
      id, type: 'desknode', width: 200, height: 56,
      data: {
        label: d.name || id, nodeKind: 'desk',
        status, subscribe: d.subscribe, emit: d.emit,
        lastRun: fmtAgo(lastRunMap[id]),
        retryCount: retryCountMap[id] || 0,
        bubble: deskStates[id]?.logs?.length
          ? deskStates[id].logs[deskStates[id].logs.length - 1].content
          : deskStates[id]?.input || bubbles[id] || null,
      },
    })
  }

  // Routing edges
  const edgeSet = new Set()
  for (const rule of routing) {
    for (const src of (emitMap[rule.on] || [])) {
      if (src === rule.to) continue
      const key = `${src}→${rule.on}→${rule.to}`
      if (edgeSet.has(key)) continue
      edgeSet.add(key)
      rawEdges.push({ id: key, source: src, target: rule.to, kind: 'routing', label: rule.on, when: rule.when })
    }
  }

  // Subscribe-based edges: parse "{sourceID}.done" patterns to link desks/groups
  const allIds = new Set([...Object.keys(desks), ...Object.keys(groups)])
  const allSubscribers = [...Object.entries(desks), ...Object.entries(groups)]
  for (const [tgt, entity] of allSubscribers) {
    for (const pattern of (entity.subscribe || [])) {
      // Extract source from patterns like "architect.done", "dev-team.architect.done", "dev-team.*"
      const parts = pattern.split('.')
      const srcId = parts[0]
      if (allIds.has(srcId) && srcId !== tgt) {
        const key = `${srcId}→${pattern}→${tgt}`
        if (edgeSet.has(key)) continue
        edgeSet.add(key)
        // Detect failure/rejection feedback loops
        const isFailure = pattern.includes('.failed') || pattern.includes('.rejected')
        rawEdges.push({ id: key, source: srcId, target: tgt, kind: isFailure ? 'failure' : 'subscribe', label: pattern })
      }
    }
  }
  // Also connect system events (task.created etc) to subscribers
  for (const [tgt, entity] of allSubscribers) {
    for (const pattern of (entity.subscribe || [])) {
      const sysId = 'sys:' + pattern
      if (rawNodes.some(n => n.id === sysId)) {
        const key = `${sysId}→${pattern}→${tgt}`
        if (edgeSet.has(key)) continue
        edgeSet.add(key)
        rawEdges.push({ id: key, source: sysId, target: tgt, kind: 'system' })
      }
    }
  }

  // Resources
  const resUsers = {}
  for (const [id, g] of Object.entries(groups))
    for (const r of (g.resources || [])) (resUsers[r] = resUsers[r] || []).push(id)
  for (const [id, d] of Object.entries(desks))
    for (const r of (d.resources || [])) (resUsers[r] = resUsers[r] || []).push(id)
  const orgRes = org?.resources || []
  for (const [id, r] of Object.entries(resources)) {
    rawNodes.push({
      id: 'res:' + id, type: 'resnode', width: 160, height: 44,
      data: { label: id, nodeKind: 'resource', resType: r.type || 'custom',
        actions: r.actions ? Object.keys(r.actions) : [], watch: r.watch || [] },
    })
    const users = [...(resUsers[id] || [])]
    if (orgRes.includes(id))
      for (const gid of Object.keys(groups)) if (!users.includes(gid)) users.push(gid)
    for (const uid of users)
      rawEdges.push({ id: `res:${id}-${uid}`, source: 'res:' + id, target: uid, kind: 'resource' })
  }

  // Determine what's a "child" (has a parent group or parent desk-group)
  const childOf = {} // id → parentId
  for (const [id, d] of Object.entries(desks)) {
    if (d.parent && groups[d.parent]) childOf[id] = d.parent
  }
  for (const [id, g] of Object.entries(groups)) {
    if (g.parent && groups[g.parent]) childOf[id] = g.parent
  }
  const isChild = (id) => !!childOf[id]

  // Find the top-level ancestor for edge mapping
  const topAncestor = (id) => {
    let cur = id
    while (childOf[cur]) cur = childOf[cur]
    return cur
  }

  // Dagre layout — top-level nodes only
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 100, nodesep: 40, edgesep: 20, marginx: 40, marginy: 40 })

  for (const n of rawNodes) {
    if (isChild(n.id)) continue
    g.setNode(n.id, { width: n.width, height: n.height })
  }
  for (const e of rawEdges) {
    if (e.kind === 'resource') continue
    const src = topAncestor(e.source)
    const tgt = topAncestor(e.target)
    if (!src || !tgt || src === tgt) continue
    if (!g.hasNode(src) || !g.hasNode(tgt)) continue
    g.setEdge(src, tgt)
  }
  dagre.layout(g)

  // Place top-level nodes
  const nodes = []
  for (const n of rawNodes) {
    if (isChild(n.id)) continue
    const pos = g.node(n.id)
    if (!pos) continue
    nodes.push({
      id: n.id, type: n.type,
      position: { x: pos.x - n.width / 2, y: pos.y - n.height / 2 },
      data: n.data,
      style: n.type === 'groupnode' ? { width: n.width, height: n.height } : undefined,
    })
  }

  // Place children (sub-groups and desks) inside their parent
  // Process in order: sub-groups first, then desks
  for (const n of rawNodes) {
    if (!isChild(n.id)) continue
    const pid = childOf[n.id]
    // Gather siblings of same type
    const isGroup = !!groups[n.id]
    const siblings = Object.keys(isGroup ? groups : desks)
      .filter(id => childOf[id] === pid && !!groups[id] === isGroup)
      .sort()
    const idx = siblings.indexOf(n.id)

    // Layout: desks in grid (3 cols), sub-groups stacked vertically
    let x, y
    if (isGroup) {
      // Sub-groups stack vertically below the header
      const deskRows = Math.ceil((groupToDesks[pid] || []).length / 3)
      x = 14
      y = 46 + deskRows * 72 + idx * ((groupSize[n.id]?.h || 100) + 10)
    } else {
      const numCols = Math.min(Math.max(siblings.length, 1), 3)
      const col = idx % numCols
      const row = Math.floor(idx / numCols)
      x = 14 + col * 218
      y = 46 + row * 72
    }

    nodes.push({
      id: n.id, type: n.type,
      position: { x, y },
      parentNode: pid, extent: 'parent',
      data: n.data,
      style: n.type === 'groupnode' ? { width: n.width, height: n.height } : undefined,
    })
  }

  // Style edges
  const styledEdges = rawEdges.map(e => {
    const nodeActive = deskStates[e.source]?.status === 'working' || deskStates[e.target]?.status === 'working'
    const edgeFired = e.label && recentEventTypes.has(e.label)
    const active = nodeActive || edgeFired
    if (e.kind === 'routing') {
      const color = active ? '#00ccff' : '#4a5a70'
      return {
        id: e.id, source: e.source, target: e.target, type: 'smoothstep',
        label: e.label + (e.when ? ` [${e.when}]` : ''),
        animated: active,
        style: { stroke: color, strokeWidth: active ? 2 : 1.2 },
        labelStyle: { fill: '#e4e8f0', fontSize: 9, fontWeight: 600, fontFamily: "'JetBrains Mono', monospace" },
        labelBgStyle: { fill: '#12151c', fillOpacity: 0.95 },
        labelBgPadding: [6, 3], labelBgBorderRadius: 4,
        markerEnd: { type: MarkerType.ArrowClosed, color, width: 12, height: 12 },
      }
    }
    if (e.kind === 'subscribe') {
      return {
        id: e.id, source: e.source, target: e.target, type: 'smoothstep',
        label: e.label,
        style: { stroke: '#3a4a60', strokeWidth: 1 },
        labelStyle: { fill: '#8a94a8', fontSize: 8, fontFamily: "'JetBrains Mono', monospace" },
        labelBgStyle: { fill: '#12151c', fillOpacity: 0.9 },
        labelBgPadding: [4, 2], labelBgBorderRadius: 3,
        markerEnd: { type: MarkerType.ArrowClosed, color: '#3a4a60', width: 10, height: 10 },
      }
    }
    if (e.kind === 'failure') {
      const color = active ? '#ff4466' : '#5a2030'
      return {
        id: e.id, source: e.source, target: e.target, type: 'smoothstep',
        label: e.label,
        animated: active,
        style: { stroke: color, strokeWidth: active ? 2 : 1.2, strokeDasharray: '6 3' },
        labelStyle: { fill: '#ff8899', fontSize: 9, fontWeight: 600, fontFamily: "'JetBrains Mono', monospace" },
        labelBgStyle: { fill: '#1e0c10', fillOpacity: 0.95 },
        labelBgPadding: [6, 3], labelBgBorderRadius: 4,
        markerEnd: { type: MarkerType.ArrowClosed, color, width: 12, height: 12 },
      }
    }
    if (e.kind === 'resource') {
      return {
        id: e.id, source: e.source, target: e.target, type: 'smoothstep',
        style: { stroke: '#332850', strokeWidth: 1, strokeDasharray: '4 3' },
        markerEnd: { type: MarkerType.ArrowClosed, color: '#332850', width: 8, height: 8 },
      }
    }
    // system
    return {
      id: e.id, source: e.source, target: e.target, type: 'smoothstep',
      style: { stroke: '#3a4a60', strokeWidth: 1, strokeDasharray: '5 3' },
      markerEnd: { type: MarkerType.ArrowClosed, color: '#3a4a60', width: 10, height: 10 },
    }
  })

  return { nodes, edges: styledEdges }
}

function getGroupStatus(deskIds, deskStates) {
  let st = 'idle'
  for (const did of deskIds) {
    const ds = deskStates[did]?.status || 'idle'
    if (ds === 'working') return 'working'
    if (ds === 'error') st = 'error'
  }
  return st
}

function fmtAgo(at) {
  if (!at) return null
  const sec = Math.round((Date.now() - new Date(at).getTime()) / 1000)
  if (sec < 60) return sec + 's ago'
  if (sec < 3600) return Math.floor(sec / 60) + 'm ago'
  return Math.floor(sec / 3600) + 'h ago'
}
