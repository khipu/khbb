package shared_test

import (
	"context"
	"testing"

	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func TestUserClause(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))
	f, _, _, _ := prtest.NewFactory(reg)
	client, _ := f.HTTPClient()
	cases := []struct{ who, want string }{
		{"@me", `author.uuid = "` + prtest.AdaUUID + `"`},
		{prtest.BobUUID, `author.uuid = "` + prtest.BobUUID + `"`},
		{"000000:bbbb", `author.account_id = "000000:bbbb"`},
		{"bob", `author.nickname = "bob"`},
	}
	for _, tc := range cases {
		got, err := shared.UserClause(context.Background(), client, "author", tc.who)
		if err != nil || got != tc.want {
			t.Errorf("UserClause(%q) = %q, %v; want %q", tc.who, got, err, tc.want)
		}
	}
}
