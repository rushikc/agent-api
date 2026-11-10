package secrets

import (
	"context"
	"testing"
	"time"
)

type memKC struct{ m map[string]string }

func (k memKC) Get(a string) (string, error) {
	v, ok := k.m[a]
	if !ok {
		return "", errNotFound("missing")
	}
	return v, nil
}
func (k memKC) Set(a, v string) error { k.m[a] = v; return nil }
func (k memKC) Delete(a string) error { delete(k.m, a); return nil }
func (k memKC) List() ([]string, error) {
	var n []string
	for a := range k.m {
		n = append(n, a)
	}
	return n, nil
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

type fakeGCP struct{ vals map[string]string }

func (f fakeGCP) ReadSecret(_ context.Context, alias, secret, version string) (string, error) {
	k := alias + "/" + secret
	if version != "" {
		k += "#" + version
	}
	v, ok := f.vals[alias+"/"+secret]
	if !ok {
		return "", errNotFound("no secret")
	}
	return v, nil
}

func TestResolveOrder(t *testing.T) {
	b := New(fakeGCP{vals: map[string]string{"dev/stripe": "gcp-val"}})
	b.KC = memKC{m: map[string]string{"stripe-test": "kc-val"}}
	b.Op = func(ref string) (string, error) { return "op-val", nil }
	ctx := context.Background()
	v, src, err := b.Resolve(ctx, "keychain://stripe-test")
	if err != nil || v != "kc-val" || src != "keychain" {
		t.Fatalf("keychain %q %s %v", v, src, err)
	}
	v, src, err = b.Resolve(ctx, "gcp://dev/stripe")
	if err != nil || v != "gcp-val" || src != "gcp" {
		t.Fatalf("gcp %q %s %v", v, src, err)
	}
	v, src, err = b.Resolve(ctx, "op://Dev/Stripe/test-key")
	if err != nil || v != "op-val" || src != "op" {
		t.Fatalf("op %q %s %v", v, src, err)
	}
}

func TestCacheTTLAndLock(t *testing.T) {
	now := time.Unix(0, 0)
	kc := memKC{m: map[string]string{"n": "secret-value"}}
	b := New(nil)
	b.KC = kc
	b.TTL = time.Minute
	b.Now = func() time.Time { return now }
	if _, _, err := b.Resolve(context.Background(), "keychain://n"); err != nil {
		t.Fatal(err)
	}
	kc.m["n"] = "changed"
	v, _, err := b.Resolve(context.Background(), "keychain://n")
	if err != nil || v != "secret-value" {
		t.Fatalf("cached %q %v", v, err)
	}
	b.Lock()
	v, _, err = b.Resolve(context.Background(), "keychain://n")
	if err != nil || v != "changed" {
		t.Fatalf("after lock %q %v", v, err)
	}
}

func TestResolveMap(t *testing.T) {
	b := New(nil)
	b.KC = memKC{m: map[string]string{"stripe-test": "tok"}}
	out := b.ResolveMap(context.Background(), map[string]string{
		"STRIPE_TEST_KEY": "keychain://stripe-test",
		"PLAIN":           "ok",
	})
	if out["STRIPE_TEST_KEY"] != "tok" || out["PLAIN"] != "ok" {
		t.Fatalf("%v", out)
	}
}
