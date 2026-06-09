import React from 'react'
import './TopBar.css'

const tabs = [
  { id: 'graph', label: '◉ Graph' },
  { id: 'routing', label: '↔ Routing' },
  { id: 'resources', label: '◈ Resources' },
  { id: 'runs', label: '▤ Runs' },
]

export default function TopBar({ org, desks, groups, events, view, setView, totalCost }) {
  const active = Object.values(desks).length
  const groupCount = Object.keys(groups).length
  const evCount = events.length

  return (
    <div className="topbar">
      <div className="logo"><span className="dot" /> Roster</div>
      <span className="org-name">{org?.name || 'Loading…'}</span>
      <div className="nav-tabs">
        {tabs.map(t => (
          <button key={t.id}
            className={`nav-tab ${view === t.id ? 'active' : ''}`}
            onClick={() => setView(t.id)}
          >{t.label}</button>
        ))}
      </div>
      <div className="spacer" />
      <div className="stat">Desks <span className="val">{active}</span></div>
      <div className="stat">Groups <span className="val">{groupCount}</span></div>
      <div className="stat">Events <span className="val">{evCount > 99 ? '99+' : evCount}</span></div>
      {totalCost > 0 && <div className="stat">Cost <span className="val">${totalCost.toFixed(4)}</span></div>}
    </div>
  )
}
