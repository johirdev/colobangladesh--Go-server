// Package meta serves static reference data for forms, e.g. the country list
// with flag images (free flag CDN: flagcdn.com) for the country dropdown.
package meta

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// countries.json: ISO 3166-1 alpha-2 code, common name, dial code, emoji
// (source: github.com/mledoze/countries, ODbL).
//
//go:embed data/countries.json
var countriesJSON []byte

// Country is one dropdown option.
type Country struct {
	Code     string `json:"code" example:"BD"`
	Name     string `json:"name" example:"Bangladesh"`
	DialCode string `json:"dial_code" example:"+880"`
	Emoji    string `json:"emoji" example:"🇧🇩"`
	FlagPNG  string `json:"flag_png" example:"https://flagcdn.com/w40/bd.png"`    // 40px wide, ideal for dropdowns
	FlagPNG2 string `json:"flag_png_2x" example:"https://flagcdn.com/w80/bd.png"` // for srcset on retina screens
	FlagSVG  string `json:"flag_svg" example:"https://flagcdn.com/bd.svg"`
}

type Module struct{ countries []Country }

func New() (*Module, error) {
	var raw []Country
	if err := json.Unmarshal(countriesJSON, &raw); err != nil {
		return nil, err
	}
	for i := range raw {
		c := strings.ToLower(raw[i].Code)
		raw[i].FlagPNG = "https://flagcdn.com/w40/" + c + ".png"
		raw[i].FlagPNG2 = "https://flagcdn.com/w80/" + c + ".png"
		raw[i].FlagSVG = "https://flagcdn.com/" + c + ".svg"
	}
	return &Module{countries: raw}, nil
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	rg.GET("/meta/countries", m.Countries)
}

// Countries godoc
//
//	@Summary		Countries with flags (for the country dropdown)
//	@Description	250 countries sorted by name: ISO code (send it as `country_code`), dial code, emoji and
//	@Description	flag image URLs from the free flagcdn.com CDN. Cached for a day.
//	@Tags			0. Meta
//	@Produce		json
//	@Success		200	{object}	response.Body{data=[]Country}
//	@Router			/meta/countries [get]
func (m *Module) Countries(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	response.OK(c, http.StatusOK, "", m.countries)
}
