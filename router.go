package main

import (
	"context"
	"encoding/json"
	"errors"
	"gliphtones/database"
	"gliphtones/templates/components"
	"gliphtones/templates/views"
	"gliphtones/utils"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

func Render(c echo.Context, cmp templ.Component) error {
	//c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
	return cmp.Render(c.Request().Context(), c.Response())
}

func setupRouter(e *echo.Echo) {
	e.RouteNotFound("/*", notFound)

	e.GET("/", index)
	e.GET("/google-login", googleLogin)
	e.GET("/google-callback", googleCallback)
	e.POST("/logout", logout)
}

func index(c echo.Context) error {
	searchName := c.QueryParam("s")
	_ = c.QueryParam("p")

	// if it is a htmx request, render only one part
	if c.Request().Header.Get("HX-Request") == "true" {
		ringtones, err := database.GetRingtones(searchName, 0, 0)
		if err != nil {
			return Render(c, views.OtherError(http.StatusInternalServerError, err))
		}
		return Render(c, components.ListOfRingtones(ringtones))
	}

	ringtones, err := database.GetRingtones(searchName, 0, 0)
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}

	phones, err := database.GetPhones()
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}
	effects, err := database.GetEffects()
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}

	_, err = c.Cookie(utils.CookieName)
	return Render(c, views.Index(ringtones, phones, effects, err == nil))
}

func googleLogin(c echo.Context) error {
	url := googleOauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func googleCallback(c echo.Context) error {
	// get the authorization code from the query parameters
	code := c.QueryParam("code")
	if code == "" {
		return Render(c, views.OtherError(http.StatusBadRequest, errors.New("Bad request")))
	}

	// exchange the code for a token
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, errors.New("Failed to exchange token")))
	}

	// use the token to get user information
	client := googleOauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, errors.New("Failed to fetch user info")))
	}
	defer resp.Body.Close()

	// decode the user information
	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, errors.New("Failed to decode user info")))
	}

	userID, err := database.CreateUser(userInfo["name"].(string), userInfo["email"].(string))
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}

	utils.WriteAuthCookie(c, userID)
	return c.Redirect(http.StatusTemporaryRedirect, "/")
}

func logout(c echo.Context) error {
	utils.RemoveAuthCookie(c)
	/* c.Response().Header().Add("HX-Refresh", "true")
	return c.NoContent(http.StatusOK) */

	return Render(c, components.Header(false))
}

func notFound(c echo.Context) error {
	_, err := c.Cookie(utils.CookieName)
	return Render(c, views.NotFound(err == nil))
}
