package redirect_back

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/moisespsena/go-route"
	"github.com/aghape/aghape/utils"
	"github.com/aghape/aghape"
	_ "github.com/aghape/session/manager"
)

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
func (redirectBack *RedirectBack) RedirectBack(w http.ResponseWriter, req *http.Request) {
	returnTo := req.Context().Value(returnToKey)

	if returnTo != nil {
		http.Redirect(w, req, fmt.Sprint(returnTo), http.StatusSeeOther)
		return
	}

	if referrer := req.Referer(); referrer != "" {
		if u, _ := url.Parse(referrer); !redirectBack.IgnorePath(u.Path) {
			http.Redirect(w, req, referrer, http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, req, redirectBack.config.FallbackPath, http.StatusSeeOther)
}

// Middleware returns a RedirectBack middleware instance that record return_to path
func (redirectBack *RedirectBack) Middleware() *route.Middleware {
	return &route.Middleware{
		Name:        "qor:redirect_back",
		After: []string{"qor:session"},
		Handler: func(chain *route.ChainHandler) {
			qorctx := qor.ContexFromChain(chain)
			returnTo := qorctx.SessionManager().Get("return_to")
			req := qorctx.Request
			req = req.WithContext(context.WithValue(req.Context(), returnToKey, returnTo))
			qorctx.Request = req

			if !redirectBack.Ignore(req) && returnTo != req.URL.String() {
				returnTo = qorctx.GenURL(req.URL.String())
				qorctx.SessionManager().Add("return_to", returnTo)
			}
			chain.Next(req)
		},
	}
}
