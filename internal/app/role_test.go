// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Bahnfrei contributors

package app

import "testing"

func TestParseRole(t *testing.T) {
	for _, r := range []Role{RoleInstanceAdmin, RoleMeetOrganizer, RoleCompetitionOffice,
		RoleFieldOfficial, RoleEntrySubmitter, RolePublic} {
		got, err := ParseRole(string(r))
		if err != nil {
			t.Fatalf("ParseRole(%q): %v", r, err)
		}
		if got != r {
			t.Errorf("ParseRole(%q) = %q, want %q", r, got, r)
		}
		if !got.Valid() {
			t.Errorf("%q.Valid() = false, want true", got)
		}
	}
}

func TestParseRoleInvalid(t *testing.T) {
	_, err := ParseRole("superuser")
	if err == nil {
		t.Fatal("ParseRole(\"superuser\") = nil error, want ErrInvalidRole")
	}
	if _, ok := err.(ErrInvalidRole); !ok {
		t.Errorf("ParseRole error = %v (%T), want ErrInvalidRole", err, err)
	}
}

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		role, min Role
		want      bool
	}{
		{RoleInstanceAdmin, RolePublic, true},
		{RolePublic, RoleInstanceAdmin, false},
		{RoleMeetOrganizer, RoleMeetOrganizer, true},
		{RoleFieldOfficial, RoleCompetitionOffice, false},
		{RoleCompetitionOffice, RoleFieldOfficial, true},
		{Role("bogus"), RolePublic, false},
	}
	for _, c := range cases {
		if got := c.role.AtLeast(c.min); got != c.want {
			t.Errorf("%q.AtLeast(%q) = %v, want %v", c.role, c.min, got, c.want)
		}
	}
}

func TestErrInvalidRoleMessage(t *testing.T) {
	err := ErrInvalidRole{Value: "wizard"}
	if got, want := err.Error(), `invalid role "wizard"`; got != want {
		t.Errorf("ErrInvalidRole.Error() = %q, want %q", got, want)
	}
}

func TestErrForbiddenMessage(t *testing.T) {
	err := ErrForbidden{Role: RoleEntrySubmitter, Capability: CapManageAccounts}
	if got, want := err.Error(), `role "entry_submitter" lacks capability "manage_accounts"`; got != want {
		t.Errorf("ErrForbidden.Error() = %q, want %q", got, want)
	}
}

func TestAuthorize(t *testing.T) {
	if err := Authorize(RoleInstanceAdmin, CapManageAccounts); err != nil {
		t.Errorf("instance admin should manage accounts: %v", err)
	}
	if err := Authorize(RoleEntrySubmitter, CapManageAccounts); err == nil {
		t.Error("entry submitter should not be able to manage accounts")
	} else {
		var forbidden ErrForbidden
		if fe, ok := err.(ErrForbidden); ok {
			forbidden = fe
		} else {
			t.Fatalf("Authorize error = %v (%T), want ErrForbidden", err, err)
		}
		if forbidden.Role != RoleEntrySubmitter || forbidden.Capability != CapManageAccounts {
			t.Errorf("ErrForbidden = %+v, want Role=%q Capability=%q", forbidden, RoleEntrySubmitter, CapManageAccounts)
		}
	}
	if err := Authorize(RolePublic, CapViewPublicResult); err != nil {
		t.Errorf("public should view public results: %v", err)
	}
	if err := Authorize(RoleInstanceAdmin, Capability("bogus")); err == nil {
		t.Error("unknown capability should error")
	}
}
