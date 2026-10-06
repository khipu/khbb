package shared_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/khipu/khbb/internal/cmdutil"
	"github.com/khipu/khbb/internal/httpmock"
	"github.com/khipu/khbb/pkg/cmd/pr/shared"
	"github.com/khipu/khbb/pkg/cmd/pr/shared/prtest"
)

func newResolver(reg *httpmock.Registry) *shared.ReviewerResolver {
	f, _, _, _ := prtest.NewFactory(reg)
	client, _ := f.HTTPClient()
	return &shared.ReviewerResolver{Client: client, Workspace: "acme"}
}

func members(users ...string) httpmock.Responder {
	items := make([]string, len(users))
	for i, u := range users {
		items[i] = prtest.Member(u)
	}
	return httpmock.JSONResponse(200, prtest.Page(items...))
}

func TestResolve_ByNicknameOrAccountID(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Bob))

	u, err := newResolver(reg).Resolve(context.Background(), "bob")
	if err != nil || u.UUID != prtest.BobUUID {
		t.Fatalf("user %+v err %v", u, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `(user.account_id = "bob" OR user.nickname = "bob")` {
		t.Errorf("q = %q", q)
	}
}

func TestResolve_ByUUID(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Cy))

	u, err := newResolver(reg).Resolve(context.Background(), prtest.CyUUID)
	if err != nil || u.Nickname != "cy" {
		t.Fatalf("user %+v err %v", u, err)
	}
	if q := reg.Calls[0].URL.Query().Get("q"); q != `user.uuid = "`+prtest.CyUUID+`"` {
		t.Errorf("q = %q", q)
	}
}

func TestResolve_Me(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", "/2.0/user", httpmock.JSONResponse(200, prtest.Ada))

	if u, err := newResolver(reg).Resolve(context.Background(), "@me"); err != nil || u.UUID != prtest.AdaUUID {
		t.Errorf("user %+v err %v", u, err)
	}
}

func TestResolve_ByDisplayNameLoadsMembersOnce(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())                      // q for "cy example"
	reg.Register("GET", prtest.Members, members(prtest.Bob, prtest.Cy)) // every member
	reg.Register("GET", prtest.Members, members())                      // q for "Bob Example"
	r := newResolver(reg)

	cy, err := r.Resolve(context.Background(), "cy example")
	if err != nil || cy.UUID != prtest.CyUUID {
		t.Fatalf("cy %+v err %v", cy, err)
	}
	if reg.Calls[1].URL.Query().Has("q") {
		t.Errorf("the display-name fallback must list every member: %s", reg.Calls[1].URL.RawQuery)
	}
	bob, err := r.Resolve(context.Background(), "Bob Example")
	if err != nil || bob.UUID != prtest.BobUUID || len(reg.Calls) != 3 {
		t.Errorf("bob %+v err %v calls %d", bob, err, len(reg.Calls))
	}
}

func TestResolve_NoMatch(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())
	reg.Register("GET", prtest.Members, members(prtest.Bob))

	_, err := newResolver(reg).Resolve(context.Background(), "zed")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || err.Error() != `no member of workspace acme matches reviewer "zed"` {
		t.Errorf("err = %v", err)
	}
}

func TestResolve_UUIDIsNeverMatchedByDisplayName(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())

	_, err := newResolver(reg).Resolve(context.Background(), "{00000000-0000-0000-0000-000000000009}")
	var notFound *cmdutil.NotFoundError
	if !errors.As(err, &notFound) || len(reg.Calls) != 1 {
		t.Errorf("err %v calls %d", err, len(reg.Calls))
	}
}

func TestResolve_Ambiguous(t *testing.T) {
	sam1 := `{"display_name":"Sam Example","nickname":"sam1","uuid":"{00000000-0000-0000-0000-000000000011}","account_id":"000000:s1"}`
	sam2 := `{"display_name":"Sam Example","nickname":"sam2","uuid":"{00000000-0000-0000-0000-000000000012}","account_id":"000000:s2"}`
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members())
	reg.Register("GET", prtest.Members, members(sam1, sam2))

	_, err := newResolver(reg).Resolve(context.Background(), "Sam Example")
	var flagErr *cmdutil.FlagError
	if !errors.As(err, &flagErr) || !strings.Contains(err.Error(), "sam1 (Sam Example,") || !strings.Contains(err.Error(), "sam2 (Sam Example,") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveAll(t *testing.T) {
	reg := httpmock.New(t)
	reg.Register("GET", prtest.Members, members(prtest.Bob))
	reg.Register("GET", prtest.Members, members(prtest.Cy))

	got, err := newResolver(reg).ResolveAll(context.Background(), []string{"bob", "cy"})
	if err != nil || !slices.Equal(got, []string{prtest.BobUUID, prtest.CyUUID}) {
		t.Errorf("got %v err %v", got, err)
	}
}

func TestUniqueUUIDs(t *testing.T) {
	got := shared.UniqueUUIDs([]string{"{b}", "{a}", "{B}", "{c}", "{d}"}, "{A}", "{d}")
	if !slices.Equal(got, []string{"{b}", "{c}"}) {
		t.Errorf("UniqueUUIDs = %v", got)
	}
	if got := shared.UniqueUUIDs(nil); got == nil || len(got) != 0 {
		t.Errorf("UniqueUUIDs(nil) = %#v, want an empty, non-nil slice", got)
	}
}
