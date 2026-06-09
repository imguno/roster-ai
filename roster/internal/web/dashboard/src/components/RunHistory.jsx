import React, { useState, useEffect } from 'react'
import * as api from '../api'

export default function RunHistory() {
  const [runs, setRuns] = useState([])
  const [selectedRun, setSelectedRun] = useState(null)
  const [steps, setSteps] = useState([])
  const [loading, setLoading] = useState(false)

  // Refresh run list every 10s so new completions appear without a page reload.
  useEffect(() => {
    const load = () => api.fetchRuns().then(r => setRuns(Array.isArray(r) ? r : [])).catch(() => {})
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [])

  // Poll step details every 3s while viewing an in-progress run.
  useEffect(() => {
    if (!selectedRun || selectedRun.status === 'completed' || selectedRun.status === 'failed') return
    const t = setInterval(async () => {
      try {
        const s = await api.fetchRunDetail(selectedRun.run_id)
        setSteps(Array.isArray(s) ? s : [])
      } catch {}
    }, 3000)
    return () => clearInterval(t)
  }, [selectedRun?.run_id, selectedRun?.status])

  async function openRun(run) {
    setSelectedRun(run)
    setLoading(true)
    try {
      const s = await api.fetchRunDetail(run.run_id)
      setSteps(Array.isArray(s) ? s : [])
    } catch {
      setSteps([])
    }
    setLoading(false)
  }

  const fmtMs = ms => { const s = Math.floor(ms / 1000); return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + (s % 60) + 's' }
  const fmtTime = t => { const d = new Date(t); return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) }


  if (selectedRun) {
    const dot = selectedRun.status === 'completed' ? 'var(--green)' : selectedRun.status === 'failed' ? 'var(--red)' : 'var(--cyan)'
    return (
      <div style={{ padding: 20, overflow: 'auto', height: '100%' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 16 }}>
          <button onClick={() => setSelectedRun(null)} style={{ background: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text2)', padding: '3px 10px', borderRadius: 4, cursor: 'pointer', fontSize: 12 }}>
            ← Back
          </button>
          <span style={{ width: 8, height: 8, borderRadius: '50%', background: dot, flexShrink: 0 }} />
          <code style={{ fontSize: 11, color: 'var(--text2)' }}>{selectedRun.run_id}</code>
          <span style={{ fontSize: 10, color: 'var(--text3)', marginLeft: 'auto' }}>
            {selectedRun.total_step_ms ? fmtMs(selectedRun.total_step_ms) : '—'}
            {(selectedRun.input_tokens || selectedRun.output_tokens) ? ` · ${selectedRun.input_tokens || 0}→${selectedRun.output_tokens || 0} tok` : ''}
          </span>
        </div>

        {loading && <p style={{ color: 'var(--text3)', textAlign: 'center', marginTop: 40 }}>Loading…</p>}
        {!loading && !steps.length && <p style={{ color: 'var(--text3)', textAlign: 'center', marginTop: 40 }}>No step data.</p>}
        {!loading && steps.map((step, i) => {
          const sc = step.status === 'completed' ? 'var(--green)' : step.status === 'failed' ? 'var(--red)' : 'var(--cyan)'
          return (
            <div key={i} style={{ borderLeft: `2px solid ${sc}`, padding: '8px 12px', marginBottom: 6, borderRadius: 3, background: 'var(--surface)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <span style={{ width: 7, height: 7, borderRadius: '50%', background: sc, flexShrink: 0 }} />
                <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text1)' }}>{step.desk_id}</span>
                <span style={{ fontSize: 10, color: 'var(--text3)', textTransform: 'uppercase', letterSpacing: 1 }}>{step.status}</span>
                {step.model && <span style={{ fontSize: 9, color: 'var(--text3)', marginLeft: 'auto' }}>{step.model}</span>}
              </div>
              <div style={{ display: 'flex', gap: 14, marginTop: 5, fontSize: 10, color: 'var(--text3)' }}>
                {step.started_at && <span>{fmtTime(step.started_at)}</span>}
                {step.duration_ms > 0 && <span>{fmtMs(step.duration_ms)}</span>}
                {(step.input_tokens || step.output_tokens) && (
                  <span>{step.input_tokens || 0}→{step.output_tokens || 0} tok</span>
                )}
              </div>
              {step.error && (
                <div style={{ marginTop: 6, fontSize: 11, color: 'var(--red)', fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                  {step.error}
                </div>
              )}
              {step.output && <StepOutput output={step.output} />}
            </div>
          )
        })}
      </div>
    )
  }

  return (
    <div style={{ padding: 20, overflow: 'auto', height: '100%' }}>
      <h2 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16 }}>Run History</h2>
      {!runs.length && <p style={{ color: 'var(--text3)', textAlign: 'center', marginTop: 40 }}>No runs recorded yet.</p>}
      {runs.map((r, i) => {
        const dot = r.status === 'completed' ? 'var(--green)' : r.status === 'failed' ? 'var(--red)' : 'var(--cyan)'
        const tokens = (r.input_tokens || r.output_tokens) ? `${r.input_tokens || 0}→${r.output_tokens || 0} tok` : ''
        return (
          <div key={i}
            style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '5px 8px', fontSize: 11, borderLeft: `2px solid ${dot}`, borderRadius: 3, marginBottom: 2, cursor: 'pointer' }}
            onClick={() => openRun(r)}
            onMouseOver={e => e.currentTarget.style.background = 'var(--surface)'}
            onMouseOut={e => e.currentTarget.style.background = ''}
          >
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: dot, flexShrink: 0 }} />
            <span style={{ color: 'var(--text2)', flexShrink: 0 }}>{r.group_id}</span>
            <code style={{ fontSize: 10, color: 'var(--text3)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={r.run_id}>
              {r.run_id.length > 28 ? r.run_id.slice(-28) : r.run_id}
            </code>
            <span style={{ fontSize: 10, color: 'var(--text3)', flexShrink: 0 }}>{r.desks?.length || 0} desks</span>
            <span style={{ fontSize: 10, color: 'var(--text2)', flexShrink: 0 }}>{r.total_step_ms ? fmtMs(r.total_step_ms) : '—'}</span>
            {tokens && <span style={{ fontSize: 9, color: 'var(--text3)', flexShrink: 0 }}>{tokens}</span>}
          </div>
        )
      })}
    </div>
  )
}

function StepOutput({ output }) {
  const [expanded, setExpanded] = React.useState(false)
  const preview = output.length > 300 ? output.slice(0, 300) + '…' : output
  return (
    <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text2)', fontFamily: 'monospace', background: 'var(--bg)', borderRadius: 3, padding: '6px 8px' }}>
      <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word', lineHeight: 1.5 }}>
        {expanded ? output : preview}
      </pre>
      {output.length > 300 && (
        <button
          onClick={() => setExpanded(e => !e)}
          style={{ marginTop: 4, background: 'none', border: 'none', color: 'var(--text3)', fontSize: 10, cursor: 'pointer', padding: 0 }}
        >
          {expanded ? '▲ collapse' : '▼ show all'}
        </button>
      )}
    </div>
  )
}
