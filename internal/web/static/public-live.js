/* SPDX-License-Identifier: AGPL-3.0-only */
/* Copyright (c) 2026 Bahnfrei contributors */

/*
 * Live-results refresh for the public results page (UC-017 #1, SYS-071):
 * the element carrying data-sse-topic subscribes to the shell's public SSE
 * bus and, on the named event, re-fetches its data-sse-refresh fragment.
 * No authentication, no polling — a confirmed result reaches an
 * already-open public page as soon as the office saves it. Served as a
 * static file because the CSP forbids inline scripts.
 *
 * Deliberately a separate file from /static/capture.js (same generic
 * mechanism) so the public surface carries no dependency, conceptual or
 * otherwise, on the authenticated capture page's script.
 */
(function () {
  "use strict";
  var el = document.querySelector("[data-sse-topic]");
  if (!el || typeof EventSource === "undefined") {
    return;
  }
  var source = new EventSource("/events/" + el.dataset.sseTopic);
  source.addEventListener(el.dataset.sseEvent || "message", function () {
    fetch(el.dataset.sseRefresh, { headers: { Accept: "text/html" } })
      .then(function (res) {
        return res.ok ? res.text() : Promise.reject(res.status);
      })
      .then(function (html) {
        el.innerHTML = html;
      })
      .catch(function () {
        /* transient fetch failures: keep the last good results */
      });
  });
  window.addEventListener("beforeunload", function () {
    source.close();
  });
})();
