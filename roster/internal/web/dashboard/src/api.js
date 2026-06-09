// API client for the Roster hub.

const BASE = '';

export async function fetchOrg() {
  const r = await fetch(`${BASE}/api/organization`);
  return r.json();
}

export async function fetchDesks() {
  const r = await fetch(`${BASE}/api/desks`);
  return r.json();
}

export async function fetchGroups() {
  const r = await fetch(`${BASE}/api/groups`);
  return r.json();
}

export async function fetchResources() {
  const r = await fetch(`${BASE}/api/resources`);
  return r.json();
}

export async function fetchEvents() {
  const r = await fetch(`${BASE}/api/events`);
  return r.json();
}

export async function fetchQueues() {
  const r = await fetch(`${BASE}/api/queues`);
  return r.json();
}

export async function fetchRuns() {
  const r = await fetch(`${BASE}/api/runs`);
  return r.json();
}

export async function fetchRunDetail(runID) {
  const r = await fetch(`${BASE}/api/runs/${encodeURIComponent(runID)}`);
  return r.json();
}

export async function fetchDeskArtifact(deskID) {
  const r = await fetch(`${BASE}/api/desks/${deskID}/artifact`);
  if (r.status === 204) return null;
  return r.text();
}

export async function fetchDeskProfile(deskID) {
  const r = await fetch(`${BASE}/api/desks/${deskID}/profile`);
  return r.json();
}

export async function fetchDeskSession(deskID) {
  const r = await fetch(`${BASE}/api/desks/${deskID}/session`);
  if (!r.ok) return [];
  return r.json();
}

export async function fetchCrons() {
  const r = await fetch(`${BASE}/api/crons`);
  return r.json();
}

export async function fetchMetrics() {
  const r = await fetch(`${BASE}/api/metrics`);
  return r.json();
}

export async function fetchBudget() {
  const r = await fetch(`${BASE}/api/budget`);
  return r.json();
}

export async function emitEvent(type, payload, source = 'dashboard') {
  const body = { type, source };
  if (payload) body.payload = btoa(unescape(encodeURIComponent(payload)));
  await fetch(`${BASE}/api/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export async function cancelRun(runID) {
  await fetch(`${BASE}/api/runs/${encodeURIComponent(runID)}/cancel`, { method: 'POST' });
}

export async function submitHuman(deskID, content) {
  await fetch(`${BASE}/api/human/${deskID}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
}

// SSE stream for real-time events.
export function connectStream(onEvent) {
  const es = new EventSource(`${BASE}/api/stream`);
  es.onmessage = (e) => {
    try {
      const ev = JSON.parse(e.data);
      if (ev.type !== 'connected') onEvent(ev);
    } catch (_) {}
  };
  return es;
}
