package main

import (
	"context"
	"encoding/json"
	"errors"
	"gliphtones/database"
	"gliphtones/templates/components"
	"gliphtones/templates/views"
	"gliphtones/utils"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	e.GET("/profile", profile)
	e.GET("/upload", uploadView)
	e.PUT("/upload", uploadFile)
	e.POST("/report/:id", reportRingtone)
	e.GET("/google-login", googleLogin)
	e.GET("/google-callback", googleCallback)
	e.POST("/logout", logout)
}

func index(c echo.Context) error {
	searchQuery := c.QueryParam("s")
	pageNumber, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		pageNumber = 1
	}

	phonesMap := make(map[int]bool)
	effetsMap := make(map[int]bool)
	var phonesQuery []string = strings.Split(c.QueryParam("p"), ",")
	var effectsQuery []string = strings.Split(c.QueryParam("e"), ",")
	for _, v := range phonesQuery {
		phoneID, err := strconv.Atoi(v)
		if err == nil {
			phonesMap[phoneID] = true
		}
	}
	for _, v := range effectsQuery {
		effectID, err := strconv.Atoi(v)
		if err == nil {
			effetsMap[effectID] = true
		}
	}

	phonesArr := slices.Collect(maps.Keys(phonesMap))
	effectsArr := slices.Collect(maps.Keys(effetsMap))

	// if it is a htmx request, render only the new results
	if c.Request().Header.Get("HX-Request") == "true" {
		ringtones, numberOfPages, err := database.GetRingtones(searchQuery, phonesArr, effectsArr, pageNumber)
		if err != nil {
			return Render(c, views.OtherError(http.StatusInternalServerError, errors.New("sus")))
		}
		return Render(c, components.ListOfRingtones(ringtones, numberOfPages, pageNumber, "index"))
	}

	phones, err := database.GetPhones()
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}
	effects, err := database.GetEffects()
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}

	var ringtones []database.RingtoneModel
	var numberOfPages int
	if len(phonesArr) == 0 && len(effectsArr) == 0 && searchQuery == "" {
		ringtones, numberOfPages, err = database.GetPopularRingtones(pageNumber)
	} else {
		ringtones, numberOfPages, err = database.GetRingtones(searchQuery, phonesArr, effectsArr, pageNumber)
	}
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}

	_, err = c.Cookie(utils.CookieName)

	var data views.IndexData = views.IndexData{
		Ringtones:     ringtones,
		Phones:        phones,
		Effects:       effects,
		SearchQuery:   searchQuery,
		NumberOfPages: numberOfPages,
		Page:          pageNumber,
		LoggedIn:      err == nil,
	}
	return Render(c, views.Index(data))
}

func profile(c echo.Context) error {
	pageNumber, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		pageNumber = 1
	}

	ringtones, numberOfPages, err := database.GetRingtones("", []int{}, []int{}, pageNumber)
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}

	var data views.ProfileData = views.ProfileData{
		Ringtones:     ringtones,
		NumberOfPages: numberOfPages,
		Page:          pageNumber,
		UserID:        utils.GetIdFromCookie(c),
	}
	return Render(c, views.Profile(data))
}

func uploadView(c echo.Context) error {
	phones, err := database.GetPhones()
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}
	effects, err := database.GetEffects()
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}

	_, err = c.Cookie(utils.CookieName)
	return Render(c, views.Upload(err == nil, phones, "", effects, ""))
}

func uploadFile(c echo.Context) error {
	userID := utils.GetIdFromCookie(c)
	if userID == 0 {
		return Render(c, views.OtherError(http.StatusBadRequest, errors.New("Only logged-in users can upload Gliphtones")))
	}
	name := c.FormValue("name")
	phone, err := strconv.Atoi(c.FormValue("p"))
	if err != nil {
		return Render(c, views.OtherError(http.StatusBadRequest, errors.New("Missing form values")))
	}
	effect, err := strconv.Atoi(c.FormValue("e"))
	if err != nil {
		return Render(c, views.OtherError(http.StatusBadRequest, errors.New("Missing form values")))
	}

	file, err := c.FormFile("ringtone")
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}
	src, err := file.Open()
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}
	defer src.Close()

	tmpFile, err := utils.CreateTemporaryFile(src)
	if err != nil {
		log.Println(err)
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}
	defer func() {
		name := tmpFile.Name()
		tmpFile.Close()
		utils.DeleteTemporaryFile(name)
	}()

	ok, err := utils.CheckFile(tmpFile)
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}
	if !ok {
		return Render(c, views.OtherError(http.StatusBadRequest, errors.New("It seems that the file provided is not a Nothing Gliphtone.")))
	}

	ringtoneID, err := database.CreateRingtone(name, phone, effect, userID)
	err = utils.CreateRingtoneFile(tmpFile, ringtoneID)
	if err != nil {
		return Render(c, views.OtherError(http.StatusInternalServerError, err))
	}

	return Render(c, views.SuccessfulUpload())
}

func reportRingtone(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	err = database.RingtoneIncreaseNotWorking(id)
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.NoContent(http.StatusOK)
}

func googleLogin(c echo.Context) error {
	url := googleOauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	return c.Redirect(http.StatusTemporaryRedirect, url)
}

func googleCallback(c echo.Context) error {
	// get the authorization code from the query parameters
	code := c.QueryParam("code")
	if code == "" {
		return Render(c, views.OtherErrorView(http.StatusBadRequest, errors.New("Bad request")))
	}

	// exchange the code for a token
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, errors.New("Failed to exchange token")))
	}

	// use the token to get user information
	client := googleOauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, errors.New("Failed to fetch user info")))
	}
	defer resp.Body.Close()

	// decode the user information
	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, errors.New("Failed to decode user info")))
	}

	userID, err := database.CreateUser(userInfo["name"].(string), userInfo["email"].(string))
	if err != nil {
		return Render(c, views.OtherErrorView(http.StatusInternalServerError, err))
	}

	utils.WriteAuthCookie(c, userID)
	return c.Redirect(http.StatusTemporaryRedirect, "/")
}

func logout(c echo.Context) error {
	utils.RemoveAuthCookie(c)
	return Render(c, components.Header(false))
}

func notFound(c echo.Context) error {
	_, err := c.Cookie(utils.CookieName)
	return Render(c, views.NotFoundView(err == nil))
}
