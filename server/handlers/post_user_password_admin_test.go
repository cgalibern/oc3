package serverhandlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shaj13/go-guardian/v2/auth"

	"github.com/opensvc/oc3/xauth"
)

// TestPostUserPasswordRefusals checks the refusals of the password set by an
// administrator that come before the database is read: the privilege, password
// sign-in turned off, and one's own account.
func TestPostUserPasswordRefusals(t *testing.T) {
	request := func(groups []string) (echo.Context, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodPost, "/users/2/password", strings.NewReader(`{"new_password": "a new password"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		ext := make(auth.Extensions)
		ext.Set(xauth.XAuthSource, xauth.AuthSourceBasic)
		ext.Set(xauth.XUserID, "1")
		c.Set("user", auth.NewUserInfo("m@example.com", "1", nil, ext))
		c.Set(XAuthMode, AuthModeUser)
		c.Set("groups", groups)
		return c, rec
	}

	for _, tc := range []struct {
		name       string
		basicUsers bool
		groups     []string
		userID     string
		code       int
		text       string
	}{
		{"no privilege", true, []string{"Everybody"}, "2", http.StatusForbidden, "UserManager privilege required"},
		{"passwords off", false, []string{"UserManager"}, "2", http.StatusForbidden, "password sign-in is disabled"},
		{"own account", true, []string{"Manager"}, "self", http.StatusConflict, "/users/self/password"},
	} {
		a := &Api{BasicUsers: tc.basicUsers}
		c, rec := request(tc.groups)
		// A refused privilege answers and returns errRequestDenied, as the
		// other handlers using requireUserManager.
		if err := a.PostUserPassword(c, tc.userID); err != nil && !errors.Is(err, errRequestDenied) {
			t.Fatalf("%s: %s", tc.name, err)
		}
		if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.text) {
			t.Errorf("%s: got %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}
