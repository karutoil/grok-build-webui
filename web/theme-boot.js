// Apply the cached WebUI theme before first paint to avoid a flash of the
// default palette; app.js re-syncs it against config.toml shortly after.
// (External file rather than an inline script so script-src 'self' holds.)
try { var __t = localStorage.getItem('gbw.theme'); if (__t) document.documentElement.dataset.theme = __t; } catch (e) {}
