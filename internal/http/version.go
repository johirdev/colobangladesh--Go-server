package http

import (
	"net/http"
	"time"

	"cholobangladesh-server/pkg/buildinfo"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// APIVersion describes one major version of the API.
type APIVersion struct {
	Version    string     `json:"version" example:"v1"`
	Status     string     `json:"status" example:"stable" enums:"stable,beta,deprecated"`
	BasePath   string     `json:"base_path" example:"/api/v1"`
	ReleasedAt string     `json:"released_at" example:"2026-11-21"`
	SunsetAt   *time.Time `json:"sunset_at,omitempty"` // set when a version is deprecated
	Docs       string     `json:"docs,omitempty" example:"/swagger/index.html"`
}

// VersionInfo is returned by GET /api/v1.
type VersionInfo struct {
	Name        string       `json:"name" example:"CholoBangladesh API"`
	APIVersion  string       `json:"api_version" example:"v1"`
	Build       BuildInfo    `json:"build"`
	Environment string       `json:"environment" example:"development"`
	ServerTime  time.Time    `json:"server_time"`
	Versions    []APIVersion `json:"versions"`
}

type BuildInfo struct {
	Version   string `json:"version" example:"1.0.0"`
	Commit    string `json:"commit" example:"a1b2c3d"`
	BuildTime string `json:"build_time" example:"2026-11-21T06:00:00Z"`
}

// versions is the registry of every API version. To ship v2: add an entry here,
// mount the v2 group in NewRouter, and mark v1 "deprecated" with a SunsetAt date.
var versions = []APIVersion{
	{Version: "v1", Status: "stable", BasePath: "/api/v1", ReleasedAt: "2026-11-21"},
}

func versionInfo(appName, env, current string, swagger bool) VersionInfo {
	list := make([]APIVersion, len(versions))
	copy(list, versions)
	if swagger {
		for i := range list {
			list[i].Docs = "/swagger/index.html"
		}
	}
	return VersionInfo{
		Name:        appName + " API",
		APIVersion:  current,
		Build:       BuildInfo{Version: buildinfo.Version, Commit: buildinfo.Commit, BuildTime: buildinfo.BuildTime},
		Environment: env,
		ServerTime:  time.Now().UTC(),
		Versions:    list,
	}
}

// versionHeaders tags every response with the API version, and adds the standard
// Deprecation / Sunset headers (RFC 8594) when the version is being retired.
func versionHeaders(v APIVersion) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-API-Version", v.Version)
		if v.Status == "deprecated" {
			c.Header("Deprecation", "true")
			if v.SunsetAt != nil {
				c.Header("Sunset", v.SunsetAt.UTC().Format(http.TimeFormat))
			}
		}
		c.Next()
	}
}

// APIIndex godoc
//
//	@Summary		API version info
//	@Description	Current API version, build (version, git commit, build time) and all available versions.
//	@Description	Every response also carries an `X-API-Version` header. Deprecated versions add
//	@Description	`Deprecation: true` and `Sunset: <date>` headers. Use it to check the server is up.
//	@Tags			0. Meta
//	@Produce		json
//	@Success		200	{object}	response.Body{data=VersionInfo}
//	@Router			/ [get]
func apiIndex(appName, env, current string, swagger bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.OK(c, http.StatusOK, "", versionInfo(appName, env, current, swagger))
	}
}
