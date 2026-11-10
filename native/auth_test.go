package main

import (
	"testing"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

func TestAccountBinding(t *testing.T) {
	accounts := []public.Account{{HomeAccountID: "alice"}, {HomeAccountID: "bob"}}
	account, err := selectAccount(accounts, "bob")
	if err != nil || account.HomeAccountID != "bob" {
		t.Fatal("selected the wrong account")
	}
	for _, id := range []string{"", "unknown"} {
		if _, err := selectAccount(accounts, id); err == nil {
			t.Fatal("silently switched accounts")
		}
	}
	for _, config := range []localConfig{{ClientID: "not-an-id", Tenant: "organizations"}, {ClientID: "11111111-1111-1111-1111-111111111111", Tenant: "../evil"}, {ClientID: "11111111-1111-1111-1111-111111111111", Tenant: "https://evil.example"}} {
		if config.validate() == nil {
			t.Fatal("accepted invalid authority configuration")
		}
	}
}
