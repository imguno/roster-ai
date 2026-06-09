import React from 'react'
import './EventLog.css'

export default function EventLog({ events, open, onToggle }) {
  const recent = events.slice(-100).reverse()
  return (
    <div className={`event-zone ${open ? '' : 'ez-collapsed'}`}>
      <div className="ez-header" onClick={onToggle}>
        <span>Events ({events.length})</span>
        <span className="ez-toggle">{open ? '▼' : '▲'}</span>
      </div>
      {open && (
        <div className="ez-list">
          {recent.map((ev, i) => {
            const t = ev.type || ''
            const cls = t.includes('completed') ? 'ev-ok' :
              t.includes('failed') ? 'ev-err' :
              t.includes('started') ? 'ev-run' : 'ev-other'
            return (
              <div key={i} className={`ev-row ${cls}`}>
                <span className="ev-time">{new Date(ev.at).toLocaleTimeString()}</span>
                <span className="ev-type">{t}</span>
                <span className="ev-step">{ev.step_id || ''}</span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
