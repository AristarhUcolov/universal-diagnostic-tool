// API access. The token proves the page was served by this program; on
// other devices of the local network it comes from the #t=... link.
function readToken() {
  const meta = document.querySelector('meta[name="udt-token"]');
  let tok = meta ? meta.content : '';
  const m = location.hash.match(/[#&]t=([0-9a-f]+)/);
  if (m) {
    tok = m[1];
    try { sessionStorage.setItem('udt-token', tok); } catch (e) { /* private mode */ }
    history.replaceState(null, '', location.pathname + '#/connect');
  }
  if (!tok) {
    try { tok = sessionStorage.getItem('udt-token') || ''; } catch (e) { /* ignore */ }
  }
  return tok;
}

export const token = readToken();

export class APIError extends Error {
  constructor(msg, status, code) { super(msg); this.status = status; this.code = code || ''; }
}

async function call(method, path, body) {
  const opts = { method, headers: { 'X-UDT-Token': token } };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, opts);
  } catch (e) {
    throw new APIError('The program is not running (connection to the local server lost).', 0);
  }
  const ct = res.headers.get('Content-Type') || '';
  const data = ct.includes('application/json') ? await res.json() : await res.text();
  if (!res.ok) {
    throw new APIError((data && data.error) || String(data) || res.statusText, res.status, data && data.code);
  }
  return data;
}

export const api = {
  get: (p) => call('GET', p),
  post: (p, b) => call('POST', p, b === undefined ? {} : b),
};

// events opens the server-sent event stream and reconnects on failure.
export function events(handlers) {
  let es;
  const open = () => {
    es = new EventSource('/api/events?t=' + encodeURIComponent(token));
    for (const [type, fn] of Object.entries(handlers)) {
      if (type === 'onerror') continue;
      es.addEventListener(type, ev => {
        try { fn(JSON.parse(ev.data)); } catch (e) { console.error(e); }
      });
    }
    es.onerror = () => {
      if (handlers.onerror) handlers.onerror();
      es.close();
      setTimeout(open, 2000);
    };
  };
  open();
}
