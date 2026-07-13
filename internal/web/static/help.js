"use strict";
// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// Contextual-help island (TASK-031, SYS-115, UC-037): drives the help-icon
// component's open/close behavior for every `.help` instance on the page
// (component markup: help.templ). Served as a static asset because the CSP
// (middleware.go) forbids inline scripts; wrapped in an IIFE so it declares
// no globals.
//
// Behavior contract (SYS-115 + WCAG 2.2 SC 1.4.13, verified by
// e2e/tests/contextual-help-UC037.spec.ts):
//   - opens on pointer hover, keyboard focus, AND click/tap (never
//     hover-only, SYS-113);
//   - hoverable: the popup is a DOM child of the `.help` wrapper, so
//     moving the pointer from the trigger onto the popup never leaves the
//     wrapper and the popup stays open;
//   - persistent: hover/focus-opened popups stay until de-hover/blur;
//     click/tap-opened ("sticky", the touch path) popups stay until
//     Escape, a second activation, or a click outside;
//   - dismissible: Escape closes the open popup WITHOUT moving focus
//     (nothing here ever calls .focus()/.blur()).
(function () {
	"use strict";
	// The single open popup: { root, sticky } or null. One at a time —
	// opening another help closes the previous (NN/g: tooltips are
	// supplementary, never a second reading pane).
	let open = null;

	function popupOf(root) {
		return root.querySelector(".help-popup");
	}
	function show(root, sticky) {
		if (open && open.root !== root) {
			hide();
		}
		popupOf(root).hidden = false;
		root.querySelector(".help-trigger").setAttribute("aria-expanded", "true");
		open = { root: root, sticky: sticky || (open !== null && open.sticky) };
	}
	function hide() {
		if (!open) {
			return;
		}
		popupOf(open.root).hidden = true;
		open.root.querySelector(".help-trigger").setAttribute("aria-expanded", "false");
		open = null;
	}

	// Click/tap: toggle sticky mode on the trigger; a click anywhere else
	// dismisses a sticky popup (hover/focus popups close by their own
	// de-hover/blur rules below).
	document.addEventListener("click", function (ev) {
		const trigger = ev.target.closest(".help-trigger");
		if (trigger) {
			const root = trigger.closest(".help");
			if (open && open.root === root && open.sticky) {
				hide();
			} else {
				show(root, true);
			}
			return;
		}
		if (open && open.sticky && !open.root.contains(ev.target)) {
			hide();
		}
	});

	// Pointer hover (mouseover/mouseout bubble; mouseenter does not).
	document.addEventListener("mouseover", function (ev) {
		const root = ev.target.closest(".help");
		if (root && !(open && open.root === root)) {
			show(root, false);
		}
	});
	document.addEventListener("mouseout", function (ev) {
		if (!open || open.sticky) {
			return;
		}
		const root = ev.target.closest(".help");
		if (root === open.root && !(ev.relatedTarget && root.contains(ev.relatedTarget))) {
			hide();
		}
	});

	// Keyboard focus on the trigger.
	document.addEventListener("focusin", function (ev) {
		const trigger = ev.target.closest(".help-trigger");
		if (trigger) {
			show(trigger.closest(".help"), false);
		}
	});
	document.addEventListener("focusout", function (ev) {
		if (!open || open.sticky) {
			return;
		}
		if (open.root.contains(ev.target) && !(ev.relatedTarget && open.root.contains(ev.relatedTarget))) {
			hide();
		}
	});

	// Escape dismisses without moving focus (SC 1.4.13 "dismissible").
	document.addEventListener("keydown", function (ev) {
		if (ev.key === "Escape" && open) {
			hide();
			ev.stopPropagation();
		}
	});
})();
