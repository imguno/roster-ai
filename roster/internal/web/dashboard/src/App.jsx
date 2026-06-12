import React, { useState, useEffect, useCallback, useRef } from 'react'
import TopBar from './components/TopBar'
import GraphView from './components/GraphView'
import EventLog from './components/EventLog'
import RoutingTable from './components/RoutingTable'
import RunHistory from './components/RunHistory'
import ResourceView from './components/ResourceView'
import * as api from './api'
import './App.css'

export default function App() {
  const [org, setOrg] = useState(null)
  const [desks, setDesks] = useState({})
  const [groups, setGroups] = useState({})
  const [resources, setResources] = useState({})
  const [events, setEvents] = useState([])
  const [deskStates, setDeskStates] = useState({})
  const [selected, setSelected] = useState(null)
  const [view, setView] = useState('graph')
  const [queues, setQueues] = useState({})
  const [totalCost, setTotalCost] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(null)
  const [evOpen, setEvOpen] = useState(true)
  const [streamStatus, setStreamStatus] = useState('connecting')
  const evRef = useRef([])
  const streamRef = useRef(null)
  const evFlushRef = useRef(null)

  const loadData = useCallback(async () => {
    setLoading(true)
    setLoadError(null)
    try {
      const [o, d, g, r, ev] = await Promise.all([
        api.fetchOrg(), api.fetchDesks(), api.fetchGroups(),
        api.fetchResources(), api.fetchEvents(),
      ])
      setOrg(o); setDesks(d || {}); setGroups(g || {}); setResources(r || {})

      const states = {}
      const evList = Array.isArray(ev) ? ev : []
      for (const e of evList) processEvent(e, states, true)
      for (const id of Object.keys(states)) {
        if (states[id]?.status === 'done') states[id].status = 'idle'
      }
      setDeskStates({ ...states })
      evRef.current = evList
      setEvents([...evList])
      setLoading(false)
    } catch (err) {
      setLoadError(err.message || 'Failed to connect to hub')
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData().then(() => {
      streamRef.current = api.connectStream((ev) => {
        evRef.current.push(ev)
        if (evRef.current.length > 5000) evRef.current = evRef.current.slice(-4000)
        if (!evFlushRef.current) {
          evFlushRef.current = setTimeout(() => {
            evFlushRef.current = null
            setEvents([...evRef.current])
          }, 500)
        }
        setDeskStates(prev => {
          const next = { ...prev }
          processEvent(ev, next, false)
          return next
        })
        // Auto-transition done/error → idle after 3s
        const t = ev.type || ''
        const sid = ev.step_id || ''
        if (sid && (t === 'step.completed' || t === 'step.failed' || t === 'step.failed.continued')) {
          setTimeout(() => {
            setDeskStates(prev => {
              const cur = prev[sid]
              if (cur && (cur.status === 'done' || cur.status === 'error')) {
                return { ...prev, [sid]: { ...cur, status: 'idle' } }
              }
              return prev
            })
          }, 3000)
        }
      }, setStreamStatus)
    })

    const updateBudget = async () => {
      try {
        const b = await api.fetchBudget()
        const total = Object.entries(b || {})
          .filter(([k]) => k.startsWith('desk:'))
          .reduce((sum, [, v]) => sum + v, 0)
        setTotalCost(total)
      } catch {}
    }
    const qi = setInterval(async () => {
      try { setQueues(await api.fetchQueues()) } catch {}
      updateBudget()
    }, 5000)
    api.fetchQueues().then(setQueues).catch(() => {})
    updateBudget()
    return () => { clearInterval(qi); if (streamRef.current) streamRef.current.close(); if (evFlushRef.current) clearTimeout(evFlushRef.current) }
  }, [])

  if (loading || loadError) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', color: 'var(--text2)', flexDirection: 'column', gap: 12 }}>
        <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text1)' }}>Roster</div>
        {loadError ? (
          <>
            <div style={{ fontSize: 12, color: '#ef4444' }}>{loadError}</div>
            <button onClick={loadData} style={{ fontSize: 12, padding: '4px 16px', background: 'var(--bg3)', color: 'var(--text1)', border: '1px solid var(--border)', borderRadius: 4, cursor: 'pointer' }}>Retry</button>
          </>
        ) : (
          <div style={{ fontSize: 12, color: 'var(--text3)' }}>Connecting to hub…</div>
        )}
      </div>
    )
  }

  return (
    <>
      <TopBar org={org} desks={desks} groups={groups} events={events} view={view} setView={setView} totalCost={totalCost} />
      {streamStatus === 'disconnected' && (
        <div className="stream-banner">Stream disconnected — reconnecting…</div>
      )}
      <div className="workspace">
        <div className="main-area">
          {view === 'graph' && (
            <GraphView
              org={org} desks={desks} groups={groups} resources={resources}
              deskStates={deskStates} queues={queues}
              selected={selected} onSelect={setSelected}
              events={events}
            />
          )}
          {view === 'routing' && <RoutingTable org={org} desks={desks} groups={groups} deskStates={deskStates} />}
          {view === 'resources' && <ResourceView resources={resources} />}
          {view === 'runs' && <RunHistory />}
        </div>
        <EventLog events={events} open={evOpen} onToggle={() => setEvOpen(v => !v)} />
      </div>
    </>
  )
}

function processEvent(ev, states, silent) {
  const id = ev.step_id || ''
  const t = ev.type || ''
  if (t === 'step.started') {
    states[id] = { status: 'working', startedAt: new Date(ev.at).getTime(), runID: ev.run_id, input: ev.input || '' }
  } else if (t === 'step.completed') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'done', durationMs: ev.duration_ms }
  } else if (t === 'step.failed') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'error', error: ev.error, durationMs: ev.duration_ms }
  } else if (t === 'step.failed.continued') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'error', error: ev.error }
  } else if (t === 'step.skipped') {
    states[id] = { status: 'idle' }
  } else if (t === 'human.waiting') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'human', startedAt: new Date(ev.at).getTime() }
  } else if (t === 'step.log') {
    const prev = states[id] || {}
    const logs = prev.logs || []
    logs.push({ type: ev.log_type, content: ev.log_content, at: ev.at })
    states[id] = { ...prev, logs, sessionDirty: true }
  }
}
