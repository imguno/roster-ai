import React, { useMemo, useCallback, useState, useEffect, useRef } from 'react'
import {
  ReactFlow, Background, Controls, MarkerType,
  Position, Handle, useNodesState, useEdgesState,
} from 'reactflow'
import dagre from 'dagre'
import * as api from '../api'
import 'reactflow/dist/style.css'
import './GraphView.css'

// ── Speech bubble — scrolls through artifact text ──
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
  const { label, status, lastRun, bubble } = data
  return (
    <div className={`desk-card ds-${status}`}>
      <Handle type="target" position={Position.Left} className="node-handle" />
      {status === 'working' && <SpeechBubble text={bubble || 'thinking …'} />}
      <div className="desk-header">
        <span className={`desk-dot ds-dot-${status}`} />
        <span className="desk-name">{label}</span>
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

// ── Draggable popover ──
function NodePopover({ node, position, desks, groups, deskStates, onClose }) {
  const [artifact, setArtifact] = useState(null)
  const [profile, setProfile] = useState(null)
  const [session, setSession] = useState(null)
  const [sessionOpen, setSessionOpen] = useState(false)
  const [pos, setPos] = useState(position)
  const [dragging, setDragging] = useState(false)
  const dragRef = useRef({ startX: 0, startY: 0, origX: 0, origY: 0 })
  const popRef = useRef(null)
  const kind = node?.data?.nodeKind
  const id = node?.id

  useEffect(() => { setPos(position) }, [position])

  useEffect(() => {
    setArtifact(null); setProfile(null); setSession(null); setSessionOpen(false)
    if (!id || kind !== 'desk') return
    api.fetchDeskArtifact(id).then(setArtifact).catch(() => {})
    api.fetchDeskProfile(id).then(setProfile).catch(() => {})
    api.fetchDeskSession(id).then(setSession).catch(() => {})
  }, [id, kind])

  // Close on outside click
  useEffect(() => {
    const handler = (e) => {
      if (popRef.current && !popRef.current.contains(e.target)) onClose()
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  // Drag logic
  const onDragStart = useCallback((e) => {
    if (e.target.closest('.pop-close')) return
    setDragging(true)
    dragRef.current = { startX: e.clientX, startY: e.clientY, origX: pos.x, origY: pos.y }
  }, [pos])

  useEffect(() => {
    if (!dragging) return
    const onMove = (e) => {
      const dx = e.clientX - dragRef.current.startX
      const dy = e.clientY - dragRef.current.startY
      setPos({ x: dragRef.current.origX + dx, y: dragRef.current.origY + dy })
    }
    const onUp = () => setDragging(false)
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp) }
  }, [dragging])

  if (!node) return null

  const state = deskStates[id] || { status: 'idle' }
  const deskName = node.data.label || id

  let emitList = [], subList = []
  if (kind === 'group') {
    const g = groups[id] || {}
    emitList = g.emit || []; subList = g.subscribe || []
  } else if (kind === 'desk') {
    const d = desks[id] || {}
    emitList = d.emit || []; subList = d.subscribe || []
  } else if (kind === 'system') {
    emitList = [id.startsWith('sys:') ? id.slice(4) : id]
  }

  const fmtMs = ms => { const s = Math.floor(ms / 1000); return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + (s % 60) + 's' }

  return (
    <div ref={popRef} className="node-popover" style={{ left: pos.x, top: pos.y }}>
      <div className="pop-header" onMouseDown={onDragStart}>
        <span className="pop-title">{deskName}</span>
        <span className="pop-kind">{kind}</span>
        <button className="pop-close" onClick={onClose}>×</button>
      </div>

      <div className="pop-body">
        {kind === 'desk' && (
          <>
            <div className="pop-row">
              <span className="pop-label">Status</span>
              <span className={`pop-status pop-st-${state.status}`}>{state.status}</span>
            </div>
            {desks[id]?.description && <div className="pop-row"><span className="pop-label">Desc</span><span>{desks[id].description}</span></div>}
            {state.runID && <div className="pop-row"><span className="pop-label">Run</span><code className="pop-code">{state.runID}</code></div>}
            {state.durationMs > 0 && <div className="pop-row"><span className="pop-label">Time</span><span>{fmtMs(state.durationMs)}</span></div>}
            {state.error && <div className="pop-row"><span className="pop-label">Error</span><span className="pop-error">{state.error}</span></div>}
            {profile && (
              <>
                <div className="pop-row"><span className="pop-label">Runs</span><span>{profile.total_runs}</span></div>
                <div className="pop-row"><span className="pop-label">Rate</span><span>{(profile.success_rate * 100).toFixed(0)}%</span></div>
                {profile.estimated_cost > 0 && <div className="pop-row"><span className="pop-label">Cost</span><span>${profile.estimated_cost.toFixed(4)}</span></div>}
              </>
            )}
            {artifact && (
              <div className="pop-section">
                <span className="pop-label">Result</span>
                <pre className="pop-artifact">{artifact}</pre>
              </div>
            )}
            {session && session.length > 0 && (
              <div className="pop-section">
                <button className="pop-toggle" onClick={() => setSessionOpen(v => !v)}>
                  Chat ({session.length}) {sessionOpen ? '▲' : '▼'}
                </button>
                {sessionOpen && (
                  <div className="pop-chat">
                    {session.map((e, i) => {
                      const isOut = e.role === 'assistant'
                      return (
                        <div key={i} className={`chat-msg ${isOut ? 'chat-msg-out' : 'chat-msg-in'}`}>
                          <div>
                            <span className="chat-sender">{isOut ? deskName : 'Task'}</span>
                            <span className="chat-time">{new Date(e.at).toLocaleTimeString()}</span>
                          </div>
                          <div className="chat-text">{e.content.length > 400 ? e.content.slice(0, 400) + '…' : e.content}</div>
                        </div>
                      )
                    })}
                  </div>
                )}
              </div>
            )}
          </>
        )}

        {kind === 'group' && (
          <>
            {groups[id]?.description && <div className="pop-row"><span className="pop-label">Desc</span><span>{groups[id].description}</span></div>}
            <div className="pop-row"><span className="pop-label">Mode</span><span>{groups[id]?.dispatch || 'sequential'}</span></div>
            {groups[id]?.lead && <div className="pop-row"><span className="pop-label">Lead</span><span>{groups[id].lead.desk}</span></div>}
            {groups[id]?.desks?.length > 0 && <div className="pop-row"><span className="pop-label">Desks</span><span>{groups[id].desks.join(', ')}</span></div>}
          </>
        )}

        {kind === 'system' && (
          <div className="pop-row"><span className="pop-label">Type</span><span>System Event</span></div>
        )}

        {(emitList.length > 0 || subList.length > 0) && (
          <EmitActions emit={emitList} subscribe={subList} sourceId={id} />
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

export default function GraphView({ org, desks, groups, resources, deskStates, queues, selected, onSelect, events }) {
  const [popover, setPopover] = useState(null)
  const [bubbles, setBubbles] = useState({})
  const draggedPositions = useRef({})

  // Poll for bubble text — show latest artifact or last assistant response
  useEffect(() => {
    const allDeskIds = Object.keys(desks)
    if (!allDeskIds.length) return

    const fetchBubbles = async () => {
      const next = {}
      for (const id of allDeskIds) {
        const st = deskStates[id]?.status
        if (st !== 'working' && st !== 'done' && st !== 'error') continue
        try {
          // Try artifact first
          const art = await api.fetchDeskArtifact(id)
          if (art) {
            next[id] = art
            continue
          }
          // Fall back to last assistant message
          const sess = await api.fetchDeskSession(id)
          if (sess?.length > 0) {
            const last = sess.find(e => e.role === 'assistant') || sess[0]
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

  const initial = useMemo(() => {
    if (!org) return { nodes: [], edges: [] }
    return buildGraph(org, desks, groups, resources || {}, deskStates, queues, events || [], bubbles)
  }, [org, desks, groups, resources, deskStates, queues, events, bubbles])

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

  const onPaneClick = useCallback(() => {
    setPopover(null)
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
          onPaneClick={onPaneClick}
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
            desks={desks} groups={groups} deskStates={deskStates}
            onClose={() => { setPopover(null); onSelect(null) }}
          />
        )}
      </div>
    </div>
  )
}

// ═══════════════════════════════════════════
function buildGraph(org, desks, groups, resources, deskStates, queues, events, bubbles) {
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

  const lastRunMap = {}
  for (const ev of events)
    if (ev.type === 'step.started' && ev.step_id) lastRunMap[ev.step_id] = ev.at

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

  // Groups
  for (const [id, g] of Object.entries(groups)) {
    const allDesks = Object.entries(desks).filter(([, d]) => d.parent === id).map(([did]) => did)
    const status = getGroupStatus(allDesks, deskStates)
    const qd = (queues[id] || 0) + allDesks.reduce((s, d) => s + (queues[d] || 0), 0)
    const deskCount = allDesks.length
    const cols = Math.min(deskCount, 3)
    const rows = Math.ceil(deskCount / cols)
    const roomW = Math.max(260, cols * 230 + 30)
    const roomH = Math.max(130, rows * 100 + 60)
    rawNodes.push({
      id, type: 'groupnode', width: roomW, height: roomH,
      data: {
        label: g.name || id, nodeKind: 'group',
        dispatch: g.dispatch || 'sequential', status, deskCount,
        subscribe: g.subscribe, emit: g.emit,
        queueDepth: qd, lastRun: fmtAgo(lastRunMap[id]), cron: g.cron,
      },
    })
  }

  // Desks
  for (const [id, d] of Object.entries(desks)) {
    const status = deskStates[id]?.status || 'idle'
    const gid = deskToGroup[id]
    rawNodes.push({
      id, type: 'desknode', width: 200, height: 56,
      data: {
        label: d.name || id, nodeKind: 'desk',
        status, subscribe: d.subscribe, emit: d.emit,
        lastRun: fmtAgo(lastRunMap[id]),
        bubble: bubbles[id] || null,
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

  // Direct emit→subscribe edges
  for (const [ev, emitters] of Object.entries(emitMap)) {
    for (const src of emitters) {
      for (const tgt of (subMap[ev] || [])) {
        if (src === tgt) continue
        const key = `${src}→${ev}→${tgt}`
        if (edgeSet.has(key)) continue
        edgeSet.add(key)
        rawEdges.push({ id: key, source: src, target: tgt, kind: 'subscribe', label: ev })
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

  // Dagre layout — top-level only
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 120, nodesep: 50, edgesep: 25, marginx: 40, marginy: 40 })

  const childDesks = new Set(Object.keys(deskToGroup).filter(d => deskToGroup[d]))

  for (const n of rawNodes) {
    if (childDesks.has(n.id)) continue
    g.setNode(n.id, { width: n.width, height: n.height })
  }
  for (const e of rawEdges) {
    if (e.kind === 'resource') continue
    if (childDesks.has(e.source) || childDesks.has(e.target)) continue
    g.setEdge(e.source, e.target)
  }
  dagre.layout(g)

  const nodes = []
  for (const n of rawNodes) {
    if (childDesks.has(n.id)) continue
    const pos = g.node(n.id)
    nodes.push({
      id: n.id, type: n.type,
      position: { x: pos.x - n.width / 2, y: pos.y - n.height / 2 },
      data: n.data,
      style: n.type === 'groupnode' ? { width: n.width, height: n.height } : undefined,
    })
  }

  // Child desks inside groups
  for (const n of rawNodes) {
    if (!childDesks.has(n.id)) continue
    const gid = deskToGroup[n.id]
    const allDesksInGroup = Object.keys(deskToGroup).filter(d => deskToGroup[d] === gid)
    const idx = allDesksInGroup.indexOf(n.id)
    const numCols = Math.max(1, Math.min(allDesksInGroup.length, 3))
    const col = idx % numCols
    const row = Math.floor(idx / numCols)
    nodes.push({
      id: n.id, type: n.type,
      position: { x: 14 + col * 218, y: 46 + row * 72 },
      parentNode: gid, extent: 'parent',
      data: n.data,
    })
  }

  // Style edges
  const styledEdges = rawEdges.map(e => {
    const active = deskStates[e.source]?.status === 'working' || deskStates[e.target]?.status === 'working'
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
