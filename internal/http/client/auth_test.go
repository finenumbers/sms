package client

import "testing"

func TestClientMeKeepsNameAndClientNameDistinct(t *testing.T) {
	got := clientMe("uid", "user@example.com", "Иванов Иван", "owner", "cid", "ООО Ромашка")
	if got["name"] != "Иванов Иван" {
		t.Fatalf("name=%v", got["name"])
	}
	if got["client_name"] != "ООО Ромашка" {
		t.Fatalf("client_name=%v", got["client_name"])
	}
	if got["email"] != "user@example.com" {
		t.Fatalf("email=%v", got["email"])
	}
	if got["name"] == got["client_name"] {
		t.Fatal("name must not equal client_name")
	}
}
