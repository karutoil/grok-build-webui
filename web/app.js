// Grok Build WebUI — workspace SPA
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];

// ---- UI themes --------------------------------------------------------------
// The WebUI follows the Grok CLI theme (`ui.theme` in config.toml). Each entry
// pairs an xterm palette with a data-theme slug styled in style.css.
const THEMES = {
  groknight: {
    label: 'GrokNight',
    term: {
      background: '#0b0b0e',
      foreground: '#e8e8ea',
      cursor: '#e05fd0',
      cursorAccent: '#0b0b0e',
      selectionBackground: 'rgba(224,95,208,0.30)',
      black: '#18181c',
      red: '#f07178',
      green: '#7fd99a',
      yellow: '#e8b931',
      blue: '#7aa2f7',
      magenta: '#e05fd0',
      cyan: '#89ddff',
      white: '#eeeff2',
      brightBlack: '#5c5c64',
      brightRed: '#ff8b90',
      brightGreen: '#9be7b3',
      brightYellow: '#f5d76e',
      brightBlue: '#9bb8ff',
      brightMagenta: '#ef87e2',
      brightCyan: '#b3ecff',
      brightWhite: '#ffffff',
    },
  },
  grokday: {
    label: 'GrokDay',
    term: {
      background: '#f8f8f9',
      foreground: '#33333b',
      cursor: '#96218a',
      cursorAccent: '#f8f8f9',
      selectionBackground: 'rgba(179,43,165,0.24)',
      black: '#3b3b44',
      red: '#c12839',
      green: '#12703a',
      yellow: '#94670f',
      blue: '#2c5cc5',
      magenta: '#a4259a',
      cyan: '#1d6e80',
      white: '#d5d5db',
      brightBlack: '#77777f',
      brightRed: '#d33948',
      brightGreen: '#23884b',
      brightYellow: '#b0821a',
      brightBlue: '#4070da',
      brightMagenta: '#b83cab',
      brightCyan: '#268296',
      brightWhite: '#ffffff',
    },
  },
  tokyonight: {
    label: 'TokyoNight',
    term: {
      background: '#1a1b26',
      foreground: '#c0caf5',
      cursor: '#7aa2f7',
      cursorAccent: '#1a1b26',
      selectionBackground: 'rgba(122,162,247,0.30)',
      black: '#15161e',
      red: '#f7768e',
      green: '#9ece6a',
      yellow: '#e0af68',
      blue: '#7aa2f7',
      magenta: '#bb9af7',
      cyan: '#7dcfff',
      white: '#a9b1d6',
      brightBlack: '#414868',
      brightRed: '#ff7a93',
      brightGreen: '#b9f27c',
      brightYellow: '#ff9e64',
      brightBlue: '#7da6ff',
      brightMagenta: '#c9a9fc',
      brightCyan: '#0db9d7',
      brightWhite: '#c0caf5',
    },
  },
  rosepine: {
    label: 'Rosé Pine Moon',
    term: {
      background: '#232136',
      foreground: '#e0def4',
      cursor: '#c4a7e7',
      cursorAccent: '#232136',
      selectionBackground: 'rgba(235,188,186,0.28)',
      black: '#393552',
      red: '#eb6f92',
      green: '#31748f',
      yellow: '#f6c177',
      blue: '#9ccfd8',
      magenta: '#c4a7e7',
      cyan: '#ebbcba',
      white: '#e0def4',
      brightBlack: '#6e6a86',
      brightRed: '#eb6f92',
      brightGreen: '#31748f',
      brightYellow: '#f6c177',
      brightBlue: '#9ccfd8',
      brightMagenta: '#c4a7e7',
      brightCyan: '#ebbcba',
      brightWhite: '#e0def4',
    },
  },
  oscura: {
    label: 'Oscura Midnight',
    term: {
      background: '#0f0d19',
      foreground: '#e6e3f2',
      cursor: '#a78bfa',
      cursorAccent: '#0f0d19',
      selectionBackground: 'rgba(167,139,250,0.30)',
      black: '#1b1829',
      red: '#ee6d85',
      green: '#6fdc9e',
      yellow: '#dcb879',
      blue: '#8aa6fa',
      magenta: '#c792ea',
      cyan: '#84dcec',
      white: '#e8e6f2',
      brightBlack: '#655e7e',
      brightRed: '#ff8b9e',
      brightGreen: '#93e6b4',
      brightYellow: '#ecd08d',
      brightBlue: '#a7c0ff',
      brightMagenta: '#c4a7e7',
      brightCyan: '#a9e9f7',
      brightWhite: '#ffffff',
    },
  },
};

const DEFAULT_THEME = 'groknight';

function canonThemeName(v) {
  const k = String(v ?? '').toLowerCase().replace(/[\s_-]/g, '');
  const map = {
    dark: 'groknight', night: 'groknight', default: DEFAULT_THEME,
    light: 'grokday', day: 'grokday',
    tokyo: 'tokyonight', tokyonight: 'tokyonight',
    rosepine: 'rosepine', rosepinemoon: 'rosepine',
    oscura: 'oscura', oscuramidnight: 'oscura',
  };
  if (THEMES[k]) return k;
  if (map[k]) return map[k];
  if (/light|day/.test(k)) return 'grokday';
  return DEFAULT_THEME;
}

// Everything needed to resolve the active look. Cached config comes from
// GET /api/settings/grok/theme; the WebUI override lives in ui_prefs.uiTheme.
const themeCtl = {
  conf: null,
  active: null,
  prefersDark: window.matchMedia('(prefers-color-scheme: dark)'),
};

function currentTermTheme() {
  return (THEMES[themeCtl.active] || THEMES[DEFAULT_THEME]).term;
}

async function fetchGrokThemeConf() {
  try { return await api('/api/settings/grok/theme'); } catch { return null; }
}

function resolveActiveTheme() {
  const override = state.prefs && state.prefs.uiTheme;
  if (override && override !== 'follow') return canonThemeName(override);
  const c = themeCtl.conf || {};
  const mode = String(c.theme || '').trim().toLowerCase();
  if (!c.exists) return DEFAULT_THEME;
  if (mode === 'auto' || mode === 'system') {
    // Mirrors the CLI: auto_dark_theme/auto_light_theme with GrokNight/GrokDay fallbacks.
    const pick = themeCtl.prefersDark.matches ? (c.auto_dark_theme || DEFAULT_THEME) : (c.auto_light_theme || 'grokday');
    return canonThemeName(pick);
  }
  if (!mode) return DEFAULT_THEME; // unset -> CLI default theme
  return canonThemeName(mode);
}

function applyUITheme(slug) {
  if (!THEMES[slug]) slug = DEFAULT_THEME;
  const changed = document.documentElement.dataset.theme !== slug || themeCtl.active !== slug;
  themeCtl.active = slug;
  if (changed) {
    document.documentElement.dataset.theme = slug;
    try { localStorage.setItem('gbw.theme', slug); } catch {}
    const term = currentTermTheme();
    for (const id of Object.keys(state.panes)) {
      try { state.panes[id].term.options.theme = term; } catch {}
    }
  }
  return changed;
}

let themeInflight = false;
async function refreshUITheme() {
  if (themeInflight) return;
  themeInflight = true;
  try {
    const conf = await fetchGrokThemeConf();
    if (conf) themeCtl.conf = conf;
    applyUITheme(resolveActiveTheme());
  } finally {
    themeInflight = false;
  }
}

function startThemeSync() {
  // Poll like the CLI's auto mode does (~5s) so /theme typed inside a running
  // session retints the whole WebUI without saving anything in Settings.
  let last = performance.now();
  setInterval(() => {
    if (document.hidden) return;
    const now = performance.now();
    if (now - last < 3000) return;
    last = now;
    refreshUITheme();
  }, 5000);
  window.addEventListener('focus', refreshUITheme);
  try { themeCtl.prefersDark.addEventListener('change', refreshUITheme); } catch {}
}


const ICONS = {
  splitRight: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/></svg>',
  splitDown: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 12h18"/></svg>',
  close: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>',
};

const state = {
  user: null,
  projects: [],
  activeProjectId: null,
  sessions: {},
  // Exclusive-tab model: one visible tab per project; each tab owns a
  // subtree of panes created by splitting inside that tab.
  tabTrees: {},
  activeSessionId: {},
  lastTreeStr: {},
  panePool: {},
  panes: {},
  maximizedId: null,
  focusedId: null,
  creating: false,
  prefs: {},
};

// Restore the last active theme before first paint so reloads don't flash
// the default palette; the server config is re-synced right after boot.
try {
  const cached = localStorage.getItem('gbw.theme');
  if (cached && THEMES[cached]) {
    document.documentElement.dataset.theme = cached;
    themeCtl.active = cached;
  }
} catch {}

function toast(text, kind = '') {
  const host = $('#toasts');
  const el = document.createElement('div');
  el.className = 'toast' + (kind ? ' ' + kind : '');
  el.textContent = text;
  host.appendChild(el);
  setTimeout(() => el.remove(), 4200);
}

// writeClipboard writes to the real OS clipboard. navigator.clipboard only
// exists on secure contexts (https or localhost); on plain-http LAN hosts it
// is undefined, so fall back to a hidden textarea + execCommand('copy').
async function writeClipboard(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (err) {
    // Permission denied / document not focused — fall through to execCommand.
  }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.top = '-1000px';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    ta.remove();
    return ok;
  } catch {
    return false;
  }
}

function setStatus(left, mid = '') {
  const l = $('#status-left');
  l.textContent = left;
  l.classList.toggle('live', /live|running|connected/i.test(left));
  $('#status-mid').textContent = mid;
}

async function api(path, opts = {}) {
  const headers = { ...(opts.headers || {}) };
  if (opts.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  const r = await fetch(path, { credentials: 'include', ...opts, headers });
  if (r.status === 401) {
    showAuth();
    throw new Error('unauthorized');
  }
  const text = await r.text();
  let data;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!r.ok) throw new Error((data && data.error) || text || r.statusText);
  return data;
}
window.api = api;

function msg(el, text, ok = false) {
  if (!el) return;
  el.textContent = text || '';
  el.className = 'msg ' + (ok ? 'ok' : text ? 'error' : '');
}

function escapeHtml(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function b64urlToBuf(s) {
  s = s.replace(/-/g, '+').replace(/_/g, '/');
  const pad = s.length % 4 ? '='.repeat(4 - (s.length % 4)) : '';
  const str = atob(s + pad);
  const buf = new Uint8Array(str.length);
  for (let i = 0; i < str.length; i++) buf[i] = str.charCodeAt(i);
  return buf.buffer;
}

function bufToB64url(buf) {
  const bytes = new Uint8Array(buf);
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function parsePublicKeyOptions(o) {
  if (o.publicKey) o = o.publicKey;
  if (o.challenge) o.challenge = b64urlToBuf(o.challenge);
  if (o.user && o.user.id) o.user.id = b64urlToBuf(o.user.id);
  if (o.allowCredentials) o.allowCredentials = o.allowCredentials.map(c => ({ ...c, id: b64urlToBuf(c.id) }));
  if (o.excludeCredentials) o.excludeCredentials = o.excludeCredentials.map(c => ({ ...c, id: b64urlToBuf(c.id) }));
  return o;
}

function formatAssertion(cred) {
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufToB64url(cred.response.authenticatorData),
      clientDataJSON: bufToB64url(cred.response.clientDataJSON),
      signature: bufToB64url(cred.response.signature),
      userHandle: cred.response.userHandle ? bufToB64url(cred.response.userHandle) : null,
    },
  };
}

function formatAttestation(cred) {
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64url(cred.response.attestationObject),
      clientDataJSON: bufToB64url(cred.response.clientDataJSON),
    },
  };
}

function showAuth() { $('#auth-overlay').classList.remove('hidden'); }
function hideAuth() { $('#auth-overlay').classList.add('hidden'); }
function showSetup() {
  showAuth();
  $('#setup-view').classList.remove('hidden');
  $('#login-view').classList.add('hidden');
  setTimeout(() => $('#setup-user').focus(), 50);
}
function showLogin() {
  showAuth();
  $('#setup-view').classList.add('hidden');
  $('#login-view').classList.remove('hidden');
  setTimeout(() => $('#login-user').focus(), 50);
}

async function checkAuth() {
  try {
    const setup = await api('/api/auth/setup-required');
    if (setup.setup_required) {
      showSetup();
      return false;
    }
  } catch (e) {
    console.error(e);
  }
  try {
    const me = await api('/api/auth/me');
    state.user = me;
    $('#user-badge').textContent = me.username;
    hideAuth();
    return true;
  } catch {
    showLogin();
    return false;
  }
}

$('#setup-form').onsubmit = async (e) => {
  e.preventDefault();
  const u = $('#setup-user').value.trim();
  const p = $('#setup-pass').value;
  if (!u || !p) { msg($('#setup-msg'), 'Need username and password'); return; }
  try {
    await api('/api/auth/setup', { method: 'POST', body: JSON.stringify({ username: u, password: p }) });
    msg($('#setup-msg'), 'Account created', true);
    setTimeout(() => location.reload(), 400);
  } catch (err) {
    msg($('#setup-msg'), err.message);
  }
};

$('#login-form').onsubmit = async (e) => {
  e.preventDefault();
  const u = $('#login-user').value.trim();
  const p = $('#login-pass').value;
  try {
    await api('/api/auth/login', { method: 'POST', body: JSON.stringify({ username: u, password: p }) });
    hideAuth();
    await initApp();
  } catch (err) {
    msg($('#login-msg'), err.message);
  }
};

$('#btn-passkey').onclick = async () => {
  msg($('#login-msg'), '');
  try {
    const username = $('#login-user').value.trim() || undefined;
    const begin = await api('/api/auth/webauthn/login/begin', {
      method: 'POST',
      body: JSON.stringify(username ? { username } : {}),
    });
    const cred = await navigator.credentials.get({ publicKey: parsePublicKeyOptions(begin) });
    if (!cred) throw new Error('No credential');
    await api('/api/auth/webauthn/login/finish', { method: 'POST', body: JSON.stringify(formatAssertion(cred)) });
    hideAuth();
    await initApp();
  } catch (err) {
    console.error(err);
    msg($('#login-msg'), err.message || 'Passkey failed');
  }
};

function disconnectAll(intentional) {
  for (const id of Object.keys(state.panes)) {
    const p = state.panes[id];
    if (!p) continue;
    p.closed = intentional;
    if (p.reconnectTimer) clearTimeout(p.reconnectTimer);
    try { if (p.ws) { p.ws._closed = true; p.ws.close(); } } catch {}
    if (intentional) {
      try { p.term?.dispose(); } catch {}
      delete state.panes[id];
    }
  }
  if (intentional) state.panePool = {};
}

$('#btn-logout').onclick = async () => {
  disconnectAll(true);
  try { await api('/api/auth/logout', { method: 'POST' }); } catch {}
  state.user = null;
  state.activeProjectId = null;
  showLogin();
  setStatus('Signed out');
};

function collectLeaves(node) {
  if (!node) return [];
  if (node.type === 'leaf') return [node.id];
  return [...collectLeaves(node.a), ...collectLeaves(node.b)];
}

function pruneTree(node, existing) {
  if (!node) return null;
  if (node.type === 'leaf') return existing.has(node.id) ? { type: 'leaf', id: node.id } : null;
  const a = pruneTree(node.a, existing);
  const b = pruneTree(node.b, existing);
  if (!a) return b;
  if (!b) return a;
  return { type: 'split', dir: node.dir, a, b };
}

function removeLeaf(node, id) {
  if (!node) return null;
  if (node.type === 'leaf') return node.id === id ? null : node;
  const a = removeLeaf(node.a, id);
  const b = removeLeaf(node.b, id);
  if (!a) return b;
  if (!b) return a;
  return { type: 'split', dir: node.dir, a, b };
}

function replaceLeaf(node, targetId, newNode) {
  if (!node) return node;
  if (node.type === 'leaf' && node.id === targetId) return newNode;
  if (node.type === 'split') {
    return {
      type: 'split',
      dir: node.dir,
      a: replaceLeaf(node.a, targetId, newNode),
      b: replaceLeaf(node.b, targetId, newNode),
    };
  }
  return node;
}

function tabState(pid) {
  if (!state.tabTrees[pid]) state.tabTrees[pid] = {};
  return state.tabTrees[pid];
}

function saveTabs(pid) {
  const payload = JSON.stringify({ trees: state.tabTrees[pid] || {}, active: state.activeSessionId[pid] || null });
  try { localStorage.setItem('tabs:' + pid, payload); } catch {}
  state.lastTreeStr[pid] = payload;
}

function loadTabs(pid) {
  try {
    const v = localStorage.getItem('tabs:' + pid);
    if (v) {
      const parsed = JSON.parse(v);
      state.tabTrees[pid] = parsed.trees || {};
      state.activeSessionId[pid] = parsed.active || null;
      state.lastTreeStr[pid] = v;
    }
  } catch {}
}

function runningCount(projectId) {
  return (state.sessions[projectId] || []).filter(s => s.status === 'running').length;
}

function renderProjects() {
  const el = $('#project-list');
  el.innerHTML = '';
  if (!state.projects.length) {
    el.innerHTML = '<div class="empty-projects">No projects yet</div>';
    return;
  }
  state.projects.forEach(p => {
    const div = document.createElement('div');
    const n = runningCount(p.id);
    div.className = 'project-item' + (p.id === state.activeProjectId ? ' active' : '');
    div.innerHTML = `<div class="meta"><div class="name">${escapeHtml(p.name)}</div><div class="path" title="${escapeHtml(p.path)}">${escapeHtml(p.path)}</div></div><div class="row"><span class="count">${n}</span><span class="edit" title="Edit project">✎</span><span class="del" title="Delete">${ICONS.close}</span></div>`;
    div.onclick = (e) => {
      if (e.target.closest('.del')) {
        e.stopPropagation();
        if (!confirm(`Delete project “${p.name}”? Running sessions will be killed.`)) return;
        deleteProject(p);
        return;
      }
      if (e.target.closest('.edit')) {
        e.stopPropagation();
        openEditProject(p);
        return;
      }
      selectProject(p.id);
    };
    el.appendChild(div);
  });
}

async function deleteProject(p) {
  try {
    await api(`/api/projects/${p.id}`, { method: 'DELETE' });
    Object.values(tabState(p.id)).flatMap(t => collectLeaves(t)).forEach(id => destroyPane(id, true));
    delete state.tabTrees[p.id];
    delete state.lastTreeStr[p.id];
    delete state.sessions[p.id];
    localStorage.removeItem('tabs:' + p.id);
    if (state.activeProjectId === p.id) {
      state.activeProjectId = null;
      $('#project-title').textContent = 'No project';
      $('#tabs-bar').innerHTML = '';
      $('#pane-container').innerHTML = '';
      $('#empty-state').classList.remove('hidden');
      setChromeEnabled(false);
    }
    toast('Project deleted');
    await loadProjects();
  } catch (e) {
    toast(e.message, 'error');
  }
}

function setChromeEnabled(on) {
  $('#btn-new-tab').disabled = !on;
  $('#btn-new-menu').disabled = !on;
  $('#btn-split-h').disabled = !on;
  $('#btn-split-v').disabled = !on;
  $('#btn-empty-new-tab').disabled = !on;
  $('#btn-empty-continue').disabled = !on;
}

function selectProject(id) {
  if (state.activeProjectId === id) return;
  stashCurrentPanes();
  $('#pane-container').innerHTML = '';
  state.activeProjectId = id;
  const proj = state.projects.find(p => p.id === id);
  $('#project-title').textContent = proj ? proj.name : 'No project';
  setChromeEnabled(!!proj);
  localStorage.setItem('activeProject', id);
  renderProjects();
  loadTabs(id);
  loadSessions(id);
}

async function loadProjects() {
  const list = await api('/api/projects');
  state.projects = list;
  await Promise.all(list.map(async p => {
    if (state.sessions[p.id]) return;
    try { state.sessions[p.id] = await api(`/api/projects/${p.id}/sessions`); } catch { state.sessions[p.id] = []; }
  }));
  renderProjects();
  const saved = localStorage.getItem('activeProject');
  const want = (saved && list.find(p => p.id === saved)) ? saved : (list[0] && list[0].id);
  if (want && state.activeProjectId !== want) selectProject(want);
  else if (state.activeProjectId) {
    setChromeEnabled(true);
    renderProjectChrome();
  } else {
    setChromeEnabled(false);
    $('#empty-state').classList.remove('hidden');
  }
}

function renderProjectChrome() {
  const proj = state.projects.find(p => p.id === state.activeProjectId);
  $('#project-title').textContent = proj ? proj.name : 'No project';
  setChromeEnabled(!!proj);
}

$('#btn-new-project').onclick = () => {
  $('#new-project-form').classList.remove('hidden');
  $('#proj-name').focus();
};
$('#btn-empty-new-project').onclick = () => {
  $('#sidebar').classList.remove('collapsed');
  $('#new-project-form').classList.remove('hidden');
  $('#proj-name').focus();
};
$('#btn-cancel-project').onclick = () => $('#new-project-form').classList.add('hidden');
$('#new-project-form').onsubmit = async (e) => {
  e.preventDefault();
  const name = $('#proj-name').value.trim();
  const path = $('#proj-path').value.trim();
  if (!name || !path) { msg($('#proj-msg'), 'Need name and path'); return; }
  try {
    const p = await api('/api/projects', { method: 'POST', body: JSON.stringify({ name, path }) });
    $('#proj-name').value = '';
    $('#proj-path').value = '';
    $('#new-project-form').classList.add('hidden');
    msg($('#proj-msg'), '');
    await loadProjects();
    selectProject(p.id);
    toast('Project created', 'ok');
  } catch (err) {
    msg($('#proj-msg'), err.message);
  }
};

async function loadSessions(projectId) {
  try {
    const list = await api(`/api/projects/${projectId}/sessions`);
    state.sessions[projectId] = list;
    if (list.length) {
      const cur = state.activeSessionId[projectId];
      const known = list.find(s => s.id === cur);
      if (!known) {
        // Prefer the most recently touched running session for first paint.
        const preferred = [...list]
          .filter(s => s.status === 'running')
          .sort((a, b) => new Date(b.last_active || b.created_at) - new Date(a.last_active || a.created_at))[0];
        state.activeSessionId[projectId] = (preferred || list[list.length - 1]).id;
      }
    } else {
      state.activeSessionId[projectId] = null;
    }
    renderProjects();
    renderTabs();
    renderPanes();
    updateStatus();
  } catch (e) {
    console.error(e);
  }
}

function sessionTitle(s, n) {
  const label = (s && s.title) || 'grok';
  return n != null ? `${label} ${n}` : label;
}

function renderTabs() {
  const bar = $('#tabs-bar');
  if (!state.activeProjectId) { bar.innerHTML = ''; return; }
  const list = state.sessions[state.activeProjectId] || [];
  bar.innerHTML = '';
  if (!list.length) {
    bar.innerHTML = '<span class="tabs-empty">No sessions — press New or Ctrl+`</span>';
    return;
  }
  list.forEach((s, i) => {
    const active = s.id === state.activeSessionId[state.activeProjectId];
    const tab = document.createElement('div');
    tab.className = 'tab' + (active ? ' active' : '') + (s.status !== 'running' ? ' exited' : '');
    tab.title = s.title || s.id;
    const labelSpan = `<span class="label">${escapeHtml(sessionTitle(s, i + 1))}</span>`;
    tab.innerHTML = `<span class="status"></span>${labelSpan}<span class="close" title="Detach / close">${ICONS.close}</span>`;
    tab.onclick = (e) => {
      if (e.target.closest('.close')) {
        e.stopPropagation();
        requestCloseSession(s.id);
        return;
      }
      activateTab(s.id);
    };
    // Double-click a tab label to rename the conversation.
    const label = tab.querySelector('.label');
    label.ondblclick = (e) => {
      e.stopPropagation();
      renameSession(s);
    };
    bar.appendChild(tab);
  });
}

function activateTab(sessionId) {
  const pid = state.activeProjectId;
  state.activeSessionId[pid] = sessionId;
  state.focusedId = sessionId;
  const trees = tabState(pid);
  // A detached (background) session reattaches as its own full-width tab.
  if (!trees[sessionId]) trees[sessionId] = { type: 'leaf', id: sessionId };
  saveTabs(pid);
  renderTabs();
  renderPanes();
  focusPane(sessionId);
}

function sessionLaunchBody(extra = {}) {
  const p = state.prefs || {};
  return {
    cols: 120,
    rows: 30,
    title: 'grok',
    model: p.model || undefined,
    permission_mode: p.permission || undefined,
    sandbox: p.sandbox || undefined,
    yolo: !!p.yolo,
    ...extra,
  };
}

async function createSession(extra = {}) {
  const pid = state.activeProjectId;
  if (!pid || state.creating) return false;
  state.creating = true;
  try {
    const s = await api(`/api/projects/${pid}/sessions`, {
      method: 'POST',
      body: JSON.stringify(sessionLaunchBody(extra)),
    });
    state.sessions[pid] = state.sessions[pid] || [];
    state.sessions[pid].push(s);
    // New conversations always open as their own exclusive tab.
    tabState(pid)[s.id] = { type: 'leaf', id: s.id };
    state.activeSessionId[pid] = s.id;
    state.focusedId = s.id;
    saveTabs(pid);
    renderProjects();
    renderTabs();
    renderPanes();
    updateStatus();
    return true;
  } catch (e) {
    toast(e.message, 'error');
    return false;
  } finally {
    state.creating = false;
  }
}

function destroyPane(id, disposeTerm) {
  const p = state.panes[id];
  if (p) {
    p.closed = true;
    if (p.reconnectTimer) clearTimeout(p.reconnectTimer);
    try { if (p.ws) { p.ws._closed = true; p.ws.close(); } } catch {}
    if (disposeTerm) {
      try { p.ro?.disconnect(); } catch {}
      try { p.term?.dispose(); } catch {}
      delete state.panes[id];
    }
  }
  if (state.panePool[id]) {
    try { state.panePool[id].remove(); } catch {}
    delete state.panePool[id];
  }
}

async function killSession(id) {
  destroyPane(id, true);
  try { await api(`/api/sessions/${id}`, { method: 'DELETE' }); } catch (e) { console.error(e); }
  for (const p in state.sessions) {
    state.sessions[p] = (state.sessions[p] || []).filter(s => s.id !== id);
  }
  for (const pid of Object.keys(state.tabTrees)) {
    const trees = tabState(pid);
    for (const sid of Object.keys(trees)) {
      trees[sid] = removeLeaf(trees[sid], id);
      if (!trees[sid]) delete trees[sid];
    }
    if (state.activeSessionId[pid] === id) {
      const remaining = state.sessions[pid] || [];
      state.activeSessionId[pid] = remaining.find(s => s.id !== id)?.id || null;
    }
    saveTabs(pid);
  }
  if (state.maximizedId === id) state.maximizedId = null;
  if (state.focusedId === id) state.focusedId = state.activeSessionId[state.activeProjectId];
  renderProjects();
  renderTabs();
  renderPanes();
  updateStatus();
}

// Detach removes the session from the current tab's layout but keeps the
// PTY process running in the background; its tab stays listed for reattach.
function detachSession(id) {
  const pid = state.activeProjectId;
  if (pid) {
    const trees = tabState(pid);
    for (const sid of Object.keys(trees)) {
      trees[sid] = removeLeaf(trees[sid], id);
      if (!trees[sid]) delete trees[sid];
    }
    saveTabs(pid);
  }
  state.panePool[id] = null;
  delete state.panePool[id];
  destroyPane(id, true);
  if (state.activeSessionId[pid] === id) {
    const others = (state.sessions[pid] || []).filter(s => s.id !== id);
    state.activeSessionId[pid] = others[others.length - 1]?.id || null;
  }
  if (state.focusedId === id) state.focusedId = state.activeSessionId[pid];
  if (state.maximizedId === id) state.maximizedId = null;
  renderTabs();
  renderPanes();
  updateStatus();
}

function stashCurrentPanes() {
  const container = $('#pane-container');
  container.querySelectorAll('.pane').forEach(paneEl => {
    const id = paneEl.dataset.id;
    if (id) {
      state.panePool[id] = paneEl;
      paneEl.remove();
    }
  });
}

function renderPanes() {
  const container = $('#pane-container');
  const empty = $('#empty-state');
  const pid = state.activeProjectId;
  if (!pid) {
    stashCurrentPanes();
    container.innerHTML = '';
    empty.classList.remove('hidden');
    return;
  }
  const list = state.sessions[pid] || [];
  const activeId = state.activeSessionId[pid];
  if (!list.length) {
    stashCurrentPanes();
    container.innerHTML = '';
    state.lastTreeStr[pid] = null;
    empty.classList.remove('hidden');
    return;
  }
  empty.classList.add('hidden');

  const existingIds = new Set(list.map(s => s.id));
  const trees = tabState(pid);
  let tree = trees[activeId];
  if (!tree) {
    tree = { type: 'leaf', id: activeId || list[0].id };
    trees[tree.id] = tree;
  }
  tree = pruneTree(tree, existingIds);
  if (!tree) {
    // Every pane in this tab was killed elsewhere; show another session.
    const fallback = list.find(s => s.id !== activeId && s.status === 'running') || list.find(s => s.id !== activeId);
    if (fallback) { activateTab(fallback.id); return; }
    container.innerHTML = '';
    empty.classList.remove('hidden');
    return;
  }
  if (activeId) trees[activeId] = tree;
  if (!state.activeSessionId[pid]) {
    state.activeSessionId[pid] = collectLeaves(tree)[0];
    saveTabs(pid);
  }

  // Maximize: temporarily render a single leaf without touching saved layout.
  const displayTree = state.maximizedId && collectLeaves(tree).includes(state.maximizedId)
    ? { type: 'leaf', id: state.maximizedId }
    : tree;

  const treeStr = JSON.stringify(displayTree);
  const needed = collectLeaves(displayTree);
  const mounted = [...container.querySelectorAll('.pane')].map(el => el.dataset.id);
  const sameMount = needed.length === mounted.length && needed.every(id => mounted.includes(id));
  if (state.lastTreeStr[pid] === treeStr + '#' + (activeId || '') && sameMount && container.children.length > 0) {
    requestAnimationFrame(() => resizeAll());
    highlightFocus();
    return;
  }
  state.lastTreeStr[pid] = treeStr + '#' + (activeId || '');

  stashCurrentPanes();
  container.innerHTML = '';
  container.appendChild(renderTree(displayTree, pid));
  requestAnimationFrame(() => {
    attachTerms(displayTree);
    highlightFocus();
    setTimeout(resizeAll, 40);
  });
}

function renderTree(node, projectId) {
  if (node.type === 'leaf') return renderLeaf(node.id, projectId);
  const split = document.createElement('div');
  split.className = 'split ' + node.dir;
  split.appendChild(renderTree(node.a, projectId));
  const resizer = document.createElement('div');
  resizer.className = 'resizer';
  split.appendChild(resizer);
  split.appendChild(renderTree(node.b, projectId));
  bindResizer(resizer, split, node.dir);
  return split;
}

function clearPaneLayout(el) {
  if (!el) return;
  el.style.flex = '';
  el.style.width = '';
  el.style.height = '';
}

function renderLeaf(id, projectId) {
  let pane = state.panePool[id];
  const sess = (state.sessions[projectId] || []).find(s => s.id === id);
  const list = state.sessions[projectId] || [];
  const idx = list.findIndex(s => s.id === id);
  const title = sessionTitle(sess, idx >= 0 ? idx + 1 : null);
  if (pane) {
    delete state.panePool[id];
    clearPaneLayout(pane);
    const titleEl = pane.querySelector('.title');
    if (titleEl) titleEl.innerHTML = `${escapeHtml(title)} <span class="muted">${escapeHtml(sess ? sess.status : '')}</span>`;
    pane.dataset.id = id;
    return pane;
  }
  pane = document.createElement('div');
  pane.className = 'pane';
  pane.dataset.id = id;
  pane.innerHTML = `
    <div class="pane-header">
      <span class="title">${escapeHtml(title)} <span class="muted">${escapeHtml(sess ? sess.status : '')}</span></span>
      <div class="actions">
        <button class="pane-btn" data-act="h" title="Split right">${ICONS.splitRight}</button>
        <button class="pane-btn" data-act="v" title="Split down">${ICONS.splitDown}</button>
        <button class="pane-btn danger" data-act="x" title="Close">${ICONS.close}</button>
      </div>
    </div>
    <div class="terminal" id="term-${id}"></div>
  `;
  pane.addEventListener('mousedown', () => focusPane(id));
  pane.querySelector('[data-act="h"]').onclick = (e) => { e.stopPropagation(); splitPane(id, 'row'); };
  pane.querySelector('[data-act="v"]').onclick = (e) => { e.stopPropagation(); splitPane(id, 'col'); };
  pane.querySelector('[data-act="x"]').onclick = (e) => { e.stopPropagation(); requestCloseSession(id); };
  return pane;
}

function bindResizer(resizer, split, dir) {
  resizer.onmousedown = (e) => {
    e.preventDefault();
    resizer.classList.add('dragging');
    const aEl = split.children[0];
    const bEl = split.children[2];
    const start = dir === 'row' ? e.clientX : e.clientY;
    const startA = dir === 'row' ? aEl.getBoundingClientRect().width : aEl.getBoundingClientRect().height;
    const startB = dir === 'row' ? bEl.getBoundingClientRect().width : bEl.getBoundingClientRect().height;
    const total = startA + startB;
    const onMove = (ev) => {
      const cur = dir === 'row' ? ev.clientX : ev.clientY;
      const delta = cur - start;
      const min = 80;
      const newA = Math.max(min, Math.min(total - min, startA + delta));
      const pctA = (newA / total) * 100;
      aEl.style.flex = `${pctA} 1 0`;
      bEl.style.flex = `${100 - pctA} 1 0`;
      resizeAll();
    };
    const onUp = () => {
      resizer.classList.remove('dragging');
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  };
}

function focusPane(id) {
  state.focusedId = id;
  if (state.activeProjectId) state.activeSessionId[state.activeProjectId] = id;
  highlightFocus();
  renderTabs();
  const p = state.panes[id];
  try { p?.term?.focus(); } catch {}
}

function highlightFocus() {
  $$('.pane').forEach(el => el.classList.toggle('focused', el.dataset.id === state.focusedId));
}

function attachTerms(node) {
  if (!node) return;
  if (node.type === 'leaf') {
    attachOne(node.id);
    return;
  }
  attachTerms(node.a);
  attachTerms(node.b);
}

function attachOne(id) {
  const el = document.getElementById('term-' + id);
  if (!el) return;
  const sess = ((state.sessions[state.activeProjectId] || []).find(s => s.id === id));
  let pane = state.panes[id];

  if (pane && pane.term && pane.el === el) {
    try { pane.fit.fit(); } catch {}
    if ((!pane.ws || pane.ws.readyState > 1) && !pane.closed && sess?.status === 'running') {
      connectWS(id);
    }
    return;
  }

  if (pane && pane.term && pane.el !== el) {
    try { pane.ro?.disconnect(); } catch {}
    try { pane.term.dispose(); } catch {}
    pane.term = null;
  }

  const term = new Terminal({
    cursorBlink: true,
    fontSize: Number(state.prefs.font) || 13,
    fontFamily: 'SF Mono, Cascadia Code, JetBrains Mono, ui-monospace, Menlo, Consolas, monospace',
    theme: currentTermTheme(),
    scrollback: Number(state.prefs.scrollback) || 5000,
    allowProposedApi: true,
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  try { term.loadAddon(new WebLinksAddon.WebLinksAddon()); } catch {}
  try { term.loadAddon(new Unicode11Addon.Unicode11Addon()); term.unicode.activeVersion = '11'; } catch {}
  term.open(el);
  try {
    const gl = new WebglAddon.WebglAddon();
    gl.onContextLoss(() => { try { gl.dispose(); } catch {} });
    term.loadAddon(gl);
  } catch {}
  try { fit.fit(); } catch {}
  const ro = new ResizeObserver(() => {
    try { fit.fit(); } catch {}
  });
  try { ro.observe(el); } catch {}
  const encoder = new TextEncoder();
  term.onData(d => {
    const p = state.panes[id];
    // Keystrokes go out as binary PTY frames; only resize stays JSON.
    if (p?.ws?.readyState === 1) p.ws.send(encoder.encode(d));
  });
  term.onResize(({ cols, rows }) => {
    const p = state.panes[id];
    if (p?.ws?.readyState === 1) p.ws.send(JSON.stringify({ type: 'resize', cols, rows }));
  });
  // OSC 52: the CLI sets the clipboard by emitting ESC ] 52 ; c ; <base64>
  // BEL. Desktop terminals honor it; without this handler xterm.js silently
  // drops the sequence, so "Copied!" in the CLI never reaches the OS clipboard.
  // Decode and write it for real. The OSC handler API lives on the parser
  // sub-object, not on the Terminal itself (xterm.js 5.x).
  term.parser.registerOscHandler(52, (data) => {
    const cur = state.panes[id];
    if (cur && cur.replaying) return true; // replayed history — not a fresh copy
    const semi = data.indexOf(';');
    if (semi < 0) return true;
    const payload = data.slice(semi + 1).trim();
    if (!payload || payload === '?') return true; // clipboard query: ignore
    try {
      const bin = atob(payload);
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
      const text = new TextDecoder().decode(bytes);
      if (!text) return true;
      writeClipboard(text).then(ok => {
        if (ok) toast('Copied to clipboard', 'ok');
        else toast('Copy failed — browser blocked clipboard access', 'error');
      });
    } catch { /* malformed base64 — ignore */ }
    return true;
  });

  term.attachCustomKeyEventHandler((ev) => {
    if (ev.ctrlKey && (ev.key === '`' || ev.key === 'b' || ev.key === ',' || ev.key === 'w')) return false;
    // Ctrl+Shift+C / Ctrl+Shift+V: copy/paste with the browser clipboard,
    // mirroring the desktop-terminal convention (Ctrl+C stays SIGINT).
    if (ev.type === 'keydown' && ev.ctrlKey && ev.shiftKey && (ev.key === 'C' || ev.key === 'V')) {
      const pane = state.panes[id];
      if (!pane || !pane.term) return false;
      if (ev.key === 'C') {
        const sel = pane.term.getSelection();
        if (!sel) return false;
        writeClipboard(sel).then(ok => {
          if (ok) toast('Copied to clipboard', 'ok');
          else toast('Copy failed — browser blocked clipboard access', 'error');
        });
      } else {
        const read = navigator.clipboard && navigator.clipboard.readText
          ? navigator.clipboard.readText()
          : Promise.reject(new Error('unavailable'));
        read.then(text => {
          if (text) pane.term.paste(text);
        }).catch(() => toast('Paste blocked — use Ctrl+V or middle-click', 'error'));
      }
      return false;
    }
    return true;
  });
  if (state.prefs.copyOnSelect) {
    term.onSelectionChange(() => {
      const sel = term.getSelection();
      if (sel) writeClipboard(sel);
    });
  }

  const prevWs = pane?.ws;
  const reuseWs = !!(prevWs && prevWs.readyState === 1);
  state.panes[id] = {
    term, fit, el, ro, ws: reuseWs ? prevWs : null,
    closed: false, retries: pane?.retries || 0, reconnectTimer: null,
    // A fresh connection replays buffered history (which may contain old
    // OSC 52 sequences); guard until that replay has been parsed.
    replaying: !reuseWs,
  };
  if (state.panes[id].ws) wireWS(id, state.panes[id].ws);
  else if (sess?.status !== 'exited') connectWS(id);
}

function connectWS(id) {
  const p = state.panes[id];
  if (!p || p.closed) return;
  if (p.ws && (p.ws.readyState === 0 || p.ws.readyState === 1)) return;
  const { cols, rows } = p.term;
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${proto}//${location.host}/api/sessions/${id}/ws?cols=${cols}&rows=${rows}`);
  ws.binaryType = 'arraybuffer';
  ws._closed = false;
  p.ws = ws;
  wireWS(id, ws);
}

function wireWS(id, ws) {
  const p = state.panes[id];
  if (!p) return;
  ws.onopen = () => {
    p.retries = 0;
    p.replaying = true;
    try { p.fit.fit(); } catch {}
    try { ws.send(JSON.stringify({ type: 'resize', cols: p.term.cols, rows: p.term.rows })); } catch {}
    markSession(id, 'running');
    updateStatus();
  };
  ws.onmessage = (ev) => {
    if (ev.data instanceof ArrayBuffer) {
      // Binary PTY output — replay history and live bytes both arrive this way.
      p.term.write(new Uint8Array(ev.data));
      return;
    }
    try {
      const m = JSON.parse(ev.data);
      if (m.type === 'exit') {
        p.closed = true;
        p.term.writeln('\r\n\x1b[31m[session exited]\x1b[0m');
        markSession(id, 'exited');
      } else if (m.type === 'sync') {
        // Replay finished: refit against current geometry and land the
        // viewport on live output instead of stale replayed scrollback.
        try { p.fit.fit(); } catch {}
        p.term.scrollToBottom();
        // The replay buffer can contain old OSC 52 clipboard sequences; the
        // queued writes (history + sync) have been parsed by now, so unguard.
        p.replaying = false;
      } else if (m.type === 'error') {
        p.term.writeln(`\r\n\x1b[31m[error: ${m.error || 'unknown'}]\x1b[0m`);
      } else if (m.type === 'pong') { /* keepalive */ }
    } catch { /* ignore malformed control frames */ }
  };
  ws.onclose = () => {
    if (p.closed || ws._closed) return;
    p.term.writeln('\r\n\x1b[33m[disconnected — reconnecting]\x1b[0m');
    const delay = Math.min(12000, 600 * Math.pow(1.7, p.retries || 0));
    p.retries = (p.retries || 0) + 1;
    p.reconnectTimer = setTimeout(async () => {
      if (!document.getElementById('term-' + id) || p.closed) return;
      try {
        const s = await api(`/api/sessions/${id}`);
        if (!s || s.status === 'exited') {
          p.closed = true;
          markSession(id, 'exited');
          p.term.writeln('\r\n\x1b[31m[session is gone]\x1b[0m');
          return;
        }
      } catch {
        p.closed = true;
        markSession(id, 'exited');
        p.term.writeln('\r\n\x1b[31m[session is gone]\x1b[0m');
        return;
      }
      connectWS(id);
    }, delay);
  };
  ws.onerror = () => {};
}

function markSession(id, status) {
  for (const pid in state.sessions) {
    const s = (state.sessions[pid] || []).find(x => x.id === id);
    if (s) s.status = status;
  }
  const pane = document.querySelector(`.pane[data-id="${id}"] .title .muted`);
  if (pane) pane.textContent = status;
  renderTabs();
  renderProjects();
  updateStatus();
}

function sendResize(id) {
  const p = state.panes[id];
  if (!p?.term) return;
  try { p.fit.fit(); } catch {}
  const { cols, rows } = p.term;
  if (p.ws?.readyState === 1) p.ws.send(JSON.stringify({ type: 'resize', cols, rows }));
}

function resizeAll() {
  for (const id in state.panes) {
    const p = state.panes[id];
    if (!p?.term || !document.getElementById('term-' + id)) continue;
    try {
      p.fit.fit();
      if (p.ws?.readyState === 1) {
        p.ws.send(JSON.stringify({ type: 'resize', cols: p.term.cols, rows: p.term.rows }));
      }
    } catch {}
  }
}

window.addEventListener('resize', () => {
  clearTimeout(window._resizeTimer);
  window._resizeTimer = setTimeout(resizeAll, 80);
});

async function splitPane(leafId, dir) {
  const pid = state.activeProjectId;
  if (!pid) return;
  try {
    const s = await api(`/api/projects/${pid}/sessions`, {
      method: 'POST',
      body: JSON.stringify(sessionLaunchBody()),
    });
    state.sessions[pid] = state.sessions[pid] || [];
    state.sessions[pid].push(s);
    // Splits live inside the current tab; the tab bar itself is unchanged.
    const trees = tabState(pid);
    const curr = trees[state.activeSessionId[pid]] || { type: 'leaf', id: leafId };
    const neu = { type: 'split', dir, a: { type: 'leaf', id: leafId }, b: { type: 'leaf', id: s.id } };
    trees[state.activeSessionId[pid]] = replaceLeaf(curr, leafId, neu) || neu;
    state.focusedId = s.id;
    saveTabs(pid);
    renderProjects();
    renderTabs();
    renderPanes();
    updateStatus();
  } catch (e) {
    toast(e.message, 'error');
  }
}

function updateStatus() {
  const pid = state.activeProjectId;
  const list = state.sessions[pid] || [];
  const live = list.filter(s => s.status === 'running').length;
  const proj = state.projects.find(p => p.id === pid);
  if (!pid) setStatus('Ready', '');
  else if (!list.length) setStatus('No sessions', proj ? proj.path : '');
  else setStatus(`${live} live`, proj ? proj.path : '');
}

$('#btn-new-tab').onclick = () => createSession();
$('#btn-empty-new-tab').onclick = () => createSession();
$('#btn-split-h').onclick = () => {
  const pid = state.activeProjectId;
  const active = state.activeSessionId[pid];
  if (active) splitPane(active, 'row');
  else createSession();
};
$('#btn-split-v').onclick = () => {
  const pid = state.activeProjectId;
  const active = state.activeSessionId[pid];
  if (active) splitPane(active, 'col');
  else createSession();
};
$('#btn-toggle-sidebar').onclick = () => {
  $('#sidebar').classList.toggle('collapsed');
  setTimeout(resizeAll, 60);
};

// ---- Close session: detach vs kill ---------------------------------------

let closeTargetId = null;

function requestCloseSession(id) {
  const s = (state.sessions[state.activeProjectId] || []).find(x => x.id === id)
    || Object.values(state.sessions).flat().find(x => x.id === id);
  const inLayout = collectLeaves(tabState(state.activeProjectId)[state.activeSessionId[state.activeProjectId]] || {}).includes(id);
  $('#close-text').textContent = s && s.status === 'running'
    ? 'Detach keeps Grok running in the background (its tab stays listed). Kill stops the PTY.'
    : 'This session has exited. Remove it from the list?';
  $('#btn-close-detach').textContent = inLayout ? 'Detach' : 'Remove pane';
  closeTargetId = id;
  $('#close-overlay').classList.remove('hidden');
}

function hideCloseDialog() {
  $('#close-overlay').classList.add('hidden');
  closeTargetId = null;
}

$('#btn-close-cancel').onclick = hideCloseDialog;
$('#btn-close-close').onclick = hideCloseDialog;
$('#close-overlay').addEventListener('click', (e) => { if (e.target.id === 'close-overlay') hideCloseDialog(); });
$('#btn-close-detach').onclick = () => {
  const id = closeTargetId;
  hideCloseDialog();
  if (id) detachSession(id);
};
$('#btn-close-kill').onclick = () => {
  const id = closeTargetId;
  hideCloseDialog();
  if (id) killSession(id);
};

async function renameSession(s) {
  const name = prompt('Rename conversation', (s && s.title) || '');
  if (name === null || !name.trim()) return;
  try {
    await api(`/api/sessions/${s.id}`, { method: 'PATCH', body: JSON.stringify({ title: name.trim() }) });
    if (s) s.title = name.trim() || '';
    renderTabs();
    renderPanes();
  } catch (e) {
    toast(e.message, 'error');
  }
}

function applyPrefsToForm(p) {
  p = p || {};
  $('#pref-model').value = p.model || '';
  $('#pref-permission').value = p.permission || '';
  $('#pref-sandbox').value = p.sandbox || '';
  $('#pref-yolo').checked = !!p.yolo;
  $('#pref-font').value = p.font || 13;
  $('#pref-scrollback').value = p.scrollback || 5000;
  $('#pref-copy-select').checked = !!p.copyOnSelect;
  const uiTheme = $('#pref-ui-theme');
  if (uiTheme) uiTheme.value = THEMES[p.uiTheme] ? p.uiTheme : 'follow';
}

function readPrefsFromForm() {
  const uiTheme = $('#pref-ui-theme');
  return {
    model: $('#pref-model').value.trim(),
    permission: $('#pref-permission').value,
    sandbox: $('#pref-sandbox').value,
    yolo: $('#pref-yolo').checked,
    font: Number($('#pref-font').value) || 13,
    scrollback: Number($('#pref-scrollback').value) || 5000,
    copyOnSelect: $('#pref-copy-select').checked,
    uiTheme: uiTheme && THEMES[uiTheme.value] ? uiTheme.value : 'follow',
  };
}

function openSettings() {
  $('#settings-overlay').classList.remove('hidden');
  if (window.GrokSettings) window.GrokSettings.init();
  api('/api/settings').then(s => {
    $('#setting-public-url').value = s.public_url || '';
    $('#rpid-display').textContent = s.rpid || '-';
    $('#grok-bin-display').textContent = s.grok_bin || 'grok';
    $('#max-sessions-display').textContent = s.max_sessions || 16;
    $('#running-display').textContent = s.running ?? 0;
    $('#setting-public-url').disabled = !!s.locked;
    $('#btn-save-public-url').disabled = !!s.locked;
    state.prefs = s.prefs || state.prefs || {};
    applyPrefsToForm(state.prefs);
    if (s.locked) msg($('#settings-msg'), 'Public URL is locked by GROK_WEBUI_PUBLIC_URL', true);
  }).catch(() => {});
  loadVersion(false);
  if (window.GrokSettings) {
    window.GrokSettings.load().catch(e => {
      const el = $('#grok-msg');
      if (el) { el.textContent = e.message || 'Failed to load Grok config'; el.className = 'msg error'; }
    });
  }
  loadPasskeys();
}

// ---- version indicator -----------------------------------------------------

const VER_LABEL = {
  up_to_date: 'up to date',
  out_of_date: 'update available',
  local_build: 'local build',
  unknown: 'status unknown',
};

function renderVersion(v) {
  const badge = $('#version-badge');
  const text = $('#version-text');
  if (!badge || !text) return;
  badge.classList.remove('ok', 'out', 'local', 'unknown');
  const st = v.status || 'unknown';
  badge.classList.add(
    st === 'up_to_date' ? 'ok' :
    st === 'out_of_date' ? 'out' :
    st === 'local_build' ? 'local' : 'unknown');
  let label = VER_LABEL[st] || VER_LABEL.unknown;
  if (st === 'out_of_date') {
    label += `: ${v.latest} (running ${v.version})`;
  } else if (st === 'local_build') {
    label += ` ${v.version} — newer than or ahead of releases`;
  } else if (st === 'up_to_date') {
    label = `${v.latest} · up to date`;
  }
  text.textContent = label;
  if (v.checked_at) {
    text.title = `Last checked ${new Date(v.checked_at).toLocaleString()}`;
  } else {
    text.removeAttribute('title');
  }
}

async function loadVersion(force) {
  const btn = $('#btn-check-version');
  try {
    if (btn) { btn.disabled = true; }
    const v = await api(`/api/settings/version${force ? '?refresh=1' : ''}`);
    renderVersion(v);
    const link = $('#ver-releases-link');
    if (link && v.releases_url) link.href = v.releases_url;
  } catch (_) {
    const badge = $('#version-badge');
    if (badge) { badge.className = 'ver-badge unknown'; }
    const text = $('#version-text');
    if (text) text.textContent = 'status unavailable';
  } finally {
    if (btn) btn.disabled = false;
  }
}
function closeSettings() {
  if (window.GrokSettings && window.GrokSettings.isDirty() && !confirm('Discard unsaved Grok config changes?')) return;
  $('#settings-overlay').classList.add('hidden');
}
function openHelp() { $('#help-overlay').classList.remove('hidden'); }
function closeHelp() { $('#help-overlay').classList.add('hidden'); }

$('#btn-settings').onclick = openSettings;
$('#btn-check-version')?.addEventListener('click', () => loadVersion(true));
$('#btn-close-settings').onclick = closeSettings;
$('#btn-close-help').onclick = closeHelp;
$('#settings-overlay').addEventListener('click', (e) => { if (e.target.id === 'settings-overlay') closeSettings(); });
$('#help-overlay').addEventListener('click', (e) => { if (e.target.id === 'help-overlay') closeHelp(); });

$('#btn-save-prefs').onclick = async () => {
  const prefs = readPrefsFromForm();
  try {
    const s = await api('/api/settings', { method: 'PUT', body: JSON.stringify({ prefs }) });
    state.prefs = (s && s.prefs) || prefs;
    msg($('#prefs-msg'), 'Preferences saved. New terminals and sessions pick them up.', true);
    if (state.prefs.uiTheme === 'follow') await refreshUITheme();
    else applyUITheme(canonThemeName(state.prefs.uiTheme));
  } catch (e) {
    msg($('#prefs-msg'), e.message);
  }
};

$('#btn-save-public-url').onclick = async () => {
  const v = $('#setting-public-url').value.trim();
  try {
    await api('/api/settings', { method: 'PUT', body: JSON.stringify({ public_url: v }) });
    const s = await api('/api/settings');
    $('#rpid-display').textContent = s.rpid;
    msg($('#settings-msg'), 'Saved. Re-login may be needed if the origin changed.', true);
  } catch (e) {
    msg($('#settings-msg'), e.message);
  }
};

async function loadPasskeys() {
  try {
    const list = await api('/api/auth/webauthn/credentials');
    const el = $('#passkey-list');
    el.innerHTML = '';
    if (!list.length) {
      el.innerHTML = '<div class="muted small">No passkeys yet</div>';
      return;
    }
    list.forEach(c => {
      const div = document.createElement('div');
      div.className = 'item';
      const when = c.created_at ? String(c.created_at).replace('T', ' ').slice(0, 19) : '';
      div.innerHTML = `<span>${escapeHtml(c.name || c.id.slice(0, 8))} <span class="muted small">${escapeHtml(when)}</span></span><button class="small">Delete</button>`;
      div.querySelector('button').onclick = async () => {
        if (!confirm('Delete this passkey?')) return;
        await api(`/api/auth/webauthn/credentials/${c.id}`, { method: 'DELETE' });
        loadPasskeys();
      };
      el.appendChild(div);
    });
  } catch (e) {
    console.error(e);
  }
}

$('#btn-add-passkey').onclick = async () => {
  const name = $('#passkey-name').value.trim() || 'passkey';
  msg($('#passkey-msg'), '');
  try {
    const begin = await api('/api/auth/webauthn/register/begin', { method: 'POST' });
    const cred = await navigator.credentials.create({ publicKey: parsePublicKeyOptions(begin) });
    if (!cred) throw new Error('No credential');
    const data = formatAttestation(cred);
    const r = await fetch('/api/auth/webauthn/register/finish?name=' + encodeURIComponent(name), {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    if (!r.ok) {
      const t = await r.text();
      let err = t;
      try { err = JSON.parse(t).error || t; } catch {}
      throw new Error(err);
    }
    msg($('#passkey-msg'), 'Passkey added', true);
    $('#passkey-name').value = '';
    loadPasskeys();
  } catch (e) {
    console.error(e);
    msg($('#passkey-msg'), e.message || 'Failed');
  }
};

// ---- Launch modes: new / continue last / resume ---------------------------

async function continueLast() {
  const pid = state.activeProjectId;
  if (!pid) return;
  return createSession({ mode: 'continue', title: 'grok' });
}

function openResume() {
  const pid = state.activeProjectId;
  if (!pid) return;
  $('#resume-overlay').classList.remove('hidden');
  $('#resume-filter').value = '';
  $('#resume-list').innerHTML = '<div class="muted small">Loading conversations…</div>';
  loadResumeList(pid, '');
  setTimeout(() => $('#resume-filter').focus(), 50);
}

async function loadResumeList(pid, filter) {
  try {
    const list = await api(`/api/projects/${pid}/conversations`);
    const el = $('#resume-list');
    el.innerHTML = '';
    const f = (filter || '').toLowerCase();
    const filtered = list.filter(c => !f || (c.title || '').toLowerCase().includes(f));
    if (!filtered.length) {
      el.innerHTML = `<div class="muted small">${list.length ? 'No matches' : 'No saved conversations found for this project'}</div>`;
      return;
    }
    filtered.sort((a, b) => new Date(b.updated_at || b.created_at) - new Date(a.updated_at || a.created_at));
    filtered.forEach(c => {
      const div = document.createElement('div');
      div.className = 'item conversation';
      const when = c.updated_at ? new Date(c.updated_at).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }) : '';
      div.innerHTML = `
        <div class="meta">
          <div class="name">${escapeHtml(c.title || 'Untitled')}</div>
          <div class="muted small">${escapeHtml(when)} · ${Number(c.num_messages || 0)} messages${c.model ? ' · ' + escapeHtml(c.model) : ''}</div>
        </div>
        <span class="muted small mono">${escapeHtml((c.id || '').slice(0, 8))}</span>`;
      div.onclick = () => resumeConversation(c);
      el.appendChild(div);
    });
  } catch (e) {
    $('#resume-list').innerHTML = '';
    toast(e.message, 'error');
  }
}

async function resumeConversation(c) {
  $('#resume-overlay').classList.add('hidden');
  const title = (c.title && c.title.trim()) || `resume ${String(c.id).slice(0, 6)}`;
  await createSession({ mode: 'resume', resume_id: c.id, title });
}

$('#btn-close-resume').onclick = () => $('#resume-overlay').classList.add('hidden');
$('#resume-overlay').addEventListener('click', (e) => { if (e.target.id === 'resume-overlay') e.target.classList.add('hidden'); });
$('#resume-filter').addEventListener('input', () => {
  if (state.activeProjectId) loadResumeList(state.activeProjectId, $('#resume-filter').value);
});

// New-session dropdown menu.
const newMenu = $('#new-menu');
$('#btn-new-menu').onclick = (e) => {
  e.stopPropagation();
  newMenu.classList.toggle('hidden');
};
document.addEventListener('click', () => newMenu.classList.add('hidden'));
newMenu.querySelectorAll('button').forEach(btn => {
  btn.onclick = () => {
    newMenu.classList.add('hidden');
    if (btn.dataset.act === 'new') createSession();
    else if (btn.dataset.act === 'continue') continueLast();
    else if (btn.dataset.act === 'resume') openResume();
  };
});
$('#btn-empty-continue').onclick = continueLast;

// ---- Command palette -------------------------------------------------------

let paletteIndex = 0;

function paletteCommands() {
  const cmds = [
    { name: 'New conversation', hint: 'Ctrl+`', run: () => createSession() },
    { name: 'Continue last conversation', hint: 'Ctrl+Shift+`', run: continueLast },
    { name: 'Resume conversation…', run: openResume },
    { name: 'Split right', run: () => { if (state.focusedId) splitPane(state.focusedId, 'row'); } },
    { name: 'Split down', run: () => { if (state.focusedId) splitPane(state.focusedId, 'col'); } },
    { name: 'Toggle sidebar', hint: 'Ctrl+B', run: toggleSidebar },
    { name: 'Maximize focused pane', hint: 'Ctrl+\\', run: toggleMaximize },
    { name: 'Next tab', run: cycleTab.bind(null, 1) },
    { name: 'Previous tab', run: cycleTab.bind(null, -1) },
    { name: 'Settings', hint: 'Ctrl+,', run: openSettings },
    { name: 'Keyboard shortcuts', hint: '?', run: openHelp },
  ];
  // Focus commands for background/detached sessions.
  for (const s of (state.sessions[state.activeProjectId] || [])) {
    if (s.id === state.activeSessionId[state.activeProjectId]) continue;
    cmds.push({ name: `Focus tab: ${sessionTitle(s)}${s.status === 'running' ? '' : ` (${s.status})`}`, run: () => activateTab(s.id) });
  }
  return cmds;
}

function openPalette() {
  $('#palette-overlay').classList.remove('hidden');
  const input = $('#palette-input');
  input.value = '';
  renderPalette('');
  setTimeout(() => input.focus(), 50);
}
function closePalette() { $('#palette-overlay').classList.add('hidden'); }

function renderPalette(filter) {
  const listEl = $('#palette-list');
  const f = (filter || '').toLowerCase();
  const all = paletteCommands();
  const items = all.map((c, i) => ({ ...c, i })).filter(c => !f || c.name.toLowerCase().includes(f));
  paletteIndex = Math.max(0, Math.min(paletteIndex, items.length - 1));
  listEl.innerHTML = '';
  items.forEach((c, n) => {
    const div = document.createElement('div');
    div.className = 'pal-item' + (n === paletteIndex ? ' active' : '');
    div.innerHTML = `<span>${escapeHtml(c.name)}</span><span class="hint">${escapeHtml(c.hint || '')}</span>`;
    div.onclick = () => { closePalette(); c.run(); };
    listEl.appendChild(div);
  });
  return items;
}

$('#palette-input').addEventListener('input', () => { paletteIndex = 0; renderPalette($('#palette-input').value); });
$('#palette-input').addEventListener('keydown', (e) => {
  const f = $('#palette-input').value.toLowerCase();
  const items = paletteCommands().filter(c => !f || c.name.toLowerCase().includes(f));
  if (e.key === 'ArrowDown') { e.preventDefault(); paletteIndex = Math.min(items.length - 1, paletteIndex + 1); renderPalette($('#palette-input').value); }
  else if (e.key === 'ArrowUp') { e.preventDefault(); paletteIndex = Math.max(0, paletteIndex - 1); renderPalette($('#palette-input').value); }
  else if (e.key === 'Enter') {
    e.preventDefault();
    const c = items[paletteIndex];
    if (c) { closePalette(); c.run(); }
  }
});
$('#palette-overlay').addEventListener('click', (e) => { if (e.target.id === 'palette-overlay') closePalette(); });

$('#btn-palette').onclick = openPalette;

// ---- Maximize and tab cycling ----------------------------------------------

function toggleMaximize() {
  if (!state.focusedId) return;
  state.lastTreeStr[state.activeProjectId] = null; // force re-render
  state.maximizedId = state.maximizedId ? null : state.focusedId;
  renderPanes();
}

function cycleTab(delta) {
  const pid = state.activeProjectId;
  const list = state.sessions[pid] || [];
  if (list.length < 2) return;
  const cur = list.findIndex(s => s.id === state.activeSessionId[pid]);
  const next = ((cur < 0 ? 0 : cur + delta) + list.length) % list.length;
  activateTab(list[next].id);
}

function toggleSidebar() {
  $('#sidebar').classList.toggle('collapsed');
  setTimeout(resizeAll, 60);
}

// ---- Folder browser ---------------------------------------------------------

let browseTargetInput = null;
let browseFilterTimer = null;

function renderBrowseEntries(entries, selectable) {
  const list = $('#browse-list');
  list.innerHTML = '';
  if (!entries || !entries.length) {
    list.innerHTML = `<div class="muted small">${selectable ? 'No matching folders' : 'No subdirectories'}</div>`;
    return;
  }
  entries.forEach(d => {
    const div = document.createElement('div');
    div.className = 'item';
    div.innerHTML = `<span>📁 ${escapeHtml(d.name)}</span><span class="muted small mono">${escapeHtml(d.path)}</span>`;
    div.onclick = () => {
      if (selectable) selectBrowseMatch(div, d.path);
      else browseInto(d.path);
    };
    list.appendChild(div);
  });
}

function selectBrowseMatch(div, path) {
  $('#browse-overlay').dataset.current = path;
  $('#browse-list').querySelectorAll('.item').forEach(el => el.classList.remove('selected'));
  div.classList.add('selected');
}

async function runBrowseSearch(q) {
  try {
    const base = $('#browse-path').textContent;
    const data = await api(`/api/browse?path=${encodeURIComponent(base)}&q=${encodeURIComponent(q)}`);
    renderBrowseEntries(data.matches, true);
  } catch (e) {
    toast(e.message, 'error');
  }
}

async function browseInto(path) {
  try {
    const data = await api('/api/browse' + (path ? `?path=${encodeURIComponent(path)}` : ''));
    $('#browse-path').textContent = data.path;
    $('#browse-filter').value = '';
    renderBrowseEntries(data.dirs, false);
    $('#btn-browse-up').disabled = !data.parent;
    $('#browse-overlay').dataset.current = data.path;
  } catch (e) {
    toast(e.message, 'error');
  }
}

function openBrowser(inputEl) {
  browseTargetInput = inputEl;
  $('#browse-overlay').classList.remove('hidden');
  browseInto('');
}
function hideBrowser() {
  $('#browse-overlay').classList.add('hidden');
  browseTargetInput = null;
}

$('#btn-close-browse').onclick = hideBrowser;
$('#browse-overlay').addEventListener('click', (e) => { if (e.target.id === 'browse-overlay') hideBrowser(); });
$('#btn-browse-up').onclick = () => browseInto($('#browse-path').textContent);
$('#browse-filter').addEventListener('input', () => {
  clearTimeout(browseFilterTimer);
  const q = $('#browse-filter').value.trim();
  if (!q) { browseInto($('#browse-path').textContent); return; }
  browseFilterTimer = setTimeout(() => runBrowseSearch(q), 200);
});
$('#browse-filter').addEventListener('keydown', (e) => {
  if (e.key !== 'Enter') return;
  e.preventDefault();
  const first = document.querySelector('#browse-list .item');
  if (first) first.click();
});
$('#btn-browse-choose').onclick = () => {
  const p = $('#browse-overlay').dataset.current;
  if (p && browseTargetInput) {
    browseTargetInput.value = p;
    browseTargetInput.dispatchEvent(new Event('input'));
  }
  hideBrowser();
};

$('#btn-browse-path').onclick = () => openBrowser($('#proj-path'));

// ---- Edit project ------------------------------------------------------------

function openEditProject(p) {
  $('#edit-project-overlay').dataset.projectId = p.id;
  $('#edit-proj-name').value = p.name;
  $('#edit-proj-path').value = p.path;
  msg($('#edit-proj-msg'), '');
  $('#edit-project-overlay').classList.remove('hidden');
}
function hideEditProject() { $('#edit-project-overlay').classList.add('hidden'); }

$('#btn-close-edit-project').onclick = hideEditProject;
$('#edit-project-overlay').addEventListener('click', (e) => { if (e.target.id === 'edit-project-overlay') hideEditProject(); });
$('#btn-browse-edit-path').onclick = () => openBrowser($('#edit-proj-path'));
$('#edit-project-form').onsubmit = async (e) => {
  e.preventDefault();
  const id = $('#edit-project-overlay').dataset.projectId;
  try {
    await api(`/api/projects/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: $('#edit-proj-name').value.trim(), path: $('#edit-proj-path').value.trim() }),
    });
    hideEditProject();
    await loadProjects();
    selectProject(id);
    toast('Project updated', 'ok');
  } catch (err) {
    msg($('#edit-proj-msg'), err.message);
  }
};

// ---- Restore after server restart ---------------------------------------------

const BOOT_KEY = 'serverboot';

async function maybeShowRestoreBanner(serverStartedAt) {
  let prev = null;
  try { prev = localStorage.getItem(BOOT_KEY); } catch {}
  try { localStorage.setItem(BOOT_KEY, serverStartedAt); } catch {}
  if (!prev || prev === serverStartedAt) return;
  // After backend status-normalization, interrupted sessions show as
  // "exited"; only offer ones that can actually be continued or resumed.
  const dead = [];
  for (const pid of Object.keys(state.sessions)) {
    for (const s of state.sessions[pid] || []) {
      if (s.status !== 'running' && (s.grok_session_id || ['continue', 'resume'].includes(s.mode))) dead.push(s);
    }
  }
  if (!dead.length) return;
  $('#restore-text').textContent =
    `Server restarted. ${dead.length} Grok session${dead.length > 1 ? 's were' : ' was'} interrupted. Respawn with continue/resume?`;
  $('#restore-banner').classList.remove('hidden');
  $('#btn-restore').onclick = async () => {
    $('#restore-banner').classList.add('hidden');
    for (const s of dead) {
      try { await api(`/api/sessions/${s.id}/restore`, { method: 'POST' }); } catch (e) { console.error(e); }
    }
    delete state.lastTreeStr[state.activeProjectId];
    await loadProjects();
    if (state.activeProjectId) loadSessions(state.activeProjectId);
    toast('Sessions restored', 'ok');
  };
}

$('#btn-dismiss-restore').onclick = () => $('#restore-banner').classList.add('hidden');

// ---- Global keyboard shortcuts -------------------------------------------------

function anyOverlayOpen() {
  return ['settings-overlay', 'help-overlay', 'palette-overlay', 'resume-overlay', 'browse-overlay', 'close-overlay', 'edit-project-overlay']
    .some(id => !$('#' + id).classList.contains('hidden'));
}

function hideTopOverlay() {
  for (const id of ['close-overlay', 'palette-overlay', 'resume-overlay', 'browse-overlay', 'edit-project-overlay', 'help-overlay']) {
    const el = $('#' + id);
    if (!el.classList.contains('hidden')) { el.classList.add('hidden'); return true; }
  }
  if (!$('#settings-overlay').classList.contains('hidden')) { closeSettings(); return true; }
  return false;
}

document.addEventListener('keydown', (e) => {
  const tag = (e.target && e.target.tagName) || '';
  const typing = tag === 'INPUT' || tag === 'TEXTAREA';
  if (e.key === 'Escape') {
    if (anyOverlayOpen()) { e.preventDefault(); hideTopOverlay(); }
    return;
  }
  const ctrl = e.ctrlKey || e.metaKey;
  if (ctrl && e.shiftKey && (e.key === '~' || e.key === '`')) {
    e.preventDefault();
    continueLast();
    return;
  }
  if (ctrl && e.key === '`') {
    e.preventDefault();
    createSession();
    return;
  }
  if ((ctrl || e.altKey) && (e.key === 'k' || e.key === 'K') && !e.shiftKey) {
    e.preventDefault();
    if ($('#palette-overlay').classList.contains('hidden')) openPalette(); else closePalette();
    return;
  }
  if (ctrl && (e.key === 'b' || e.key === 'B')) {
    e.preventDefault();
    toggleSidebar();
    return;
  }
  if (ctrl && e.key === ',') {
    e.preventDefault();
    openSettings();
    return;
  }
  if (ctrl && (e.key === 'w' || e.key === 'W')) {
    if (state.focusedId) {
      e.preventDefault();
      requestCloseSession(state.focusedId);
    }
    return;
  }
  if (ctrl && e.key === '\\') {
    e.preventDefault();
    toggleMaximize();
    return;
  }
  if (ctrl && e.key === '\t') {
    e.preventDefault();
    cycleTab(e.shiftKey ? -1 : 1);
    return;
  }
  if (e.altKey && e.key === 'ArrowRight') { e.preventDefault(); cycleTab(1); return; }
  if (e.altKey && e.key === 'ArrowLeft') { e.preventDefault(); cycleTab(-1); return; }
  if (!typing && e.key === '?' && !e.ctrlKey && !e.metaKey && !e.altKey) {
    e.preventDefault();
    openHelp();
  }
});

async function initApp() {
  if (!await checkAuth()) return;
  let startedAt = null;
  try {
    const s = await api('/api/settings');
    state.prefs = s.prefs || {};
    startedAt = s.started_at || null;
  } catch {}
  // Re-resolve now that prefs (possible pin override) are known, then keep
  // following config.toml for the lifetime of the page.
  await refreshUITheme();
  startThemeSync();
  if (window.GrokSettings) window.GrokSettings.init();
  await loadProjects();
  if (startedAt) maybeShowRestoreBanner(startedAt);
}

// Hook used by settings.js to resync right after saving/reverting config.toml.
window.GrokBuildTheme = {
  refresh: refreshUITheme,
};

initApp();
