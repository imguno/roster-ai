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
            const isHuman = ev.source === 'human' || ev.source === 'dashboard'
            const cls = t.includes('completed') ? 'ev-ok' :
              t.includes('failed') ? 'ev-err' :
              t.includes('started') ? 'ev-run' :
              t === 'step.log' ? 'ev-log' :
              isHuman ? 'ev-human' : 'ev-other'
            const logPreview = t === 'step.log' && ev.log_content
              ? ` — ${ev.log_content.length > 60 ? ev.log_content.slice(0, 60) + '…' : ev.log_content}`
              : ''
            return (
              <div key={i} className={`ev-row ${cls}`}>
                <span className="ev-time">{new Date(ev.at).toLocaleTimeString()}</span>
                {isHuman && <span className="ev-human-icon" title="Human-originated">H</span>}
                <span className="ev-type">{t}{ev.log_type ? `:${ev.log_type}` : ''}</span>
                <span className="ev-step">{ev.step_id || ''}{logPreview}</span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
