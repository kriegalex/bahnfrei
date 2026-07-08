#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (c) 2026 Bahnfrei contributors
"""Extract the UBS Kids Cup scoring table from the official PDF into JSON.

Provenance tooling for `internal/domain/data/scoring/ubs-kids-cup.json`
(SYS-053 / CON-01: scoring tables are data, not code). Input is the official
"Swiss Athletics Wertungstabelle — Version UBS Kids Cup" PDF published at
https://www.ubs-kidscup.ch/fileadmin/ubskidscup/downloads/UBS_Kids_Cup_Wertungstabelle_Disziplinen.pdf

Usage:
    pdftotext -layout UBS_Kids_Cup_Wertungstabelle_Disziplinen.pdf table.txt
    scripts/extract-ukc-scoring-table.py table.txt > \
        internal/domain/data/scoring/ubs-kids-cup.json

The PDF lays each page out as a mirrored pair of blocks:
    points | 60m manual | 60m electronic | long jump | ball throw | points
for male (left) and female (right) — 12 physical columns. Cells are blank
when the same mark still maps to a higher points row (the table lists, for
every reachable mark, the highest points that mark earns; the Reglement's
"nächsttiefere Punktzahl" rule fills the gaps). Column x-positions shift
between pages, so tokens are clustered into columns per page by their end
position (the numbers are right-aligned).

The script hard-fails on any structural surprise: wrong cluster count,
mismatched points columns, non-monotonic marks, or gaps in the points
sequence 1100..1.
"""

import json
import re
import sys

COLUMNS = [
    # (sex, catalog discipline code, timing variant) in physical page order,
    # points columns omitted. Discipline codes match
    # internal/domain/data/disciplines/catalog.json.
    ("M", "60m", "manual"),
    ("M", "60m", "electronic"),
    ("M", "ZoneLJ", ""),
    ("M", "BallThrow200g", ""),
    ("W", "60m", "manual"),
    ("W", "60m", "electronic"),
    ("W", "ZoneLJ", ""),
    ("W", "BallThrow200g", ""),
]
# Physical cluster indexes: 0,5,6,11 are points; the rest map to COLUMNS.
VALUE_CLUSTERS = [1, 2, 3, 4, 7, 8, 9, 10]
POINTS_CLUSTERS = [0, 5, 6, 11]
NUM = re.compile(r"\S+")


def cluster_columns(lines):
    """Group token end-positions of one page's data lines into 12 columns."""
    ends = sorted({m.end() for line in lines for m in NUM.finditer(line)})
    clusters, cur = [], [ends[0]]
    for e in ends[1:]:
        if e - cur[-1] <= 3:  # right-aligned digits of one column
            cur.append(e)
        else:
            clusters.append(cur)
            cur = [e]
    clusters.append(cur)
    if len(clusters) != 12:
        sys.exit(f"expected 12 columns on page, found {len(clusters)}: {clusters}")
    return [(c[0], c[-1]) for c in clusters]


def parse(path):
    pages, page = [], []
    for line in open(path, encoding="utf-8"):
        if "Wertungstabelle" in line and page:
            pages.append(page)
            page = []
        # data rows start and end with the same integer points value
        if re.match(r"^\s*\d+\s+\S.*\s\d+\s*$", line.rstrip()):
            page.append(line.rstrip("\n"))
    pages.append(page)

    marks = {key: [] for key in COLUMNS}
    seen_points = []
    for page in pages:
        if not page:
            continue
        bounds = cluster_columns(page)
        for line in page:
            row = [None] * 12
            for m in NUM.finditer(line):
                idx = next(
                    (i for i, (lo, hi) in enumerate(bounds) if lo - 3 <= m.end() <= hi + 3),
                    None,
                )
                if idx is None or row[idx] is not None:
                    sys.exit(f"unassignable token {m.group()!r} at {m.end()} in: {line!r}")
                row[idx] = m.group()
            pts = {row[i] for i in POINTS_CLUSTERS}
            if len(pts) != 1 or None in pts:
                sys.exit(f"points columns disagree in: {line!r}")
            points = int(pts.pop())
            seen_points.append(points)
            for cluster, key in zip(VALUE_CLUSTERS, COLUMNS):
                if row[cluster] is not None:
                    marks[key].append((points, row[cluster]))

    if seen_points != list(range(1100, 0, -1)):
        sys.exit(
            f"points sequence broken: {len(seen_points)} rows, "
            f"first {seen_points[:3]}, last {seen_points[-3:]}"
        )
    return marks


def check_monotonic(marks):
    for (sex, col, _timing), rows in marks.items():
        lower_is_better = col == "60m"
        for (p1, m1), (p2, m2) in zip(rows, rows[1:]):
            v1, v2 = float(m1), float(m2)
            ok = v1 < v2 if lower_is_better else v1 > v2
            if p2 >= p1 or not ok:
                sys.exit(f"{sex}/{col}: non-monotonic {p1}:{m1} -> {p2}:{m2}")


SPOT_CHECKS = [
    # transcribed by eye from the PDF text as independent verification
    ("M", "ZoneLJ", "", 1100, "7.99"),
    ("M", "60m", "manual", 1099, "6.48"),
    ("M", "60m", "electronic", 1099, "6.72"),
    ("M", "BallThrow200g", "", 1100, "95.88"),
    ("W", "60m", "manual", 1100, "7.00"),
    ("W", "60m", "electronic", 1100, "7.24"),
    ("W", "ZoneLJ", "", 1098, "6.65"),
    ("W", "BallThrow200g", "", 1100, "73.09"),
    ("M", "60m", "manual", 1, "13.88"),
    ("M", "ZoneLJ", "", 1, "1.32"),
    ("M", "BallThrow200g", "", 1, "6.04"),
    ("W", "60m", "electronic", 1, "14.15"),
    ("W", "ZoneLJ", "", 1, "1.26"),
    ("W", "BallThrow200g", "", 1, "5.03"),
    # mid-table rows (PDF text lines 342-347 / 409-415 of the extraction)
    ("M", "ZoneLJ", "", 799, "6.30"),
    ("M", "60m", "manual", 797, "7.43"),
    ("W", "60m", "electronic", 797, "8.13"),
    ("W", "BallThrow200g", "", 740, "48.84"),
    ("M", "BallThrow200g", "", 735, "63.43"),
    ("W", "ZoneLJ", "", 735, "5.00"),
]


def main():
    marks = parse(sys.argv[1])
    check_monotonic(marks)
    for sex, col, timing, points, mark in SPOT_CHECKS:
        got = dict(marks[(sex, col, timing)]).get(points)
        if got != mark:
            sys.exit(f"spot check failed: {sex}/{col}/{timing} {points} = {got!r}, want {mark!r}")

    out = {
        "id": "ubs-kids-cup",
        "version": "2025",
        "name": "Swiss Athletics Wertungstabelle 2025 — Version UBS Kids Cup",
        "source": (
            "Official PDF: https://www.ubs-kidscup.ch/fileadmin/ubskidscup/downloads/"
            "UBS_Kids_Cup_Wertungstabelle_Disziplinen.pdf (fetched 2026-07-08); "
            "rounding and ranking rules: UBS Kids Cup Reglement (Stand Dezember 2025), "
            "§3 Punktewertung/Rangierung. Extracted by scripts/extract-ukc-scoring-table.py."
        ),
        "rounding": "next-lower-points",
        "columns": [
            {
                "disciplineCode": disc,
                "timing": timing,
                "sex": sex,
                "betterDirection": "lower" if disc == "60m" else "higher",
                "marks": [[p, m] for p, m in rows],
            }
            for (sex, disc, timing), rows in marks.items()
        ],
    }
    json.dump(out, sys.stdout, indent=1, ensure_ascii=False)
    print()


if __name__ == "__main__":
    main()
