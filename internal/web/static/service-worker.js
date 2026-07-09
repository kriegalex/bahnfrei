"use strict";
// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Capture-surface service worker (TASK-009, UC-034 #3; ADR-004 §8). Scoped
// tightly to the field-capture pages (registered with scope /meets/…/capture/)
// so a reload or browser restart while offline still serves the capture page
// and its static assets. The durable capture queue lives in IndexedDB and
// survives independently of this cache.
//
// Strategy: network-first with cache fallback. Online, every response comes
// from the server (the source of truth) and successful navigations + static
// assets are cached; offline, the cached copy is served. GET only — the JSON
// checkout/sync POSTs and the SSE stream are never intercepted, so normal
// online HTMX/SSE behaviour is untouched. Compiled to
// /static/service-worker.js and served at /capture-sw.js (a root-path URL is
// required to claim a /meets/ scope).
//
// Its own tsconfig (WebWorker lib) — the DOM lib the other islands use is
// incompatible in a single TS program.
/// <reference lib="webworker" />
const sw = self;
const CACHE = "bahnfrei-capture-v1";
const ASSETS = [
    "/static/htmx.min.js",
    "/static/base.css",
    "/static/capture.js",
    "/static/capture-offline.js",
    "/static/office-banner.js",
];
sw.addEventListener("install", (event) => {
    const e = event;
    e.waitUntil(caches
        .open(CACHE)
        .then((cache) => cache.addAll(ASSETS))
        .then(() => sw.skipWaiting()));
});
sw.addEventListener("activate", (event) => {
    const e = event;
    e.waitUntil(caches
        .keys()
        .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
        .then(() => sw.clients.claim()));
});
// The island posts {type: "cache-page", url} right after registration: the
// first navigation happened before this worker controlled the page, so the
// capture page must be cached explicitly for an offline reload immediately
// after the first visit to work (UC-034 #3). The fetch carries the session
// cookie (same-origin credentials), matching what a navigation would send.
sw.addEventListener("message", (event) => {
    const e = event;
    const data = e.data;
    if (!data || data.type !== "cache-page" || typeof data.url !== "string") {
        return;
    }
    const url = new URL(data.url, sw.location.origin);
    if (url.origin !== sw.location.origin) {
        return;
    }
    e.waitUntil(caches
        .open(CACHE)
        .then((cache) => cache.add(new Request(url.href, { credentials: "same-origin" })))
        .catch(() => {
        /* caching the page is best-effort; a later online reload also caches */
    }));
});
function shouldCache(request, url) {
    return request.mode === "navigate" || url.pathname.startsWith("/static/");
}
async function networkFirst(request) {
    try {
        const res = await fetch(request);
        if (res && res.ok) {
            const url = new URL(request.url);
            if (shouldCache(request, url)) {
                const cache = await caches.open(CACHE);
                void cache.put(request, res.clone());
            }
        }
        return res;
    }
    catch (err) {
        const cached = await caches.match(request);
        if (cached) {
            return cached;
        }
        throw err;
    }
}
sw.addEventListener("fetch", (event) => {
    const e = event;
    const request = e.request;
    if (request.method !== "GET") {
        return; // checkout/sync POSTs go straight to the network
    }
    const url = new URL(request.url);
    if (url.origin !== sw.location.origin) {
        return;
    }
    if (url.pathname.startsWith("/events/")) {
        return; // SSE stream: default handling, never buffered/cached
    }
    e.respondWith(networkFirst(request));
});
