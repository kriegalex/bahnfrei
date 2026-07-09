// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Office blip-tolerance island (TASK-009, UC-034 #7; SYS-087). A small shared
// enhancement for every operator surface: when connectivity drops (the
// `offline` event, or a failed HTMX/fetch request) it shows an i18n'd
// degraded-state banner, and it blocks plain-form submits while offline so a
// failed navigation can never clear the operator's typed input. On reconnect
// (the `online` event, or the next successful request) the banner clears and
// normal operation resumes — no restart, no re-login, no re-entry. It never
// touches the capture grid: that surface has its own offline island.
//
// Compiled to /static/office-banner.js and embedded; the CSP forbids inline
// scripts. Wrapped in an IIFE so it declares no globals.
(function () {
  "use strict";

  const banner = document.getElementById("offline-banner");
  if (!banner) {
    return; // only rendered on authenticated operator surfaces
  }

  function show(): void {
    banner!.hidden = false;
    banner!.dataset.state = "degraded";
  }
  function hide(): void {
    banner!.hidden = true;
    banner!.dataset.state = "ok";
  }

  // A form the capture island owns (the attempt grid): leave it alone.
  function isCaptureForm(form: HTMLElement): boolean {
    return form.classList.contains("cell-form") || !!form.closest(".capture-grid");
  }

  // Block plain-form submits while offline so the page never navigates to a
  // failed request and loses the typed input (SYS-087: no data re-entry). The
  // values stay in the DOM; the operator submits again once reconnected.
  document.addEventListener(
    "submit",
    (ev) => {
      if (navigator.onLine) {
        return;
      }
      const form = ev.target as HTMLElement | null;
      if (!form || isCaptureForm(form)) {
        return;
      }
      ev.preventDefault();
      show();
    },
    true,
  );

  window.addEventListener("offline", show);
  window.addEventListener("online", hide);

  // HTMX transport errors (network down / server unreachable) degrade too;
  // a subsequent successful request restores the surface.
  document.body.addEventListener("htmx:sendError", show);
  document.body.addEventListener("htmx:responseError", show);
  document.body.addEventListener("htmx:afterRequest", (ev) => {
    const detail = (ev as CustomEvent).detail as { successful?: boolean } | undefined;
    if (detail && detail.successful && navigator.onLine) {
      hide();
    }
  });

  if (!navigator.onLine) {
    show();
  }
})();
