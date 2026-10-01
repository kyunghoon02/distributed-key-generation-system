package agentpayment

import (
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"testing"
	"time"
)

func testPolicy(now time.Time) Policy {
	return Policy{BudgetID: "budget-1", AgentID: "agent-1", KeyID: "agent-payments",
		KeyVersion: 1, Currency: "USD", AllowedMerchantIDs: []string{"merchant-1"},
		MaxPerPaymentMinor: 5000, MaxTotalMinor: 10000, ValidUntil: now.Add(time.Hour)}
}

func testRequest(now time.Time) Request {
	return Request{RequestID: "request-1", AgentID: "agent-1", MerchantID: "merchant-1",
		BudgetID: "budget-1", KeyID: "agent-payments", KeyVersion: 1,
		AmountMinor: 1200, Currency: "USD", ExpiresAt: now.Add(5 * time.Minute)}
}

func TestPolicyRejectsWrongMerchantBudgetAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	policy := testPolicy(now)
	base := testRequest(now)
	if err := policy.Check(base, now, 0); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Request){
		"merchant":    func(r *Request) { r.MerchantID = "untrusted" },
		"amount":      func(r *Request) { r.AmountMinor = 5001 },
		"budget":      func(r *Request) { r.BudgetID = "other" },
		"key version": func(r *Request) { r.KeyVersion = 2 },
		"expired":     func(r *Request) { r.ExpiresAt = now },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			request := base
			change(&request)
			if err := policy.Check(request, now, 0); err == nil {
				t.Fatal("invalid request was allowed")
			}
		})
	}
	if err := policy.Check(base, now, 9000); err == nil {
		t.Fatal("total budget was exceeded")
	}
}

func TestSandboxGatewayVerifiesAuthorizationAndIdempotence(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	policy := testPolicy(now)
	gateway, err := NewSandboxGateway(policy, public)
	if err != nil {
		t.Fatal(err)
	}
	request := testRequest(now)
	message, err := request.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	authorization := Authorization{Request: request, Signature: ed25519.Sign(private, message), GroupPublic: public}
	receipt, err := gateway.Pay(authorization, now)
	if err != nil || receipt.Status != "simulated_paid" {
		t.Fatalf("valid authorization rejected: %+v, %v", receipt, err)
	}
	duplicate, err := gateway.Pay(authorization, now.Add(time.Second))
	if err != nil || !reflect.DeepEqual(receipt, duplicate) {
		t.Fatalf("idempotent replay changed receipt: %+v, %v", duplicate, err)
	}
	tampered := authorization
	tampered.Request.AmountMinor++
	if _, err := gateway.Pay(tampered, now); err == nil {
		t.Fatal("tampered amount accepted")
	}
	tampered = authorization
	tampered.Request.RequestID = "request-2"
	if _, err := gateway.Pay(tampered, now); err == nil {
		t.Fatal("reused signature accepted for a new request")
	}
}

func TestSandboxGatewayEnforcesTotalBudget(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := NewSandboxGateway(testPolicy(now), public)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"request-1", "request-2", "request-3"} {
		request := testRequest(now)
		request.RequestID = id
		request.AmountMinor = 4000
		message, err := request.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		_, err = gateway.Pay(Authorization{Request: request,
			Signature: ed25519.Sign(private, message), GroupPublic: public}, now)
		if i < 2 && err != nil {
			t.Fatal(err)
		}
		if i == 2 && err == nil {
			t.Fatal("third payment exceeded total budget")
		}
	}
}
