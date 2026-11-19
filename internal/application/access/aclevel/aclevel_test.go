package aclevel

import "testing"

func TestCanAccess(t *testing.T) {
	cases := []struct {
		name    string
		caller  Level
		data    Level
		allowed bool
	}{
		{"intern cannot read secret (CEO salary)", RoleLevel("intern"), Secret, false},
		{"viewer cannot read secret", RoleLevel("viewer"), Secret, false},
		{"employee(internal) can read public", RoleLevel("contributor"), Public, true},
		{"employee(internal) can read internal", RoleLevel("contributor"), Internal, true},
		{"employee cannot read confidential (perf)", Internal, Confidential, false},
		{"admin can read confidential", RoleLevel("admin"), Confidential, true},
		{"admin cannot read secret", RoleLevel("admin"), Secret, false},
		{"owner can read everything", RoleLevel("owner"), Secret, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanAccess(c.caller, c.data); got != c.allowed {
				t.Fatalf("CanAccess(%s,%s)=%v want %v", c.caller, c.data, got, c.allowed)
			}
		})
	}
}

func TestFilterResults_BlocksCEOSalaryFromIntern(t *testing.T) {
	results := []Result{
		{ID: "DOCREF-HR-001", Clearance: Internal},   // employee handbook
		{ID: "DOCREF-FIN-002", Clearance: Secret},    // CEO salary table
		{ID: "DOCREF-TECH-001", Clearance: Public},   // API spec
	}
	caller := RoleLevel("intern")
	allowed, blocked := FilterResults(caller, results)

	if len(allowed) != 2 {
		t.Fatalf("allowed=%d want 2, got %+v", len(allowed), allowed)
	}
	if len(blocked) != 1 || blocked[0] != "DOCREF-FIN-002" {
		t.Fatalf("blocked=%v want [DOCREF-FIN-002]", blocked)
	}
	for _, r := range allowed {
		if r.ID == "DOCREF-FIN-002" {
			t.Fatal("secret salary document leaked to intern!")
		}
	}
}

func TestExtractClearance(t *testing.T) {
	cases := []struct {
		meta map[string]interface{}
		want Level
	}{
		{map[string]interface{}{"clearance": "secret"}, Secret},
		{map[string]interface{}{"密级": "confidential"}, Confidential},
		{map[string]interface{}{"clearance": "internal"}, Internal},
		{map[string]interface{}{"other": "x"}, Public},
		{nil, Public},
	}
	for _, c := range cases {
		if got := ExtractClearance(c.meta); got != c.want {
			t.Fatalf("ExtractClearance(%v)=%s want %s", c.meta, got, c.want)
		}
	}
}
