package budget

import "testing"

func TestEvaluate_NoLimits(t *testing.T) {
	v := Evaluate(Limit{}, 10.0, 0, 0, 0)
	if v != nil {
		t.Error("expected nil for zero limits")
	}
}

func TestEvaluate_PerRunExceeded(t *testing.T) {
	v := Evaluate(Limit{PerRun: 5.0}, 3.0, 3.0, 0, 0)
	if v == nil || v.LimitType != "per_run" {
		t.Errorf("expected per_run violation, got %v", v)
	}
}

func TestEvaluate_PerRunOK(t *testing.T) {
	v := Evaluate(Limit{PerRun: 5.0}, 2.0, 2.0, 0, 0)
	if v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestEvaluate_DailyExceeded(t *testing.T) {
	v := Evaluate(Limit{Daily: 50.0}, 5.0, 0, 48.0, 0)
	if v == nil || v.LimitType != "daily" {
		t.Errorf("expected daily violation, got %v", v)
	}
}

func TestEvaluate_MonthlyExceeded(t *testing.T) {
	v := Evaluate(Limit{Monthly: 100.0}, 5.0, 0, 0, 98.0)
	if v == nil || v.LimitType != "monthly" {
		t.Errorf("expected monthly violation, got %v", v)
	}
}

func TestEvaluate_PriorityOrder(t *testing.T) {
	// per_run checked first
	v := Evaluate(Limit{PerRun: 1.0, Daily: 1.0}, 2.0, 0, 0, 0)
	if v == nil || v.LimitType != "per_run" {
		t.Errorf("expected per_run (highest priority), got %v", v)
	}
}

func TestEvaluatePreRun_DailyAlreadyHit(t *testing.T) {
	v := EvaluatePreRun(Limit{Daily: 50.0}, 50.0, 0)
	if v == nil || v.LimitType != "daily" {
		t.Errorf("expected daily violation, got %v", v)
	}
}

func TestEvaluatePreRun_UnderLimit(t *testing.T) {
	v := EvaluatePreRun(Limit{Daily: 50.0}, 49.0, 0)
	if v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestHasLimits(t *testing.T) {
	if (Limit{}).HasLimits() {
		t.Error("zero limit should return false")
	}
	if !(Limit{PerRun: 1.0}).HasLimits() {
		t.Error("non-zero limit should return true")
	}
}
