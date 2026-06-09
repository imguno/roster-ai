import React from 'react'

export default function RoutingTable({ org, desks, groups, deskStates }) {
  const routing = org?.routing || []
  const emitMap = {}
  for (const [id, g] of Object.entries(groups)) for (const ev of (g.emit || [])) (emitMap[ev] = emitMap[ev] || []).push(id)
  for (const [id, d] of Object.entries(desks)) for (const ev of (d.emit || [])) (emitMap[ev] = emitMap[ev] || []).push(id)

  return (
    <div style={{ padding: 20, overflow: 'auto', height: '100%' }}>
      <h2 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16 }}>Event Routing</h2>
      <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
        <thead>
          <tr style={{ borderBottom: '1px solid var(--border)' }}>
            <th style={{ textAlign: 'left', padding: '6px 10px', color: 'var(--text3)', fontSize: 10 }}>EMITTERS</th>
            <th style={{ textAlign: 'left', padding: '6px 10px', color: 'var(--text3)', fontSize: 10 }}>EVENT</th>
            <th style={{ textAlign: 'left', padding: '6px 10px', color: 'var(--text3)', fontSize: 10 }}>TARGET</th>
            <th style={{ textAlign: 'left', padding: '6px 10px', color: 'var(--text3)', fontSize: 10 }}>WHEN</th>
          </tr>
        </thead>
        <tbody>
          {routing.map((r, i) => (
            <tr key={i} style={{ borderBottom: '1px solid var(--border)' }}>
              <td style={{ padding: '8px 10px' }}>
                {(emitMap[r.on] || []).map(s => <span key={s} style={{ background: 'var(--bg4)', border: '1px solid var(--border)', padding: '1px 6px', borderRadius: 3, fontSize: 10, marginRight: 3 }}>{s}</span>)}
                {!(emitMap[r.on]?.length) && <span style={{ color: 'var(--text3)', fontStyle: 'italic', fontSize: 10 }}>external</span>}
              </td>
              <td style={{ padding: '8px 10px' }}>
                <span style={{ color: 'var(--cyan)', fontWeight: 600, padding: '1px 5px', border: '1px solid rgba(0,212,255,0.2)', borderRadius: 3, background: 'rgba(0,212,255,0.05)' }}>{r.on}</span>
              </td>
              <td style={{ padding: '8px 10px' }}>
                <span style={{ fontWeight: 500 }}>{r.to}</span>
              </td>
              <td style={{ padding: '8px 10px', color: 'var(--text3)', fontSize: 10 }}>{r.when || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2 style={{ fontSize: 14, fontWeight: 600, margin: '24px 0 12px' }}>Groups</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 8 }}>
        {Object.entries(groups).map(([id, g]) => (
          <div key={id} style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 4, padding: '10px 12px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
              <span style={{ fontSize: 12, fontWeight: 600 }}>{g.name || id}</span>
              <span style={{ fontSize: 9, padding: '1px 5px', borderRadius: 3, background: 'var(--bg4)', color: 'var(--text3)', border: '1px solid var(--border)' }}>{g.dispatch || 'sequential'}</span>
            </div>
            {g.subscribe?.length > 0 && (
              <div style={{ display: 'flex', gap: 3, flexWrap: 'wrap', marginBottom: 4 }}>
                {g.subscribe.map(s => <span key={s} style={{ fontSize: 8, color: 'var(--amber)', background: 'rgba(255,184,0,0.06)', border: '1px solid rgba(255,184,0,0.2)', padding: '0 4px', borderRadius: 2 }}>⇠ {s}</span>)}
              </div>
            )}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 3, margin: '4px 0' }}>
              {g.lead && <span style={{ fontSize: 9, color: 'var(--cyan)', padding: '1px 5px', background: 'var(--bg4)', borderRadius: 2, border: '1px solid rgba(0,212,255,0.3)' }}>◆ {g.lead.desk}</span>}
              {(g.desks || []).map(d => <span key={d} style={{ fontSize: 9, color: 'var(--text2)', padding: '1px 5px', background: 'var(--bg4)', borderRadius: 2, border: '1px solid var(--border)' }}>{d}</span>)}
            </div>
            {g.emit?.length > 0 && (
              <div style={{ display: 'flex', gap: 3, flexWrap: 'wrap' }}>
                {g.emit.map(e => <span key={e} style={{ fontSize: 8, color: 'var(--green)', background: 'rgba(0,255,136,0.06)', border: '1px solid rgba(0,255,136,0.2)', padding: '0 4px', borderRadius: 2 }}>⇢ {e}</span>)}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}
