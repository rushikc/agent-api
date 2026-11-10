package profile

import "testing"

func TestOrderShadows(t *testing.T) {
	in := []Shadow{
		{Name: "web", DependsOn: []string{"api"}},
		{Name: "api", DependsOn: []string{"db"}},
		{Name: "db"},
	}
	out, err := OrderShadows(in)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Name != "db" || out[1].Name != "api" || out[2].Name != "web" {
		t.Fatalf("%+v", out)
	}
}

func TestOrderShadowsCycle(t *testing.T) {
	_, err := OrderShadows([]Shadow{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle")
	}
}

func TestOrderUnknownDep(t *testing.T) {
	_, err := OrderShadows([]Shadow{{Name: "web", DependsOn: []string{"nope"}}})
	if err == nil {
		t.Fatal("expected unknown dep")
	}
}
