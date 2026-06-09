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
  const [evOpen, setEvOpen] = useState(true)
  const evRef = useRef([])

  useEffect(() => {
    (async () => {
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

      api.connectStream((ev) => {
        evRef.current = [...evRef.current, ev]
        setEvents([...evRef.current])
        setDeskStates(prev => {
          const next = { ...prev }
          processEvent(ev, next, false)
          return next
        })
      })
    })()

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
    return () => clearInterval(qi)
  }, [])

  if (loading) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', color: 'var(--text2)', flexDirection: 'column', gap: 12 }}>
        <div style={{ fontSize: 20, fontWeight: 600, color: 'var(--text1)' }}>Roster</div>
        <div style={{ fontSize: 12, color: 'var(--text3)' }}>Connecting to hub…</div>
      </div>
    )
  }

  return (
    <>
      <TopBar org={org} desks={desks} groups={groups} events={events} view={view} setView={setView} totalCost={totalCost} />
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
    states[id] = { status: 'working', startedAt: new Date(ev.at).getTime(), runID: ev.run_id }
  } else if (t === 'step.completed') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'done', durationMs: ev.duration_ms }
  } else if (t === 'step.failed') {
    const prev = states[id] || {}
    states[id] = { ...prev, status: 'error', error: ev.error, durationMs: ev.duration_ms }
  } else if (t === 'step.skipped') {
    states[id] = { status: 'idle' }
  } else if (t === 'human.waiting') {
    states[id] = { status: 'human', startedAt: new Date(ev.at).getTime() }
  }
}
