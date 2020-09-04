package redirect_back

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ecletus/core"
	"github.com/ecletus/core/utils"
	_ "github.com/ecletus/session/manager"
	"github.com/moisespsena-go/xroute"
)

const SessionKey = "redirect_to"

var returnToKey utils.ContextKey = "redirect_back_return_to"

// Config redirect back config
type Config struct {
	FallbackPath      string
	IgnoredPaths      []string
	IgnoredPrefixes   []string
	AllowedExtensions []string
	IgnoreFunc        func(*http.Request) bool
}

// New initialize redirect back instance
func New(config *Config) *RedirectBack {
	if config.FallbackPath == "" {
		config.FallbackPath = "/"
	}

	if config.AllowedExtensions == nil {
		config.AllowedExtensions = []string{"", ".html"}
	}

	redirectBack := &RedirectBack{config: config}
	redirectBack.compile()
	return redirectBack
}

// RedirectBack redirect back struct
type RedirectBack struct {
	config               *Config
	ignoredPathsMap      map[string]bool
	allowedExtensionsMap map[string]bool

	Ignore     func(req *http.Request) bool
	IgnorePath func(pth string) bool
}

func (redirectBack *RedirectBack) compile() {
	redirectBack.ignoredPathsMap = map[string]bool{}

	for _, pth := range redirectBack.config.IgnoredPaths {
		redirectBack.ignoredPathsMap[pth] = true
	}

	redirectBack.allowedExtensionsMap = map[string]bool{}
	for _, ext := range redirectBack.config.AllowedExtensions {
		redirectBack.allowedExtensionsMap[ext] = true
	}

	redirectBack.IgnorePath = func(pth string) bool {
		if !redirectBack.allowedExtensionsMap[filepath.Ext(pth)] {
			return true
		}

		if redirectBack.ignoredPathsMap[pth] {
			return true
		}

		for _, prefix := range redirectBack.config.IgnoredPrefixes {
			if strings.HasPrefix(pth, prefix) {
				return true
			}
		}

		return false
	}

	redirectBack.Ignore = func(req *http.Request) bool {
		if req.Method != "GET" {
			return true
		}

		if redirectBack.config.IgnoreFunc != nil {
			return redirectBack.config.IgnoreFunc(req)
		}

		return redirectBack.IgnorePath(req.URL.Path)
	}
}

// RedirectBack redirect back to last visited page
func (redirectBack *RedirectBack) RedirectBack(w http.ResponseWriter, req *http.Request, fallback ...string) {
	returnTo := req.Context().Value(returnToKey)
	ctx := core.ContextFromRequest(req)

	if returnTo != nil {
		ctx.SessionManager().Pop(SessionKey)
		http.Redirect(w, req, returnTo.(string), http.StatusSeeOther)
		return
	}

	if referrer := req.Referer(); referrer != "" {
		if u, _ := url.Parse(referrer); !redirectBack.IgnorePath(u.Path) && u.Path != req.RequestURI {
			if ctx != nil && !(u.Host == req.Host && u.Path == ctx.OriginalURL.Path) {
				http.Redirect(w, req, referrer, http.StatusSeeOther)
				return
			}
		}
	}

	var fb string
	for _, fb = range fallback {}
	if fb == "" {
		fb = redirectBack.config.FallbackPath
	}
	if fb[0] == '/' {
		fb = ctx.Path(fb)
	}

	http.Redirect(w, req, fb, http.StatusSeeOther)
}

// Middleware returns a RedirectBack middleware instance that record return_to path
func (redirectBack *RedirectBack) Middleware() *xroute.Middleware {
	return &xroute.Middleware{
		Name:  "qor:redirect_back",
		After: []string{"qor:session"},
		Handler: func(chain *xroute.ChainHandler) {
			ctx := core.ContextFromRequest(chain.Request())
			if returnTo := ctx.SessionManager().Pop(SessionKey); returnTo != "" {
				req := ctx.Request
				req = req.WithContext(context.WithValue(req.Context(), returnToKey, returnTo))
				ctx.Request = req

				if !redirectBack.Ignore(req) && returnTo != req.URL.String() {
					returnTo = ctx.Path(req.URL.String())
					ctx.SessionManager().Add(SessionKey, returnTo)
				}
				chain.SetRequest(req)
			}
			chain.Pass()
		},
	}
}

func ReturnToUrl(ctx *core.Context) string {
	if returnTo := ctx.Request.Context().Value(returnToKey); returnTo != nil {
		return returnTo.(string)
	}
	if returnTo := ctx.Request.URL.Query().Get(SessionKey); returnTo != "" {
		return returnTo
	}
	if returnTo := ctx.SessionManager().Get(SessionKey); returnTo != "" {
		return returnTo
	}
	return ""
}

func Set(ctx *core.Context) error {
	return ctx.SessionManager().Add(SessionKey, ctx.OriginalURL.String())
}
