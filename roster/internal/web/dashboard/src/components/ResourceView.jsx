import React from 'react'

export default function ResourceView({ resources }) {
  const entries = Object.entries(resources || {})
  return (
    <div style={{ padding: 20, overflow: 'auto', height: '100%' }}>
      <h2 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16 }}>Resources</h2>
      {!entries.length && <p style={{ color: 'var(--text3)', fontSize: 12 }}>No resources configured.</p>}
      {entries.map(([id, r]) => (
        <div key={id} style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 4, padding: '10px 12px', marginBottom: 6 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ fontSize: 12, fontWeight: 600 }}>{id}</span>
            <span style={{ fontSize: 10, color: 'var(--text3)', background: 'var(--bg4)', padding: '1px 6px', borderRadius: 3 }}>{r.type || 'custom'}</span>
          </div>
          <div style={{ fontSize: 10, color: 'var(--text2)' }}>
            {r.watch?.length > 0 && <span>Watch: {r.watch.join(', ')}</span>}
            {r.actions && <span>{r.watch?.length > 0 ? ' | ' : ''}Actions: {Object.keys(r.actions).join(', ')}</span>}
          </div>
        </div>
      ))}
    </div>
  )
}
