package serverhandlers

import (
	"context"
	"net/http"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
	"github.com/spf13/viper"

	"github.com/opensvc/oc3/cdb"
	"github.com/opensvc/oc3/server"
	"github.com/opensvc/oc3/util/echolog"
	"github.com/opensvc/oc3/util/logkey"
	"github.com/opensvc/oc3/xauth"
)

// PostUserPassword handles POST /users/{user_id}/password: a UserManager (or a
// Manager) sets the password of another user, without the current one, as an
// administrator resets it. The rules are those of the deletion of an account: a
// Manager's password is set by a Manager only, whose privileges exceed a
// UserManager's, and one's own goes through POST /users/self/password, which asks
// for the current one. No password is set while impersonating, nor when password
// sign-in is disabled, nor on an account linked to an OpenID Connect identity,
// whose password is the provider's. The change is logged, without the password.
func (a *Api) PostUserPassword(c echo.Context, userId string) error {
	log := echolog.GetLogHandler(c, "PostUserPassword")
	ctx, cancel := context.WithTimeout(c.Request().Context(), a.SyncTimeout)
	defer cancel()
	odb := a.ODB

	if err := requireUserManager(c); err != nil {
		return err
	}
	selfID := authUserID(c)
	if selfID == nil {
		return JSONProblemf(c, http.StatusUnauthorized, "user authentication required")
	}
	if IsImpersonating(c) {
		return JSONProblemf(c, http.StatusForbidden, "a password cannot be set while impersonating")
	}
	if !a.BasicUsers {
		return JSONProblemf(c, http.StatusForbidden, "password sign-in is disabled on this collector")
	}
	if userId == "self" {
		return JSONProblemf(c, http.StatusConflict, "change your own password with POST /users/self/password")
	}

	targetID, found, err := odb.UserIDForPrefs(ctx, userId, cdb.ListParams{
		Groups: UserGroupsFromContext(c), IsManager: IsManager(c), UserID: selfID,
	})
	if err != nil {
		log.Error("cannot resolve user", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot resolve the user")
	}
	if !found {
		return JSONProblemf(c, http.StatusNotFound, "user %s not found", userId)
	}
	if targetID == *selfID {
		return JSONProblemf(c, http.StatusConflict, "change your own password with POST /users/self/password")
	}
	target, found, err := odb.GetUserIdentity(ctx, targetID)
	if err != nil {
		log.Error("cannot read user", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot read the user")
	}
	if !found {
		return JSONProblemf(c, http.StatusNotFound, "user %s not found", userId)
	}
	isTargetManager, err := odb.UserInRole(ctx, targetID, "Manager")
	if err != nil {
		log.Error("cannot read the roles of the user", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot read the user")
	}
	if isTargetManager && !IsManager(c) {
		return JSONProblemf(c, http.StatusForbidden, "only a Manager may set the password of a Manager")
	}

	// An account linked to the identity provider signs in there, where its
	// password, its second factor and its deactivation live: a collector
	// password would open a way in around all three.
	linked, err := odb.UserHasIdentity(ctx, targetID)
	if err != nil {
		log.Error("cannot read the identities of the user", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot read the user")
	}
	if linked {
		return JSONProblemf(c, http.StatusConflict, "the password of %s is managed by the identity provider", target.Email)
	}

	var body server.PostUserPasswordJSONRequestBody
	if err := c.Bind(&body); err != nil {
		return JSONProblem(c, http.StatusBadRequest, err.Error())
	}
	if utf8.RuneCountInString(body.NewPassword) < minResetPasswordLength {
		return JSONProblemf(c, http.StatusBadRequest, "the new password must be at least %d characters long", minResetPasswordLength)
	}

	hash, err := xauth.HashWeb2pyPassword(body.NewPassword, viper.GetString("w2p_hmac"))
	if err != nil {
		log.Error("cannot hash password", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot hash password")
	}
	if err := odb.SetUserPassword(ctx, targetID, hash); err != nil {
		log.Error("cannot store password", logkey.Error, err)
		return JSONProblemf(c, http.StatusInternalServerError, "cannot store the new password")
	}

	callerEmail, _ := c.Get(XUserEmail).(string)
	if logErr := odb.Log(ctx, cdb.LogEntry{
		Action: "user.change",
		User:   callerEmail,
		Fmt:    "change user %(email)s: password, set by an administrator",
		Dict:   map[string]any{"email": target.Email},
		Level:  "info",
	}); logErr != nil {
		log.Error("cannot write audit log", logkey.Error, logErr)
	}
	log.Info("password set", "user_id", targetID)
	return c.JSON(http.StatusOK, map[string]string{"info": "password set for " + target.Email})
}
