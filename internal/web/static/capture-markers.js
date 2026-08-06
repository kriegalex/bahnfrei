/* SPDX-License-Identifier: AGPL-3.0-only */
/* Copyright (c) 2026 Bahnfrei contributors */

/* Letter-marker quick-action buttons (SYS-147, UC-039 #3, TASK-045): the
 * field-horizontal and vertical-jump capture grids' value inputs declare
 * inputmode="decimal" or inputmode="none" so a numeric (or no) virtual
 * keyboard shows instead of the full alphabet — these buttons are the
 * always-available alternate path for the non-numeric D5.2 markers (X
 * foul, – pass, r retirement, o clear) so every marker stays enterable
 * without ever needing the alpha keyboard.
 *
 * Deliberately a plain, family-agnostic script rather than folded into
 * either capture.js (SSE standings refresh, not loaded on the vertical
 * page today) or the capture-offline.ts island (field-horizontal only):
 * clicking a marker button just sets the sibling value input and
 * re-submits the enclosing form via requestSubmit(), so the SAME markup
 * works whether that submit is a plain synchronous POST (track/vertical)
 * or intercepted by the offline-capture island (field horizontal, whose
 * own submit listener runs exactly as if the operator had typed the
 * marker and pressed Save). Served as a static file because the CSP
 * forbids inline scripts.
 */
(function () {
  "use strict";
  document.querySelectorAll(".marker-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var form = btn.closest("form");
      if (!form) {
        return;
      }
      var input = form.querySelector('input[name="value"]');
      if (!input) {
        return;
      }
      input.value = btn.dataset.marker || "";
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
      } else {
        form.submit();
      }
    });
  });
})();
