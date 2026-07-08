/* SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2026 Bahnfrei contributors
 *
 * Live standings refresh for the capture page (UC-011 #4, SYS-071): the
 * element carrying data-sse-topic subscribes to the shell's SSE bus and,
 * on the named event, re-fetches its data-sse-refresh fragment. Only the
 * standings section refreshes — attempt forms are never touched, so an
 * update from another session cannot clobber input in progress. Served as
 * a static file because the CSP forbids inline scripts.
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
        /* transient fetch failures: keep the last good standings */
      });
  });
  window.addEventListener("beforeunload", function () {
    source.close();
  });
})();
