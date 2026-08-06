// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors
//
// HTTP-level seeding for the UC-034 suite, driving the same operator forms
// the Go web tests use (internal/web/capture_test.go's ukcCaptureFixture):
// first-run setup, login, a UBS Kids Cup template meet, two athletes. No
// test-only server hooks — everything goes through real product endpoints.
// Uses the browser context's own APIRequestContext so cookies (session,
// CSRF) are shared with the page.
import { expect, type APIRequestContext } from "@playwright/test";

export const ADMIN = { username: "admin", password: "s3cret-passphrase" };

export function csrfFrom(html: string): string {
  const m = html.match(/name="csrf_token" value="([^"]+)"/);
  if (!m) {
    throw new Error("csrf_token hidden field not found");
  }
  return m[1];
}

async function getBody(request: APIRequestContext, url: string): Promise<string> {
  const res = await request.get(url);
  expect(res.ok(), `GET ${url} -> ${res.status()}`).toBe(true);
  return res.text();
}

async function postForm(
  request: APIRequestContext,
  csrfPage: string,
  target: string,
  form: Record<string, string>,
): Promise<string> {
  const csrf = csrfFrom(await getBody(request, csrfPage));
  const res = await request.post(target, {
    form: { ...form, csrf_token: csrf },
    maxRedirects: 0,
  });
  expect(res.status(), `POST ${target} -> ${res.status()}`).toBe(303);
  return res.headers()["location"] ?? "";
}

/** First-run setup + login as the admin account (all roles). */
export async function setupAndLogin(
  request: APIRequestContext,
  baseURL: string,
): Promise<void> {
  // Setup only exists while no account does (it redirects to /login once
  // bootstrapped); a second context just logs in.
  const setup = await request.get(baseURL + "/setup");
  if (setup.ok() && setup.url().includes("/setup")) {
    const res = await request.post(baseURL + "/setup", {
      form: {
        username: ADMIN.username,
        display_name: "Administrator",
        password: ADMIN.password,
        csrf_token: csrfFrom(await setup.text()),
      },
      maxRedirects: 0,
    });
    expect(res.status()).toBe(303);
  }
  await postForm(request, baseURL + "/login", baseURL + "/login", {
    username: ADMIN.username,
    password: ADMIN.password,
  });
}

export interface UkcFixture {
  meetID: string;
  /** localized discipline name -> unit ID (e.g. "Zonen-Weitsprung (UKC)",
   *  the DE catalog string — SYS-111/F6, TASK-050 localized the capture
   *  index that this map is scraped from). */
  units: Record<string, string>;
  /** bib -> athlete ID. */
  athletes: Record<string, string>;
  unitURL: string; // the zone long jump capture page (the offline surface)
}

/** Creates a UKC template meet with two W12 girls and maps its units. */
export async function seedUkcMeet(
  request: APIRequestContext,
  baseURL: string,
): Promise<UkcFixture> {
  const location = await postForm(
    request,
    baseURL + "/meets/from-template",
    baseURL + "/meets/from-template",
    { template: "ubs-kids-cup", date: "2026-08-15", venue: "Le Mouret" },
  );
  const meetID = location.replace("/meets/", "");
  expect(meetID).toMatch(/^[0-9A-Za-z]+$/);

  for (const athlete of [
    { first_name: "Anna", last_name: "Muster", birth_year: "2014", sex: "W", bib: "101" },
    { first_name: "Bea", last_name: "Beispiel", birth_year: "2014", sex: "W", bib: "102" },
  ]) {
    await postForm(
      request,
      `${baseURL}/meets/${meetID}/roster`,
      `${baseURL}/meets/${meetID}/roster`,
      athlete,
    );
  }

  const index = await getBody(request, `${baseURL}/meets/${meetID}/capture`);
  const units: Record<string, string> = {};
  for (const m of index.matchAll(
    new RegExp(`/meets/${meetID}/capture/([0-9A-Za-z]+)">([^<]+)<`, "g"),
  )) {
    units[m[2]] = m[1];
  }
  expect(Object.keys(units), "the 3 UKC disciplines").toHaveLength(3);

  const unitID = units["Zonen-Weitsprung (UKC)"];
  const unitURL = `${baseURL}/meets/${meetID}/capture/${unitID}`;
  const page = await getBody(request, unitURL);
  const athletes: Record<string, string> = {};
  for (const m of page.matchAll(
    /<tr>\s*<td>(\d+)<\/td>[\s\S]*?name="athlete" value="([0-9A-Za-z]+)"/g,
  )) {
    athletes[m[1]] = m[2];
  }
  expect(Object.keys(athletes).length).toBeGreaterThan(0);
  return { meetID, units, athletes, unitURL };
}

export interface TrackMeetFixture {
  meetID: string;
  eventID: string;
  /** office check-in page for the 100m event (UC-007). */
  checkinURL: string;
  /** field-official capture page for the 100m unit (UC-010). */
  unitURL: string;
}

/**
 * Creates a plain (non-template) meet with one 100m/U18 W final event,
 * publishes it, and submits one individual online entry per name (landing
 * in status "entered" — not yet checked in). Mirrors
 * internal/web/seeding_test.go's TestCheckInFlowHTTPSYS025UC007 fixture, but
 * entirely over HTTP (no direct app-layer calls) so it can seed a browser
 * context's cookies. Used by the TASK-030 keyboard-only suite (SYS-114):
 * UC-007 check-in and UC-010 track result entry both need a real event with
 * real entries, which the UBS Kids Cup template's simplified roster-only
 * flow (seedUkcMeet) does not exercise (it has no check-in step at all).
 */
export async function seedTrackMeet(
  request: APIRequestContext,
  baseURL: string,
  names: string[],
): Promise<TrackMeetFixture> {
  const location = await postForm(request, baseURL + "/meets/new", baseURL + "/meets", {
    name: "Abendmeeting Uster",
    venue: "Stadion Buchholz",
    homologation_ref: "CH-ZH-042",
    start_date: "2027-06-12",
    end_date: "2027-06-13",
    tier: "C-Meeting",
    scheme: "swiss-athletics",
    session_day_0: "2027-06-12",
    session_label_0: "Session 1",
  });
  const meetID = location.replace("/meets/", "");
  expect(meetID).toMatch(/^[0-9A-Za-z]+$/);
  const meetPage = `${baseURL}/meets/${meetID}`;

  await postForm(request, meetPage, `${meetPage}/events`, {
    discipline: "100m",
    categories: "U18 W",
    round_final: "1",
  });
  await postForm(request, meetPage, `${meetPage}/publish`, { version: "1" });

  const entriesPage = `${meetPage}/entries`;
  const entriesHTML = await getBody(request, entriesPage);
  const eventMatch = entriesHTML.match(/<select name="event"><option value="([0-9A-Za-z]+)"/);
  if (!eventMatch) {
    throw new Error("entries page missing the event <select> option (100m)");
  }
  const eventID = eventMatch[1];

  for (const name of names) {
    await postForm(request, entriesPage, `${meetPage}/entries/individual`, {
      event: eventID,
      first_name: name,
      last_name: "Test",
      birth_year: "2005",
      sex: "W",
      seed: "13.50",
    });
  }

  const captureIndex = await getBody(request, `${meetPage}/capture`);
  // The capture index localizes discipline names (SYS-111/F6, TASK-050):
  // the DE catalog renders code "100m" as "100 m", not the catalog's
  // canonical English "100 metres".
  const unitMatch = captureIndex.match(
    new RegExp(`/meets/${meetID}/capture/([0-9A-Za-z]+)">100 m<`),
  );
  if (!unitMatch) {
    throw new Error("capture index missing the 100m unit");
  }

  return {
    meetID,
    eventID,
    checkinURL: `${meetPage}/events/${eventID}/checkin`,
    unitURL: `${meetPage}/capture/${unitMatch[1]}`,
  };
}

/**
 * Schedules one unit and publishes the timetable (mirrors
 * internal/web/public_test.go's scheduleAndPublishTimetable), so the
 * public timetable page (GET /m/{id}/timetable) has real content instead
 * of 404ing — used by the accessibility audit (SYS-112/113, UC-026),
 * which needs every public page type populated, not just the ones the
 * capture-flow fixtures happen to touch.
 */
export async function publishTimetable(
  request: APIRequestContext,
  baseURL: string,
  meetID: string,
  unitID: string,
): Promise<void> {
  const meetPage = `${baseURL}/meets/${meetID}`;
  await postForm(request, meetPage, `${meetPage}/units/${unitID}/schedule`, {
    scheduled_at: "2027-06-12T14:30",
    location: "Bahn 1",
    version: "1",
  });
  await postForm(request, meetPage, `${meetPage}/timetable/publish`, {});
}
