// API client for the Roster hub.

const BASE = '';

async function jsonOrThrow(r) {
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
  return r.json();
}

export async function fetchOrg() {
  return jsonOrThrow(await fetch(`${BASE}/api/organization`));
}

export async function fetchDesks() {
  return jsonOrThrow(await fetch(`${BASE}/api/desks`));
}

export async function fetchGroups() {
  return jsonOrThrow(await fetch(`${BASE}/api/groups`));
}

export async function fetchResources() {
  return jsonOrThrow(await fetch(`${BASE}/api/resources`));
}

export async function fetchEvents() {
  return jsonOrThrow(await fetch(`${BASE}/api/events`));
}

export async function fetchQueues() {
  return jsonOrThrow(await fetch(`${BASE}/api/queues`));
}

export async function fetchRuns() {
  return jsonOrThrow(await fetch(`${BASE}/api/runs`));
}

export async function fetchRunDetail(runID) {
  return jsonOrThrow(await fetch(`${BASE}/api/runs/${encodeURIComponent(runID)}`));
}

export async function fetchDeskProfile(deskID) {
  return jsonOrThrow(await fetch(`${BASE}/api/desks/${deskID}/profile`));
}

export async function fetchDeskSession(deskID) {
  const r = await fetch(`${BASE}/api/desks/${deskID}/session`);
  if (!r.ok) return [];
  return r.json();
}

export async function fetchDeskLogs(deskID) {
  const r = await fetch(`${BASE}/api/desks/${deskID}/logs`);
  if (!r.ok) return [];
  return r.json();
}

export async function fetchBudget() {
  return jsonOrThrow(await fetch(`${BASE}/api/budget`));
}

export async function emitEvent(type, payload, source = 'dashboard') {
  const body = { type, source };
  if (payload) body.payload = btoa(unescape(encodeURIComponent(payload)));
  const r = await fetch(`${BASE}/api/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
}

export async function cancelRun(runID) {
  const r = await fetch(`${BASE}/api/runs/${encodeURIComponent(runID)}/cancel`, { method: 'POST' });
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
}

export async function submitHuman(deskID, content) {
  const r = await fetch(`${BASE}/api/human/${deskID}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
}

// SSE stream for real-time events with auto-reconnect.
export function connectStream(onEvent, onStatus) {
  let es = null;
  let delay = 1000;
  let stopped = false;

  function connect() {
    if (stopped) return;
    es = new EventSource(`${BASE}/api/stream`);
    es.onopen = () => {
      delay = 1000;
      if (onStatus) onStatus('connected');
    };
    es.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data);
        if (ev.type !== 'connected') onEvent(ev);
      } catch (_) {}
    };
    es.onerror = () => {
      es.close();
      if (onStatus) onStatus('disconnected');
      if (stopped) return;
      setTimeout(connect, delay);
      delay = Math.min(delay * 2, 30000);
    };
  }

  connect();

  return {
    close() {
      stopped = true;
      if (es) es.close();
    }
  };
}
