(function () {
  'use strict';

  function getToken() {
    const m = document.cookie.match(/(?:^|;\s*)token=([^;]+)/);
    return m ? m[1] : '';
  }

  function authHeader(headers) {
    headers = headers || {};
    const token = getToken();
    if (token) {
      headers['Authorization'] = 'Bearer ' + token;
    }
    return headers;
  }

  function apiFetch(url, options) {
    options = options || {};
    options.headers = authHeader(options.headers || {});
    return fetch(url, options).then(function (res) {
      if (res.status === 401) {
        window.location.href = '/login';
        throw new Error('unauthorized');
      }
      return res;
    });
  }

  function apiJSON(url, options) {
    return apiFetch(url, options).then(function (res) {
      return res.json();
    });
  }

  function startSSE(url, handlers) {
    const token = getToken();
    const fullUrl = url + (url.indexOf('?') >= 0 ? '&' : '?') + 'token=' + encodeURIComponent(token);
    const es = new EventSource(fullUrl);
    Object.keys(handlers || {}).forEach(function (type) {
      es.addEventListener(type, function (e) {
        try {
          const data = JSON.parse(e.data);
          handlers[type](data, e);
        } catch (err) {
          console.warn('SSE parse error', err);
        }
      });
    });
    return es;
  }

  function refreshPage() {
    window.location.reload();
  }

  function formatTime(s) {
    if (!s) return '';
    const t = new Date(s);
    if (isNaN(t.getTime())) return s;
    const pad = function (n) { return n < 10 ? '0' + n : '' + n; };
    return t.getFullYear() + '-' + pad(t.getMonth() + 1) + '-' + pad(t.getDate()) +
      ' ' + pad(t.getHours()) + ':' + pad(t.getMinutes()) + ':' + pad(t.getSeconds());
  }

  function debounce(fn, wait) {
    let timer = null;
    return function () {
      const ctx = this, args = arguments;
      clearTimeout(timer);
      timer = setTimeout(function () { fn.apply(ctx, args); }, wait);
    };
  }

  function setupLogoutLink() {
    const link = document.getElementById('logoutLink');
    if (link) {
      link.addEventListener('click', function (e) {
        e.preventDefault();
        document.cookie = 'token=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT';
        window.location.href = '/login';
      });
    }
  }

  function setupAutoRefresh() {
    const el = document.querySelector('[data-auto-refresh]');
    if (!el) return;
    const interval = parseInt(el.getAttribute('data-auto-refresh'), 10) || 30;
    setTimeout(refreshPage, interval * 1000);
  }

  document.addEventListener('DOMContentLoaded', function () {
    setupLogoutLink();
    setupAutoRefresh();
  });

  window.App = {
    getToken: getToken,
    authHeader: authHeader,
    apiFetch: apiFetch,
    apiJSON: apiJSON,
    startSSE: startSSE,
    refreshPage: refreshPage,
    formatTime: formatTime,
    debounce: debounce,
  };
})();