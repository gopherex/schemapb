package schemapb_test

import (
	"testing"
	"time"

	root "github.com/stroppy-io/schemapb"
	pb "github.com/stroppy-io/schemapb/schemapb"
)

func TestBuildSchema(t *testing.T) {
	s := root.NewSchema(
		root.SchemaName("signup"),
		root.SchemaFields(
			root.NewField("age",
				root.Int32(root.Int32Gte(0), root.Int32Lte(150)),
				root.FieldRequired(),
				root.FieldUI(root.NewUI(
					root.UILabel("Age"),
					root.UIWidget(root.WidgetSlider),
				)),
			),
			root.NewField("name",
				root.String(root.StringMinLen(1), root.StringMaxLen(64)),
				root.FieldDescription("full name"),
			),
			root.NewField("timeout",
				root.Duration(root.DurationGte(time.Second)),
			),
			root.NewField("tags",
				root.List(root.ListMinItems(1), root.ListItems(
					root.NewField("tag", root.String()),
				)),
			),
			root.NewField("address",
				root.Object(root.NewSchema(root.SchemaFields(
					root.NewField("zip", root.String(root.StringLen(5))),
				))),
			),
		),
		root.SchemaRules(
			root.NewRule("root.age >= 18", "must be adult", root.RuleID("adult"), root.RuleSeverity(root.SeverityError)),
		),
	)

	if s.GetName() != "signup" {
		t.Fatalf("name = %q", s.GetName())
	}
	if len(s.GetFields()) != 5 {
		t.Fatalf("fields = %d", len(s.GetFields()))
	}

	age := s.GetFields()[0]
	if !age.GetRequired() {
		t.Error("age not required")
	}
	if age.GetInt32().GetGte() != 0 || age.GetInt32().GetLte() != 150 {
		t.Errorf("age bounds: %v", age.GetInt32())
	}
	if age.GetUi().GetWidget() != pb.Schema_Filed_UI_WIDGET_SLIDER {
		t.Errorf("age widget: %v", age.GetUi().GetWidget())
	}

	name := s.GetFields()[1]
	if name.GetString_().GetMaxLen() != 64 {
		t.Errorf("name maxlen: %d", name.GetString_().GetMaxLen())
	}

	timeout := s.GetFields()[2]
	if timeout.GetDuration().GetGte().GetSeconds() != 1 {
		t.Errorf("timeout gte: %v", timeout.GetDuration().GetGte())
	}

	tags := s.GetFields()[3]
	if tags.GetList().GetMinItems() != 1 || len(tags.GetList().GetItems()) != 1 {
		t.Errorf("tags list: %v", tags.GetList())
	}

	addr := s.GetFields()[4]
	if got := addr.GetObject().GetSchema().GetFields()[0].GetString_().GetLen(); got != 5 {
		t.Errorf("zip len: %d", got)
	}

	if len(s.GetRules()) != 1 || s.GetRules()[0].GetSeverity() != pb.Schema_Filed_ERROR {
		t.Errorf("rules: %v", s.GetRules())
	}
}

func TestFieldError(t *testing.T) {
	e := root.NewFieldError("age", "too low",
		root.FieldErrorRuleID("adult"),
		root.FieldErrorSeverity(root.SeverityWarning),
	)
	if e.GetField() != "age" || e.GetRuleId() != "adult" || e.GetSeverity() != pb.Schema_Filed_WARNING {
		t.Errorf("field error: %v", e)
	}
}
