// Grok config.toml editor for the settings modal.
(function () {
  const $ = (s, r = document) => r.querySelector(s);

  const state = {
    doc: null,
    work: null,
    tab: 'webui',
    group: 'models',
    query: '',
    showUnset: true,
    dirty: false,
    expanded: {},
  };

  function clone(v) {
    return JSON.parse(JSON.stringify(v));
  }

  function eq(a, b) {
    return JSON.stringify(normVal(a)) === JSON.stringify(normVal(b));
  }

  function normVal(v) {
    if (v == null) return null;
    return v;
  }

  function origField(key) {
    for (const sec of state.doc.sections) {
      const f = sec.fields.find(x => x.key === key);
      if (f) return f;
    }
    return null;
  }

  function origItem(colId, id) {
    const col = state.doc.collections.find(c => c.id === colId);
    if (!col) return null;
    return (col.items || []).find(i => i.id === id) || null;
  }

  function fromDoc(doc) {
    const fields = {};
    for (const sec of doc.sections) {
      for (const f of sec.fields) {
        fields[f.key] = { set: !!f.set, value: f.set ? clone(f.value) : clone(f.default ?? null) };
      }
    }
    const collections = {};
    for (const col of doc.collections) {
      const items = {};
      const order = [];
      for (const item of col.items || []) {
        order.push(item.id);
        const fmap = {};
        for (const f of item.fields) {
          fmap[f.key] = { set: !!f.set, value: f.set ? clone(f.value) : clone(f.default ?? null) };
        }
        items[item.id] = { fields: fmap, extra: item.extra || [] };
      }
      collections[col.id] = { items, order, deleted: [], renamed: {} };
    }
    return { fields, collections, raw: doc.raw || '' };
  }

  function markDirty() {
    state.dirty = true;
    renderNav();
  }

  function matchesQuery(text) {
    const q = state.query.trim().toLowerCase();
    if (!q) return true;
    return String(text || '').toLowerCase().includes(q);
  }

  function fieldMatches(f) {
    return matchesQuery([f.label, f.key, f.description, f.value].join(' '));
  }

  function listToText(v) {
    if (v == null || v === '') return '';
    if (Array.isArray(v)) return v.join('\n');
    return String(v);
  }

  function mapToText(v) {
    if (!v || typeof v !== 'object' || Array.isArray(v)) return '';
    return Object.entries(v).map(([k, val]) => `${k} = ${val}`).join('\n');
  }

  function textToList(s) {
    return String(s || '').split('\n').map(x => x.trim()).filter(Boolean);
  }

  function textToMap(s) {
    const out = {};
    for (const line of String(s || '').split('\n')) {
      const t = line.trim();
      if (!t) continue;
      let i = t.indexOf('=');
      if (i < 0) i = t.indexOf(':');
      if (i < 0) continue;
      const k = t.slice(0, i).trim();
      const val = t.slice(i + 1).trim();
      if (!k) continue;
      if (val === 'true') out[k] = true;
      else if (val === 'false') out[k] = false;
      else out[k] = val;
    }
    return out;
  }

  function textToIntMap(s) {
    const out = {};
    for (const [k, val] of Object.entries(textToMap(s))) {
      const n = Number(val);
      if (Number.isFinite(n)) out[k] = n;
    }
    return out;
  }

  function displayValue(f, slot) {
    if (slot.set) return slot.value;
    return f.default;
  }

  function renderNav() {
    const nav = $('#settings-nav');
    if (!nav || !state.doc) return;
    const active = nav.querySelector('button.active');
    const prev = active ? (active.dataset.tab || '') : 'webui';
    nav.innerHTML = '';
    const mk = (tab, group, label) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.dataset.tab = tab;
      if (group) b.dataset.group = group;
      b.textContent = label + (state.dirty && tab !== 'webui' ? ' ·' : '');
      if ((tab === 'webui' && state.tab === 'webui') ||
          (tab === 'raw' && state.tab === 'raw') ||
          (tab === 'grok' && state.tab === 'grok' && state.group === group)) {
        b.classList.add('active');
      }
      b.onclick = () => selectTab(tab, group);
      nav.appendChild(b);
    };
    mk('webui', '', 'WebUI');
    for (const g of state.doc.groups || []) mk('grok', g.id, g.title);
    mk('raw', '', 'Raw TOML');
    if (!nav.querySelector('button.active') && prev) {
      const fallback = nav.querySelector(`[data-tab="${prev}"]`) || nav.querySelector('[data-tab="webui"]');
      if (fallback) fallback.classList.add('active');
    }
  }

  function selectTab(tab, group) {
    state.tab = tab;
    if (group) state.group = group;
    $('#settings-tab-webui').classList.toggle('hidden', tab !== 'webui');
    $('#settings-tab-grok').classList.toggle('hidden', tab !== 'grok');
    $('#settings-tab-raw').classList.toggle('hidden', tab !== 'raw');
    $('#btn-save-grok').textContent = tab === 'raw' ? 'Save raw TOML' : 'Save Grok config';
    if (tab === 'raw') $('#grok-raw').value = state.work.raw || '';
    if (tab === 'grok') renderGrok();
    renderNav();
  }

  function renderGrok() {
    const root = $('#grok-settings-root');
    if (!root || !state.doc) return;
    root.innerHTML = '';
    const group = (state.doc.groups || []).find(g => g.id === state.group);
    if (group && group.description && !state.query) {
      const p = document.createElement('p');
      p.className = 'muted small';
      p.textContent = group.description;
      root.appendChild(p);
    }
    for (const col of state.doc.collections || []) {
      if (col.group !== state.group) continue;
      root.appendChild(renderCollection(col));
    }
    for (const sec of state.doc.sections || []) {
      if (sec.group !== state.group) continue;
      const el = renderSection(sec);
      if (el) root.appendChild(el);
    }
    if (!root.children.length) {
      const empty = document.createElement('div');
      empty.className = 'gempty';
      empty.textContent = state.query ? 'No settings match this filter.' : 'Nothing in this section.';
      root.appendChild(empty);
    }
  }

  function renderSection(sec) {
    const wrap = document.createElement('div');
    wrap.className = 'gsection';
    const fields = sec.fields.filter(f => {
      const slot = state.work.fields[f.key];
      if (!state.showUnset && slot && !slot.set) return false;
      return fieldMatches(f) || matchesQuery(sec.title);
    });
    if (!fields.length && state.query) return null;
    wrap.innerHTML = `<h3>${esc(sec.title)}</h3>`;
    if (sec.description) {
      const d = document.createElement('p');
      d.className = 'muted small desc';
      d.textContent = sec.description;
      wrap.appendChild(d);
    }
    const list = document.createElement('div');
    list.className = 'gfields';
    for (const f of fields) {
      list.appendChild(renderField(f, state.work.fields[f.key], (slot) => {
        state.work.fields[f.key] = slot;
        markDirty();
      }));
    }
    wrap.appendChild(list);
    return wrap;
  }

  function renderCollection(col) {
    const wrap = document.createElement('div');
    wrap.className = 'gsection';
    wrap.innerHTML = `<h3>${esc(col.title)}</h3>`;
    if (col.description) {
      const d = document.createElement('p');
      d.className = 'muted small desc';
      d.textContent = col.description;
      wrap.appendChild(d);
    }

    const add = document.createElement('div');
    add.className = 'add-row';
    const idInput = document.createElement('input');
    idInput.placeholder = col.key_label || 'id';
    const tpl = document.createElement('select');
    const opt0 = document.createElement('option');
    opt0.value = '';
    opt0.textContent = 'Blank';
    tpl.appendChild(opt0);
    for (const t of col.templates || []) {
      const o = document.createElement('option');
      o.value = t.id;
      o.textContent = t.label;
      tpl.appendChild(o);
    }
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'primary small';
    btn.textContent = 'Add';
    btn.onclick = () => {
      let id = idInput.value.trim();
      const t = (col.templates || []).find(x => x.id === tpl.value);
      if (!id) id = (t && t.suggested_id) || '';
      if (!id) {
        idInput.focus();
        return;
      }
      addItem(col, id, t);
      idInput.value = '';
    };
    add.appendChild(idInput);
    add.appendChild(tpl);
    add.appendChild(btn);
    wrap.appendChild(add);

    const bag = state.work.collections[col.id];
    const ids = (bag.order || []).filter(id => !bag.deleted.includes(id));
    const q = state.query.trim().toLowerCase();
    const shown = ids.filter(id => {
      if (!q) return true;
      const item = bag.items[id];
      const name = item && item.fields.name && item.fields.name.value;
      return id.toLowerCase().includes(q) || String(name || '').toLowerCase().includes(q);
    });
    if (!shown.length) {
      const empty = document.createElement('div');
      empty.className = 'gempty';
      empty.textContent = ids.length ? 'No entries match this filter.' : 'None yet — add one above.';
      wrap.appendChild(empty);
    }
    for (const id of shown) wrap.appendChild(renderItem(col, id));
    return wrap;
  }

  function addItem(col, id, template) {
    const bag = state.work.collections[col.id];
    if (bag.items[id] && !bag.deleted.includes(id)) {
      toastLocal('That id already exists');
      return;
    }
    bag.deleted = bag.deleted.filter(x => x !== id);
    const fmap = {};
    for (const f of col.item_fields) {
      fmap[f.key] = { set: false, value: clone(f.default ?? null) };
    }
    if (template && template.values) {
      for (const [k, v] of Object.entries(template.values)) {
        fmap[k] = { set: true, value: clone(v) };
      }
    }
    bag.items[id] = { fields: fmap, extra: [] };
    if (!bag.order.includes(id)) bag.order.push(id);
    state.expanded[col.id + ':' + id] = true;
    markDirty();
    renderGrok();
  }

  function renderItem(col, id) {
    const bag = state.work.collections[col.id];
    const item = bag.items[id];
    const key = col.id + ':' + id;
    const open = state.expanded[key] !== false;
    const card = document.createElement('div');
    card.className = 'gcard' + (open ? '' : ' collapsed');
    const head = document.createElement('div');
    head.className = 'gcard-head';
    const title = document.createElement('span');
    title.className = 'id';
    title.textContent = id;
    const name = item.fields.name && item.fields.name.set ? item.fields.name.value : (item.fields.model && item.fields.model.value);
    const sub = document.createElement('span');
    sub.className = 'sub';
    const setCount = Object.values(item.fields).filter(s => s.set).length;
    sub.textContent = (name ? name + ' · ' : '') + setCount + ' set';
    const del = document.createElement('button');
    del.type = 'button';
    del.className = 'small';
    del.textContent = 'Delete';
    del.onclick = (e) => {
      e.stopPropagation();
      if (!confirm(`Remove ${col.key_label} “${id}”?`)) return;
      bag.deleted.push(id);
      markDirty();
      renderGrok();
    };
    head.appendChild(title);
    head.appendChild(sub);
    head.appendChild(del);
    head.onclick = (e) => {
      if (e.target === del) return;
      state.expanded[key] = !!card.classList.contains('collapsed');
      card.classList.toggle('collapsed');
    };
    const body = document.createElement('div');
    body.className = 'gcard-body';

    const idRow = document.createElement('label');
    idRow.textContent = col.key_label;
    const idInput = document.createElement('input');
    idInput.value = id;
    idInput.onchange = () => {
      const next = idInput.value.trim();
      if (!next || next === id) return;
      if (bag.items[next] && !bag.deleted.includes(next)) {
        toastLocal('That id already exists');
        idInput.value = id;
        return;
      }
      bag.items[next] = bag.items[id];
      delete bag.items[id];
      bag.order = bag.order.map(x => x === id ? next : x);
      if (origItem(col.id, id)) bag.renamed[id] = next;
      state.expanded[col.id + ':' + next] = true;
      markDirty();
      renderGrok();
    };
    idRow.appendChild(idInput);
    body.appendChild(idRow);

    const list = document.createElement('div');
    list.className = 'gfields';
    for (const f of col.item_fields) {
      const slot = item.fields[f.key] || { set: false, value: f.default ?? null };
      if (!state.showUnset && !slot.set && !state.query) continue;
      if (state.query && !fieldMatches(f) && !matchesQuery(id)) continue;
      list.appendChild(renderField(f, slot, (next) => {
        item.fields[f.key] = next;
        markDirty();
      }));
    }
    body.appendChild(list);
    card.appendChild(head);
    card.appendChild(body);
    return card;
  }

  function renderField(f, slot, onChange) {
    const wrap = document.createElement('div');
    wrap.className = 'gfield ' + (slot.set ? 'set' : 'unset');
    const meta = document.createElement('div');
    meta.className = 'meta';
    meta.innerHTML = `<span class="lab">${esc(f.label)}</span>` +
      (f.description ? `<span class="help">${esc(f.description)}</span>` : '') +
      `<span class="help"><code>${esc(f.key)}</code></span>`;
    const control = document.createElement('div');
    control.className = 'control';
    const actions = document.createElement('div');
    actions.className = 'actions';
    const badge = document.createElement('span');
    badge.className = 'badge ' + (slot.set ? 'set' : 'default');
    badge.textContent = slot.set ? 'Set' : 'Default';
    const reset = document.createElement('button');
    reset.type = 'button';
    reset.className = 'reset';
    reset.textContent = 'Use default';
    reset.disabled = !slot.set;
    reset.onclick = () => {
      onChange({ set: false, value: clone(f.default ?? null) });
      renderCurrent();
    };

    const val = displayValue(f, slot);
    const emit = (value, set) => {
      onChange({ set, value });
      wrap.className = 'gfield ' + (set ? 'set' : 'unset');
      badge.className = 'badge ' + (set ? 'set' : 'default');
      badge.textContent = set ? 'Set' : 'Default';
      reset.disabled = !set;
    };

    if (f.type === 'bool') {
      const lab = document.createElement('label');
      lab.className = 'check';
      const inp = document.createElement('input');
      inp.type = 'checkbox';
      inp.checked = !!val;
      inp.onchange = () => emit(inp.checked, true);
      lab.appendChild(inp);
      lab.appendChild(document.createTextNode(inp.checked ? 'On' : 'Off'));
      inp.addEventListener('change', () => {
        lab.lastChild.textContent = inp.checked ? 'On' : 'Off';
      });
      control.appendChild(lab);
    } else if (f.type === 'enum') {
      const sel = document.createElement('select');
      const blank = document.createElement('option');
      blank.value = '';
      blank.textContent = slot.set ? '(choose)' : `Default (${f.default == null ? 'unset' : f.default})`;
      sel.appendChild(blank);
      for (const o of f.options || []) {
        const opt = document.createElement('option');
        opt.value = o;
        opt.textContent = o;
        sel.appendChild(opt);
      }
      sel.value = slot.set ? (val ?? '') : '';
      sel.onchange = () => {
        if (!sel.value) emit(clone(f.default ?? null), false);
        else emit(sel.value, true);
      };
      control.appendChild(sel);
    } else if (f.type === 'string_list') {
      const ta = document.createElement('textarea');
      ta.placeholder = 'One value per line';
      ta.value = listToText(val);
      ta.oninput = () => emit(textToList(ta.value), true);
      control.appendChild(ta);
    } else if (f.type === 'map' || f.type === 'int_map') {
      const ta = document.createElement('textarea');
      ta.placeholder = 'KEY = value';
      ta.value = mapToText(val);
      ta.oninput = () => emit(f.type === 'int_map' ? textToIntMap(ta.value) : textToMap(ta.value), true);
      control.appendChild(ta);
    } else if (f.type === 'int' || f.type === 'float') {
      const inp = document.createElement('input');
      inp.type = 'number';
      if (f.type === 'int') inp.step = '1';
      else inp.step = 'any';
      if (f.min != null) inp.min = f.min;
      if (f.max != null) inp.max = f.max;
      if (f.placeholder) inp.placeholder = f.placeholder;
      else if (f.default != null) inp.placeholder = String(f.default);
      inp.value = val == null ? '' : val;
      inp.oninput = () => {
        if (inp.value === '') {
          emit(clone(f.default ?? null), false);
          return;
        }
        emit(f.type === 'int' ? Number(inp.value) : Number(inp.value), true);
      };
      control.appendChild(inp);
    } else {
      const inp = document.createElement('input');
      inp.type = f.secret ? 'password' : 'text';
      inp.autocomplete = 'off';
      if (f.placeholder) inp.placeholder = f.placeholder;
      else if (f.default != null) inp.placeholder = String(f.default);
      inp.value = val == null ? '' : String(val);
      inp.oninput = () => {
        if (inp.value === '') emit(clone(f.default ?? null), false);
        else emit(inp.value, true);
      };
      if (f.secret) {
        const row = document.createElement('div');
        row.className = 'secret-row';
        const tog = document.createElement('button');
        tog.type = 'button';
        tog.className = 'small';
        tog.textContent = 'Show';
        tog.onclick = () => {
          inp.type = inp.type === 'password' ? 'text' : 'password';
          tog.textContent = inp.type === 'password' ? 'Show' : 'Hide';
        };
        row.appendChild(inp);
        row.appendChild(tog);
        control.appendChild(row);
      } else {
        control.appendChild(inp);
      }
    }

    actions.appendChild(badge);
    actions.appendChild(reset);
    wrap.appendChild(meta);
    wrap.appendChild(control);
    wrap.appendChild(actions);
    return wrap;
  }

  function renderCurrent() {
    if (state.tab === 'grok') renderGrok();
    else if (state.tab === 'raw') $('#grok-raw').value = state.work.raw || '';
    renderNav();
  }

  function buildPatch() {
    const set = {};
    const unset = [];
    for (const sec of state.doc.sections) {
      for (const f of sec.fields) {
        const slot = state.work.fields[f.key];
        const orig = origField(f.key);
        if (!slot.set) {
          if (orig && orig.set) unset.push(f.key);
          continue;
        }
        if (!orig || !orig.set || !eq(orig.value, slot.value)) set[f.key] = slot.value;
      }
    }
    const collections = {};
    for (const col of state.doc.collections) {
      const bag = state.work.collections[col.id];
      const cp = { items: {}, delete: bag.deleted.slice(), rename: { ...bag.renamed } };
      for (const id of bag.order) {
        if (bag.deleted.includes(id)) continue;
        const item = bag.items[id];
        let origId = id;
        for (const [from, to] of Object.entries(bag.renamed)) {
          if (to === id) origId = from;
        }
        const orig = origItem(col.id, origId);
        const ip = { set: {}, unset: [] };
        for (const f of col.item_fields) {
          const slot = item.fields[f.key] || { set: false };
          const of = orig ? orig.fields.find(x => x.key === f.key) : null;
          if (!slot.set) {
            if (of && of.set) ip.unset.push(f.key);
            continue;
          }
          if (!of || !of.set || !eq(of.value, slot.value)) ip.set[f.key] = slot.value;
        }
        if (!orig) {
          // brand new item: send every set field
          ip.unset = [];
        }
        if (Object.keys(ip.set).length || ip.unset.length || !orig) {
          cp.items[id] = ip;
        }
      }
      if (Object.keys(cp.items).length || cp.delete.length || Object.keys(cp.rename).length) {
        collections[col.id] = cp;
      }
    }
    return {
      set,
      unset,
      collections,
      if_match_mtime: state.doc.mtime || '',
    };
  }

  function toastLocal(text) {
    const el = $('#grok-msg');
    if (!el) return;
    el.textContent = text;
    el.className = 'msg error';
  }

  async function load() {
    const doc = await window.api('/api/settings/grok');
    state.doc = doc;
    state.work = fromDoc(doc);
    state.dirty = false;
    state.expanded = {};
    $('#grok-path').textContent = doc.path || '';
    $('#grok-raw').value = doc.raw || '';
    $('#grok-msg').textContent = '';
    $('#grok-msg').className = 'msg';
    renderNav();
    if (state.tab === 'grok') renderGrok();
    if (state.tab === 'raw') $('#grok-raw').value = state.work.raw || '';
    return doc;
  }

  async function save() {
    if (!state.doc || !state.work) return;
    const el = $('#grok-msg');
    el.textContent = '';
    try {
      let body;
      if (state.tab === 'raw') {
        body = { raw: $('#grok-raw').value, if_match_mtime: state.doc.mtime || '' };
      } else {
        body = buildPatch();
      }
      const doc = await window.api('/api/settings/grok', { method: 'PUT', body: JSON.stringify(body) });
      state.doc = doc;
      state.work = fromDoc(doc);
      state.dirty = false;
      $('#grok-path').textContent = doc.path || '';
      $('#grok-raw').value = doc.raw || '';
      el.textContent = 'Saved. New Grok sessions will use this config.';
      el.className = 'msg ok';
      renderCurrent();
      // Saving config.toml can change ui.theme; retint the WebUI right away.
      if (window.GrokBuildTheme) window.GrokBuildTheme.refresh();
    } catch (e) {
      el.textContent = e.message || String(e);
      el.className = 'msg error';
    }
  }

  let bound = false;
  function bind() {
    if (bound) return;
    bound = true;
    const search = $('#grok-search');
    if (search) {
      search.oninput = () => {
        state.query = search.value;
        if (state.tab === 'grok') renderGrok();
      };
    }
    const show = $('#grok-show-unset');
    if (show) {
      show.onchange = () => {
        state.showUnset = show.checked;
        if (state.tab === 'grok') renderGrok();
      };
    }
    $('#btn-save-grok').onclick = save;
    $('#btn-revert-grok').onclick = async () => {
      if (state.dirty && !confirm('Discard unsaved Grok config changes?')) return;
      try { await load(); if (window.GrokBuildTheme) window.GrokBuildTheme.refresh(); } catch (e) { toastLocal(e.message); }
    };
    $('#grok-raw').addEventListener('input', () => {
      if (!state.work) return;
      state.work.raw = $('#grok-raw').value;
      markDirty();
    });
  }

  function esc(s) {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
  }

  window.GrokSettings = {
    load,
    isDirty: () => state.dirty,
    selectWebUI: () => selectTab('webui'),
    init: bind,
  };
})();
